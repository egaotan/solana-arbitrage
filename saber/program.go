package saber

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
	cb              program.Callback
	ctx             context.Context
	trace           bool
	id              solana.PublicKey
	splTokenProgram *spltoken.Program
	systemProgram   *system.Program
	stableSwaps     map[solana.PublicKey]*KeyedStableSwap
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
		trace:           true,
		id:              id,
		splTokenProgram: splTokenProgram,
		systemProgram:   systemProgram,
		stableSwaps:     make(map[solana.PublicKey]*KeyedStableSwap),
		models:          make(map[solana.PublicKey]*Model),
	}
	return p
}

func (p *Program) Name() string {
	return "saber"
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

func (p *Program) searchMarket(tokenA solana.PublicKey, tokenB solana.PublicKey) *KeyedStableSwap {
	for _, market := range p.stableSwaps {
		if market.TokenA == tokenA && market.TokenB == tokenB {
			return market
		}
	}
	return nil
}

func (p *Program) programAccounts() ([]*backend.Account, error) {
	if p.which == config.MarketFromChain {
		return p.backend.ProgramAccounts(p.id, []uint64{uint64(StableSwapLayoutSize)})
	} else {
		return p.backend.Accounts(p.env.Markets(p.id))
	}
}

func (p *Program) upsertStableSwap(pubkey solana.PublicKey, height uint64, swap StableSwapLayout) *KeyedStableSwap {
	keyedStableSwap, ok := p.stableSwaps[pubkey]
	if !ok {
		keyedStableSwap = &KeyedStableSwap{
			Key:              pubkey,
			Height:           height,
			StableSwapLayout: swap,
		}
		p.stableSwaps[pubkey] = keyedStableSwap
	} else {
		keyedStableSwap.StableSwapLayout = swap
		keyedStableSwap.Height = height
	}
	return keyedStableSwap
}

func (p *Program) upsertModel(tokenSwap *KeyedStableSwap, tokenA *spltoken.KeyedUser, tokenB *spltoken.KeyedUser) *Model {
	model, ok := p.models[tokenSwap.Key]
	if !ok {
		model = &Model{
			StableSwap: tokenSwap,
			SwapA:      tokenA,
			SwapB:      tokenB,
			ProgramId:  p.id,
			States:     make(map[string]interface{}),
		}
		p.models[tokenSwap.Key] = model
	} else {
		model.StableSwap = tokenSwap
		model.SwapA = tokenA
		model.SwapB = tokenB
	}
	return model
}

func (p *Program) callback(model *Model) {
	if p.cb != nil {
		if err := p.cb.OnModelInit(model); err != nil {
			p.log.Printf("saber program call back err: %v", err)
		}
	}
}

func (p *Program) parseAccount(account *backend.Account) (StableSwapLayout, error) {
	stableSwap := StableSwapLayout{}
	if account.Account.Owner != p.id {
		return stableSwap, fmt.Errorf("account (%s) is not saber program account, expected: %s, actual: %s", account.PubKey, p.id, account.Account.Owner)
	}
	accountData := account.Account.Data.GetBinary()
	if len(accountData) != StableSwapLayoutSize {
		return stableSwap, fmt.Errorf("saber account (%s) data size is not valid, expected: %d, actual: %d", account.PubKey, StableSwapLayoutSize, len(accountData))
	}
	buf := bytes.NewReader(accountData)
	err := binary.Read(buf, binary.LittleEndian, &stableSwap)
	if err != nil {
		return stableSwap, fmt.Errorf("saber account(%s) data is not valid, err: %s", account.PubKey, err)
	}
	return stableSwap, nil
}

func (p *Program) buildAccounts(accounts []*backend.Account) ([]*KeyedStableSwap, error) {
	stableSwaps := make([]*KeyedStableSwap, 0)
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		stableSwap, err := p.parseAccount(account)
		if err != nil {
			p.log.Printf("parser account err: %s", err)
			continue
		}
		keyedStableSwap := p.upsertStableSwap(account.PubKey, account.Height, stableSwap)
		stableSwaps = append(stableSwaps, keyedStableSwap)
	}
	return stableSwaps, nil
}

