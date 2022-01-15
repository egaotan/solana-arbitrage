package calculator

import "github.com/egaotan/solana-arbitrage/program"

type NodeData struct {
	index      int
	amount     uint64
	usdcAmount uint64
	path       []int
	models     []program.Model
}

type Node struct {
	child []*Node
	data  *NodeData
}
