package orca

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
	swaps           map[solana.PublicKey]*KeyedSwap
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
		models:          make(map[solana.PublicKey]*Model),
		swaps:           make(map[solana.PublicKey]*KeyedSwap),
	}
	return p
}

func (p *Program) Name() string {
	return "orca"
}

func (p *Program) Id() solana.PublicKey {
	return p.id
}

func (p *Program) Type() string {
	return program.AMM
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
	swaps, err := p.buildAccounts(accounts)
	if err != nil {
		return err
	}
	models, err := p.buildModels(swaps)
	if err != nil {
		return err
	}
	for _, model := range models {
		p.callback(model)
	}
	return nil
}

func (p *Program) Stop() error {
	p.log.Printf("stop %s, program: %s, type: %s", p.Name(), p.Id(), p.Type())
	p.save2Cache()
	return nil
}

func (p *Program) Flash() error {
	p.log.Printf("flash %s, program: %s, type: %s", p.Name(), p.id, p.Type())
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

func (p *Program) searchMarket(tokenA solana.PublicKey, tokenB solana.PublicKey) *KeyedSwap {
	for _, market := range p.swaps {
		if market.TokenA == tokenA && market.TokenB == tokenB {
			return market
		}
	}
	return nil
}

func (p *Program) programAccounts() ([]*backend.Account, error) {
	if p.which == config.MarketFromChain {
		return p.backend.ProgramAccounts(p.id, []uint64{uint64(SwapLayoutSize)})
		//return p.backend.ProgramAccounts(p.id, nil)
	} else {
		return p.backend.Accounts(p.env.Markets(p.id))
	}
}

func (p *Program) upsertSwap(pubkey solana.PublicKey, height uint64, swap SwapLayout) *KeyedSwap {
	keyedSwap, ok := p.swaps[pubkey]
	if !ok {
		keyedSwap = &KeyedSwap{
			Key:        pubkey,
			Height:     height,
			SwapLayout: swap,
		}
		p.swaps[pubkey] = keyedSwap
	} else {
		keyedSwap.SwapLayout = swap
		keyedSwap.Height = height
	}
	return keyedSwap
}

func (p *Program) upsertModel(tokenSwap *KeyedSwap, tokenA *spltoken.KeyedUser, tokenB *spltoken.KeyedUser) *Model {
	model, ok := p.models[tokenSwap.Key]
	if !ok {
		model = &Model{
			TokenSwap: tokenSwap,
			SwapA:     tokenA,
			SwapB:     tokenB,
			ProgramId: p.id,
			States:    make(map[string]interface{}),
		}
		p.models[tokenSwap.Key] = model
	} else {
		model.TokenSwap = tokenSwap
		model.SwapA = tokenA
		model.SwapB = tokenB
	}
	return model
}

func (p *Program) callback(model *Model) {
	if model.TokenSwap.SwapCurve.CurveType != ConstantProduct {
		return
	}
	if p.cb != nil {
		if err := p.cb.OnModelInit(model); err != nil {
			p.log.Printf("orca program callback err: %v", err)
		}
	}
}

func (p *Program) parseAccount(account *backend.Account) (SwapLayout, error) {
	swap := SwapLayout{}
	if account.Account.Owner != p.id {
		return swap, fmt.Errorf("account(%s) is not orca program account, expected: %s, actual: %s", account.PubKey, p.id, account.Account.Owner)
	}
	accountData := account.Account.Data.GetBinary()
	if len(accountData) != SwapLayoutSize {
		return swap, fmt.Errorf("orca account(%s) data size is not valid, expected: %d, actual: %d", account.PubKey, SwapLayoutSize, len(accountData))
	}
	buf := bytes.NewReader(accountData)
	err := binary.Read(buf, binary.LittleEndian, &swap)
	if err != nil {
		return swap, fmt.Errorf("orca account(%s) data is not valid, err: %s", account.PubKey, err)
	}
	return swap, nil
}

func (p *Program) buildAccounts(accounts []*backend.Account) ([]*KeyedSwap, error) {
	swaps := make([]*KeyedSwap, 0)
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		layout, err := p.parseAccount(account)
		if err != nil {
			p.log.Printf("parse account err: %s", err.Error())
			continue
		}
		keyedSwap := p.upsertSwap(account.PubKey, account.Height, layout)
		swaps = append(swaps, keyedSwap)
	}
	return swaps, nil
}

