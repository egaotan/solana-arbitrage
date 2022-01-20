package serumv1

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/egaotan/solana-arbitrage/system"
	"github.com/gagliardetto/solana-go"
	"log"
	"os"
)

type Program struct {
	backend         *backend.Backend
	env             *env.Env
	which           int
	log             *log.Logger
	ctx             context.Context
	cb              program.Callback
	id              solana.PublicKey
	splTokenProgram *spltoken.Program
	systemProgram   *system.Program
	unknownAccounts map[solana.PublicKey]bool
	markets         map[solana.PublicKey]*KeyedMarket
	requests        map[solana.PublicKey]*KeyedRequest
	events          map[solana.PublicKey]*KeyedEvent
	orderBooks      map[solana.PublicKey]*KeyedOrderBook
	openOrders      map[solana.PublicKey]*KeyedOpenOrders
	models          map[solana.PublicKey]*Model
}

func NewProgram(id solana.PublicKey, context context.Context, which int, env *env.Env, backend *backend.Backend, splTokenProgram *spltoken.Program, systemProgram *system.Program, cb program.Callback) *Program {
	p := &Program{
		ctx:             context,
		backend:         backend,
		env:             env,
		which:           which,
		log:             log.Default(),
		cb:              cb,
		id:              id,
		splTokenProgram: splTokenProgram,
		systemProgram:   systemProgram,
		markets:         make(map[solana.PublicKey]*KeyedMarket),
		models:          make(map[solana.PublicKey]*Model),
		requests:        make(map[solana.PublicKey]*KeyedRequest),
		events:          make(map[solana.PublicKey]*KeyedEvent),
		orderBooks:      make(map[solana.PublicKey]*KeyedOrderBook),
		openOrders:      make(map[solana.PublicKey]*KeyedOpenOrders),
		unknownAccounts: make(map[solana.PublicKey]bool),
	}
	return p
}

func (p *Program) Name() string {
	return "serum v1"
}

func (p *Program) Id() solana.PublicKey {
	return p.id
}

func (p *Program) Type() string {
	return program.OrderBook
}

func (p *Program) save2Cache() {
	{
		infoJson, _ := json.MarshalIndent(p.models, "", "    ")
		name := fmt.Sprintf("%s%s_%s.json", config.CachePath, p.Name(), p.Id())
		err := os.WriteFile(name, infoJson, 0644)
		if err != nil {
			panic(err)
		}
	}
	if false {
		modelIds := make(map[solana.PublicKey]bool)
		for _, model := range p.models {
			modelIds[model.Id()] = false
			if model.HasState(program.StateCustomUsed) {
				modelIds[model.Id()] = true
			}
		}
		infoJson, _ := json.MarshalIndent(modelIds, "", "    ")
		name := fmt.Sprintf("%s%s_%s_allmodels.json", config.CachePath, p.Name(), p.Id())
		err := os.WriteFile(name, infoJson, 0644)
		if err != nil {
			panic(err)
		}
	}
}

func (p *Program) Start() error {
	p.log.Printf("start %s, program: %s, type: %s", p.Name(), p.Id(), p.Type())
	accounts, err := p.programAccounts()
	if err != nil {
		return err
	}
	markets, err := p.buildAccounts(accounts)
	if err != nil {
		return err
	}
	models, err := p.buildModels(markets)
	if err != nil {
		return err
	}
	for _, model := range models {
		p.callback(model)
	}
	for account, _ := range p.unknownAccounts {
		p.log.Printf("serum account(%s) is unknown", account)
	}
	return nil
}

func (p *Program) Stop() error {
	p.log.Printf("stop %s, program: %s, type: %s", p.Name(), p.Id(), p.Type())
	p.save2Cache()
	return nil
}

func (p *Program) Flash() error {
	p.log.Printf("flash %s, program: %s, type: %s", p.Name(), p.Id(), p.Type())
	p.subscribeUpdate()
	return nil
}

func (p *Program) Markets() []program.Model {
	models := make([]program.Model, 0)
	for _, model := range p.models {
		models = append(models, model)
	}
	return models
}

