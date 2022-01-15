package env

import (
	"encoding/json"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/gagliardetto/solana-go"
	"os"
)

func (e *Env) loadMarketOpenOrders() {
	infoJson, err := os.ReadFile(config.MarketOpenOrdersFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(infoJson, &e.marketOpenOrders)
	if err != nil {
		panic(err)
	}
}

func (e *Env) loadMarketOpenOrdersSimulate() {
	infoJson, err := os.ReadFile(config.MarketOpenOrdersSimulateFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(infoJson, &e.marketOpenOrdersSimulate)
	if err != nil {
		panic(err)
	}
}

func (e *Env) MarketOpenOrder(market solana.PublicKey) solana.PublicKey {
	item, ok := e.marketOpenOrders[market]
	if !ok {
		return solana.PublicKey{}
	}
	return item
}

func (e *Env) MarketOpenOrderSimulate(market solana.PublicKey) solana.PublicKey {
	item, ok := e.marketOpenOrdersSimulate[market]
	if !ok {
		return solana.PublicKey{}
	}
	return item
}
