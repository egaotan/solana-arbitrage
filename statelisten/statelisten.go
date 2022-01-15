package statelisten

import (
	"context"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"log"
	"strings"
	"sync"
	"time"
)

type StateListen struct {
	ctx      context.Context
	wg       sync.WaitGroup
	programs []program.Program
	env      *env.Env
	logs     map[solana.PublicKey]*log.Logger
}

func NewStateListen(ctx context.Context, programs map[solana.PublicKey]program.Program, env *env.Env) *StateListen {
	ps := make([]program.Program, 0)
	for _, p := range programs {
		ps = append(ps, p)
	}
	sl := &StateListen{
		ctx:      ctx,
		programs: ps,
		env:      env,
		logs:     make(map[solana.PublicKey]*log.Logger),
	}
	return sl
}

func (sl *StateListen) Start() {
	sl.createLog()
	sl.wg.Add(1)
	go sl.listen()
}

func (sl *StateListen) createLog() {
	for _, program := range sl.programs {
		markets := program.Markets()
		for _, market := range markets {
			pair := market.TokenPair()
			tokenA := sl.env.Token(pair[0])
			tokenB := sl.env.Token(pair[1])
			logger := utils.NewLog(config.LogPath, "price_"+strings.ToLower(tokenA.Symbol)+"_"+strings.ToLower(tokenB.Symbol))
			sl.logs[market.Id()] = logger
		}
	}
}

func (sl *StateListen) listen() {
	defer sl.wg.Done()
	timer2 := time.NewTicker(time.Millisecond * 500)
	for {
		select {
		case <-timer2.C:
			sl.DumpState()
		case <-sl.ctx.Done():
			return
		}
	}
}

func (sl *StateListen) DumpState() {
	for _, program := range sl.programs {
		markets := program.Markets()
		for _, market := range markets {
			logger, ok := sl.logs[market.Id()]
			if !ok {
				continue
			}
			state, err := program.RetrieveState(market.Id())
			if err != nil {
				continue
			}
			logger.Printf(state)
		}
	}
}
