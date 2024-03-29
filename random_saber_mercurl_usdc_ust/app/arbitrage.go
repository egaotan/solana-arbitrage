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
	"github.com/shopspring/decimal"
	"log"
	"os"
	"sync"
	"time"
)

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
	blockHash    int
	nonce        byte
	latestNotify uint64
	dsdk         *dingsdk.DingSdk
	balances     map[solana.PublicKey]uint64
	frequency    int64
	counter      int64
	latestUpdate int64
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
		ctx:       ctx,
		config:    cfg,
		blockHash: 2,
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
	dsdk := dingsdk.NewDingSdk(cfg.DingUrl)
	arb.dsdk = dsdk
	//
	programs := make(map[solana.PublicKey]program.Program)
	for _, program := range cfg.Programs {
		programs[program] = NewProgram(program, ctx, cfg.Which, env, backend, splToken, system, arb)
	}
	arb.programs = programs
	arb.frequency = 10
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
	/*
		monitor usdc accounts
	*/
	/*
	usdcAccounts := make([]solana.PublicKey, 0)
	arb.balances = make(map[solana.PublicKey]uint64, 0)
	for _, acc := range arb.config.UsdcAccount {
		item := solana.MustPublicKeyFromBase58(acc)
		usdcAccounts = append(usdcAccounts, item)
		arb.balances[item] = 0
	}
	err := arb.splToken.RetrieveUsers(usdcAccounts)
	if err != nil {
		arb.log.Printf("usdc account err!")
	}
	arb.splToken.SubscribeUsers(usdcAccounts)
	arb.wg.Add(1)
	 */
	arb.backend.SubscribeSlot(arb)
	go arb.randomArbitrage()
	//arb.Arbitrage()
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

func (arb *Arbitrage) OnBalanceUpdate(userKey solana.PublicKey, oldBalance uint64, newBalance uint64, tokenKey solana.PublicKey, slot uint64) error {
	initBalance, ok := arb.balances[userKey]
	if !ok {
		return fmt.Errorf("not monitor account")
	}
	arb.log.Printf("new balanece: %d, init balance: %d", newBalance, initBalance)
	if newBalance == initBalance {
		return nil
	}
	if initBalance != 0 {
		arb.frequency = 5
		arb.latestUpdate = arb.counter
	}

	context := "arbitrage account balance update: \n"
	balance1 := decimal.NewFromInt(int64(initBalance)).Div(decimal.NewFromInt(1000000))
	balance2 := decimal.NewFromInt(int64(newBalance)).Div(decimal.NewFromInt(1000000))
	diff := decimal.NewFromInt(0)
	if initBalance != uint64(0) {
		diff = balance2.Sub(balance1)
	}
	context = context + fmt.Sprintf("%s -> %s (%s);\n",
		balance1.StringFixed(2), balance2.StringFixed(2), diff.StringFixed(2))

	context = context + fmt.Sprintf("time: %s;", time.Now().Format("2006-01-02 15:04:05"))
	dingNotify := &dingsdk.DingNotify{
		MsgType: "text",
		Text: dingsdk.DingContent{
			Content: context,
		},
		At: dingsdk.DingAt{
			IsAtAll: false,
		},
	}
	arb.dsdk.Notify(dingNotify)
	arb.balances[userKey] = newBalance
	return nil
}

func (arb *Arbitrage) OnStateUpdate(slot uint64) error {
	return nil
}

func (arb *Arbitrage) randomArbitrage() {
	ticker := time.NewTicker(time.Millisecond * time.Duration(arb.config.RandomTicker))
	counter := 0
	for {
		select {
		case <-ticker.C:
			counter ++
			if counter > 5 {
				//continue
			}
			if arb.config.USTUSDC {
				arb.Arbitrage_saber_mercurl_usdc_ust()
				arb.Arbitrage_mercurl_saber_usdc_ust()
			}
			if arb.config.USDTUSDC {
				arb.Arbitrage_saber_mercurl_usdc_usdt()
				arb.Arbitrage_mercurl_saber_usdc_usdt()
			}
			if arb.config.SOLMSOL {
				arb.Arbitrage_mercurl_saber_sol_msol()
				arb.Arbitrage_saber_mercurl_sol_msol()
			}
			if arb.config.SOLSTSOL {
				arb.Arbitrage_mercurl_saber_sol_stsol()
				arb.Arbitrage_saber_mercurl_sol_stsol()
			}
			if arb.config.USDTUSDC3POOL {
				arb.Arbitrage_saber_mercurl_usdc_usdt_3pool()
				arb.Arbitrage_mercurl_saber_usdc_usdt_3pool()
			}
			if arb.config.SOLUSDC {
				arb.Arbitrage_orca_raydium_sol_usdc()
			}
			if arb.config.GSTUSDC {
				arb.Arbitrage_orca_raydium_gst_usdc()
			}
			if arb.config.GMTUSDC {
				arb.Arbitrage_orca_raydium_gmt_usdc()
			}
			if arb.config.WhirlUSTUSDC {
				//arb.Arbitrage_saber_whirl_usdc_ust()
				arb.Arbitrage_whirl_saber_usdc_ust()
			}
			if arb.config.WhirlSOLUSDC {
				arb.Arbitrage_orca_whirl_sol_usdc()
				arb.Arbitrage_whirl_orca_sol_usdc()
			}
			if arb.config.SerumWhirl {
				arb.Arbitrage_serum_whirl_sol_usdc()
				arb.Arbitrage_whirl_serum_sol_usdc()
			}
		case <-arb.ctx.Done():
			return
		}
	}
}

