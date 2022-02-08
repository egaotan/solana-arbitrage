package app

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/dingsdk"
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
	nonce     byte
	latestNotify uint64
	dsdk *dingsdk.DingSdk
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
	dsdk := dingsdk.NewDingSdk(cfg.DingUrl)
	arb.dsdk = dsdk
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

func (arb *Arbitrage) Arbitrage() error {
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.ExchangeContract, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SerumV22, IsSigner: false, IsWritable: false})
	// serum
	{
		p, ok := arb.programs[program.SerumV22]
		if !ok {
			return fmt.Errorf("program %s is invalid", program.SerumV22)
		}
		parameter := make(map[string]interface{})
		parameter["tokenA"] = arb.config.TokenA
		parameter["tokenB"] = arb.config.TokenB
		accs, err := p.RandomAccounts(parameter)
		if err != nil {
			return err
		}
		accounts = append(accounts, accs...)
	}
	// orca
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.OrcaV2, IsSigner: false, IsWritable: false})
	{
		p, ok := arb.programs[program.OrcaV2]
		if !ok {
			return fmt.Errorf("program %s is invalid", program.OrcaV2)
		}
		parameter := make(map[string]interface{})
		parameter["tokenA"] = arb.config.TokenA
		parameter["tokenB"] = arb.config.TokenB
		accs, err := p.RandomAccounts(parameter)
		if err != nil {
			return err
		}
		accounts = append(accounts, accs...)
	}
	ins := make([]solana.Instruction, 0)
	//
	usdc_acc := arb.env.TokenUser(arb.config.TokenB)
	other_acc := arb.env.TokenUser(arb.config.TokenA)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: other_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysRent, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 100
	nonce := arb.nonce
	if arb.config.TokenA == program.MSOL && arb.config.TokenB == program.USDC {
		nonce += 100
	}
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 1
		data[1] = nonce
		data[2] = byte(i)
		if i == arb.config.InstructionSize-1 {
			data[2] = byte(100)
		}

		instruction := &program.Instruction{
			IsAccounts:  accounts,
			IsData:      data,
			IsProgramID: program.Arbitrage,
		}
		ins = append(ins, instruction)
	}
	{
		id := uint64(time.Now().UnixNano() / 1000)
		arb.backend.Commit(arb.blockHash, id, ins, false, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3

		//
		if id - arb.latestNotify > 1000000 * 60 * 5 {
			arb.latestNotify = id
			arb.tryNotify(id)
		}
	}
	return nil
}

func (arb *Arbitrage) tryNotify(id uint64) {
	ttStr := time.Unix(int64(id)/1000000, int64(id)%1000000*1000).Format("2006-01-02 15:04:05.000")
	text := fmt.Sprintf("arbitrage: check tt: %s", ttStr)
	dingNotify := &dingsdk.DingNotify{
		MsgType: "text",
		Text: dingsdk.DingContent{
			Content: text,
		},
		At: dingsdk.DingAt{
			IsAtAll: false,
		},
	}
	arb.dsdk.Notify(dingNotify)
}