func (p *Program) buildModels(swaps []*KeyedSwap) ([]*Model, error) {
	checks := make(map[solana.PublicKey]bool)
	pubkeys := make([]solana.PublicKey, 0)
	for _, swap := range swaps {
		if _, ok := checks[swap.SwapA]; !ok {
			pubkeys = append(pubkeys, swap.SwapA)
			checks[swap.SwapA] = true
		}
		if _, ok := checks[swap.SwapB]; !ok {
			pubkeys = append(pubkeys, swap.SwapB)
			checks[swap.SwapB] = true
		}
	}
	err := p.splTokenProgram.RetrieveUsers(pubkeys)
	if err != nil {
		return nil, err
	}
	models := make([]*Model, 0)
	for _, swap := range swaps {
		swapA := p.splTokenProgram.GetUser(swap.SwapA)
		if swapA == nil {
			p.log.Printf("account(%s) is not retrieved in orca program", swap.SwapA)
			continue
		}
		swapB := p.splTokenProgram.GetUser(swap.SwapB)
		if swapB == nil {
			p.log.Printf("account(%s) is not retrieved in orca program", swap.SwapB)
			continue
		}
		model := p.upsertModel(swap, swapA, swapB)
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
		if _, ok := checks[model.TokenSwap.SwapA]; !ok {
			subscribes = append(subscribes, model.TokenSwap.SwapA)
			checks[model.TokenSwap.SwapA] = true
		}
		if _, ok := checks[model.TokenSwap.SwapB]; !ok {
			subscribes = append(subscribes, model.TokenSwap.SwapB)
			checks[model.TokenSwap.SwapB] = true
		}
	}
	p.splTokenProgram.SubscribeUsers(subscribes)
}

func (p *Program) RetrieveState(market solana.PublicKey) (string, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return "", fmt.Errorf("no model of the key - %s", market)
	} else {
		model = item
	}
	tokenA := p.env.Token(model.TokenSwap.TokenA)
	tokenB := p.env.Token(model.TokenSwap.TokenB)
	amountTokenA := tokenA.AmountUi(model.SwapA.Amount)
	amountTokenB := tokenB.AmountUi(model.SwapB.Amount)
	price := amountTokenB.Div(amountTokenA).StringFixed(5)
	state0 := fmt.Sprintf("\n    slot: %d", model.CurrentSlot())
	state1 := fmt.Sprintf("\n    %s/%s: %s", tokenA.Symbol, tokenB.Symbol, price)
	state2 := fmt.Sprintf("\n    token pool: (%s %s)(%s %s)",
		tokenA.Symbol, amountTokenA.StringFixed(2), tokenB.Symbol, amountTokenB.StringFixed(2))
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
	if token != model.TokenSwap.TokenA && token != model.TokenSwap.TokenB {
		return nil, fmt.Errorf("token is not the swap token pair - (%s %s)", market, token)
	}
	//
	swapResult, err := model.Swap(token, amount)
	if err != nil {
		return nil, err
	}
	// logs
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

func (p *Program) ArbitrageStep(parameter map[string]interface{}) ([]solana.Instruction, error) {
	var market solana.PublicKey
	if item, ok := parameter["market"]; !ok {
		return nil, fmt.Errorf("no parameter - market in instruct construction parameter")
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
		return nil, fmt.Errorf("no parameter - amount in instruct construction parameter")
	} else {
		flag = item.(uint8)
	}
	//
	instructions := make([]solana.Instruction, 0)
	instruction, err := p.InstructionArbitrageStep(market, token, amount, flag)
	if err != nil {
		return nil, err
	}
	instructions = append(instructions, instruction)
	return instructions, nil
}