func (arb *Arbitrage) Arbitrage_saber_mercurl_usdc_ust() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("KwnjUuZhTMTSGAaavkLEmSyfobY16JNH4poL9oeeEvE"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9osV5a7FXEjuMujxZJGBRXVAyQ5fJfBFNkyAf6fSz9kw"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("BnKQtTdLw9qPCDgZkWX3sURkBAoKCUYL1yahh6Mw7mRK"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("J63v6qEZmQpDqCD8bd4PXu2Pq5ZbyXrFcSa3Xt1HdAPQ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("BYgyVxdrGa3XNj1cx1XHAVyRG8qYhBnv1DS59Bsvmg5h"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("UST3iPxDFwUUiToMyLF7DYqSP9uaoz7Mzs2LxRYxVJG"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("44K7k9pjjKB6LcWZ1sJ7TvksR3sb4AXpBxwzF1pcEJ5n"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("65sR8agQm768HYCjktunDJG3bbQszi7U8VD4pAKEYiXW"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G5Q7dTUPYw5pEXbfzZbAFSaPstP9bdFmEJ7XXcyrkxVJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EZd87x1Fu1ufV7pVRuXAEcL9y6aEMWWqtpcr7AHdU8ms"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	ust_acc := arb.env.TokenUser(program.UST)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: ust_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 31
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}


func (arb *Arbitrage) Arbitrage_mercurl_saber_usdc_ust() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("KwnjUuZhTMTSGAaavkLEmSyfobY16JNH4poL9oeeEvE"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9osV5a7FXEjuMujxZJGBRXVAyQ5fJfBFNkyAf6fSz9kw"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("J63v6qEZmQpDqCD8bd4PXu2Pq5ZbyXrFcSa3Xt1HdAPQ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("BnKQtTdLw9qPCDgZkWX3sURkBAoKCUYL1yahh6Mw7mRK"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G9nt2GazsDj3Ey3KdA49Sfaq9K95Dc72Ejps4NKTP2SR"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("UST3iPxDFwUUiToMyLF7DYqSP9uaoz7Mzs2LxRYxVJG"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("44K7k9pjjKB6LcWZ1sJ7TvksR3sb4AXpBxwzF1pcEJ5n"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("65sR8agQm768HYCjktunDJG3bbQszi7U8VD4pAKEYiXW"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G5Q7dTUPYw5pEXbfzZbAFSaPstP9bdFmEJ7XXcyrkxVJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EZd87x1Fu1ufV7pVRuXAEcL9y6aEMWWqtpcr7AHdU8ms"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	ust_acc := arb.env.TokenUser(program.UST)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: ust_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 41
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_saber_mercurl_usdc_usdt() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("YAkoNb6HKmSxQN9L8hiBE5tPJRsniSSMzND1boHmZxe"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5C1k9yV7y4CjMnKv8eGYDgWND8P89Pdfj79Trk2qmfGo"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CfWX7o2TswwbxusJ4hCaPobu2jLCb1hfXuXJQjVq3jQF"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EnTrdMMpdhugeH6Ban6gYZWXughWxKtVGfCwFn78ZmY3"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("63aJYYuZddSnCGyE8FNrCVQWnXhjh6CQSRwcDeSMhdVC"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("UST3iPxDFwUUiToMyLF7DYqSP9uaoz7Mzs2LxRYxVJG"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("44K7k9pjjKB6LcWZ1sJ7TvksR3sb4AXpBxwzF1pcEJ5n"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("65sR8agQm768HYCjktunDJG3bbQszi7U8VD4pAKEYiXW"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G5Q7dTUPYw5pEXbfzZbAFSaPstP9bdFmEJ7XXcyrkxVJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EZd87x1Fu1ufV7pVRuXAEcL9y6aEMWWqtpcr7AHdU8ms"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	usdt_acc := arb.env.TokenUser(program.USDT)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdt_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 51
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}


func (arb *Arbitrage) Arbitrage_mercurl_saber_usdc_usdt() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("YAkoNb6HKmSxQN9L8hiBE5tPJRsniSSMzND1boHmZxe"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5C1k9yV7y4CjMnKv8eGYDgWND8P89Pdfj79Trk2qmfGo"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EnTrdMMpdhugeH6Ban6gYZWXughWxKtVGfCwFn78ZmY3"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CfWX7o2TswwbxusJ4hCaPobu2jLCb1hfXuXJQjVq3jQF"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("XZuQG7CQrAA6y6tHM9CLrDjDUWwuUU2SBoV7pLaGDQT"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("UST3iPxDFwUUiToMyLF7DYqSP9uaoz7Mzs2LxRYxVJG"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("44K7k9pjjKB6LcWZ1sJ7TvksR3sb4AXpBxwzF1pcEJ5n"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("65sR8agQm768HYCjktunDJG3bbQszi7U8VD4pAKEYiXW"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G5Q7dTUPYw5pEXbfzZbAFSaPstP9bdFmEJ7XXcyrkxVJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EZd87x1Fu1ufV7pVRuXAEcL9y6aEMWWqtpcr7AHdU8ms"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	usdt_acc := arb.env.TokenUser(program.USDT)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdt_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 61
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_saber_mercurl_usdc_usdt_3pool() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("YAkoNb6HKmSxQN9L8hiBE5tPJRsniSSMzND1boHmZxe"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5C1k9yV7y4CjMnKv8eGYDgWND8P89Pdfj79Trk2qmfGo"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CfWX7o2TswwbxusJ4hCaPobu2jLCb1hfXuXJQjVq3jQF"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EnTrdMMpdhugeH6Ban6gYZWXughWxKtVGfCwFn78ZmY3"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("63aJYYuZddSnCGyE8FNrCVQWnXhjh6CQSRwcDeSMhdVC"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("SWABtvDnJwWwAb9CbSA3nv7nTnrtYjrACAVtuP3gyBB"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2dc3UgMuVkASzW4sABDjDB5PjFbPTncyECUnZL73bmQR"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("J7xkQZ4eCsyHR7XDMcGbHeiFZo6vXPmQC2Va8aqo8jLx"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("bqPxs71QGXNW2SXEvjAaBgLzjhSZfQpmhcVBYoisBo6"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EpehbDhGq8xEc3Yy5nJ2dgY2zWRHaRsbUiMTgh36N8J7"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	usdt_acc := arb.env.TokenUser(program.USDT)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdt_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 51
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}


func (arb *Arbitrage) Arbitrage_mercurl_saber_usdc_usdt_3pool() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("YAkoNb6HKmSxQN9L8hiBE5tPJRsniSSMzND1boHmZxe"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5C1k9yV7y4CjMnKv8eGYDgWND8P89Pdfj79Trk2qmfGo"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EnTrdMMpdhugeH6Ban6gYZWXughWxKtVGfCwFn78ZmY3"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CfWX7o2TswwbxusJ4hCaPobu2jLCb1hfXuXJQjVq3jQF"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("XZuQG7CQrAA6y6tHM9CLrDjDUWwuUU2SBoV7pLaGDQT"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("SWABtvDnJwWwAb9CbSA3nv7nTnrtYjrACAVtuP3gyBB"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2dc3UgMuVkASzW4sABDjDB5PjFbPTncyECUnZL73bmQR"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("J7xkQZ4eCsyHR7XDMcGbHeiFZo6vXPmQC2Va8aqo8jLx"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("bqPxs71QGXNW2SXEvjAaBgLzjhSZfQpmhcVBYoisBo6"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EpehbDhGq8xEc3Yy5nJ2dgY2zWRHaRsbUiMTgh36N8J7"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	usdt_acc := arb.env.TokenUser(program.USDT)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdt_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 61
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_saber_mercurl_sol_msol() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Lee1XZJfJ9Hm2K1qTyeCz1LXNc1YBZaKZszvNY4KCDw"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2Sj4MZvmLhud4uRmGHJvDxq612nmF4JJsU1R4ZjNNGMS"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2hNHZg7XBhuhHVZ3JDEi4buq2fPQwuWBdQ9xkH7t1GQX"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9DgFSWkPDGijNKcLGbr3p5xoJbHsPgXUTr6QvGBJ5vGN"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HzZRDMiJSqS5oxzfu17c35DChnkx58LZtas16Pgmuunn"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MAR1zHjHaQcniE2gXsDptkyKUnNfMEsLBVcfP7vLyv7"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("GcJckEnDiWjpjQ8sqDKuNZjJUKAFZSiuQZ9WmuQpC92a"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EWy2hPdVT4uGrYokx65nAyn2GFBv7bUYA2pFPY96pw7Y"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("GM48qFn8rnqhyNMrBHyPJgUVwXQ1JvMbcu3b9zkThW9L"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.SOL)
	usdt_acc := arb.env.TokenUser(program.MSOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdt_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 71
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}


func (arb *Arbitrage) Arbitrage_mercurl_saber_sol_msol() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Lee1XZJfJ9Hm2K1qTyeCz1LXNc1YBZaKZszvNY4KCDw"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2Sj4MZvmLhud4uRmGHJvDxq612nmF4JJsU1R4ZjNNGMS"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9DgFSWkPDGijNKcLGbr3p5xoJbHsPgXUTr6QvGBJ5vGN"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2hNHZg7XBhuhHVZ3JDEi4buq2fPQwuWBdQ9xkH7t1GQX"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("3oebZVvPqba2egfdcbNXa1uS13SfSebxMaNVE82FMk7R"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MAR1zHjHaQcniE2gXsDptkyKUnNfMEsLBVcfP7vLyv7"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("GcJckEnDiWjpjQ8sqDKuNZjJUKAFZSiuQZ9WmuQpC92a"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EWy2hPdVT4uGrYokx65nAyn2GFBv7bUYA2pFPY96pw7Y"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("GM48qFn8rnqhyNMrBHyPJgUVwXQ1JvMbcu3b9zkThW9L"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.SOL)
	usdt_acc := arb.env.TokenUser(program.MSOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdt_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 81
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_saber_mercurl_sol_stsol() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Lid8SLUxQ9RmF7XMqUA8c24RitTwzja8VSKngJxRcUa"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8eyi347MTDeH5F6eVv2qjPxVnU685FFZLDGcj5QWHZ6y"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AtymwxoVN9peZo7EXTcDz9jKVc4vRmisJKKrNfe3ewBa"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("4PgzyzLtds9bKZ2to9PMnKqJzKEUpjvNUaeN23phegax"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2AbLYRQa7PV6gG6XgMjaey18RtPh85sXFmMmP4HsDdQK"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("LiDoU8ymvYptqxenJ4YpcURBchn4ef63tcbdznBCKJh"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("pG6noYMPVR9ykNgD4XSNa6paKKGGwciU2LckEQPDoSW"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("FqLtqRJvoVNYU5Xpcgp5FNPv8cXCZw7CiNddjQ4nqkRo"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9gizzFG33czvcTzn5N4V23uunV6YR55UNW8ow6qNPoX3"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.SOL)
	usdt_acc := arb.env.TokenUser(program.STSOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdt_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 91
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}


func (arb *Arbitrage) Arbitrage_mercurl_saber_sol_stsol() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Lid8SLUxQ9RmF7XMqUA8c24RitTwzja8VSKngJxRcUa"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8eyi347MTDeH5F6eVv2qjPxVnU685FFZLDGcj5QWHZ6y"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("4PgzyzLtds9bKZ2to9PMnKqJzKEUpjvNUaeN23phegax"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AtymwxoVN9peZo7EXTcDz9jKVc4vRmisJKKrNfe3ewBa"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Cv3YNq8iY1ttMS3iDgwBxd7QxnMC2pwcXUomtR7CTD8W"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("LiDoU8ymvYptqxenJ4YpcURBchn4ef63tcbdznBCKJh"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("pG6noYMPVR9ykNgD4XSNa6paKKGGwciU2LckEQPDoSW"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("FqLtqRJvoVNYU5Xpcgp5FNPv8cXCZw7CiNddjQ4nqkRo"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9gizzFG33czvcTzn5N4V23uunV6YR55UNW8ow6qNPoX3"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.SOL)
	usdt_acc := arb.env.TokenUser(program.STSOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdt_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 101
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_saber_whirl_usdc_ust() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: false})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("KwnjUuZhTMTSGAaavkLEmSyfobY16JNH4poL9oeeEvE"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9osV5a7FXEjuMujxZJGBRXVAyQ5fJfBFNkyAf6fSz9kw"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("BnKQtTdLw9qPCDgZkWX3sURkBAoKCUYL1yahh6Mw7mRK"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("J63v6qEZmQpDqCD8bd4PXu2Pq5ZbyXrFcSa3Xt1HdAPQ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("BYgyVxdrGa3XNj1cx1XHAVyRG8qYhBnv1DS59Bsvmg5h"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HyBtxWGiYKzHG5WERyA6Ks9tq6k33UyXrU9Aj1BUFK2J"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CWArXimAPJrsmDXHA93aYn9ezcXoPfj6zdVZMJddiH1f"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8ahvzqK1Ljtopm3fR3AHRAypYFbdXTXn7FLYuXUfeH4o"), IsSigner: false, IsWritable: true})
	//accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("jPBxUQHcj3TRHcufH9CEQd5fMCFUMNHUWSxEuJ546Qu"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("7H8uttY2in2XpDC8jcgrr1S8TDW8hcPNjtCUHTF68mcR"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("3Jb2dZhYzB3rXgD4zxpkVvWtt1WFXaPk7WvYGVaGMHwn"), IsSigner: false, IsWritable: false})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	ust_acc := arb.env.TokenUser(program.UST)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: ust_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 151
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_whirl_saber_usdc_ust() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: false})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("KwnjUuZhTMTSGAaavkLEmSyfobY16JNH4poL9oeeEvE"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9osV5a7FXEjuMujxZJGBRXVAyQ5fJfBFNkyAf6fSz9kw"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("J63v6qEZmQpDqCD8bd4PXu2Pq5ZbyXrFcSa3Xt1HdAPQ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("BnKQtTdLw9qPCDgZkWX3sURkBAoKCUYL1yahh6Mw7mRK"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G9nt2GazsDj3Ey3KdA49Sfaq9K95Dc72Ejps4NKTP2SR"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HyBtxWGiYKzHG5WERyA6Ks9tq6k33UyXrU9Aj1BUFK2J"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CWArXimAPJrsmDXHA93aYn9ezcXoPfj6zdVZMJddiH1f"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8ahvzqK1Ljtopm3fR3AHRAypYFbdXTXn7FLYuXUfeH4o"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("jPBxUQHcj3TRHcufH9CEQd5fMCFUMNHUWSxEuJ546Qu"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("3Jb2dZhYzB3rXgD4zxpkVvWtt1WFXaPk7WvYGVaGMHwn"), IsSigner: false, IsWritable: false})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	ust_acc := arb.env.TokenUser(program.UST)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: ust_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 152
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_orca_whirl_sol_usdc() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.OrcaV2, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("JU8kmKzDHF9sXWsnoznaFDFezLsE5uomX2JkRMbmsQP"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("ANP74VNsHwSrq9uUSjiSNyNWvf6ZPrKTmE4gHoNd13Lg"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("75HgnSvXbWKZBpZHveX68ZzAhDqMzNDS29X6BGLtxMo1"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("APDFRM3HMr8CAGXwKHiu2f5ePSpaiEJhaURwhsRrUUt9"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8JnSiuvQq3BVuCU3n4DrSTw9chBSPvEMswrhtifVkr1o"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HJPjoWUrhoZzkNfRpHuieeFk9WcZWjwy6PBjZ81ngndJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("3YQm7ujtXWJU2e9jhp2QGHpnn1ShXn12QjvzMvDgabpX"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2JTw1fE2wz1SymWUQ7UqpVtrTuKjcd6mWwYwUJUCh2rq"), IsSigner: false, IsWritable: true})
	//accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("jPBxUQHcj3TRHcufH9CEQd5fMCFUMNHUWSxEuJ546Qu"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2Eh8HEeu45tCWxY6ruLLRN6VcTSD7bfshGj7bZA87Kne"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("4GkRbcYg1VKsZropgai4dMf2Nj2PkXNLf43knFpavrSi"), IsSigner: false, IsWritable: false})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	ust_acc := arb.env.TokenUser(program.SOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: ust_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 167
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_whirl_orca_sol_usdc() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.OrcaV2, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("JU8kmKzDHF9sXWsnoznaFDFezLsE5uomX2JkRMbmsQP"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("ANP74VNsHwSrq9uUSjiSNyNWvf6ZPrKTmE4gHoNd13Lg"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("75HgnSvXbWKZBpZHveX68ZzAhDqMzNDS29X6BGLtxMo1"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("APDFRM3HMr8CAGXwKHiu2f5ePSpaiEJhaURwhsRrUUt9"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8JnSiuvQq3BVuCU3n4DrSTw9chBSPvEMswrhtifVkr1o"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HJPjoWUrhoZzkNfRpHuieeFk9WcZWjwy6PBjZ81ngndJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("3YQm7ujtXWJU2e9jhp2QGHpnn1ShXn12QjvzMvDgabpX"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2JTw1fE2wz1SymWUQ7UqpVtrTuKjcd6mWwYwUJUCh2rq"), IsSigner: false, IsWritable: true})
	//accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("jPBxUQHcj3TRHcufH9CEQd5fMCFUMNHUWSxEuJ546Qu"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2Eh8HEeu45tCWxY6ruLLRN6VcTSD7bfshGj7bZA87Kne"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("4GkRbcYg1VKsZropgai4dMf2Nj2PkXNLf43knFpavrSi"), IsSigner: false, IsWritable: false})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	ust_acc := arb.env.TokenUser(program.SOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: ust_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 168
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_serum_whirl_sol_usdc() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SerumV22, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9wFFyRfZBsuAha4YcuxcXLKwMxJR43S7fPfQLusDBzvT"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("fYaDiwbBY4ZWrAMnryNEZK2ADdHunWWycT7nDzqRAQJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AZG3tFCFtiCqEwyardENBQNpHqxgzbMw8uKeZEw2nRG5"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5KKsLVU6TcbVDK4BS6K1DGDxnh4Q9xjYJ8XaDCG5t8ht"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("14ivtgssEBoBjuZJtSAPKYgpUK7DmnSwuPMqJoVTSgKJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CEQdAFKdycHugujQg9k2wbmxjcpdYZyVLfV9WerTnafJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("36c6YqAwyGKQG66XEp2dJc5JqjaBNv7sVghEtJv4c7u6"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8CFo8bL8mZQK8abbFyypFMwEDd8tVJjHTTojMLgQTUSZ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("F8Vyqk3unwxkXukZFQeYyGmFfTG3CAX4v24iyrjEYBJV"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Lid8SLUxQ9RmF7XMqUA8c24RitTwzja8VSKngJxRcUa"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8eyi347MTDeH5F6eVv2qjPxVnU685FFZLDGcj5QWHZ6y"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AtymwxoVN9peZo7EXTcDz9jKVc4vRmisJKKrNfe3ewBa"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("4PgzyzLtds9bKZ2to9PMnKqJzKEUpjvNUaeN23phegax"), IsSigner: false, IsWritable: true})
	//accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("jPBxUQHcj3TRHcufH9CEQd5fMCFUMNHUWSxEuJ546Qu"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Cv3YNq8iY1ttMS3iDgwBxd7QxnMC2pwcXUomtR7CTD8W"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2AbLYRQa7PV6gG6XgMjaey18RtPh85sXFmMmP4HsDdQK"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Whirl, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AXtdSZ2mpagmtM5aipN5kV9CyGBA8dxhSBnqMRp7UpdN"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5GXtHDjrM1okAYJZfrmUwpphcsSsLAJEVsn4qx5epZd"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("7HxXmF3PE6oQeoxofLidBKMjvig2L8WFAGbHWLqEYcP1"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HJzLnhuYDB5SscB6s2eQ4tWJceTwHq5gKj2BRSmy2Yzn"), IsSigner: false, IsWritable: true})
	//accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("jPBxUQHcj3TRHcufH9CEQd5fMCFUMNHUWSxEuJ546Qu"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AENaN27djcuLeDFwkcwmHMUiwnXESNcq8c3eARD4nwUd"), IsSigner: false, IsWritable: false})

	usdc_acc := arb.env.TokenUser(program.USDC)
	stsol_acc := arb.env.TokenUser(program.STSOL)
	sol_acc := arb.env.TokenUser(program.SOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: stsol_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: sol_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysRent, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Raydium, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0;i < 2;i ++{
		// very dangerous
		data := make([]byte, 2)
		data[0] = 12
		data[1] = arb.nonce

		instruction := &program.Instruction{
			IsAccounts:  accounts,
			IsData:      data,
			IsProgramID: program.Arbitrage,
		}
		ins = append(ins, instruction)
	}
	{
		id := uint64(time.Now().UnixNano() / 1000)
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_whirl_serum_sol_usdc() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SerumV22, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9wFFyRfZBsuAha4YcuxcXLKwMxJR43S7fPfQLusDBzvT"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("fYaDiwbBY4ZWrAMnryNEZK2ADdHunWWycT7nDzqRAQJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AZG3tFCFtiCqEwyardENBQNpHqxgzbMw8uKeZEw2nRG5"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5KKsLVU6TcbVDK4BS6K1DGDxnh4Q9xjYJ8XaDCG5t8ht"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("14ivtgssEBoBjuZJtSAPKYgpUK7DmnSwuPMqJoVTSgKJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CEQdAFKdycHugujQg9k2wbmxjcpdYZyVLfV9WerTnafJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("36c6YqAwyGKQG66XEp2dJc5JqjaBNv7sVghEtJv4c7u6"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8CFo8bL8mZQK8abbFyypFMwEDd8tVJjHTTojMLgQTUSZ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("F8Vyqk3unwxkXukZFQeYyGmFfTG3CAX4v24iyrjEYBJV"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Lid8SLUxQ9RmF7XMqUA8c24RitTwzja8VSKngJxRcUa"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8eyi347MTDeH5F6eVv2qjPxVnU685FFZLDGcj5QWHZ6y"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AtymwxoVN9peZo7EXTcDz9jKVc4vRmisJKKrNfe3ewBa"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("4PgzyzLtds9bKZ2to9PMnKqJzKEUpjvNUaeN23phegax"), IsSigner: false, IsWritable: true})
	//accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("jPBxUQHcj3TRHcufH9CEQd5fMCFUMNHUWSxEuJ546Qu"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("Cv3YNq8iY1ttMS3iDgwBxd7QxnMC2pwcXUomtR7CTD8W"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("2AbLYRQa7PV6gG6XgMjaey18RtPh85sXFmMmP4HsDdQK"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Whirl, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AXtdSZ2mpagmtM5aipN5kV9CyGBA8dxhSBnqMRp7UpdN"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5GXtHDjrM1okAYJZfrmUwpphcsSsLAJEVsn4qx5epZd"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("7HxXmF3PE6oQeoxofLidBKMjvig2L8WFAGbHWLqEYcP1"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HJzLnhuYDB5SscB6s2eQ4tWJceTwHq5gKj2BRSmy2Yzn"), IsSigner: false, IsWritable: true})
	//accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("jPBxUQHcj3TRHcufH9CEQd5fMCFUMNHUWSxEuJ546Qu"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("AENaN27djcuLeDFwkcwmHMUiwnXESNcq8c3eARD4nwUd"), IsSigner: false, IsWritable: false})

	usdc_acc := arb.env.TokenUser(program.USDC)
	stsol_acc := arb.env.TokenUser(program.STSOL)
	sol_acc := arb.env.TokenUser(program.SOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: stsol_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: sol_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysRent, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Raydium, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	{
		// very dangerous
		data := make([]byte, 2)
		data[0] = 13
		data[1] = arb.nonce
		instruction := &program.Instruction{
			IsAccounts:  accounts,
			IsData:      data,
			IsProgramID: program.Arbitrage,
		}
		ins = append(ins, instruction)
	}
	{
		id := uint64(time.Now().UnixNano() / 1000)
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_orca_raydium_sol_usdc() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Raydium, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("58oQChx4yWmvKdwLLZzBi4ChoCc2fqCUWBkwMihLYQo2"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HRk9CMrpq7Jn9sh7mzxE8CChHG8dneX9p475QKz4Fsfc"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CZza3Ej4Mc58MnxWA385itCC9jCo3L1D7zc3LKy1bZMR"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("DQyrAcCrDXQ7NeoqGgDCZwBvWDcYmFCjSb9JtteuvPpz"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("HLmqeL62xR1QoZ1HKKbXRrdN1p3phKpxRMb2VVopvBBz"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SerumV22, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9wFFyRfZBsuAha4YcuxcXLKwMxJR43S7fPfQLusDBzvT"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("14ivtgssEBoBjuZJtSAPKYgpUK7DmnSwuPMqJoVTSgKJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("CEQdAFKdycHugujQg9k2wbmxjcpdYZyVLfV9WerTnafJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("5KKsLVU6TcbVDK4BS6K1DGDxnh4Q9xjYJ8XaDCG5t8ht"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("36c6YqAwyGKQG66XEp2dJc5JqjaBNv7sVghEtJv4c7u6"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8CFo8bL8mZQK8abbFyypFMwEDd8tVJjHTTojMLgQTUSZ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("F8Vyqk3unwxkXukZFQeYyGmFfTG3CAX4v24iyrjEYBJV"), IsSigner: false, IsWritable: false})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.OrcaV2, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("JU8kmKzDHF9sXWsnoznaFDFezLsE5uomX2JkRMbmsQP"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("ANP74VNsHwSrq9uUSjiSNyNWvf6ZPrKTmE4gHoNd13Lg"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("75HgnSvXbWKZBpZHveX68ZzAhDqMzNDS29X6BGLtxMo1"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("APDFRM3HMr8CAGXwKHiu2f5ePSpaiEJhaURwhsRrUUt9"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("8JnSiuvQq3BVuCU3n4DrSTw9chBSPvEMswrhtifVkr1o"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	other_acc := arb.env.TokenUser(program.SOL)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: other_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize2; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 1
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize2-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_orca_raydium_gst_usdc() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("KwnjUuZhTMTSGAaavkLEmSyfobY16JNH4poL9oeeEvE"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9osV5a7FXEjuMujxZJGBRXVAyQ5fJfBFNkyAf6fSz9kw"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("J63v6qEZmQpDqCD8bd4PXu2Pq5ZbyXrFcSa3Xt1HdAPQ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("BnKQtTdLw9qPCDgZkWX3sURkBAoKCUYL1yahh6Mw7mRK"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G9nt2GazsDj3Ey3KdA49Sfaq9K95Dc72Ejps4NKTP2SR"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("UST3iPxDFwUUiToMyLF7DYqSP9uaoz7Mzs2LxRYxVJG"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("44K7k9pjjKB6LcWZ1sJ7TvksR3sb4AXpBxwzF1pcEJ5n"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("65sR8agQm768HYCjktunDJG3bbQszi7U8VD4pAKEYiXW"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G5Q7dTUPYw5pEXbfzZbAFSaPstP9bdFmEJ7XXcyrkxVJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EZd87x1Fu1ufV7pVRuXAEcL9y6aEMWWqtpcr7AHdU8ms"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	ust_acc := arb.env.TokenUser(program.UST)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: ust_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 41
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
	}
	return nil
}

func (arb *Arbitrage) Arbitrage_orca_raydium_gmt_usdc() error {
	ins := make([]solana.Instruction, 0)
	//
	accounts := make([]*solana.AccountMeta, 0)
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Exchange, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Saber, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("KwnjUuZhTMTSGAaavkLEmSyfobY16JNH4poL9oeeEvE"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("9osV5a7FXEjuMujxZJGBRXVAyQ5fJfBFNkyAf6fSz9kw"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("J63v6qEZmQpDqCD8bd4PXu2Pq5ZbyXrFcSa3Xt1HdAPQ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("BnKQtTdLw9qPCDgZkWX3sURkBAoKCUYL1yahh6Mw7mRK"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G9nt2GazsDj3Ey3KdA49Sfaq9K95Dc72Ejps4NKTP2SR"), IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("UST3iPxDFwUUiToMyLF7DYqSP9uaoz7Mzs2LxRYxVJG"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("44K7k9pjjKB6LcWZ1sJ7TvksR3sb4AXpBxwzF1pcEJ5n"), IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("65sR8agQm768HYCjktunDJG3bbQszi7U8VD4pAKEYiXW"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("G5Q7dTUPYw5pEXbfzZbAFSaPstP9bdFmEJ7XXcyrkxVJ"), IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: solana.MustPublicKeyFromBase58("EZd87x1Fu1ufV7pVRuXAEcL9y6aEMWWqtpcr7AHdU8ms"), IsSigner: false, IsWritable: true})
	//
	usdc_acc := arb.env.TokenUser(program.USDC)
	ust_acc := arb.env.TokenUser(program.UST)

	accounts = append(accounts, &solana.AccountMeta{PublicKey: arb.config.User, IsSigner: true, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: usdc_acc, IsSigner: false, IsWritable: true})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: ust_acc, IsSigner: false, IsWritable: true})
	//
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.Token, IsSigner: false, IsWritable: false})
	accounts = append(accounts, &solana.AccountMeta{PublicKey: program.SysClock, IsSigner: false, IsWritable: false})

	arb.nonce++
	arb.nonce = arb.nonce % 90
	for i := 0; i < arb.config.InstructionSize; i++ {
		// very dangerous
		data := make([]byte, 3)
		data[0] = 41
		data[1] = byte(i)
		data[2] = arb.nonce
		if i == arb.config.InstructionSize-1 {
			data[1] = byte(100)
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
		arb.backend.Commit(arb.blockHash, id, ins, false, nil, nil)
		arb.blockHash++
		arb.blockHash = arb.blockHash % 3
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
