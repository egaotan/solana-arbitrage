package calculator

import (
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/gagliardetto/solana-go"
	"log"
)

type SearchResult struct {
	Dst        solana.PublicKey
	Amount     uint64
	UsdcAmount uint64
	Path       []int
	Models     []program.Model
}

type AdjacencyItem struct {
	Items []program.Model
}

type AdjacencyMatrix struct {
	Indexes   []solana.PublicKey
	UsdcIndex int
	Matrices  [][]*AdjacencyItem
	Depth     int
}

func NewAdjacencyMatrix(depth int) *AdjacencyMatrix {
	am := &AdjacencyMatrix{
		Depth: depth,
	}
	return am
}

func (am *AdjacencyMatrix) AddItem(model program.Model) {
	tokenPair := model.TokenPair()
	tokenAKey, tokenBKey := tokenPair[0], tokenPair[1]
	tokenAIndex, tokenBIndex := -1, -1
	for i, index := range am.Indexes {
		if index == tokenAKey {
			tokenAIndex = i
		}
		if index == tokenBKey {
			tokenBIndex = i
		}
	}
	if tokenAIndex == -1 {
		am.Indexes = append(am.Indexes, tokenAKey)
		newSize := len(am.Indexes)
		for i, col := range am.Matrices {
			am.Matrices[i] = append(col, &AdjacencyItem{Items: make([]program.Model, 0)})
		}
		am.Matrices = append(am.Matrices, make([]*AdjacencyItem, newSize))
		for i := 0; i < newSize; i++ {
			am.Matrices[newSize-1][i] = &AdjacencyItem{Items: make([]program.Model, 0)}
		}
		tokenAIndex = newSize - 1
		if tokenAKey == program.USDC {
			am.UsdcIndex = tokenAIndex
		}
	}
	if tokenBIndex == -1 {
		am.Indexes = append(am.Indexes, tokenBKey)
		newSize := len(am.Indexes)
		for i, col := range am.Matrices {
			am.Matrices[i] = append(col, &AdjacencyItem{Items: make([]program.Model, 0)})
		}
		am.Matrices = append(am.Matrices, make([]*AdjacencyItem, newSize))
		for i := 0; i < newSize; i++ {
			am.Matrices[newSize-1][i] = &AdjacencyItem{Items: make([]program.Model, 0)}
		}
		tokenBIndex = newSize - 1
		if tokenBKey == program.USDC {
			am.UsdcIndex = tokenBIndex
		}
	}
	am.Matrices[tokenAIndex][tokenBIndex].Items = append(am.Matrices[tokenAIndex][tokenBIndex].Items, model)
	am.Matrices[tokenBIndex][tokenAIndex].Items = append(am.Matrices[tokenBIndex][tokenAIndex].Items, model)
}

func (am *AdjacencyMatrix) Index(token solana.PublicKey) int {
	for i, index := range am.Indexes {
		if index == token {
			return i
		}
	}
	return -1
}

func (am *AdjacencyMatrix) Search(src solana.PublicKey, amount uint64, dst solana.PublicKey, logger *log.Logger) *SearchResult {
	srcIndex := am.Index(src)
	if srcIndex == -1 {
		logger.Printf("token %s is not in matrix", src)
		return nil
	}
	dstIndex := am.Index(dst)
	if dstIndex == -1 {
		logger.Printf("token %s is not in matrix", dst)
		return nil
	}
	//
	data := &NodeData{
		index:  srcIndex,
		amount: amount,
		path:   make([]int, 0),
		models: make([]program.Model, 0),
	}
	if data.index == am.UsdcIndex {
		data.usdcAmount = data.amount
	}
	data.path = append(data.path, srcIndex)
	tree := &Node{
		child: make([]*Node, 0),
		data:  data,
	}
	am.bsf(tree, dstIndex, logger)
	node := am.target(tree, dstIndex, logger)
	if node == nil {
		return nil
	}
	if node.data.index != dstIndex {
		return nil
	}
	sr := &SearchResult{
		Dst:        solana.PublicKey{},
		Amount:     0,
		UsdcAmount: 0,
		Path:       make([]int, 0),
		Models:     make([]program.Model, 0),
	}
	sr.Dst = dst
	sr.Amount = node.data.amount
	sr.UsdcAmount = node.data.usdcAmount
	sr.Path = append(sr.Path, node.data.path...)
	sr.Models = append(sr.Models, node.data.models...)
	return sr
}

func (am *AdjacencyMatrix) bsf(curNode *Node, dstIndex int, logger *log.Logger) {
	curData := curNode.data
	curIndex := curData.index
	curAmount := curData.amount
	for i := 0; i < len(am.Matrices[curIndex]); i++ {
		if i == curIndex {
			continue
		}
		if len(am.Matrices[curIndex][i].Items) == 0 {
			continue
		}
		maxAmount := uint64(0)
		var useModel program.Model
		for _, model := range am.Matrices[curIndex][i].Items {
			result, err := model.Swap(am.Indexes[curIndex], curAmount)
			if err != nil {
				logger.Printf("swap err: %v, program: %s, market: %s, in token: %s, in Amount: %d", err, model.Program(), model.Id(), am.Indexes[curIndex], curAmount)
				continue
			}
			if result.TokenIn != am.Indexes[curIndex] {
				logger.Printf("swap source token %s is invalid, program: %s, market: %s", result.TokenIn, model.Program(), model.Id())
				continue
			}
			if result.TokenOut != am.Indexes[i] {
				logger.Printf("swap destination token %s is invalid, program: %s, market: %s", result.TokenOut, model.Program(), model.Id())
				continue
			}
			if result.AmountOut > maxAmount {
				maxAmount = result.AmountOut
				useModel = model
			}
		}
		childData := &NodeData{
			index:  i,
			amount: maxAmount,
			path:   make([]int, 0),
			models: make([]program.Model, 0),
		}
		childData.usdcAmount = curData.usdcAmount
		if childData.index == am.UsdcIndex {
			childData.usdcAmount = childData.amount
		}
		childData.path = append(childData.path, curData.path...)
		childData.path = append(childData.path, i)
		childData.models = append(childData.models, curData.models...)
		childData.models = append(childData.models, useModel)
		childNode := &Node{
			child: make([]*Node, 0),
			data:  childData,
		}
		curNode.child = append(curNode.child, childNode)
		// check
		end := false
		for j := 0; j < len(childData.path)-1; j++ {
			if childData.path[j] == childData.path[len(childData.path)-1] {
				end = true
				break
			}
		}
		if i == dstIndex {
			end = true
		}
		if len(childData.path) == am.Depth {
			end = true
		}
		if childData.amount == 0 {
			end = true
		}
		if end == true {
			continue
		}
		am.bsf(childNode, dstIndex, logger)
	}
}

func (am *AdjacencyMatrix) target(tree *Node, targetIndex int, logger *log.Logger) *Node {
	if len(tree.child) == 0 {
		if tree.data.index == targetIndex {
			//logger.Printf("xxxxxxxx one path: ")
			//for _, i := range tree.data.path {
			//	logger.Printf("%s", am.Indexes[i])
			//}
			//logger.Printf("yield: %d", tree.data.amount)
			return tree
		} else {
			return nil
		}
	}
	maxAmount := uint64(0)
	var max *Node
	for _, child := range tree.child {
		node := am.target(child, targetIndex, logger)
		if node == nil {
			continue
		}
		if node.data.amount > maxAmount {
			maxAmount = node.data.amount
			max = node
		}
	}
	return max
}
