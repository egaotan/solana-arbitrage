package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/balancelisten"
	"github.com/egaotan/solana-arbitrage/calculator"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/dingsdk"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/networkdetect"
	"github.com/egaotan/solana-arbitrage/orca"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/saber"
	"github.com/egaotan/solana-arbitrage/serumv1"
	"github.com/egaotan/solana-arbitrage/serumv2"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/egaotan/solana-arbitrage/statelisten"
	"github.com/egaotan/solana-arbitrage/store"
	"github.com/egaotan/solana-arbitrage/system"
	"github.com/egaotan/solana-arbitrage/tokenswap"
	"github.com/gagliardetto/solana-go"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	Init    = int32(0)
	Started = int32(1)
	Pause   = int32(2)
	Stopped = int32(3)
)

var (
	CacheSize = 1
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
	status       int32
	trade        chan *InfoUpdated
	backend      *backend.Backend
	env          *env.Env
	splToken     *spltoken.Program
	system       *system.Program
	programs     map[solana.PublicKey]program.Program
	tokens       map[solana.PublicKey]bool
	swapAccounts map[solana.PublicKey]bool
	//calculators   map[string]calculator.Calculator
	calculators      []calculator.Calculator
	validYield       int64
	nodeId int
	store            *store.Store
	balanceListen    *balancelisten.BalanceListen
	notify           *Notify
	stateListen      *statelisten.StateListen
	httpServer       *http.Server
	rpcPort          string
	cache            map[string][]*ArbitrageData
	selector         int64
	startTime        int64
	latestCommitTime int64
	nd *networkdetect.NetworkDetector
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

func NewCalculator(name string, ctx context.Context, env *env.Env, cb calculator.Callback) calculator.Calculator {
	if name == calculator.CyclicArb {
		return calculator.NewCyclic(calculator.SBF, ctx, env, cb)
	}
	if name == calculator.SerumArb {
		return calculator.NewSerum(calculator.SBF, ctx, env, cb)
	}
	panic(fmt.Errorf("calculator (%s) is not support", name))
}

func NewArbitrage(ctx context.Context, cfg *config.Config) *Arbitrage {
	arb := &Arbitrage{
		ctx:          ctx,
		config:       cfg,
		trade:        make(chan *InfoUpdated),
		tokens:       make(map[solana.PublicKey]bool),
		swapAccounts: make(map[solana.PublicKey]bool),
		cache:        make(map[string][]*ArbitrageData),
		validYield:   cfg.ValidYield,
		rpcPort:      cfg.Listen,
		nodeId: cfg.NodeId,
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
	//peer, ttl := networkdetect.DetectPeers(cfg.Nodes[0].Wss)
	//logger.Printf("peer: %s, ttl: %d", peer, ttl/1000000)
	//
	store := store.NewStore(ctx, cfg.DBUrl, cfg.DBScheme, cfg.DBUser, cfg.DBPasswd)
	arb.store = store
	backend := backend.NewBackend(ctx, cfg.Nodes, true, cfg.TransactionNodes, cfg.BlochHash, cfg.TpuClient, cfg.TransactionSend)
	backend.ImportWallet(cfg.Key)
	backend.SetPlayer(cfg.User)
	backend.SetStore(store)
	arb.backend = backend
	splToken := spltoken.NewProgram(ctx, backend, arb)
	arb.splToken = splToken
	system := system.NewProgram(ctx, backend)
	arb.system = system
	env := env.NewEnv(ctx)
	arb.env = env
	//
	dsdk := dingsdk.NewDingSdk(cfg.DingUrl)
	arb.balanceListen = balancelisten.NewBalanceListen(ctx, splToken, cfg.UsdcAccount, dsdk)
	arb.notify = NewNotify(ctx, env, dsdk)
	arb.nd = networkdetect.NewNetworkDetector(cfg.Nodes[0].Ws, dsdk)
	//
	programs := make(map[solana.PublicKey]program.Program)
	for _, program := range cfg.Programs {
		programs[program] = NewProgram(program, ctx, cfg.Which, env, backend, splToken, system, arb)
	}
	arb.programs = programs
	arb.stateListen = statelisten.NewStateListen(ctx, programs, env)
	arbitrages := make([]calculator.Calculator, 0, len(cfg.Calculators))
	for _, arbitrage := range cfg.Calculators {
		arbitrages = append(arbitrages, NewCalculator(arbitrage, ctx, env, arb))
	}
	arb.calculators = arbitrages
	arb.status = Init
	return arb
}

func (arb *Arbitrage) Service() {
	arb.Start()
	arb.StartRPC()
	<-arb.ctx.Done()
	arb.StopRPC()
	arb.Stop()
}

func (arb *Arbitrage) Start() {
	arb.nd.Start()
	arb.store.Start()
	arb.backend.Start()
	arb.env.Start()
	if err := arb.splToken.Start(); err != nil {
		arb.log.Printf("spl token program start err: %v", err)
	}
	if err := arb.system.Start(); err != nil {
		arb.log.Printf("system program start err: %v", err)
	}
	arb.balanceListen.Start()
	arb.notify.Start()
	for _, program := range arb.programs {
		if err := program.Start(); err != nil {
			arb.log.Printf("program %s start err: %v", program.Name(), err)
		}
	}
	for _, arbitrage := range arb.calculators {
		if err := arbitrage.Start(); err != nil {
			arb.log.Printf("calculator %s:%s start err: %v", arbitrage.Name(), arbitrage.Algorithm(), err)
		}
	}
	for _, program := range arb.programs {
		if err := program.Flash(); err != nil {
			arb.log.Printf("program %s flash err: %v", program.Name(), err)
		}
	}
	arb.stateListen.Start()
	arb.wg.Add(1)
	go arb.Tick()
	arb.backend.SubscribeSlot(arb)
	arb.status = Started
	arb.startTime = time.Now().Unix()
	arb.log.Printf("auto trader has started......")
}

func (arb *Arbitrage) Stop() {
	arb.nd.Stop()
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
	for _, calculator := range arb.calculators {
		if err := calculator.Stop(); err != nil {
			arb.log.Printf("calculator %s:%s stop err: %v", calculator.Name(), calculator.Algorithm(), err)
		}
	}
	arb.env.Stop()
	arb.store.Stop()
	arb.save2Cache()
	arb.status = Stopped
	arb.log.Printf("auto trader has stopped......")
}

func (arb *Arbitrage) StartRPC() {
	router := gin.New()
	g := router.Group("/api")
	g.GET("/arbitrage", arb.getArbitrage)
	arb.httpServer = &http.Server{
		Addr:    arb.rpcPort,
		Handler: router,
	}
	arb.log.Printf("start rpc server......")
	go func() {
		if err := arb.httpServer.ListenAndServe(); err != nil {
			arb.log.Printf("ListenAndServe: %s", err.Error())
		}
	}()
}

func (arb *Arbitrage) StopRPC() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := arb.httpServer.Shutdown(ctx); err != nil {
		panic(err)
	}
	arb.log.Printf("rpc server has stopped......")
}

func (arb *Arbitrage) save2Cache() {
	{
		infoJson, _ := json.MarshalIndent(arb.tokens, "", "    ")
		err := os.WriteFile(config.MarketUsableTokensFile, infoJson, 0644)
		if err != nil {
			panic(err)
		}
	}
	{
		infoJson, _ := json.MarshalIndent(arb.swapAccounts, "", "    ")
		err := os.WriteFile(config.MarketUsablePoolAccounts, infoJson, 0644)
		if err != nil {
			panic(err)
		}
	}
}

func (arb *Arbitrage) OnModelInit(model program.Model) error {
	if !arb.env.UseMarket(model.Program(), model.Id()) {
		return nil
	}

	tokenPairs := model.TokenPair()
	arb.tokens[tokenPairs[0]] = true
	arb.tokens[tokenPairs[1]] = true

	poolPairs := model.PoolPair()
	arb.swapAccounts[poolPairs[0]] = true
	arb.swapAccounts[poolPairs[1]] = true

	for _, calculator := range arb.calculators {
		if err := calculator.AddModel(model); err != nil {
			return err
		}
	}
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
	locked := atomic.CompareAndSwapInt32(&arb.status, Started, Pause)
	if !locked {
		return nil
	}
	arb.trade <- &InfoUpdated{
		Slot: slot.Number,
	}
	return nil
}

func (arb *Arbitrage) OnBalanceUpdate(userKey solana.PublicKey, newBalance uint64, oldBalance uint64, tokenKey solana.PublicKey, slot uint64) error {
	locked := atomic.CompareAndSwapInt32(&arb.status, Started, Pause)
	if !locked {
		return nil
	}
	arb.trade <- &InfoUpdated{
		Slot:       slot,
		UserKey:    userKey,
		NewBalance: newBalance,
		OldBalance: oldBalance,
		TokenKey:   tokenKey,
	}
	return nil
}

func (arb *Arbitrage) OnStateUpdate(slot uint64) error {
	locked := atomic.CompareAndSwapInt32(&arb.status, Started, Pause)
	if !locked {
		return nil
	}
	arb.trade <- &InfoUpdated{
		Slot: slot,
	}
	return nil
}

func (arb *Arbitrage) Tick() {
	defer arb.wg.Done()
	for {
		select {
		case info := <-arb.trade:
			arb.try(info)
			atomic.StoreInt32(&arb.status, Started)
		case <-arb.ctx.Done():
			arb.log.Printf("trade tick exit")
			return
		}
	}
}

func (arb *Arbitrage) try(info *InfoUpdated) {
	if !info.TokenKey.IsZero() && info.NewBalance != 0 && info.OldBalance != 0 {
		balance1 := decimal.NewFromInt(int64(info.NewBalance))
		balance2 := decimal.NewFromInt(int64(info.OldBalance))
		balanceDiff := balance1.Sub(balance2).Abs()
		token := arb.env.Token(info.TokenKey)
		balanceUi := token.AmountUi(balanceDiff.BigInt().Uint64())
		usdAmount := balanceUi.Mul(token.Price)
		if usdAmount.Cmp(decimal.NewFromInt(10000)) < 0 {
			return
		}
	}
	arb.log.Printf("**************** slot update: %d ****************", info.Slot)
	arb.selector = 0
	for _, calculator := range arb.calculators {
		calculator.Calculate()
	}
	//
	/*
	tt := time.Now().Unix()
	if tt-arb.startTime > 12*60*60 && tt-arb.latestCommitTime > 5*60 {
		// restart
		arb.log.Printf("restart server")
		syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
	}
	*/
}

func (arb *Arbitrage) OnArbitrage(result *calculator.Result) error {
	yield := (int64(result.AmountOut) - int64(result.AmountIn)) * 10000 / int64(result.AmountIn)
	arb.log.Printf("got an calculator, yield: %d, id: %d, arbitrager: %s", yield, result.Id, result.Calculator)
	err := arb.Arbitrage(result.Id, result.TokenIn, result.AmountIn, result.UsdcAmount, result.Models, yield)
	if err != nil {
		arb.log.Printf("calculator err: %v", err)
	}
	return nil
}

func (arb *Arbitrage) Arbitrage(id uint64, token solana.PublicKey, amount uint64, usdcAmount uint64, models []program.Model, yield int64) error {
	//
	if arb.selector == 0 {
		arb.selector = yield
	} else if yield-arb.selector < 20 {
		arb.log.Printf("has arbitrage in cyclic")
		return nil
	}
	//
	if yield < arb.validYield {
		arb.log.Printf("the yield is %d, too low, retry......", yield)
		return nil
	}
	if usdcAmount < 100*1000000 {
		arb.log.Printf("usdc amount is too small, %d", usdcAmount)
		return nil
	}
	arb.log.Printf("try this calculator......")
	//
	cacheSize := CacheSize
	level := 0
	if usdcAmount < 10000*1000000 {
		if yield > 200 {
			cacheSize += 1
			level ++
		}
	} else if usdcAmount < 20000*1000000 {
		if yield > 150 {
			cacheSize += 1
			level ++
		}
		if yield > 350 {
			cacheSize += 1
			level ++
		}
	} else if usdcAmount > 20000*1000000 {
		if yield > 150 {
			cacheSize += 1
			level ++
		}
		if yield > 300 {
			cacheSize += 1
			level ++
		}
	}

	sb := strings.Builder{}
	for _, model := range models {
		sb.Write(model.Id().Bytes())
	}
	cacheKey := sb.String()
	caches := arb.cache[cacheKey]
	newCaches := make([]*ArbitrageData, 0, 3)
	for _, cache := range caches {
		if id - cache.id < 60 * 1000000 {
			newCaches = append(newCaches, cache)
		}
	}
	caches = newCaches
	if len(caches) >= cacheSize {
		amountDiff := decimal.NewFromInt(int64(usdcAmount)).Sub(decimal.NewFromInt(int64(caches[0].initUsdcAmount))).Abs()
		yieldDiff := decimal.NewFromInt(yield).Sub(decimal.NewFromInt(caches[0].yield)).Abs()
		if amountDiff.Cmp(decimal.NewFromInt(1000*1000000)) < 0 && yieldDiff.Cmp(decimal.NewFromInt(100)) < 0 {
			arb.log.Printf("so much arbitrage transactions in 60 seconds, usdc amount: %d......", usdcAmount)
			return nil
		}
	}
	//
	data := &ArbitrageData{
		id:             id,
		initUsdcAmount: usdcAmount,
		steps:          make([]*ArbitrageStep, 0),
	}
	if arb.config.Local {
		cToken := token
		cAmount := amount
		for _, m := range models {
			p, ok := arb.programs[m.Program()]
			if !ok {
				return fmt.Errorf("program %s is invalid", m.Program())
			}
			parameter := make(map[string]interface{})
			parameter["market"] = m.Id()
			parameter["token"] = cToken
			parameter["amount"] = cAmount
			localState, err := p.Local(parameter)
			if err != nil {
				return err
			}
			//
			data.steps = append(data.steps, &ArbitrageStep{
				program:   p,
				model:     m,
				tokenIn:   localState.TokenIn,
				amountIn:  localState.AmountIn,
				slotIn:    localState.SlotIn,
				tokenOut:  localState.TokenOut,
				amountOut: localState.AmountOut,
				slotOut:   localState.SlotOut,
			})
			//
			cToken = localState.TokenOut
			cAmount = localState.AmountOut
		}
		data.yield = (int64(cAmount) - int64(amount)) * 10000 / int64(amount)
	}

	if data.yield < arb.validYield {
		arb.log.Printf("the yield is %d, too low, retry......", yield)
		return nil
	}
	//
	caches = append(caches, data)
	if len(caches) > cacheSize {
		caches = caches[len(caches)-cacheSize:]
	}
	arb.cache[cacheKey] = caches
	//
	blockHashIndex := (len(caches) - 1 + arb.nodeId) % cacheSize
	//
	defer func() {
		committedArbitrage := &store.CommittedArbitrage{
			Id:                      data.id,
			Amount:                  data.amount,
			CommittedArbitrageSteps: make([]*store.CommittedArbitrageStep, 0, len(data.steps)),
		}
		for _, step := range data.steps {
			committedArbitrageStep := &store.CommittedArbitrageStep{
				Program:  step.program.Id().String(),
				Market:   step.model.Id().String(),
				TokenIn:  step.tokenIn.String(),
				AmountIn: step.amountIn,
				TokenOut: step.tokenOut.String(),
				AmountOut: step.amountOut,
			}
			committedArbitrage.CommittedArbitrageSteps = append(committedArbitrage.CommittedArbitrageSteps, committedArbitrageStep)
		}
		arb.store.StoreCommittedArbitrage(committedArbitrage)
		arb.notify.Commit(data)
	}()

	localArbitrage := &store.LocalArbitrage{
		Id:                  data.id,
		Yield:               data.yield,
		LocalArbitrageSteps: make([]*store.LocalArbitrageStep, 0, len(data.steps)),
	}
	for _, step := range data.steps {
		localStep := &store.LocalArbitrageStep{
			Program:   step.program.Id().String(),
			Market:    step.model.Id().String(),
			TokenIn:   step.tokenIn.String(),
			AmountIn:  step.amountIn,
			SlotIn:    step.slotIn,
			TokenOut:  step.tokenOut.String(),
			AmountOut: step.amountOut,
			SlotOut:   step.slotOut,
		}
		localArbitrage.LocalArbitrageSteps = append(localArbitrage.LocalArbitrageSteps, localStep)
	}
	arb.store.StoreLocalArbitrage(localArbitrage)

	// reorder, usdc must be first
	i := 0
	for i = 0; i < len(data.steps); i++ {
		if data.steps[i].tokenIn == program.USDC {
			break
		}
	}
	if i == len(data.steps) {
		arb.log.Printf("there is no usdc in this paths")
		return nil
	}
	newSteps := make([]*ArbitrageStep, 0, len(data.steps))
	for j := i; j < len(data.steps); j++ {
		newSteps = append(newSteps, data.steps[j])
	}
	for j := 0; j < i; j++ {
		newSteps = append(newSteps, data.steps[j])
	}
	data.steps = newSteps
	data.amount = data.steps[0].amountIn
	// update first usdc amount
	if data.amount > config.USDC_AMOUNT {
		data.amount = config.USDC_AMOUNT
	} else if data.amount < 200000000 && data.yield < 200 {
		arb.log.Printf("amount is too small")
		return nil
	}
	data.amount = (data.amount / 100000000) * 100000000
	if true {
		ins := make([]solana.Instruction, 0)
		for j, step := range data.steps {
			p := step.program
			m := step.model
			tokenIn := step.tokenIn
			amountIn := data.amount
			key := m.Id()
			parameter := make(map[string]interface{})
			parameter["market"] = key
			parameter["token"] = tokenIn
			parameter["amount"] = amountIn
			if j == 0 {
				parameter["flag"] = uint8(0)
			} else if j == len(data.steps)-1 {
				parameter["flag"] = uint8(2)
			} else {
				parameter["flag"] = uint8(1)
			}
			result, err := p.ArbitrageStep(parameter)
			if err != nil {
				arb.log.Printf("err: %v", err)
				return err
			}
			ins = append(ins, result...)
		}
		arb.backend.Commit(blockHashIndex, id, ins, false, nil)
		arb.latestCommitTime = time.Now().Unix()
	}
	return nil
}

func (arb *Arbitrage) OnNetwork(ttl int64) {

}

type ArbitrageInfo struct {
	LocalArbitrages     []*LocalArbitrage
	CommittedArbitrages []*CommittedArbitrage
	ExecutedArbitrages  []*ExecutedArbitrage
}

func (arb *Arbitrage) getArbitrage(c *gin.Context) {
	idStr, ok := c.GetQuery("id")
	if !ok {
		c.JSON(500, "parameter is invalid")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(500, err)
		return
	}
	localArbitrages, err := arb.store.GetLocalArbitrage(id)
	if err != nil {
		c.JSON(500, err)
		return
	}
	committedArbitrages, err := arb.store.GetCommittedArbitrage(id)
	if err != nil {
		c.JSON(500, err)
		return
	}
	executedArbitrages, err := arb.store.GetExecutedArbitrage(id)
	if err != nil {
		c.JSON(500, err)
		return
	}
	c.JSON(200, &ArbitrageInfo{
		LocalArbitrages:     buildLocalArbitrages(localArbitrages, arb.env),
		CommittedArbitrages: buildCommittedArbitrages(committedArbitrages, arb.env),
		ExecutedArbitrages:  buildExecutedArbitrages(executedArbitrages),
	})
}