/*
func (p *Program) Simulate(parameter map[string]interface{}) (*program.SimulateState, error) {
	var market solana.PublicKey
	if item, ok := parameter["market"]; !ok {
		return nil, fmt.Errorf("no parameter - market in instruct construction parameter")
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
	instructions := make([]solana.Instruction, 0)
	instruction1, err := p.InstructionSwap(market, token, amount, true)
	if err != nil {
		return nil, err
	}
	instructions = append(instructions, instruction1)

	//
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of this market - %s", market)
	} else {
		model = item
	}
	// src and dst
	mintSrcKey, _, mintDstKey, _, err := model.getSwapAccounts(token)
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
	tokenAKey := model.TokenSwap.SwapA
	mintAKey := model.TokenSwap.SwapA
	tokenBKey := model.TokenSwap.SwapB
	mintBKey := model.TokenSwap.SwapB
	oldStates := make([]*spltoken.KeyedUser, 0)
	{
		err = p.splTokenProgram.RetrieveUsers([]solana.PublicKey{userSrcKey, userDstKey, tokenAKey, tokenBKey}, true)
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
		tokenA := p.splTokenProgram.GetUser(tokenAKey)
		if tokenA == nil {
			return nil, fmt.Errorf("token (%s) is not retrieved", tokenAKey)
		}
		tokenB := p.splTokenProgram.GetUser(tokenBKey)
		if tokenB == nil {
			return nil, fmt.Errorf("token (%s) is not retrieved", tokenBKey)
		}
		oldStates = append(oldStates, []*spltoken.KeyedUser{userSrc, userDst, tokenA, tokenB}...)
	}
	//
	sr := &program.SimulateState{
		TokenIn:       mintSrcKey,
		TokenOut:  mintDstKey,
		AmountIn:      0,
		AmountOut: 0,
	}
	updateAccounts, txs, logs, consumed, err := p.backend.Simulate(instructions, []solana.PublicKey{userSrcKey, userDstKey, tokenAKey, tokenBKey})
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
		}, {
			Height:  0,
			PubKey:     tokenAKey,
			Account: updateAccounts[2],
		}, {
			Height:  0,
			PubKey:     tokenBKey,
			Account: updateAccounts[3],
		},
	})
	if err != nil {
		return sr, err
	}
	sr.AmountIn = oldStates[0].Amount - updateStates[0].Amount
	sr.AmountOut = updateStates[1].Amount - oldStates[1].Amount
	//
	mintSrcName, stateDiffSrc := p.backend.TokenInfo(mintSrcKey, oldStates[0].Amount-updateStates[0].Amount)
	mintDstName, stateDiffDst := p.backend.TokenInfo(mintDstKey, updateStates[1].Amount-oldStates[1].Amount)

	state1 := fmt.Sprintf("    token pairs: (%s %s)\n    pool: (%s %d)(%s %d)\n    pool: (%s %d)(%s %d)\n",
		mintAKey, mintBKey,
		tokenAKey, oldStates[2].Amount, tokenBKey, oldStates[3].Amount,
		tokenAKey, updateStates[2].Amount, tokenBKey, updateStates[3].Amount)
	state2 := fmt.Sprintf("    in: (%s %s), out: (%s %s)\n", mintSrcName, stateDiffSrc.String(), mintDstName, stateDiffDst.String())
	state3 := fmt.Sprintf("    in: (%s), out: (%s)\n", mintSrcKey, mintDstKey)
	sr.State = []byte("states: \n" + state1 + state2 + state3)
	return sr, nil
}
*/

func (p *Program) InstructionSwap(market solana.PublicKey, tokenIn solana.PublicKey, amountIn uint64, simulate bool) (solana.Instruction, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of this market - %s", market)
	} else {
		model = item
	}
	// src and dst
	mintSrc, tokenSrc, mintDst, tokenDst, err := model.getSwapAccounts(tokenIn)
	if err != nil {
		return nil, err
	}
	// build accounts
	authority, _, err := solana.FindProgramAddress([][]byte{market.Bytes()}, p.id)
	if err != nil {
		return nil, err
	}
	userSrc := p.env.TokenUser(mintSrc)
	if userSrc.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", mintSrc)
	}
	userSrcOwner := p.env.UsersOwner(userSrc)
	if userSrcOwner.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", mintSrc)
	}
	userDst := p.env.TokenUser(mintDst)
	if userDst.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", mintDst)
	}
	// build instruction
	data := make([]byte, 17)
	data[0] = 1
	binary.LittleEndian.PutUint64(data[1:], amountIn)
	binary.LittleEndian.PutUint64(data[9:], 0)
	instruction := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: market, IsSigner: false, IsWritable: false},
			{PublicKey: authority, IsSigner: false, IsWritable: false},
			{PublicKey: userSrcOwner, IsSigner: true, IsWritable: false},
			{PublicKey: userSrc, IsSigner: false, IsWritable: true},
			{PublicKey: tokenSrc, IsSigner: false, IsWritable: true},
			{PublicKey: tokenDst, IsSigner: false, IsWritable: true},
			{PublicKey: userDst, IsSigner: false, IsWritable: true},
			{PublicKey: model.TokenSwap.PoolToken, IsSigner: false, IsWritable: true},
			{PublicKey: model.TokenSwap.PoolFeeAccount, IsSigner: false, IsWritable: true},
			{PublicKey: p.splTokenProgram.Id(), IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: p.id,
	}
	return instruction, nil
}

