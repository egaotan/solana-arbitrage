package serumv2

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
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
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
	systemProgram   *system.Program
	splTokenProgram *spltoken.Program
	unknownAccounts map[solana.PublicKey]bool
	markets         map[solana.PublicKey]*KeyedMarket
	requests        map[solana.PublicKey]*KeyedRequest
	events          map[solana.PublicKey]*KeyedEvent
	orderBooks      map[solana.PublicKey]*KeyedOrderBook
	openOrders      map[solana.PublicKey]*KeyedOpenOrders
	models          map[solana.PublicKey]*Model
	orderBook2Model map[solana.PublicKey]*Model
	updateAccounts  chan *backend.Account
}

func NewProgram(id solana.PublicKey, context context.Context, which int, env *env.Env, be *backend.Backend, splTokenProgram *spltoken.Program, systemProgram *system.Program, cb program.Callback) *Program {
	p := &Program{
		ctx:     context,
		backend: be,
		env:     env,
		which:   which,
		cb:      cb,
		//log:             log.Default(),
		id:              id,
		splTokenProgram: splTokenProgram,
		systemProgram:   systemProgram,
		markets:         make(map[solana.PublicKey]*KeyedMarket),
		models:          make(map[solana.PublicKey]*Model),
		requests:        make(map[solana.PublicKey]*KeyedRequest),
		events:          make(map[solana.PublicKey]*KeyedEvent),
		orderBooks:      make(map[solana.PublicKey]*KeyedOrderBook),
		openOrders:      make(map[solana.PublicKey]*KeyedOpenOrders),
		updateAccounts:  make(chan *backend.Account, 1024),
		orderBook2Model: make(map[solana.PublicKey]*Model),
		unknownAccounts: make(map[solana.PublicKey]bool),
	}
	return p
}

func (p *Program) Name() string {
	return "serum v2"
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
	p.log = utils.NewLog(config.LogPath, fmt.Sprintf("%s", p.Name()))
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
	go p.update()
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
		keyedOrderBook.Height = height
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
			Market: market,
			//Request:    request,
			//Event:      event,
			Bid:        bid,
			Ask:        ask,
			AskTick0:   [5]Tick{},
			BidTick0:   [5]Tick{},
			BaseVault:  baseVault,
			QuoteVault: quoteVault,
			ProgramId:  p.id,
			States:     make(map[string]interface{}),
		}
		p.models[market.Key] = model
	} else {
		model.Market = market
		//model.Request = request
		//model.Event = event
		model.Bid = bid
		model.Ask = ask
		//model.BaseVault = baseVault
		//model.QuoteVault = quoteVault
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
	//requests := make([]solana.PublicKey, 0)
	//events := make([]solana.PublicKey, 0)
	orderBooks := make([]solana.PublicKey, 0)
	//splAccounts := make([]solana.PublicKey, 0)
	for _, market := range markets {
		/*
			if _, ok := checks[market.RequestQueue]; !ok {
				requests = append(requests, market.RequestQueue)
				checks[market.RequestQueue] = true
			}
			if _, ok := checks[market.EventQueue]; !ok {
				events = append(events, market.EventQueue)
				checks[market.EventQueue] = true
			}
		*/
		if _, ok := checks[market.Bids]; !ok {
			orderBooks = append(orderBooks, market.Bids)
			checks[market.Bids] = true
		}
		if _, ok := checks[market.Asks]; !ok {
			orderBooks = append(orderBooks, market.Asks)
			checks[market.Asks] = true
		}
		/*
			if _, ok := checks[market.BaseVault]; !ok {
				splAccounts = append(splAccounts, market.BaseVault)
				checks[market.BaseVault] = true
			}
			if _, ok := checks[market.QuoteVault]; !ok {
				splAccounts = append(splAccounts, market.QuoteVault)
				checks[market.QuoteVault] = true
			}
		*/
	}
	if len(checks) == 0 {
		return nil, nil
	}
	/*
		if err := p.RetrieveRequests(requests); err != nil {
			return nil, err
		}
		if err := p.RetrieveEvents(events); err != nil {
			return nil, err
		}
	*/
	/*
		if err := p.splTokenProgram.RetrieveUsers(splAccounts); err != nil {
			return nil, err
		}
	*/
	if err := p.RetrieveOrderBooks(orderBooks); err != nil {
		return nil, err
	}
	models := make([]*Model, 0)
	for _, market := range markets {
		/*
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
		*/
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
		/*
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
		*/
		model := p.upsertModel(market, nil, nil, ask, bid, nil, nil)
		p.orderBook2Model[ask.Key] = model
		p.orderBook2Model[bid.Key] = model
		// update tick
		bidTicks := model.bids(5)
		for i, bidTick := range bidTicks {
			model.BidTick0[i].Copy(bidTick)
		}
		askTicks := model.asks(5)
		for i, askTick := range askTicks {
			model.AskTick0[i].Copy(askTick)
		}
		models = append(models, model)
	}
	return models, nil
}