func (p *Program) buildModels(stableSwaps []*KeyedStableSwap) ([]*Model, error) {
	checks := make(map[solana.PublicKey]bool)
	pubkeys := make([]solana.PublicKey, 0)
	for _, swap := range stableSwaps {
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
	for _, swap := range stableSwaps {
		tokenA := p.splTokenProgram.GetUser(swap.SwapA)
		if tokenA == nil {
			p.log.Printf("account(%s) is not retrieved in saber program", swap.SwapA)
			continue
		}
		tokenB := p.splTokenProgram.GetUser(swap.SwapB)
		if tokenB == nil {
			p.log.Printf("account(%s) is not retrieved in saber program", swap.SwapB)
			continue
		}
		model := p.upsertModel(swap, tokenA, tokenB)
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
		if _, ok := checks[model.StableSwap.SwapA]; !ok {
			subscribes = append(subscribes, model.StableSwap.SwapA)
			checks[model.StableSwap.SwapA] = true
		}
		if _, ok := checks[model.StableSwap.SwapB]; !ok {
			subscribes = append(subscribes, model.StableSwap.SwapB)
			checks[model.StableSwap.SwapB] = true
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
	tokenA := p.env.Token(model.StableSwap.TokenA)
	tokenB := p.env.Token(model.StableSwap.TokenB)
	amountTokenA := tokenA.AmountUi(model.SwapA.Amount)
	amountTokenB := tokenB.AmountUi(model.SwapB.Amount)
	state0 := fmt.Sprintf("\n    slot: %d", model.CurrentSlot())
	state1 := fmt.Sprintf("\n    %s/%s: %s", tokenA.Symbol, tokenB.Symbol, amountTokenB.Div(amountTokenA).StringFixed(5))
	state2 := fmt.Sprintf("\n    pool: (%s %s)(%s %s)",
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
	if token != model.StableSwap.TokenA && token != model.StableSwap.TokenB {
		return nil, fmt.Errorf("token is not the swap token pair - (%s %s)", market, token)
	}
	//
	swapResult, err := model.Swap(token, amount)
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
	var nonce uint8
	if item, ok := parameter["nonce"]; !ok {
		nonce = 0
		//return nil, fmt.Errorf("no parameter - amount in instruct construction parameter")
	} else {
		nonce = item.(uint8)
	}
	//
	instructions := make([]solana.Instruction, 0)
	instruction, err := p.InstructionArbitrageStep(market, token, amount, flag, nonce)
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
	mintSrcKey, tokenSrcKey, mintDstKey, tokenDstKey, _, err := model.getSwapAccounts(token)
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
	{
		err = p.splTokenProgram.RetrieveUsers([]solana.PublicKey{userSrcKey, userDstKey, tokenSrcKey, tokenDstKey}, true)
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
		tokenSrc := p.splTokenProgram.GetUser(tokenSrcKey)
		if tokenSrc == nil {
			return nil, fmt.Errorf("token (%s) is not retrieved", tokenSrcKey)
		}
		tokenDst := p.splTokenProgram.GetUser(tokenDstKey)
		if tokenDst == nil {
			return nil, fmt.Errorf("token (%s) is not retrieved", tokenDstKey)
		}
		oldStates = append(oldStates, []*spltoken.KeyedUser{userSrc, userDst, tokenSrc, tokenDst}...)
	}
	//
	sr := &program.SimulateState{
		SourceToken:       mintSrcKey,
		DestinationToken:  mintDstKey,
		SourceAmount:      0,
		DestinationAmount: 0,
	}
	updateAccounts, txs, logs, consumed, err := p.backend.Simulate(instructions, []solana.PublicKey{userSrcKey, userDstKey, tokenSrcKey, tokenDstKey})
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
			PubKey:     tokenSrcKey,
			Account: updateAccounts[2],
		}, {
			Height:  0,
			PubKey:     tokenDstKey,
			Account: updateAccounts[3],
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

	state1 := fmt.Sprintf("    token pairs: (%s %s)\n    pool: (%s %d)(%s %d)\n    pool: (%s %d)(%s %d)\n",
		mintSrcKey, mintDstKey,
		tokenSrcKey, oldStates[2].Amount, tokenDstKey, oldStates[3].Amount,
		tokenSrcKey, updateStates[2].Amount, tokenDstKey, updateStates[3].Amount)
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
	mintSrc, tokenSrc, mintDst, tokenDst, feeAdmin, err := model.getSwapAccounts(tokenIn)
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
			{PublicKey: feeAdmin, IsSigner: false, IsWritable: true},
			{PublicKey: p.splTokenProgram.Id(), IsSigner: false, IsWritable: false},
			{PublicKey: program.SysClock, IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: p.id,
	}

	return instruction, nil
}

func (p *Program) InstructionArbitrageStep(market solana.PublicKey, tokenIn solana.PublicKey, amountIn uint64, flag uint8, nonce uint8) (solana.Instruction, error) {
	var model *Model
	if item, ok := p.models[market]; !ok {
		return nil, fmt.Errorf("no model of this market - %s", market)
	} else {
		model = item
	}
	// src and dst
	tokenSrc, swapSrc, tokenDst, swapDst, feeAdmin, err := model.getSwapAccounts(tokenIn)
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
	data := make([]byte, 13)
	data[0] = 0
	binary.LittleEndian.PutUint64(data[1:], amountIn)
	data[9] = 1
	data[10] = 0
	data[11] = flag
	data[12] = nonce
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
			{PublicKey: feeAdmin, IsSigner: false, IsWritable: true},
			{PublicKey: p.splTokenProgram.Id(), IsSigner: false, IsWritable: false},
			{PublicKey: program.SysClock, IsSigner: false, IsWritable: false},
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
		{PublicKey: market.Key, IsSigner: false, IsWritable: true},
		{PublicKey: authority, IsSigner: false, IsWritable: true},
		{PublicKey: market.SwapA, IsSigner: false, IsWritable: true},
		{PublicKey: market.SwapB, IsSigner: false, IsWritable: true},
		{PublicKey: market.AdminFeeKeyA, IsSigner: false, IsWritable: true},
		{PublicKey: market.AdminFeeKeyB, IsSigner: false, IsWritable: true},
	}
	return IsAccounts, nil
}

func (p *Program) MatchOrders(parameter map[string]interface{}) ([]*solana.AccountMeta, error) {
	return nil, nil
}

func (p *Program) ConsumeEvents(parameter map[string]interface{}) ([]*solana.AccountMeta, error) {
	return nil, nil
}
