package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/orca"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/saber"
	"github.com/egaotan/solana-arbitrage/serumv1"
	"github.com/egaotan/solana-arbitrage/serumv2"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/egaotan/solana-arbitrage/system"
	"github.com/egaotan/solana-arbitrage/tokenswap"
	"github.com/gagliardetto/solana-go"
	"log"
	"os"
	"sync"
	"time"
)

type Arbitrage struct {
	ctx       context.Context
	log       *log.Logger
	config    *config.Config
	wg        sync.WaitGroup
	backend   *backend.Backend
	env       *env.Env
	splToken  *spltoken.Program
	system    *system.Program
	programs  map[solana.PublicKey]program.Program
	blockHash int
	nonce     uint8
}

func NewProgram(programId solana.PublicKey, ctx context.Context, which int, env *env.Env, b *backend.Backend, splToken *spltoken.Program, system *system.Program, cb program.Callback) program.Program {
	if programId == program.TokenSwap {
		return tokenswap.NewProgram(programId, ctx, which, env, b, splToken, system, cb)
	}
	if programId == program.OrcaV1 {
		return orca.NewProgram(programId, ctx, which, env, b, splToken, system, cb)
	}
	if programId == program.OrcaV2 {
		return orca.NewProgram(programId, ctx, which, env, b, splToken, system, cb)
	}
	if programId == program.Saber {
		return saber.NewProgram(programId, ctx, which, env, b, splToken, system, cb)
	}
	if programId == program.SerumV11 {
		return serumv1.NewProgram(programId, ctx, which, env, b, splToken, system, cb)
	}
	if programId == program.SerumV12 {
		return serumv1.NewProgram(programId, ctx, which, env, b, splToken, system, cb)
	}
	if programId == program.SerumV21 {
		return serumv2.NewProgram(programId, ctx, which, env, b, splToken, system, cb)
	}
	if programId == program.SerumV22 {
		return serumv2.NewProgram(programId, ctx, which, env, b, splToken, system, cb)
	}
	panic(fmt.Errorf("program (%s) is not support", programId))
}

func NewArbitrage(ctx context.Context, cfg *config.Config) *Arbitrage {
	arb := &Arbitrage{
		ctx:    ctx,
		config: cfg,
	}
	//
	logger := log.Default()
	fileName := fmt.Sprintf("%s%s.log", config.LogPath, "arbitrage")
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	logger.SetFlags(log.LstdFlags | log.Lmicroseconds)
	logger.SetOutput(file)
	arb.log = logger
	//
	program.Arbitrage = cfg.ArbitrageContract
	program.Exchange = cfg.ExchangeContract
	//
	//peer, ttl := networkdetect.DetectPeers(cfg.Nodes[0].Wss)
	//logger.Printf("peer: %s, ttl: %d", peer, ttl/1000000)
	//
	backend := backend.NewBackend(ctx, cfg.Nodes, true, cfg.TransactionNodes, cfg.BlochHash, cfg.TpuClient, cfg.Senders, cfg.TransactionSend, cfg.PreExecute)
	backend.ImportWallet(cfg.Key)
	backend.SetPlayer(cfg.User)
	backend.SetStore(nil)
	arb.backend = backend
	splToken := spltoken.NewProgram(ctx, backend, arb)
	arb.splToken = splToken
	system := system.NewProgram(ctx, backend)
	arb.system = system
	env := env.NewEnv(ctx)
	arb.env = env
	//
	programs := make(map[solana.PublicKey]program.Program)
	for _, program := range cfg.Programs {
		programs[program] = NewProgram(program, ctx, cfg.Which, env, backend, splToken, system, arb)
	}
	arb.programs = programs
	return arb
}

func (arb *Arbitrage) Service() {
	arb.Start()
	<-arb.ctx.Done()
	arb.Stop()
}