func (p *Program) GetMarket(key solana.PublicKey) program.Model {
	model, ok := p.models[key]
	if !ok {
		return nil
	}
	return model
}

func (p *Program) programAccounts() ([]*backend.Account, error) {
	if p.which == config.MarketFromChain {
		return p.backend.ProgramAccounts(p.id, []uint64{uint64(MarketLayoutSize)})
	} else {
		return p.backend.Accounts(p.env.Markets(p.id))
	}
}

func (p *Program) upsertMarket(pubkey solana.PublicKey, height uint64, market MarketLayout) *KeyedMarket {
	keyedSwap, ok := p.markets[pubkey]
	if !ok {
		keyedSwap = &KeyedMarket{
			Key:          pubkey,
			Height:       height,
			MarketLayout: market,
		}
		p.markets[pubkey] = keyedSwap
	} else {
		keyedSwap.MarketLayout = market
	}
	return keyedSwap
}

func (p *Program) upsertRequest(pubkey solana.PublicKey, height uint64, request RequestLayout) *KeyedRequest {
	keyedRequest, ok := p.requests[pubkey]
	if !ok {
		keyedRequest = &KeyedRequest{
			Key:           pubkey,
			Height:        height,
			RequestLayout: request,
		}
		p.requests[pubkey] = keyedRequest
	} else {
		keyedRequest.RequestLayout = request
	}
	return keyedRequest
}

func (p *Program) upsertEvent(pubkey solana.PublicKey, height uint64, event EventLayout) *KeyedEvent {
	keyedEvent, ok := p.events[pubkey]
	if !ok {
		keyedEvent = &KeyedEvent{
			Key:         pubkey,
			Height:      height,
			EventLayout: event,
		}
		p.events[pubkey] = keyedEvent
	} else {
		keyedEvent.EventLayout = event
	}
	return keyedEvent
}

func (p *Program) upsertOrderBook(pubkey solana.PublicKey, height uint64, orderBook OrderBookLayout) *KeyedOrderBook {
	keyedOrderBook, ok := p.orderBooks[pubkey]
	if !ok {
		keyedOrderBook = &KeyedOrderBook{
			Key:             pubkey,
			Height:          height,
			OrderBookLayout: orderBook,
		}
		p.orderBooks[pubkey] = keyedOrderBook
	} else {
		keyedOrderBook.OrderBookLayout = orderBook
	}
	return keyedOrderBook
}

func (p *Program) upsertOpenOrders(pubkey solana.PublicKey, height uint64, openOrders OpenOrdersLayout) *KeyedOpenOrders {
	keyedOpenOrders, ok := p.openOrders[pubkey]
	if !ok {
		keyedOpenOrders = &KeyedOpenOrders{
			Key:              pubkey,
			Height:           height,
			OpenOrdersLayout: openOrders,
		}
		p.openOrders[pubkey] = keyedOpenOrders
	} else {
		keyedOpenOrders.OpenOrdersLayout = openOrders
	}
	return keyedOpenOrders
}

func (p *Program) upsertModel(market *KeyedMarket, request *KeyedRequest, event *KeyedEvent,
	bid *KeyedOrderBook, ask *KeyedOrderBook, baseVault *spltoken.KeyedUser, quoteVault *spltoken.KeyedUser) *Model {
	model, ok := p.models[market.Key]
	if !ok {
		model = &Model{
			Market:     market,
			Request:    request,
			Event:      event,
			Bid:        bid,
			Ask:        ask,
			BaseVault:  baseVault,
			QuoteVault: quoteVault,
			ProgramId:  p.id,
			States:     make(map[string]interface{}),
		}
		p.models[market.Key] = model
	} else {
		model.Market = market
		model.Request = request
		model.Event = event
		model.Bid = bid
		model.Ask = ask
		model.BaseVault = baseVault
		model.QuoteVault = quoteVault
	}
	return model
}

func (p *Program) callback(model *Model) {
	if p.cb != nil {
		if err := p.cb.OnModelInit(model); err != nil {
			p.log.Printf("serum program callback err: %v", err)
		}
	}
}

