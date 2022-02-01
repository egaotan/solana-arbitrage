package calculator

import (
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	"golang.org/x/net/context"
	"log"
	"os"
	"strings"
	"time"
)

type Cyclic struct {
	ctx       context.Context
	cb        Callback
	log       *log.Logger
	logs      map[solana.PublicKey]*log.Logger
	algorithm string
	env       *env.Env
	models    []program.Model
	am        *AdjacencyMatrix
}

func NewCyclic(algorithm string, ctx context.Context, env *env.Env, cb Callback) *Cyclic {
	sg := &Cyclic{
		algorithm: algorithm,
		log:       log.Default(),
		logs:      make(map[solana.PublicKey]*log.Logger),
		cb:        cb,
		ctx:       ctx,
		env:       env,
		am:        NewAdjacencyMatrix(4),
		models:    make([]program.Model, 0),
	}
	return sg
}

func (cyclic *Cyclic) Start() error {
	cyclic.log.Printf("start calculator: %s, %s......", cyclic.Name(), cyclic.Algorithm())
	cyclic.price()
	cyclic.build()
	cyclic.arbitrageLogs()
	return nil
}

func (cyclic *Cyclic) Stop() error {
	cyclic.log.Printf("stop calculator: %s, %s......", cyclic.Name(), cyclic.Algorithm())
	cyclic.save2Cache()
	return nil
}

func (cyclic *Cyclic) Name() string {
	return CyclicArb
}

func (cyclic *Cyclic) Algorithm() string {
	return cyclic.algorithm
}

func (cyclic *Cyclic) Algorithms() []string {
	return []string{SBF}
}

func (cyclic *Cyclic) arbitrageLogs() {
	tokens := cyclic.am.Indexes
	for _, tokenKey := range tokens {
		if tokenKey != program.USDC {
			continue
		}
		token := cyclic.env.Token(tokenKey)
		if token == nil {
			cyclic.log.Printf("token %s is not in supported", tokenKey)
			continue
		}
		cyclic.logs[tokenKey] = utils.NewLog(config.LogPath, fmt.Sprintf("%s_%s", cyclic.Name(), strings.ToLower(token.Symbol)))
	}
}

type CyclicData struct {
	AdjacencyMatrix *AdjacencyMatrix
	Models          []program.Model
}

func (cyclic *Cyclic) save2Cache() {
	name := fmt.Sprintf("%sarbitrage_%s.json", config.CachePath, cyclic.Name())
	cd := CyclicData{
		AdjacencyMatrix: cyclic.am,
		Models:          cyclic.models,
	}
	infoJson, _ := json.MarshalIndent(cd, "", "    ")
	err := os.WriteFile(name, infoJson, 0644)
	if err != nil {
		panic(err)
	}
}

func (cyclic *Cyclic) AddModel(model program.Model) error {
	if model.Type() != program.AMM {
		return nil
	}
	cyclic.models = append(cyclic.models, model)
	model.SetState(program.StateUsed, true)
	return nil
}

func (cyclic *Cyclic) build() {
	for _, model := range cyclic.models {
		tokenPair := model.TokenPair()
		tokenAKey := tokenPair[0]
		tokenBKey := tokenPair[1]
		tokenA := cyclic.env.Token(tokenAKey)
		if tokenA == nil {
			cyclic.log.Printf("token %s is not in supported", tokenAKey)
			continue
		}
		tokenB := cyclic.env.Token(tokenBKey)
		if tokenB == nil {
			cyclic.log.Printf("token %s is not in supported", tokenBKey)
			continue
		}
		if tokenA.Price.IsPositive() && tokenB.Price.IsPositive() {
			cyclic.am.AddItem(model)
			model.SetState(program.StateCustomUsed, true)
		}
	}
}

func (cyclic *Cyclic) Calculate() error {
	tokens := cyclic.am.Indexes
	for _, tokenKey := range tokens {
		if tokenKey != program.USDC {
			continue
		}
		logger, ok := cyclic.logs[tokenKey]
		if !ok {
			cyclic.log.Printf("calculator: %s", cyclic.Name())
			cyclic.log.Printf("no log for this calculator, token: %s", tokenKey)
			continue
		}
		//logger.Printf("calculate, calculator name: %s", cyclic.Name())
		token := cyclic.env.Token(tokenKey)
		if token == nil {
			log.Printf("token %s is not in supported", tokenKey)
			continue
		}
		//amountIn := decimal.NewFromInt(1000).Div(token.Price).Mul(decimal.NewFromInt(int64(token.Decimal))).BigInt().Uint64()
		amountIn := config.USDC_AMOUNT // only for usdc
		//logger.Printf("calculate, parameters, in token: %s, in amount: %d", tokenKey, amountIn)
		sr := cyclic.am.Search(tokenKey, amountIn, tokenKey, logger)
		if sr == nil {
			continue
		}
		result := &Result{
			Id:         uint64(time.Now().UnixNano() / time.Microsecond.Nanoseconds()),
			Calculator: cyclic.Name(),
			TokenIn:    tokenKey,
			AmountIn:   amountIn,
			UsdcAmount: sr.UsdcAmount,
			TokenOut:   sr.Dst,
			AmountOut:  sr.Amount,
			Models:     sr.Models,
		}
		cyclic.cb.OnArbitrage(result)
		// adjust
		/*
			decimal.NewFromInt(int64(sr.Amount)).Div(decimal.NewFromInt(int64(amountIn)))
			p := decimal.NewFromInt(int64(sr.Amount)).Sub(decimal.NewFromInt(int64(amountIn))).Div(decimal.NewFromInt(int64(amountIn)))
			if p.IsNegative() {
				cyclic.cb.OnArbitrage(result)
				continue
			} else {
				additional := p.Mul(decimal.NewFromInt(100)).Mul(decimal.NewFromInt(int64(amountIn)))
				result.AmountIn = result.AmountIn + additional.BigInt().Uint64()
				cyclic.cb.OnArbitrage(result)
			}
		*/
	}
	return nil
}

func (cyclic *Cyclic) price() error {
	am := NewAdjacencyMatrix(3)
	for _, model := range cyclic.models {
		am.AddItem(model)
	}
	indexes := am.Indexes
	logger := utils.NewLog(config.LogPath, "cyclic_price")
	for _, index := range indexes {
		token := cyclic.env.Token(index)
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
