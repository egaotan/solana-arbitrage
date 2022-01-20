package spltoken

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"log"
)

type Callback interface {
	OnBalanceUpdate(userKey solana.PublicKey, newBalance uint64, oldBalance uint64, tokenKey solana.PublicKey, slot uint64) error
}

type Program struct {
	backend        *backend.Backend
	log            *log.Logger
	ctx            context.Context
	id             solana.PublicKey
	tokens         map[solana.PublicKey]*KeyedToken
	users          map[solana.PublicKey]*KeyedUser
	updateAccounts chan *backend.Account
	cb             Callback
}

func NewProgram(context context.Context, be *backend.Backend, cb Callback) *Program {
	p := &Program{
		ctx:     context,
		backend: be,
		//log:            log.Default(),
		id:             program.Token,
		tokens:         make(map[solana.PublicKey]*KeyedToken),
		users:          make(map[solana.PublicKey]*KeyedUser),
		updateAccounts: make(chan *backend.Account, 1024),
		cb:             cb,
	}
	return p
}

func (p *Program) Name() string {
	return "spl token"
}

func (p *Program) Id() solana.PublicKey {
	return p.id
}

func (p *Program) Start() error {
	p.log = utils.NewLog(config.LogPath, fmt.Sprintf("%s", p.Name()))
	p.log.Printf("start spl token program: %s......", p.Id())
	go p.update()
	return nil
}

func (p *Program) Stop() error {
	p.log.Printf("stop spl token program......")
	return nil
}

func (p *Program) RetrieveUsers(pubkeys []solana.PublicKey) error {
	accounts, err := p.backend.Accounts(pubkeys)
	if err != nil {
		return err
	}
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		user, err := p.parseUser(account)
		if err != nil {
			p.log.Printf("account(%s) err: %s", account.PubKey, err)
			continue
		}
		p.upsertUser(account.PubKey, account.Height, user)
	}
	return nil
}

func (p *Program) GetUser(key solana.PublicKey) *KeyedUser {
	user, ok := p.users[key]
	if !ok {
		return nil
	}
	return user
}

func (p *Program) RetrieveTokens(pubkeys []solana.PublicKey, force bool) error {
	accounts, err := p.backend.Accounts(pubkeys)
	if err != nil {
		return err
	}
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		token, err := p.parseToken(account)
		if err != nil {
			p.log.Printf("account(%s) %s", account.PubKey, err)
			continue
		}
		p.upsertToken(account.PubKey, account.Height, token)
	}
	return nil
}

func (p *Program) GetToken(key solana.PublicKey) *KeyedToken {
	token, ok := p.tokens[key]
	if !ok {
		return nil
	}
	return token
}

func (p *Program) parseUser(account *backend.Account) (UserLayout, error) {
	user := UserLayout{}
	if account.Account == nil {
		return user, fmt.Errorf("account(%s) is misssing")
	}
	if account.Account.Owner != p.id {
		return user, fmt.Errorf("account(%s) is not spl token program account, expected: %s, actual: %s", account.PubKey, p.id, account.Account.Owner)
	}
	userData := account.Account.Data.GetBinary()
	if len(userData) != TokenLayoutSize {
		return user, fmt.Errorf("spl token account(%s) data size is not valid, expected: %d, actual: %d", account.PubKey, TokenLayoutSize, len(userData))
	}
	buf := bytes.NewReader(userData)
	err := binary.Read(buf, binary.LittleEndian, &user)
	if err != nil {
		return user, fmt.Errorf("spl token account(%s) data is not valid, err: %s", account.PubKey, err)
	}
	return user, nil
}

func (p *Program) parseToken(account *backend.Account) (TokenLayout, error) {
	token := TokenLayout{}
	if account.Account.Owner != p.id {
		return token, fmt.Errorf("account(%s) is not spl token program account", account.PubKey)
	}
	tokenData := account.Account.Data.GetBinary()
	if len(tokenData) != MintLayoutSize {
		return token, fmt.Errorf("account(%s) data size is not valid", account.PubKey)
	}
	buf := bytes.NewReader(tokenData)
	err := binary.Read(buf, binary.LittleEndian, &token)
	if err != nil {
		return token, fmt.Errorf("account(%s) data is not valid, err: %s", account.PubKey, err)
	}
	return token, nil
}