func (p *Program) buildAccounts(accounts []*backend.Account) ([]*KeyedMarket, error) {
	markets := make([]*KeyedMarket, 0)
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		if account.Account.Owner != p.id {
			p.log.Printf("account(%s) is not serum program account, expected: %s, actual: %s", account.PubKey, p.id, account.Account.Owner)
			continue
		}
		accountData := account.Account.Data.GetBinary()
		if len(accountData) == MarketLayoutSize {
			market := MarketLayout{}
			buf := bytes.NewReader(accountData)
			err := binary.Read(buf, binary.LittleEndian, &market)
			if err != nil {
				p.log.Printf("serum account(%s) data is not valid, err: %s", account.PubKey, err)
				continue
			}
			keyedMarket := p.upsertMarket(account.PubKey, account.Height, market)
			markets = append(markets, keyedMarket)
		} else if len(accountData) == OpenOrdersLayoutSize {
			openOrders := OpenOrdersLayout{}
			buf := bytes.NewReader(accountData)
			err := binary.Read(buf, binary.LittleEndian, &openOrders)
			if err != nil {
				p.log.Printf("serum account(%s) data is not valid, err: %s", account.PubKey, err)
				continue
			}
			p.upsertOpenOrders(account.PubKey, account.Height, openOrders)
		} else {
			//p.log.Printf("serum account(%s) data size is not valid, expected: %d, actual: %d", account.Pubkey, MarketLayoutSize, len(accountData))
			p.unknownAccounts[account.PubKey] = true
			continue
		}
	}
	return markets, nil
}

func (p *Program) buildModels(markets []*KeyedMarket) ([]*Model, error) {
	checks := make(map[solana.PublicKey]bool)
	requests := make([]solana.PublicKey, 0)
	events := make([]solana.PublicKey, 0)
	orderBooks := make([]solana.PublicKey, 0)
	splAccounts := make([]solana.PublicKey, 0)
	for _, market := range markets {
		if _, ok := checks[market.RequestQueue]; !ok {
			requests = append(requests, market.RequestQueue)
			checks[market.RequestQueue] = true
		}
		if _, ok := checks[market.EventQueue]; !ok {
			events = append(events, market.EventQueue)
			checks[market.EventQueue] = true
		}
		if _, ok := checks[market.Bids]; !ok {
			orderBooks = append(orderBooks, market.Bids)
			checks[market.Bids] = true
		}
		if _, ok := checks[market.Asks]; !ok {
			orderBooks = append(orderBooks, market.Asks)
			checks[market.Asks] = true
		}
		if _, ok := checks[market.BaseVault]; !ok {
			splAccounts = append(splAccounts, market.BaseVault)
			checks[market.BaseVault] = true
		}
		if _, ok := checks[market.QuoteVault]; !ok {
			splAccounts = append(splAccounts, market.QuoteVault)
			checks[market.QuoteVault] = true
		}
	}
	if len(checks) == 0 {
		return nil, nil
	}
	if err := p.splTokenProgram.RetrieveUsers(splAccounts); err != nil {
		return nil, err
	}
	if err := p.RetrieveRequests(requests); err != nil {
		return nil, err
	}
	if err := p.RetrieveEvents(events); err != nil {
		return nil, err
	}
	if err := p.RetrieveOrderBooks(orderBooks); err != nil {
		return nil, err
	}
	models := make([]*Model, 0)
	for _, market := range markets {
		request, ok := p.requests[market.RequestQueue]
		if !ok {
			p.log.Printf("account(%s) is not retrieved in serum v1 program", market.RequestQueue)
			continue
		}
		delete(p.unknownAccounts, market.RequestQueue)
		event, ok := p.events[market.EventQueue]
		if !ok {
			p.log.Printf("account(%s) is not retrieved in serum v1 program", market.EventQueue)
			continue
		}
		delete(p.unknownAccounts, market.EventQueue)
		ask, ok := p.orderBooks[market.Asks]
		if !ok {
			p.log.Printf("account(%s) is not retrieved in serum v1 program", market.Asks)
			continue
		}
		delete(p.unknownAccounts, market.Asks)
		bid, ok := p.orderBooks[market.Bids]
		if !ok {
			p.log.Printf("account(%s) is not retrieved in serum v1 program", market.Bids)
			continue
		}
		delete(p.unknownAccounts, market.Bids)
		baseVault := p.splTokenProgram.GetUser(market.BaseVault)
		if baseVault == nil {
			p.log.Printf("account(%s) is not retrieved in serum v1 program", market.BaseVault)
			continue
		}
		quoteVault := p.splTokenProgram.GetUser(market.QuoteVault)
		if quoteVault == nil {
			p.log.Printf("account(%s) is not retrieved in serum v1 program", market.QuoteVault)
			continue
		}
		model := p.upsertModel(market, request, event, ask, bid, baseVault, quoteVault)
		models = append(models, model)
	}
	return models, nil
}

