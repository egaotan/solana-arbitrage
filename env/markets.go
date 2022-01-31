package env

import (
	"encoding/json"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/program"
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

func (e *Env) Markets(program1 solana.PublicKey) []solana.PublicKey {
	marketKeys := make([]solana.PublicKey, 0)
	markets, ok := e.markets[program1]
	if !ok {
		return marketKeys
	}
	for marketKey, _ := range markets {
		marketKeys = append(marketKeys, marketKey)
	}
	if program1 != program.SerumV22 {
		return marketKeys
	}
	//
	marketKeys = make([]solana.PublicKey, 0)
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("9wFFyRfZBsuAha4YcuxcXLKwMxJR43S7fPfQLusDBzvT"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("HWHvQhFmJB3NUcu1aihKmrKegfVxBEHzwVX6yZCKEsi1"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("6oGsL2puUgySccKzn9XA9afqF217LfxP5ocq4B3LWsjy"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("HxkQdUnrPdHwXP5T9kewEXs3ApgvbufuTfdw9v1nApFd"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("Di66GTLsV64JgCCYGVcY21RZ173BHkjJVgPyezNN7P1K"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("CVJVpXU9xksCt2uSduVDrrqVw6fLZCAtNusuqLKc5DhW"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("HxFLKUAmAMLz1jtT3hbvCMELwH5H9tpM2QugP8sKyfhW"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("HCWgghHfDefcGZsPsLAdMP3NigJwBrptZnXemeQchZ69"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("FR3SPJmgfRSKKQ2ysUZBu7vJLpzTixXnjzb84bY3Diif"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("HXBi8YBwbh4TXF6PjVw81m8Z3Cc4WBofvauj5SBFdgUs"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("DvmDTjsdnN77q7SST7gngLydP1ASNNpUVi4cNfU95oCr"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("77quYg4MGneUdjgXCunt9GgM1usmrxKY31twEy3WHwcS"))
	marketKeys = append(marketKeys, solana.MustPublicKeyFromBase58("5cLrMai1DsLRYc1Nio9qMTicsWtvzjzZfJPXyAoF4t1Z"))
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