func (p *Program) upsertUser(pubkey solana.PublicKey, height uint64, account UserLayout) *KeyedUser {
	keyedToken, ok := p.users[pubkey]
	if !ok {
		keyedToken = &KeyedUser{
			Key:        pubkey,
			Height:     height,
			UserLayout: account,
		}
		p.users[pubkey] = keyedToken
	} else {
		keyedToken.UserLayout = account
		keyedToken.Height = height
	}
	return keyedToken
}

func (p *Program) upsertToken(pubkey solana.PublicKey, height uint64, mint TokenLayout) *KeyedToken {
	keyedMint, ok := p.tokens[pubkey]
	if !ok {
		keyedMint = &KeyedToken{
			Key:         pubkey,
			Height:      height,
			TokenLayout: mint,
		}
		p.tokens[pubkey] = keyedMint
	} else {
		keyedMint.TokenLayout = mint
		keyedMint.Height = height
	}
	return keyedMint
}

func (p *Program) OnAccountUpdate(account *backend.Account) error {
	p.updateAccounts <- account
	return nil
}

func (p *Program) SubscribeUsers(pubkeys []solana.PublicKey) error {
	for _, pubkey := range pubkeys {
		_, ok := p.users[pubkey]
		if !ok {
			p.log.Printf("user %s is not in cache", pubkey.String())
			continue
		}
		p.backend.SubscribeAccount(pubkey, p, 0)
	}
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
	// slot
	p.log.Printf("update account slot diff: %d, %d", account.Height, program.GlobalSlot)
	// old
	balanceOld := uint64(0)
	keyedUserOld, ok := p.users[account.PubKey]
	if ok {
		balanceOld = keyedUserOld.Amount
	}
	//
	user, err := p.parseUser(account)
	if err != nil {
		p.log.Printf("parse user err: %s", err.Error())
		return
	}
	p.upsertUser(account.PubKey, account.Height, user)
	//
	balanceNew := user.Amount
	if balanceOld != 0 && p.cb != nil {
		p.cb.OnBalanceUpdate(account.PubKey, balanceOld, balanceNew, user.Mint, account.Height)
	}
}

func (p *Program) GetBalance(key solana.PublicKey) (uint64, error) {
	err := p.RetrieveUsers([]solana.PublicKey{key})
	if err != nil {
		return 0, err
	}
	user := p.GetUser(key)
	return user.Amount, nil
}

func (p *Program) InstructionInitUser(user solana.PublicKey, token solana.PublicKey, owner solana.PublicKey) (solana.Instruction, error) {
	data := make([]byte, 1)
	data[0] = 1
	instruction := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: user, IsSigner: true, IsWritable: true},
			{PublicKey: token, IsSigner: false, IsWritable: false},
			{PublicKey: owner, IsSigner: false, IsWritable: false},
			{PublicKey: program.SysRent, IsSigner: false, IsWritable: false},
		},
		IsData:      data,
		IsProgramID: p.id,
	}
	return instruction, nil
}

func (p *Program) BuildAccount(accounts []*backend.Account) ([]*KeyedUser, error) {
	tokens := make([]*KeyedUser, 0)
	for i := 0; i < len(accounts); i++ {
		account := accounts[i]
		token, err := p.parseUser(account)
		if err != nil {
			p.log.Printf("%v", err)
			continue
		}
		keyedToken := &KeyedUser{
			Key:        account.PubKey,
			Height:     account.Height,
			UserLayout: token,
		}
		tokens = append(tokens, keyedToken)
	}
	return tokens, nil
}

func (p *Program) DecodeInstruction(in *backend.Instruction) (solana.PublicKey, solana.PublicKey, uint64, error) {
	if in.DataLen != 9 {
		return solana.PublicKey{}, solana.PublicKey{}, 0, fmt.Errorf("data is invalid")
	}
	command := in.Data[0]
	if command != 3 {
		return solana.PublicKey{}, solana.PublicKey{}, 0, fmt.Errorf("is not transfer")
	}
	return in.Accounts[0], in.Accounts[1], binary.LittleEndian.Uint64(in.Data[1:]), nil
}
