package app

import (
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/store"
	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	time2 "time"
)

type Token struct {
	Key    string `json:"key"`
	Symbol string `json:"symbol"`
}

type LocalArbitrageStep struct {
	Program          string `json:"program"`
	Market           string `json:"market"`
	TokenIn          *Token `json:"token_in"`
	AmountIn         string `json:"amount_in"`
	SlotIn           uint64 `json:"slot_in"`
	TokenOut         *Token `json:"token_out"`
	AmountOut        string `json:"amount_out"`
	SlotOut          uint64 `json:"slot_out"`
	LocalArbitrageId uint64 `json:"local_arbitrage_id"`
}

type LocalArbitrage struct {
	Id                  uint64                `json:"id"`
	Time                string                `json:"time"`
	Yield               string                `json:"yield"`
	LocalArbitrageSteps []*LocalArbitrageStep `json:"local_arbitrage_steps"`
}

type CommittedArbitrageStep struct {
	Program              string `json:"program"`
	Market               string `json:"market"`
	TokenIn              *Token `json:"token_in"`
	TokenOut             *Token `json:"token_out"`
	CommittedArbitrageId uint64 `json:"committed_arbitrage_id"`
}

type CommittedArbitrage struct {
	Id                      uint64                    `json:"id"`
	Time                    string                    `json:"time"`
	Amount                  string                    `json:"amount"`
	CommittedArbitrageSteps []*CommittedArbitrageStep `json:"committed_arbitrage_steps"`
}

type ExecutedArbitrage struct {
	Id             uint64 `json:"id"`
	Time           string `json:"time"`
	SendTime       string `json:"send_time"`
	ResponseTime   string `json:"response_time"`
	FinishTime     string `json:"finish_time"`
	ExecutorId     int    `json:"executor_id"`
	ExecuteCounter int    `json:"execute_counter"`
	Signature      string `json:"signature"`
}

func buildLocalArbitrageStep(arb *store.LocalArbitrageStep, env *env.Env) *LocalArbitrageStep {
	tokenInKey := solana.MustPublicKeyFromBase58(arb.TokenIn)
	tokenIn := env.Token(tokenInKey)
	tokenOutKey := solana.MustPublicKeyFromBase58(arb.TokenOut)
	tokenOut := env.Token(tokenOutKey)
	newStep := &LocalArbitrageStep{
		Program: arb.Program,
		Market:  arb.Market,
		TokenIn: &Token{
			Key:    arb.TokenIn,
			Symbol: tokenIn.Symbol,
		},
		AmountIn: tokenIn.AmountUi(arb.AmountIn).StringFixed(2),
		SlotIn:   arb.SlotIn,
		TokenOut: &Token{
			Key:    arb.TokenOut,
			Symbol: tokenOut.Symbol,
		},
		AmountOut:        tokenOut.AmountUi(arb.AmountOut).StringFixed(2),
		SlotOut:          arb.SlotOut,
		LocalArbitrageId: arb.LocalArbitrageId,
	}
	return newStep
}

func buildLocalArbitrageSteps(steps []*store.LocalArbitrageStep, env *env.Env) []*LocalArbitrageStep {
	newSteps := make([]*LocalArbitrageStep, 0, len(steps))
	for _, step := range steps {
		newSteps = append(newSteps, buildLocalArbitrageStep(step, env))
	}
	return newSteps
}

func buildLocalArbitrage(arb *store.LocalArbitrage, env *env.Env) *LocalArbitrage {
	newArb := &LocalArbitrage{
		Id:                  arb.Id,
		Yield:               decimal.NewFromInt(arb.Yield).Div(decimal.NewFromInt(100)).StringFixed(2),
		Time:                time2.Unix(int64(arb.Id)/1000000, int64(arb.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"),
		LocalArbitrageSteps: buildLocalArbitrageSteps(arb.LocalArbitrageSteps, env),
	}
	return newArb
}

func buildLocalArbitrages(arbs []*store.LocalArbitrage, env *env.Env) []*LocalArbitrage {
	newArbs := make([]*LocalArbitrage, 0, len(arbs))
	for _, arb := range arbs {
		newArbs = append(newArbs, buildLocalArbitrage(arb, env))
	}
	return newArbs
}

func buildCommittedArbitrageStep(arb *store.CommittedArbitrageStep, env *env.Env) *CommittedArbitrageStep {
	tokenInKey := solana.MustPublicKeyFromBase58(arb.TokenIn)
	tokenIn := env.Token(tokenInKey)
	tokenOutKey := solana.MustPublicKeyFromBase58(arb.TokenOut)
	tokenOut := env.Token(tokenOutKey)
	newStep := &CommittedArbitrageStep{
		Program: arb.Program,
		Market:  arb.Market,
		TokenIn: &Token{
			Key:    arb.TokenIn,
			Symbol: tokenIn.Symbol,
		},
		TokenOut: &Token{
			Key:    arb.TokenOut,
			Symbol: tokenOut.Symbol,
		},
		CommittedArbitrageId: arb.CommittedArbitrageId,
	}
	return newStep
}

func buildCommittedArbitrageSteps(steps []*store.CommittedArbitrageStep, env *env.Env) []*CommittedArbitrageStep {
	newSteps := make([]*CommittedArbitrageStep, 0, len(steps))
	for _, step := range steps {
		newSteps = append(newSteps, buildCommittedArbitrageStep(step, env))
	}
	return newSteps
}

func buildCommittedArbitrage(arb *store.CommittedArbitrage, env *env.Env) *CommittedArbitrage {
	tokenUsdc := env.Token(program.USDC)
	newArb := &CommittedArbitrage{
		Id:                      arb.Id,
		Time:                    time2.Unix(int64(arb.Id)/1000000, int64(arb.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"),
		Amount:                  tokenUsdc.AmountUi(arb.Amount).StringFixed(2),
		CommittedArbitrageSteps: buildCommittedArbitrageSteps(arb.CommittedArbitrageSteps, env),
	}
	return newArb
}

func buildCommittedArbitrages(arbs []*store.CommittedArbitrage, env *env.Env) []*CommittedArbitrage {
	newArbs := make([]*CommittedArbitrage, 0, len(arbs))
	for _, arb := range arbs {
		newArbs = append(newArbs, buildCommittedArbitrage(arb, env))
	}
	return newArbs
}

func buildExecutedArbitrage(arb *store.ExecutedArbitrage) *ExecutedArbitrage {
	newArb := &ExecutedArbitrage{
		Id:             arb.Id,
		ExecutorId:     arb.ExecuteId,
		Time:           time2.Unix(int64(arb.Id)/1000000, int64(arb.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"),
		SendTime:       time2.Unix(int64(arb.SendTime)/1000000, int64(arb.SendTime)%1000000*1000).Format("2006-01-02 15:04:05.000000"),
		ResponseTime:   time2.Unix(int64(arb.ResponseTime)/1000000, int64(arb.ResponseTime)%1000000*1000).Format("2006-01-02 15:04:05.000000"),
		FinishTime:     time2.Unix(int64(arb.FinishTime)/1000000, int64(arb.FinishTime)%1000000*1000).Format("2006-01-02 15:04:05.000000"),
		ExecuteCounter: arb.ExecuteCounter,
		Signature:      arb.Signature,
	}
	return newArb
}

func buildExecutedArbitrages(arbs []*store.ExecutedArbitrage) []*ExecutedArbitrage {
	newArbs := make([]*ExecutedArbitrage, 0, len(arbs))
	for _, arb := range arbs {
		newArbs = append(newArbs, buildExecutedArbitrage(arb))
	}
	return newArbs
}