func (p *Program) subscribeUpdate() {
	checks := make(map[solana.PublicKey]bool)
	subscribes := make([]solana.PublicKey, 0)
	//users := make([]solana.PublicKey, 0)
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
		/*
			if _, ok := checks[model.Market.BaseVault]; !ok {
				users = append(users, model.Market.BaseVault)
				checks[model.Market.BaseVault] = true
			}
			if _, ok := checks[model.Market.QuoteVault]; !ok {
				users = append(users, model.Market.QuoteVault)
				checks[model.Market.QuoteVault] = true
			}
		*/
	}
	p.SubscribeOrderBooks(subscribes)
	//p.splTokenProgram.SubscribeUsers(users)
}

func (p *Program) RetrieveRequests(pubkeys []solana.PublicKey) error {
	accounts, err := p.backend.Accounts(pubkeys)
	if err != nil {
		return err
	}
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		if account.Account.Owner != p.id {
			p.log.Printf("account(%s) is not serum v1 program account - RetrieveRequests", account.PubKey)
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

func (p *Program) RetrieveEvents(pubkeys []solana.PublicKey) error {
	accounts, err := p.backend.Accounts(pubkeys)
	if err != nil {
		return err
	}
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		if account.Account.Owner != p.id {
			p.log.Printf("account(%s) is not serum v1 program account- RetrieveEvents", account.PubKey)
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
			p.log.Printf("account(%s) is not serum v1 program account- RetrieveOrderBooks", account.PubKey)
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

func (p *Program) RetrieveOpenOrders(pubkeys []solana.PublicKey) error {
	accounts, err := p.backend.Accounts(pubkeys)
	if err != nil {
		return err
	}
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		openOrders, err := p.parseOpenOrders(account)
		if err != nil {
			p.log.Printf("parse openorders err: %v", err)
			continue
		}
		p.upsertOpenOrders(account.PubKey, account.Height, openOrders)
	}
	return nil
}

func (p *Program) parseOpenOrders(account *backend.Account) (OpenOrdersLayout, error) {
	openOrders := OpenOrdersLayout{}
	if account.Account.Owner != p.id {
		return openOrders, fmt.Errorf("account(%s) is not serum v1 program account- parseOpenOrders", account.PubKey)
	}
	openOrderData := account.Account.Data.GetBinary()
	buf := bytes.NewReader(openOrderData)
	err := binary.Read(buf, binary.LittleEndian, &openOrders)
	return openOrders, err
}

func (p *Program) SubscribeOrderBooks(pubkeys []solana.PublicKey) error {
	for _, pubkey := range pubkeys {
		p.backend.SubscribeAccount(pubkey, p, 1)
	}
	return nil
}

func (p *Program) OnAccountUpdate(account *backend.Account) error {
	p.updateAccounts <- account
	return nil
}

func (p *Program) update() {
	for {
		select {
		case updateAccount := <-p.updateAccounts:
			p.updateAccount(updateAccount)
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *Program) updateAccount(account *backend.Account) {
	//
	p.log.Printf("update account slot diff: %d, %d", account.Height, program.GlobalSlot)
	if account.Account.Owner != p.id {
		p.log.Printf("account(%s) is not serum v1 program account- OnAccountUpdate", account.PubKey)
		return
	}
	orderBookData := account.Account.Data.GetBinary()
	orderBook := OrderBookLayout{}
	err := unpackOrderBookLayout(orderBookData, &orderBook)
	if err != nil {
		p.log.Printf("account(%s) data is not valid, err: %s", account.PubKey, err)
		return
	}
	p.upsertOrderBook(account.PubKey, account.Height, orderBook)
	// update tick
	model, ok := p.orderBook2Model[account.PubKey]
	if !ok {
		p.log.Printf("there is no market for the order book: %s", account.PubKey.String())
		return
	}
	oldBidTick0 := model.BidTick0[0]
	oldAskTick0 := model.AskTick0[0]
	bidTicks := model.bids(5)
	for i, bidTick := range bidTicks {
		model.BidTick0[i].Copy(bidTick)
	}
	askTicks := model.asks(5)
	for i, askTick := range askTicks {
		model.AskTick0[i].Copy(askTick)
	}
	if model.BidTick0[0].Price.Div(oldBidTick0.Price).Cmp(decimal.NewFromFloat(0.992)) < 0 ||
		model.AskTick0[0].Price.Div(oldAskTick0.Price).Cmp(decimal.NewFromFloat(1.008)) > 0 {
		if p.cb != nil {
			p.cb.OnStateUpdate(account.Height)
		}
	}
	//

}

func (p *Program) RetrieveState(market solana.PublicKey) (string, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return "", fmt.Errorf("no model of the key - %s", market)
	} else {
		model = item
	}
	tokenBaseKey := model.Market.BaseToken
	tokenQuoteKey := model.Market.QuoteToken
	tokenBase := p.env.Token(tokenBaseKey)
	tokenQuote := p.env.Token(tokenQuoteKey)
	price := model.AskTick0[0].Price.Add(model.BidTick0[0].Price)
	price = price.Div(decimal.NewFromInt(2))
	state0 := fmt.Sprintf("\n    slot: %d", model.CurrentSlot())
	state1 := fmt.Sprintf("\n    %s/%s: %s", tokenBase.Symbol, tokenQuote.Symbol, priceNumberToUi(price, tokenBase.Decimal, tokenQuote.Decimal).String())
	state2 := fmt.Sprintf("\n    bid 1: (%s %s)\n    ask 1: (%s %s)",
		baseSizeNumberToUi(model.BidTick0[0].Quantity, tokenBase.Decimal, tokenQuote.Decimal).String(), priceNumberToUi(model.BidTick0[0].Price, tokenBase.Decimal, tokenQuote.Decimal).String(),
		baseSizeNumberToUi(model.AskTick0[0].Quantity, tokenBase.Decimal, tokenQuote.Decimal).String(), priceNumberToUi(model.AskTick0[0].Price, tokenBase.Decimal, tokenQuote.Decimal).String())
	return fmt.Sprintf("%s%s%s%s", p.Name(), state0, state1, state2), nil
}

func (p *Program) Local(parameter map[string]interface{}) (*program.LocalState, error) {
	var market solana.PublicKey
	if item, ok := parameter["market"]; !ok {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	} else {
		market = item.(solana.PublicKey)
	}
	var token solana.PublicKey
	if item, ok := parameter["token"]; !ok {
		return nil, fmt.Errorf("no parameter - token in instruct construction parameter")
	} else {
		token = item.(solana.PublicKey)
	}
	var amount uint64
	if item, ok := parameter["amount"]; !ok {
		return nil, fmt.Errorf("no parameter - amount in instruct construction parameter")
	} else {
		amount = item.(uint64)
	}
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of the key - %s", market)
	} else {
		model = item
	}
	if token != model.Market.BaseToken && token != model.Market.QuoteToken {
		return nil, fmt.Errorf("token is not the swap token pair - (%s %s)", market, token)
	}
	//
	swapResult, err := model.Swap1(token, amount)
	if err != nil {
		return nil, err
	}

	localState := &program.LocalState{
		TokenIn:   swapResult.TokenIn,
		AmountIn:  swapResult.AmountIn,
		SlotIn:    swapResult.SlotIn,
		TokenOut:  swapResult.TokenOut,
		AmountOut: swapResult.AmountOut,
		SlotOut:   swapResult.SlotOut,
	}
	return localState, nil
}

/*
func (p *Program) Simulate(parameter map[string]interface{}) (*program.SimulateState, error) {
	var market solana.PublicKey
	if item, ok := parameter["market"]; !ok {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	} else {
		market = item.(solana.PublicKey)
	}
	var token solana.PublicKey
	if item, ok := parameter["token"]; !ok {
		return nil, fmt.Errorf("no parameter - token in instruct construction parameter")
	} else {
		token = item.(solana.PublicKey)
	}
	var amount uint64
	if item, ok := parameter["amount"]; !ok {
		return nil, fmt.Errorf("no parameter - amount in instruct construction parameter")
	} else {
		amount = item.(uint64)
	}

	//
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of this market - %s", market)
	} else {
		model = item
	}
	mintSrcKey, mintDstKey, err := model.getMarketAccounts(token)
	if err != nil {
		return nil, err
	}
	userSrc := p.splTokenProgram.GetUserByToken(mintSrcKey, true)
	if userSrc == nil {
		return nil, fmt.Errorf("no user account for minter - %s", mintSrcKey)
	}
	userDst := p.splTokenProgram.GetUserByToken(mintDstKey, true)
	if userDst == nil {
		return nil, fmt.Errorf("no user account for minter - %s", mintDstKey)
	}
	userSrcKey := userSrc.PubKey
	userDstKey := userDst.PubKey
	// old states
	oldStates := make([]*spltoken.KeyedUser, 0)
	bid0 := &Tick{}
	ask0 := &Tick{}
	mintBaseKey := model.Market.BaseToken
	mintQuoteKey := model.Market.QuoteToken
	{
		err = p.splTokenProgram.RetrieveUsers([]solana.PublicKey{userSrcKey, userDstKey}, true)
		if err != nil {
			return nil, err
		}
		userSrc = p.splTokenProgram.GetUser(userSrcKey)
		if userSrc == nil {
			return nil, fmt.Errorf("token (%s) is not retrieved", userSrcKey)
		}
		userDst = p.splTokenProgram.GetUser(userDstKey)
		if userDst == nil {
			return nil, fmt.Errorf("token (%s) is not retrieved", userDstKey)
		}
		oldStates = append(oldStates, []*spltoken.KeyedUser{userSrc, userDst}...)
		//
		err = p.RetrieveOrderBooks([]solana.PublicKey{model.Market.Asks, model.Market.Bids})
		if err != nil {
			return nil, err
		}
		bids := model.bids(1)
		asks := model.asks(1)
		if len(bids) > 0 {
			bid0 = bids[0]
		}
		if len(asks) > 0 {
			ask0 = asks[0]
		}
	}
	sr := &program.SimulateState{
		SourceToken:       mintSrcKey,
		DestinationToken:  mintDstKey,
		SourceAmount:      0,
		DestinationAmount: 0,
	}

	instructions := make([]solana.Instruction, 0)
	openorder, _ := p.backend.UserOpenOrder(market, true)
	hasOpenOder := p.backend.HasAccount(openorder)
	if !hasOpenOder {
		instruction, err := p.InstructionCreateOpenOrders(market, token, true)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, instruction)
	}
	if !hasOpenOder {
		instruction, err := p.InstructionInitOpenOrders(market, token, true)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, instruction)
	}
	{
		instruction, err := p.InstructionNewOrder(market, token, amount, true)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, instruction)
	}
	{
		instruction, err := p.InstructionSettleFund(market, token, true)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, instruction)
	}

	updateAccounts, txs, logs, consumed, err := p.backend.Simulate(instructions, []solana.PublicKey{userSrcKey, userDstKey})
	sr.Txs = txs
	sr.Logs = logs
	sr.UnitsConsumed = consumed
	if err != nil {
		return sr, err
	}
	updateStates, err := p.splTokenProgram.BuildAccount([]*backend.Account{
		{
			Height:  0,
			PubKey:     userSrcKey,
			Account: updateAccounts[0],
		}, {
			Height:  0,
			PubKey:     userDstKey,
			Account: updateAccounts[1],
		},
	})
	if err != nil {
		return sr, err
	}
	sr.SourceAmount = oldStates[0].Amount - updateStates[0].Amount
	sr.DestinationAmount = updateStates[1].Amount - oldStates[1].Amount
	//
	mintSrcName, stateDiffSrc := p.backend.TokenInfo(mintSrcKey, oldStates[0].Amount-updateStates[0].Amount)
	mintDstName, stateDiffDst := p.backend.TokenInfo(mintDstKey, updateStates[1].Amount-oldStates[1].Amount)

	tokenBase := p.backend.Token(mintBaseKey)
	tokenQuote := p.backend.Token(mintQuoteKey)
	state1 := fmt.Sprintf("    token pair: (%s %s)\n    bid 1: (%s %s)\n    ask 1: (%s %s)\n",
		mintBaseKey, mintQuoteKey,
		baseSizeNumberToUi(bid0.Quantity, tokenBase.Decimal, tokenQuote.Decimal).String(), priceNumberToUi(bid0.Price, tokenBase.Decimal, tokenQuote.Decimal).String(),
		baseSizeNumberToUi(ask0.Quantity, tokenBase.Decimal, tokenQuote.Decimal).String(), priceNumberToUi(ask0.Price, tokenBase.Decimal, tokenQuote.Decimal).String())
	state2 := fmt.Sprintf("    in: (%s %s), out: (%s %s)\n", mintSrcName, stateDiffSrc.String(), mintDstName, stateDiffDst.String())
	state3 := fmt.Sprintf("    in: (%s), out: (%s)\n", mintSrcKey, mintDstKey)
	sr.State = []byte("states: \n" + state1 + state2 + state3)

	return sr, nil
}
*/

func (p *Program) ArbitrageStep(parameter map[string]interface{}) ([]solana.Instruction, error) {
	var market solana.PublicKey
	if item, ok := parameter["market"]; !ok {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	} else {
		market = item.(solana.PublicKey)
	}
	var token solana.PublicKey
	if item, ok := parameter["token"]; !ok {
		return nil, fmt.Errorf("no parameter - token in instruct construction parameter")
	} else {
		token = item.(solana.PublicKey)
	}
	var amount uint64
	if item, ok := parameter["amount"]; !ok {
		return nil, fmt.Errorf("no parameter - amount in instruct construction parameter")
	} else {
		amount = item.(uint64)
	}
	var flag uint8
	if item, ok := parameter["flag"]; !ok {
		return nil, fmt.Errorf("no parameter - flag in instruct construction parameter")
	} else {
		flag = item.(uint8)
	}

	instructions := make([]solana.Instruction, 0)
	instruction, err := p.InstructionArbitrageStep(market, token, amount, flag)
	if err != nil {
		return nil, err
	}
	instructions = append(instructions, instruction)
	/*
		{
			instruction, err := p.InstructionSettleFund(market, token, false)
			if err != nil {
				return nil, err
			}
			instructions = append(instructions, instruction)
		}
	*/
	return instructions, nil
}

func (p *Program) InstructionCreateOpenOrders(market solana.PublicKey, owner solana.PublicKey, simulate bool) (solana.Instruction, error) {
	openOrdersKey := p.env.MarketOpenOrder(market)
	if openOrdersKey.IsZero() {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	}
	return p.systemProgram.InstructionCreateAccount(owner, openOrdersKey, uint64(OpenOrdersLayoutSize), p.id)
}

func (p *Program) InstructionInitOpenOrders(market solana.PublicKey, openOrdersKey solana.PublicKey, owner solana.PublicKey, simulate bool) (solana.Instruction, error) {
	data := make([]byte, 5)
	data[0] = 0
	binary.LittleEndian.PutUint32(data[1:], uint32(15))
	instruction := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: openOrdersKey, IsSigner: false, IsWritable: true},
			{PublicKey: owner, IsSigner: true, IsWritable: false},
			{PublicKey: market, IsSigner: false, IsWritable: true},
			{PublicKey: program.SysRent, IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: p.id,
	}
	return instruction, nil
}

func (p *Program) InstructionNewOrder(market solana.PublicKey, token solana.PublicKey, amount uint64, simulate bool) (solana.Instruction, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of this market - %s", market)
	} else {
		model = item
	}
	// build all parameters
	var side uint32
	var limitPrice uint64
	var maxBaseQuantity uint64
	var maxQuoteQuantity uint64
	tokenBase := p.env.Token(model.Market.BaseToken)
	tokenQuote := p.env.Token(model.Market.QuoteToken)
	if token == model.Market.BaseToken { // sell
		side = 1
		asks := model.asksRaw(1)
		if len(asks) == 0 {
			return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
		}
		limitPrice = asks[0].Price.BigInt().Uint64() * 98 / 100
		//maxBaseQuantity = asks[0].Quantity.BigInt().Uint64()
		maxBaseQuantity = model.baseSizeNumberToLots(amount)
		maxQuoteQuantity = model.Market.QuoteLotSize * maxBaseQuantity * limitPrice
		/*
			side = 1
			priceNumer := priceUiToNumber(price, tokenBase.Decimal, tokenQuote.Decimal)
			priceLots := model.priceNumberToLots(priceNumer)
			limitPrice = priceLots
			sizeLots := model.baseSizeNumberToLots(amount)
			maxBaseQuantity = sizeLots
			maxQuoteQuantity = model.UseMarket.QuoteLotSize * maxBaseQuantity * limitPrice
		*/
	} else { // buy
		/*
			side = 0
			bids := model.bidsRaw(1)
			if len(bids) == 0 {
				return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
			}
			limitPrice = bids[0].Price.BigInt().Uint64()
			maxBaseQuantity = bids[0].Quantity.BigInt().Uint64()
			maxQuoteQuantity = model.UseMarket.QuoteLotSize * maxBaseQuantity * limitPrice
		*/
		side = 0
		bids := model.bidsRaw(1)
		if len(bids) == 0 {
			return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
		}
		limitPrice = bids[0].Price.BigInt().Uint64() * 102 / 100
		//
		priceNumber := model.priceLotsToNumber(limitPrice)
		priceUi := priceNumberToUi(priceNumber, tokenBase.Decimal, tokenQuote.Decimal)
		baseAmountUi := decimal.NewFromInt(int64(amount)).Div(decimal.NewFromInt(int64(tokenQuote.Decimal))).Div(priceUi)
		//fmt.Printf("price: %s, amount: %s\n", priceUi.String(), baseAmountUi.String())
		maxBaseQuantity = model.baseSizeNumberToLots(baseAmountUi.Mul(decimal.NewFromInt(int64(tokenBase.Decimal))).BigInt().Uint64())
		maxQuoteQuantity = model.Market.QuoteLotSize * maxBaseQuantity * limitPrice
	}
	orderType := uint32(1)         // ImmediateOrCancel
	selfTradeBehavior := uint32(0) // decrementTake
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
	//
	user := p.env.TokenUser(token)
	if user.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", token)
	}
	userOwner := p.env.UsersOwnerSimulate(user)
	if userOwner.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", token)
	}
	openOrdersKey := p.env.MarketOpenOrder(market)
	if openOrdersKey.IsZero() {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	}
	instruction := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: market, IsSigner: false, IsWritable: true},
			{PublicKey: openOrdersKey, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.RequestQueue, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.EventQueue, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.Bids, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.Asks, IsSigner: false, IsWritable: true},
			{PublicKey: user, IsSigner: false, IsWritable: true},
			{PublicKey: userOwner, IsSigner: true, IsWritable: false},
			{PublicKey: model.Market.BaseVault, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.QuoteVault, IsSigner: false, IsWritable: true},
			{PublicKey: p.splTokenProgram.Id(), IsSigner: false, IsWritable: false},
			{PublicKey: program.SysRent, IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: p.id,
	}
	return instruction, nil
}