func (p *Program) InstructionArbitrageStep(market solana.PublicKey, tokenIn solana.PublicKey, amountIn uint64, flag uint8) (solana.Instruction, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of this market - %s", market)
	} else {
		model = item
	}
	// src and dst
	tokenSrc, swapSrc, tokenDst, swapDst, err := model.getSwapAccounts(tokenIn)
	if err != nil {
		return nil, err
	}
	// build accounts
	authority, _, err := solana.FindProgramAddress([][]byte{market.Bytes()}, p.id)
	if err != nil {
		return nil, err
	}
	userSrc := p.env.TokenUser(tokenSrc)
	if userSrc.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", tokenSrc)
	}
	userSrcOwner := p.env.UsersOwner(userSrc)
	if userSrcOwner.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", userSrc)
	}
	userDst := p.env.TokenUser(tokenDst)
	if userDst.IsZero() {
		return nil, fmt.Errorf("no user account for minter - %s", tokenDst)
	}
	// build instruction
	data := make([]byte, 12)
	data[0] = 0
	binary.LittleEndian.PutUint64(data[1:], amountIn)
	data[9] = 0
	data[10] = 0
	data[11] = flag
	instruction := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: program.Exchange, IsSigner: false, IsWritable: true},
			{PublicKey: p.id, IsSigner: false, IsWritable: false},
			{PublicKey: market, IsSigner: false, IsWritable: false},
			{PublicKey: authority, IsSigner: false, IsWritable: false},
			{PublicKey: userSrcOwner, IsSigner: true, IsWritable: false},
			{PublicKey: userSrc, IsSigner: false, IsWritable: true},
			{PublicKey: swapSrc, IsSigner: false, IsWritable: true},
			{PublicKey: swapDst, IsSigner: false, IsWritable: true},
			{PublicKey: userDst, IsSigner: false, IsWritable: true},
			{PublicKey: model.TokenSwap.PoolToken, IsSigner: false, IsWritable: true},
			{PublicKey: model.TokenSwap.PoolFeeAccount, IsSigner: false, IsWritable: true},
			{PublicKey: p.splTokenProgram.Id(), IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: program.Arbitrage,
	}
	return instruction, nil
}

func (p *Program) RandomAccounts(parameter map[string]interface{}) ([]*solana.AccountMeta, error) {
	var tokenA solana.PublicKey
	if item, ok := parameter["tokenA"]; !ok {
		return nil, fmt.Errorf("no parameter token A - swap in instruct construction parameter")
	} else {
		tokenA = item.(solana.PublicKey)
	}

	var tokenB solana.PublicKey
	if item, ok := parameter["tokenB"]; !ok {
		return nil, fmt.Errorf("no parameter token B - swap in instruct construction parameter")
	} else {
		tokenB = item.(solana.PublicKey)
	}

	market := p.searchMarket(tokenA, tokenB)
	if market == nil {
		return nil, fmt.Errorf("no market for tokens")
	}

	authority, _, err := solana.FindProgramAddress([][]byte{market.Key.Bytes()}, p.id)
	if err != nil {
		return nil, err
	}
	IsAccounts := []*solana.AccountMeta{
		{PublicKey: market.Key, IsSigner: false, IsWritable: false},
		{PublicKey: authority, IsSigner: false, IsWritable: false},
		{PublicKey: market.SwapA, IsSigner: false, IsWritable: true},
		{PublicKey: market.SwapB, IsSigner: false, IsWritable: true},
		{PublicKey: market.PoolToken, IsSigner: false, IsWritable: true},
		{PublicKey: market.PoolFeeAccount, IsSigner: false, IsWritable: true},
	}
	return IsAccounts, nil
}