func (p *Program) subscribeUpdate() {
	checks := make(map[solana.PublicKey]bool)
	subscribes := make([]solana.PublicKey, 0)
	for _, model := range p.models {
		used := model.HasState(program.StateUsed)
		if !used {
			continue
		}
		if _, ok := checks[model.Market.Bids]; !ok {
			subscribes = append(subscribes, model.Market.Bids)
			checks[model.Market.Bids] = true
		}
		if _, ok := checks[model.Market.Asks]; !ok {
			subscribes = append(subscribes, model.Market.Asks)
			checks[model.Market.Asks] = true
		}
	}
	p.SubscribeOrderBooks(subscribes)
}

func (p *Program) RetrieveRequests(pubkeys []solana.PublicKey) error {
	accounts, err := p.backend.Accounts(pubkeys)
	if err != nil {
		return err
	}
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		if account.Account.Owner != p.id {
			p.log.Printf("account(%s) is not serum v1 program request queue account", account.PubKey)
			continue
		}
		requestData := account.Account.Data.GetBinary()
		request := RequestLayout{}
		err = unpackRequestLayout(requestData, &request)
		if err != nil {
			p.log.Printf("account(%s) data is not valid, err: %s", account.PubKey, err)
			continue
		}
		p.upsertRequest(account.PubKey, account.Height, request)
	}
	return nil
}

func (p *Program) RetrieveState(market solana.PublicKey) (string, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return "", fmt.Errorf("no model of the key - %s", market)
	} else {
		model = item
	}
	mintBaseKey := model.Market.BaseMint
	mintQuoteKey := model.Market.QuoteMint
	tokenBase := p.env.Token(mintBaseKey)
	tokenQuote := p.env.Token(mintQuoteKey)
	bid0 := &Tick{}
	ask0 := &Tick{}
	{
		bids := model.bids(1)
		asks := model.asks(1)
		if len(bids) > 0 {
			bid0 = bids[0]
		}
		if len(asks) > 0 {
			ask0 = asks[0]
		}
	}
	state1 := fmt.Sprintf("    market: %s\n    token pair: (%s %s)\n    bid 1: (%s %s)\n    ask 1: (%s %s)\n",
		market,
		mintBaseKey, mintQuoteKey,
		baseSizeNumberToUi(bid0.Quantity, tokenBase.Decimal, tokenQuote.Decimal).String(), priceNumberToUi(bid0.Price, tokenBase.Decimal, tokenQuote.Decimal).String(),
		baseSizeNumberToUi(ask0.Quantity, tokenBase.Decimal, tokenQuote.Decimal).String(), priceNumberToUi(ask0.Price, tokenBase.Decimal, tokenQuote.Decimal).String())
	return state1, nil
}

func (p *Program) RetrieveEvents(pubkeys []solana.PublicKey) error {
	accounts, err := p.backend.Accounts(pubkeys)
	if err != nil {
		return err
	}
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		if account.Account.Owner != p.id {
			p.log.Printf("account(%s) is not serum v1 program event queue account", account.PubKey)
			continue
		}
		eventData := account.Account.Data.GetBinary()
		event := EventLayout{}
		err = unpackEventLayout(eventData, &event)
		if err != nil {
			p.log.Printf("account(%s) data is not valid, err: %s", account.PubKey, err)
			continue
		}
		p.upsertEvent(account.PubKey, account.Height, event)
	}
	return nil
}