func (p *Program) InstructionSettleFund(market solana.PublicKey, token solana.PublicKey, simulate bool) (solana.Instruction, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of this market - %s", market)
	} else {
		model = item
	}
	user := p.env.TokenUser(token)
	if user.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", token)
	}
	userOwner := p.env.UsersOwner(user)
	if userOwner.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", token)
	}
	openOrdersKey := p.env.MarketOpenOrder(market)
	if openOrdersKey.IsZero() {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	}
	baseUser := p.env.TokenUser(model.Market.BaseToken)
	if baseUser.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", model.Market.BaseToken)
	}
	quoteUser := p.env.TokenUser(model.Market.QuoteToken)
	if quoteUser.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", model.Market.QuoteToken)
	}
	nonce := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonce, model.Market.VaultSignerNonce)
	vaultSigner, err := solana.CreateProgramAddress([][]byte{market.Bytes(), nonce}, p.id)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 5)
	data[0] = 0
	binary.LittleEndian.PutUint32(data[1:], uint32(5))
	instruction := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: market, IsSigner: false, IsWritable: true},
			{PublicKey: openOrdersKey, IsSigner: false, IsWritable: true},
			{PublicKey: userOwner, IsSigner: true, IsWritable: false},
			{PublicKey: model.Market.BaseVault, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.QuoteVault, IsSigner: false, IsWritable: true},
			{PublicKey: baseUser, IsSigner: false, IsWritable: true},
			{PublicKey: quoteUser, IsSigner: false, IsWritable: true},
			{PublicKey: vaultSigner, IsSigner: false, IsWritable: false},
			{PublicKey: p.splTokenProgram.Id(), IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: p.id,
	}
	return instruction, nil
}