func (arb *Arbitrage) Start() {
	arb.backend.Start()
	arb.env.Start()
	if err := arb.splToken.Start(); err != nil {
		arb.log.Printf("spl token program start err: %v", err)
	}
	if err := arb.system.Start(); err != nil {
		arb.log.Printf("system program start err: %v", err)
	}
	for _, program := range arb.programs {
		if err := program.Start(); err != nil {
			arb.log.Printf("program %s start err: %v", program.Name(), err)
		}
	}
	arb.wg.Add(1)
	arb.backend.SubscribeSlot(arb)
	go arb.randomArbitrage()
	arb.log.Printf("auto trader has started......")
}

func (arb *Arbitrage) Stop() {
	arb.backend.Stop()
	//arb.wg.Wait()
	if err := arb.splToken.Stop(); err != nil {
		arb.log.Printf("spl token program stop err: %v", err)
	}
	if err := arb.system.Stop(); err != nil {
		arb.log.Printf("system program stop err: %v", err)
	}
	for _, program := range arb.programs {
		if err := program.Stop(); err != nil {
			arb.log.Printf("program %s stop err: %v", program.Name(), err)
		}
	}
	arb.env.Stop()
	arb.log.Printf("auto trader has stopped......")
}

func (arb *Arbitrage) OnModelInit(model program.Model) error {
	return nil
}

func (arb *Arbitrage) GetProgram(id solana.PublicKey) program.Program {
	for _, program := range arb.programs {
		if program.Id() == id {
			return program
		}
	}
	return nil
}

func (arb *Arbitrage) OnSlotUpdate(slot *backend.Slot) error {
	return nil
}

func (arb *Arbitrage) OnStateUpdate(slot uint64) error {
	return nil
}

func (arb *Arbitrage) OnBalanceUpdate(userKey solana.PublicKey, oldBalance uint64, newBalance uint64, tokenKey solana.PublicKey, slot uint64) error {
	return nil
}

func (arb *Arbitrage) randomArbitrage() {
	ticker := time.NewTicker(time.Millisecond * time.Duration(arb.config.RandomTicker))
	for {
		select {
		case <-ticker.C:
			arb.Arbitrage()
		case <-arb.ctx.Done():
			return
		}
	}
}

type Path struct {
	Program solana.PublicKey `json:"program"`
	Market  solana.PublicKey `json:"market"`
	TokenIn solana.PublicKey `json:"token_in"`
}

type Exchange struct {
	AmountIn uint64 `json:"amount"`
	Paths    []Path `json:"paths"`
}

type ExchangeCase struct {
	Exchanges []Exchange `json:"cases"`
}

func (arb *Arbitrage) Arbitrage() error {
	//
	infoJson, err := os.ReadFile(config.RandomCaseFile)
	if err != nil {
		return err
	}
	var allCase ExchangeCase
	err = json.Unmarshal(infoJson, &allCase)
	if err != nil {
		return err
	}
	//
	arb.nonce++
	if arb.nonce == 0 {
		arb.nonce++
	}
	for _, oneCase := range allCase.Exchanges {
		ins := make([]solana.Instruction, 0)
		for j, step := range oneCase.Paths {
			amountIn := oneCase.AmountIn
			parameter := make(map[string]interface{})
			parameter["market"] = step.Market
			parameter["token"] = step.TokenIn
			parameter["amount"] = amountIn
			parameter["nonce"] = arb.nonce
			if j == 0 {
				parameter["flag"] = uint8(0)
			} else if j == len(oneCase.Paths)-1 {
				parameter["flag"] = uint8(2)
			} else {
				parameter["flag"] = uint8(1)
			}
			p := arb.GetProgram(step.Program)
			result, err := p.ArbitrageStep(parameter)
			if err != nil {
				return err
			}
			ins = append(ins, result...)
		}
		id := uint64(time.Now().UnixNano() / 1000)
		arb.backend.Commit(0, id, ins, false, nil, nil)
	}
	return nil
}
