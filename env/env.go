package env

import (
	"context"
	"github.com/gagliardetto/solana-go"
	"log"
)

type Env struct {
	logger                   *log.Logger
	ctx                      context.Context
	tokens                   map[solana.PublicKey]*Token
	tokensUser               map[solana.PublicKey]solana.PublicKey
	tokensUserSimulate       map[solana.PublicKey]solana.PublicKey
	usersOwner               map[solana.PublicKey]solana.PublicKey
	usersOwnerSimulate       map[solana.PublicKey]solana.PublicKey
	marketOpenOrders         map[solana.PublicKey]solana.PublicKey
	marketOpenOrdersSimulate map[solana.PublicKey]solana.PublicKey
	markets                  map[solana.PublicKey]map[solana.PublicKey]bool
}

func NewEnv(ctx context.Context) *Env {
	env := &Env{
		ctx:                      ctx,
		logger:                   log.Default(),
		tokens:                   make(map[solana.PublicKey]*Token),
		tokensUser:               make(map[solana.PublicKey]solana.PublicKey),
		tokensUserSimulate:       make(map[solana.PublicKey]solana.PublicKey),
		usersOwner:               make(map[solana.PublicKey]solana.PublicKey),
		usersOwnerSimulate:       make(map[solana.PublicKey]solana.PublicKey),
		marketOpenOrders:         make(map[solana.PublicKey]solana.PublicKey),
		marketOpenOrdersSimulate: make(map[solana.PublicKey]solana.PublicKey),
		markets:                  make(map[solana.PublicKey]map[solana.PublicKey]bool),
	}
	return env
}

func (e *Env) Start() {
	e.logger.Printf("start env......")
	e.loadTokens()
	e.loadTokensUser()
	e.loadTokensUserSimulate()
	e.loadUsersOwner()
	e.loadUsersOwnerSimulate()
	e.loadMarketOpenOrders()
	e.loadMarketOpenOrdersSimulate()
	e.loadMarkets()
}

func (e *Env) Stop() {
	e.logger.Printf("stop env......")
}
