package app

import (
	"context"
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

type ArbitrageStep struct {
	program   program.Program
	model     program.Model
	tokenIn   solana.PublicKey
	amountIn  uint64
	slotIn    uint64
	tokenOut  solana.PublicKey
	amountOut uint64
	slotOut   uint64
}

type ArbitrageData struct {
	id             uint64
	yield          int64
	amount         uint64
	initUsdcAmount uint64
	steps          []*ArbitrageStep
}

func (ad *ArbitrageData) Copy() *ArbitrageData {
	c := &ArbitrageData{
		id:     ad.id,
		yield:  ad.yield,
		amount: ad.amount,
		steps:  make([]*ArbitrageStep, 0, len(ad.steps)),
	}
	c.steps = append(c.steps, ad.steps...)
	return c
}

type InfoUpdated struct {
	Slot       uint64
	UserKey    solana.PublicKey
	NewBalance uint64
	OldBalance uint64
	TokenKey   solana.PublicKey
}

type Arbitrage struct {
	ctx          context.Context
	log          *log.Logger
	config       *config.Config
	wg           sync.WaitGroup
	backend      *backend.Backend
	env          *env.Env
	splToken     *spltoken.Program
	system       *system.Program
	programs     map[solana.PublicKey]program.Program
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
		ctx:          ctx,
		config:       cfg,
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
	program.Arbitrage = solana.MustPublicKeyFromBase58(cfg.ArbitrageContract)
	program.Exchange = solana.MustPublicKeyFromBase58(cfg.ExchangeContract)
	//
	//peer, ttl := networkdetect.DetectPeers(cfg.Nodes[0].Wss)
	//logger.Printf("peer: %s, ttl: %d", peer, ttl/1000000)
	//
	backend := backend.NewBackend(ctx, cfg.Nodes, true, cfg.TransactionNodes, cfg.BlochHash, cfg.TpuClient, cfg.TransactionSend)
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
	arb.wg.Wait()
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

func (arb *Arbitrage) OnBalanceUpdate(userKey solana.PublicKey, newBalance uint64, oldBalance uint64, tokenKey solana.PublicKey, slot uint64) error {
	return nil
}

func (arb *Arbitrage) OnStateUpdate(slot uint64) error {
	return nil
}

func (arb *Arbitrage) randomArbitrage() {
	ticker := time.NewTicker(time.Second * 1)
	for {
		select {
		case <- ticker.C:
			arb.Arbitrage()
		case <- arb.ctx.Done():
			return
		}
	}
}

func (arb *Arbitrage) Arbitrage() error {
	//
	amount := arb.config.UsdcAccount
	if true {
		ins := make([]solana.Instruction, 0)
		{
			p := program.SerumV22
			tokenIn := program.USDC
			amountIn := amount
			key := solana.MustPublicKeyFromBase58("9wFFyRfZBsuAha4YcuxcXLKwMxJR43S7fPfQLusDBzvT")
			parameter := make(map[string]interface{})
			parameter["market"] = key
			parameter["token"] = tokenIn
			parameter["amount"] = amountIn
			parameter["flag"] = uint8(0)
			result, err := arb.GetProgram(p).ArbitrageStep(parameter)
			if err != nil {
				arb.log.Printf("err: %v", err)
				return err
			}
			ins = append(ins, result...)
		}
		{
			p := program.OrcaV2
			tokenIn := program.SOL
			amountIn := amount
			key := solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U")
			parameter := make(map[string]interface{})
			parameter["market"] = key
			parameter["token"] = tokenIn
			parameter["amount"] = amountIn
			parameter["flag"] = uint8(2)
			result, err := arb.GetProgram(p).ArbitrageStep(parameter)
			if err != nil {
				arb.log.Printf("err: %v", err)
				return err
			}
			ins = append(ins, result...)
		}
		{
			id := time.Now().UnixNano() / 1000
			arb.backend.Commit(0, uint64(id), ins, false, nil)
		}
	}
	if true {
		ins := make([]solana.Instruction, 0)
		{
			p := program.OrcaV2
			tokenIn := program.USDC
			amountIn := amount
			key := solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U")
			parameter := make(map[string]interface{})
			parameter["market"] = key
			parameter["token"] = tokenIn
			parameter["amount"] = amountIn
			parameter["flag"] = uint8(0)
			result, err := arb.GetProgram(p).ArbitrageStep(parameter)
			if err != nil {
				arb.log.Printf("err: %v", err)
				return err
			}
			ins = append(ins, result...)
		}
		{
			p := program.SerumV22
			tokenIn := program.SOL
			amountIn := amount
			key := solana.MustPublicKeyFromBase58("9wFFyRfZBsuAha4YcuxcXLKwMxJR43S7fPfQLusDBzvT")
			parameter := make(map[string]interface{})
			parameter["market"] = key
			parameter["token"] = tokenIn
			parameter["amount"] = amountIn
			parameter["flag"] = uint8(2)
			result, err := arb.GetProgram(p).ArbitrageStep(parameter)
			if err != nil {
				arb.log.Printf("err: %v", err)
				return err
			}
			ins = append(ins, result...)
		}
		{
			id := time.Now().UnixNano() / 1000
			arb.backend.Commit(0, uint64(id), ins, false, nil)
		}
	}
	return nil
}


