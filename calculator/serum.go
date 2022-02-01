package calculator

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	"log"
	"os"
	"strings"
	"time"
)

type Serum struct {
	ctx         context.Context
	cb          Callback
	log         *log.Logger
	logs        map[solana.PublicKey]*log.Logger
	algorithm   string
	salt        uint64
	env         *env.Env
	ammModels   []program.Model
	am          *AdjacencyMatrix
	serumModels []program.Model
}

func NewSerum(algorithm string, ctx context.Context, env *env.Env, cb Callback) *Serum {
	sg := &Serum{
		algorithm:   algorithm,
		log:         log.Default(),
		logs:        make(map[solana.PublicKey]*log.Logger),
		cb:          cb,
		ctx:         ctx,
		env:         env,
		serumModels: make([]program.Model, 0),
		ammModels:   make([]program.Model, 0),
		am:          NewAdjacencyMatrix(3),
	}
	return sg
}

func (serum *Serum) Start() error {
	serum.log.Printf("start calculator: %s, %s......", serum.Name(), serum.Algorithm())
	serum.price()
	serum.build()
	serum.arbitrageLogs()
	return nil
}

func (serum *Serum) Stop() error {
	serum.log.Printf("stop calculator: %s, %s......", serum.Name(), serum.Algorithm())
	serum.save2Cache()
	return nil
}

func (serum *Serum) Name() string {
	return SerumArb
}

func (serum *Serum) Algorithm() string {
	return serum.algorithm
}

func (serum *Serum) Algorithms() []string {
	return []string{SBF}
}

func (serum *Serum) arbitrageLogs() {
	for _, model := range serum.serumModels {
		customUsed := model.HasState(program.StateCustomUsed)
		if !customUsed {
			continue
		}
		tokenPair := model.TokenPair()
		tokenAKey, tokenBKey := tokenPair[0], tokenPair[1]
		tokenA := serum.env.Token(tokenAKey)
		if tokenA == nil {
			serum.log.Printf("token %s is not in supported", tokenAKey)
			continue
		}
		tokenB := serum.env.Token(tokenBKey)
		if tokenB == nil {
			serum.log.Printf("token %s is not in supported", tokenBKey)
			continue
		}
		serum.logs[model.Id()] = utils.NewLog(config.LogPath, fmt.Sprintf("%s_%s_%s", serum.Name(), strings.ToLower(tokenA.Symbol), strings.ToLower(tokenB.Symbol)))
	}
}

type SerumData struct {
	AdjacencyMatrix *AdjacencyMatrix
	SerumModels     []program.Model
	AmmModels       []program.Model
}

func (serum *Serum) save2Cache() {
	sd := &SerumData{
		AdjacencyMatrix: serum.am,
		SerumModels:     serum.serumModels,
		AmmModels:       serum.ammModels,
	}
	name := fmt.Sprintf("%sarbitrage_%s.json", config.CachePath, serum.Name())
	infoJson, _ := json.MarshalIndent(sd, "", "    ")
	err := os.WriteFile(name, infoJson, 0644)
	if err != nil {
		panic(err)
	}
}

func (serum *Serum) build() error {
	for _, model := range serum.ammModels {
		tokenPair := model.TokenPair()
		tokenAKey := tokenPair[0]
		tokenBKey := tokenPair[1]
		tokenA := serum.env.Token(tokenAKey)
		if tokenA == nil {
			serum.log.Printf("token %s is not in supported", tokenAKey)
			continue
		}
		tokenB := serum.env.Token(tokenBKey)
		if tokenB == nil {
			serum.log.Printf("token %s is not in supported", tokenBKey)
			continue
		}
		if tokenA.Price.IsPositive() && tokenB.Price.IsPositive() {
			serum.am.AddItem(model)
			model.SetState(program.StateCustomUsed, true)
		}
	}
	for _, model := range serum.serumModels {
		tokenPair := model.TokenPair()
		tokenAKey, tokenBKey := tokenPair[0], tokenPair[1]
		// check whether in adjacency matrix or not
		if serum.am.Index(tokenAKey) == -1 {
			continue
		}
		if serum.am.Index(tokenBKey) == -1 {
			continue
		}
		model.SetState(program.StateCustomUsed, true)
	}
	return nil
}

func (serum *Serum) AddModel(model program.Model) error {
	if model.Type() == program.OrderBook {
		serum.serumModels = append(serum.serumModels, model)
		model.SetState(program.StateUsed, true)
	} else if model.Type() == program.AMM {
		serum.ammModels = append(serum.ammModels, model)
		model.SetState(program.StateUsed, true)
	}
	return nil
}

func (serum *Serum) Calculate() error {
	serum.salt = 0
	for _, model := range serum.serumModels {
		customUsed := model.HasState(program.StateCustomUsed)
		if !customUsed {
			continue
		}
		serum.calculateOnModel(model)
	}
	return nil
}

func (serum *Serum) price() error {
	am := NewAdjacencyMatrix(3)
	for _, model := range serum.ammModels {
		am.AddItem(model)
	}
	indexes := am.Indexes
	logger := utils.NewLog(config.LogPath, "serum_price")
	for _, index := range indexes {
		token := serum.env.Token(index)
		result := am.Search(index, token.Decimal, program.USDC, logger)
		if result == nil {
			continue
		}
		if result.Amount <= 10 {
			continue
		}
		token.Price = decimal.NewFromInt(int64(result.Amount)).Div(decimal.NewFromInt(1000000))
	}
	return nil
}

func (serum *Serum) calculateOnModel(model program.Model) {
	logger, ok := serum.logs[model.Id()]
	if !ok {
		serum.log.Printf("calculator: %s", serum.Name())
		serum.log.Printf("no log for this calculator, serum: %s", model.Id())
		return
	}
	//logger.Printf("calculate, calculator name: %s", serum.Name())
	//logger.Printf("calculate, serum program: %s, serum market: %s", model.Program(), model.ArbitrageId())
	tokenPair := model.TokenPair()
	for _, tokenKey := range tokenPair {
		//logger.Printf("calculate, parameters, in token: %s, in amount: %d", tokenKey, 0)
		token := serum.env.Token(tokenKey)
		xx := token.Price.Mul(decimal.NewFromInt(1000000000000)).Div(decimal.NewFromInt(int64(token.Decimal)))
		serumSwap, err := model.Swap(tokenKey, xx.BigInt().Uint64())
		if err != nil {
			logger.Printf("serum swap err: %v", err)
			continue
		}
		//
		sr := serum.am.Search(serumSwap.TokenOut, serumSwap.AmountOut, serumSwap.TokenIn, logger)
		if sr == nil {
			continue
		}
		result := &Result{
			Id:         uint64(time.Now().UnixNano()/time.Microsecond.Nanoseconds()) + serum.salt,
			Calculator: serum.Name(),
			TokenIn:    tokenKey,
			AmountIn:   serumSwap.AmountIn,
			UsdcAmount: sr.UsdcAmount,
			TokenOut:   sr.Dst,
			AmountOut:  sr.Amount,
		}
		result.Models = append(result.Models, model)
		result.Models = append(result.Models, sr.Models...)
		serum.salt++
		serum.cb.OnArbitrage(result)
	}
}
