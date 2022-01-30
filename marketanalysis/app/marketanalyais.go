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
	"time"
)

type Transfer struct {
	from   solana.PublicKey
	to     solana.PublicKey
	amount uint64
	token  solana.PublicKey
}
type MarketAnalysis struct {
	ctx       context.Context
	log       *log.Logger
	config    *config.Config
	backend   *backend.Backend
	env       *env.Env
	splToken  *spltoken.Program
	system    *system.Program
	programs  map[solana.PublicKey]program.Program
	market    solana.PublicKey
	slotStart uint64
	slotEnd   uint64
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

func NewMarketAnalysis(ctx context.Context, cfg *config.Config, market string, slotStart uint64, slotEnd uint64) *MarketAnalysis {
	arb := &MarketAnalysis{
		ctx:       ctx,
		config:    cfg,
		market:    solana.MustPublicKeyFromBase58(market),
		slotStart: slotStart,
		slotEnd:   slotEnd,
	}
	//
	logger := log.Default()
	fileName := fmt.Sprintf("%s%s_%d_%d.log", config.LogPath, market, slotStart, slotEnd)
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	logger.SetFlags(log.LstdFlags | log.Lmicroseconds)
	logger.SetOutput(file)
	arb.log = logger
	//
	backend := backend.NewBackend(ctx, cfg.Nodes, false, nil, cfg.BlochHash, cfg.TpuClient, 1)
	arb.backend = backend
	splToken := spltoken.NewProgram(ctx, backend, nil)
	arb.splToken = splToken
	system := system.NewProgram(ctx, backend)
	arb.system = system
	env := env.NewEnv(ctx)
	arb.env = env
	//
	programs := make(map[solana.PublicKey]program.Program)
	for _, program := range cfg.Programs {
		programs[program] = NewProgram(program, ctx, cfg.Which, env, backend, splToken, system, nil)
	}
	arb.programs = programs
	return arb
}

func (arb *MarketAnalysis) Service() {
	arb.Start()
	arb.Analysis(arb.market, arb.slotStart, arb.slotEnd)
	arb.Stop()
}

func (arb *MarketAnalysis) Start() {
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
	arb.log.Printf("market analysis has started......")
}

func (arb *MarketAnalysis) Stop() {
	arb.backend.Stop()
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
	arb.log.Printf("market analysis has stopped......")
}

func (arb *MarketAnalysis) GetProgram(id solana.PublicKey) program.Program {
	for _, program := range arb.programs {
		if program.Id() == id {
			return program
		}
	}
	return nil
}

func (arb *MarketAnalysis) Analysis(market solana.PublicKey, slotStart uint64, slotEnd uint64) {
	arb.log.Printf("market: %s, slot start: %d, slot end: %d", market.String(), slotStart, slotEnd)
	programKey := arb.env.FindMarketProgram(market)
	if programKey.IsZero() {
		arb.log.Printf("market %s is not used")
		return
	}
	pro := arb.GetProgram(programKey)
	if pro == nil {
		arb.log.Printf("program is not right")
		return
	}
	model := pro.GetMarket(market)
	if model == nil {
		arb.log.Printf("there is no market: %s", market.String())
		return
	}
	user2Token := make(map[solana.PublicKey]solana.PublicKey)
	slot := slotStart
	for slot <= slotEnd {
		time.Sleep(time.Second)
		block, err := arb.backend.GetBlock(slot)
		if err != nil {
			arb.log.Printf("get block err: %s", err.Error())
			slot++
			continue
		}
		arb.log.Printf("block: %d, %s", slot, time.Unix(block.Time, 0).UTC().Format("2006-01-02 15:04:05"))
		for _, i := range block.Transactions {
			hasMarket := false
			for _, account := range i.Accounts {
				if account == market {
					hasMarket = true
					break
				}
			}
			if !hasMarket {
				continue
			}
			arb.log.Printf("transaction: %s, %t", i.Signature.String(), i.Status)
			for _, j := range i.Ins {
				for _, k := range j.InnerInstructions {
					if k.Program != arb.splToken.Id() {
						continue
					}
					from, to, amount, err := arb.splToken.DecodeInstruction(k)
					if err != nil {
						//arb.log.Printf("decode instruction err: %s", err.Error())
						continue
					}
					tokenKey := solana.PublicKey{}
					if _, ok := user2Token[from]; ok {
						tokenKey = user2Token[from]
					}
					if _, ok := user2Token[to]; ok {
						tokenKey = user2Token[to]
					}
					if tokenKey.IsZero() {
						time.Sleep(time.Second)
						arb.splToken.RetrieveUsers([]solana.PublicKey{from})
						user := arb.splToken.GetUser(from)
						if user == nil {
							arb.log.Printf("user %s is missing", from.String())
							continue
						}
						user2Token[from] = user.Mint
						user2Token[to] = user.Mint
						tokenKey = user.Mint
					}
					token := arb.env.Token(tokenKey)
					if token != nil {
						arb.log.Printf("%s -> %s (%s, %s)", from.String(), to.String(), token.Symbol, token.AmountUi(amount))
					} else {
						arb.log.Printf("%s -> %s (%s, %s)", from.String(), to.String(), tokenKey.String(), fmt.Sprintf("%d", amount))
					}
				}
			}
		}
		slot++
	}
	return
}