func (p *Program) RetrieveOrderBooks(pubkeys []solana.PublicKey) error {
	accounts, err := p.backend.Accounts(pubkeys)
	if err != nil {
		return err
	}
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		if account.Account.Owner != p.id {
			p.log.Printf("account(%s) is not serum v1 program order book account", account.PubKey)
			continue
		}
		orderBookData := account.Account.Data.GetBinary()
		orderBook := OrderBookLayout{}
		err = unpackOrderBookLayout(orderBookData, &orderBook)
		if err != nil {
			p.log.Printf("account(%s) data is not valid, err: %s", account.PubKey, err)
			continue
		}
		p.upsertOrderBook(account.PubKey, account.Height, orderBook)
	}
	return nil
}

func (p *Program) SubscribeOrderBooks(pubkeys []solana.PublicKey) error {
	for _, pubkey := range pubkeys {
		p.backend.SubscribeAccount(pubkey, p, 1)
	}
	return nil
}

func (p *Program) OnAccountUpdate(account *backend.Account) error {
	if account.Account.Owner != p.id {
		p.log.Printf("account(%s) is not serum v1 program account", account.PubKey)
		return nil
	}
	orderBookData := account.Account.Data.GetBinary()
	orderBook := OrderBookLayout{}
	err := unpackOrderBookLayout(orderBookData, &orderBook)
	if err != nil {
		p.log.Printf("account(%s) data is not valid, err: %s", account.PubKey, err)
		return nil
	}
	p.upsertOrderBook(account.PubKey, account.Height, orderBook)
	return nil
}

func (p *Program) Local(parameter map[string]interface{}) (*program.LocalState, error) {
	var market solana.PublicKey
	if item, ok := parameter["market"]; !ok {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	} else {
		market = item.(solana.PublicKey)
	}
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of the key - %s", market)
	} else {
		model = item
	}
	var token solana.PublicKey
	if item, ok := parameter["token"]; !ok {
		return nil, fmt.Errorf("no parameter - token in instruct construction parameter")
	} else {
		token = item.(solana.PublicKey)
	}
	if token != model.Market.BaseMint && token != model.Market.QuoteMint {
		return nil, fmt.Errorf("token is not the swap token pair - (%s %s)", market, token)
	}
	var amount uint64
	if item, ok := parameter["amount"]; !ok {
		return nil, fmt.Errorf("no parameter - amount in instruct construction parameter")
	} else {
		amount = item.(uint64)
	}
	swapResult, err := model.Swap(token, amount)
	if err != nil {
		return nil, err
	}
	// logs
	localState := &program.LocalState{
		TokenIn:   swapResult.TokenIn,
		AmountIn:  swapResult.AmountIn,
		TokenOut:  swapResult.TokenOut,
		AmountOut: swapResult.AmountOut,
	}
	return localState, nil
}

func (p *Program) ArbitrageStep(parameter map[string]interface{}) ([]solana.Instruction, error) {
	panic("not implement")
}

func (p *Program) Simulate(parameter map[string]interface{}) (*program.SimulateState, error) {
	in, err := p.Instruction(parameter, true)
	if err != nil {
		return nil, err
	}
	return p.Execute(in)
}

