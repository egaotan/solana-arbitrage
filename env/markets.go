package env

import (
	"encoding/json"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/gagliardetto/solana-go"
	"os"
)

func (e *Env) loadMarkets() {
	infoJson, err := os.ReadFile(config.MarketsFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(infoJson, &e.markets)
	if err != nil {
		panic(err)
	}
}

func (e *Env) UseMarket(program solana.PublicKey, key solana.PublicKey) bool {
	if item, ok := e.markets[program]; ok {
		if _, ok := item[key]; ok {
			return true
		}
	}
	return false
}

func (e *Env) Markets(program solana.PublicKey) []solana.PublicKey {
	marketKeys := make([]solana.PublicKey, 0)
	markets, ok := e.markets[program]
	if !ok {
		return marketKeys
	}
	for marketKey, _ := range markets {
		marketKeys = append(marketKeys, marketKey)
	}
	return marketKeys
}

func (e *Env) FindMarketProgram(key solana.PublicKey) solana.PublicKey {
	for program, markets := range e.markets {
		for market, use := range markets {
			if market == key && use {
				return program
			}
		}
	}
	return solana.PublicKey{}
}