func (p *Program) InstructionArbitrageStep(market solana.PublicKey, token solana.PublicKey, amount uint64, flag uint8) (solana.Instruction, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of this market - %s", market)
	} else {
		model = item
	}
	data := make([]byte, 12)
	data[0] = 0
	binary.LittleEndian.PutUint64(data[1:], amount)
	data[9] = 2
	if token == model.Market.BaseToken {
		data[10] = 1
	} else {
		data[10] = 0 // buy
	}
	data[11] = flag
	//
	tokenSrc, tokenDst, err := model.getMarketAccounts(token)
	if err != nil {
		return nil, fmt.Errorf("token is invalid")
	}
	userSrc := p.env.TokenUser(tokenSrc)
	if userSrc.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", tokenSrc)
	}
	userDst := p.env.TokenUser(tokenDst)
	if userDst.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", tokenDst)
	}
	userBase := p.env.TokenUser(model.Market.BaseToken)
	if userBase.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", tokenSrc)
	}
	userQuote := p.env.TokenUser(model.Market.QuoteToken)
	if userQuote.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", tokenDst)
	}
	openOrdersKey := p.env.MarketOpenOrder(market)
	if openOrdersKey.IsZero() {
		return nil, fmt.Errorf("no parameter - swap in instruct construction parameter")
	}
	userSrcOwner := p.env.UsersOwner(userSrc)
	if userSrcOwner.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", token)
	}
	nonce := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonce, model.Market.VaultSignerNonce)
	vaultSigner, err := solana.CreateProgramAddress([][]byte{market.Bytes(), nonce}, p.id)
	if err != nil {
		return nil, err
	}
	instruction := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: program.Exchange, IsSigner: false, IsWritable: true},
			{PublicKey: p.id, IsSigner: false, IsWritable: false},
			{PublicKey: market, IsSigner: false, IsWritable: true},
			{PublicKey: openOrdersKey, IsSigner: false, IsWritable: true},
			{PublicKey: userSrcOwner, IsSigner: true, IsWritable: false},
			{PublicKey: model.Market.RequestQueue, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.EventQueue, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.Bids, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.Asks, IsSigner: false, IsWritable: true},
			{PublicKey: userSrc, IsSigner: false, IsWritable: true},
			{PublicKey: userDst, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.BaseVault, IsSigner: false, IsWritable: true},
			{PublicKey: model.Market.QuoteVault, IsSigner: false, IsWritable: true},
			{PublicKey: userBase, IsSigner: false, IsWritable: true},
			{PublicKey: userQuote, IsSigner: false, IsWritable: true},
			{PublicKey: vaultSigner, IsSigner: false, IsWritable: false},
			{PublicKey: p.splTokenProgram.Id(), IsSigner: false, IsWritable: false},
			{PublicKey: program.SysRent, IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: program.Arbitrage,
	}
	return instruction, nil
}