func (p *Program) Instruction(parameter map[string]interface{}, simulate bool) ([]solana.Instruction, error) {
	var market solana.PublicKey
	if item, ok := parameter["market"]; !ok {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	} else {
		market = item.(solana.PublicKey)
	}
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of the key - %s", market)
	} else {
		model = item
	}
	var token solana.PublicKey
	if item, ok := parameter["token"]; !ok {
		return nil, fmt.Errorf("no parameter - token in instruct construction parameter")
	} else {
		token = item.(solana.PublicKey)
	}
	if token != model.Market.BaseMint && token != model.Market.QuoteMint {
		return nil, fmt.Errorf("token is not the swap token pair - (%s %s)", market, token)
	}
	/*
		var amount uint64
		if item, ok := parameter["amount"]; !ok {
			return nil, fmt.Errorf("no parameter - amount in instruct construction parameter")
		} else {
			amount = item.(uint64)
		}
	*/
	var side uint32
	var limitPrice uint64
	var maxBaseQuantity uint64
	var maxQuoteQuantity uint64
	if token == model.Market.BaseMint { // bid
		side = 0
		asks := model.asksRaw(1)
		if len(asks) == 0 {
			return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
		}
		limitPrice = asks[0].Price.BigInt().Uint64()
		maxBaseQuantity = asks[0].Quantity.BigInt().Uint64()
		maxQuoteQuantity = model.Market.QuoteLotSize * maxBaseQuantity * limitPrice
	} else { // ask
		side = 1
		bids := model.bidsRaw(1)
		if len(bids) == 0 {
			return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
		}
		limitPrice = bids[0].Price.BigInt().Uint64()
		maxBaseQuantity = bids[0].Quantity.BigInt().Uint64()
		maxQuoteQuantity = model.Market.QuoteLotSize * maxBaseQuantity * limitPrice
	}
	orderType := uint32(0)         // limit
	selfTradeBehavior := uint32(0) // decrementTake
	userA := p.env.TokenUserSimulate(model.Market.BaseMint)
	if userA.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", userA)
	}
	userAOwner := p.env.UsersOwnerSimulate(userA)
	if userA.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", userA)
	}
	openOrder := p.env.MarketOpenOrder(userA)
	if openOrder.IsZero() {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	}

	instruction1, _ := p.systemProgram.InstructionCreateAccount(userAOwner, openOrder, uint64(OpenOrdersLayoutSize), p.id)
	//
	//
	data := make([]byte, 51)
	data[0] = 0
	binary.LittleEndian.PutUint32(data[1:], uint32(10))
	binary.LittleEndian.PutUint32(data[5:], side)
	binary.LittleEndian.PutUint64(data[9:], limitPrice)
	binary.LittleEndian.PutUint64(data[17:], maxBaseQuantity)
	binary.LittleEndian.PutUint64(data[25:], maxQuoteQuantity)
	binary.LittleEndian.PutUint32(data[33:], selfTradeBehavior)
	binary.LittleEndian.PutUint32(data[37:], orderType)
	binary.LittleEndian.PutUint64(data[41:], 0)
	binary.LittleEndian.PutUint16(data[49:], 65535)
	instruction2 := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: market, IsSigner: false, IsWritable: true},
			{PublicKey: openOrder, IsSigner: true, IsWritable: true},
			{PublicKey: model.Market.RequestQueue, IsSigner: true, IsWritable: false},
			{PublicKey: model.Market.EventQueue, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.Bids, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.Asks, IsSigner: false, IsWritable: true},
			{PublicKey: userA, IsSigner: false, IsWritable: true},
			{PublicKey: userAOwner, IsSigner: true, IsWritable: false},
			{PublicKey: model.Market.BaseVault, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.QuoteVault, IsSigner: false, IsWritable: true},
			{PublicKey: p.splTokenProgram.Id(), IsSigner: false, IsWritable: false},
			{PublicKey: program.SysRent, IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: p.id,
	}
	return []solana.Instruction{instruction1, instruction2}, nil
}

func (p *Program) Execute(is []solana.Instruction) (*program.SimulateState, error) {
	sr := &program.SimulateState{
		SourceAmount:      0,
		DestinationAmount: 0,
	}
	_, txs, logs, UnitsConsumed, err := p.backend.Simulate(is, []solana.PublicKey{})
	sr.Txs = txs
	sr.Logs = logs
	sr.UnitsConsumed = UnitsConsumed
	if err != nil {
		return sr, err
	}
	return sr, nil
	/*
		if len(is) != 2 {
			return nil, fmt.Errorf("instruction is invalid")
		}
		accounts := is[1].Accounts()
		marketKey := accounts[0].PublicKey
		var model *Model
		if item, ok := p.models[marketKey]; !ok {
			return nil, fmt.Errorf("no model of the key - %s", marketKey)
		} else {
			model = item
		}
		openOrdersKey := accounts[1].PublicKey
		p.RetrieveOpenOrders([]solana.PublicKey{openOrdersKey})
		openOrders, ok := p.openOrders[openOrdersKey]
		if !ok {
			return nil, fmt.Errorf("no open orders of the key - %s", openOrdersKey)
		}
	*/
	return nil, fmt.Errorf("not implement")
}
