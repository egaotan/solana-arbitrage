package env

import (
	"encoding/json"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	"os"
)

type Token struct {
	Symbol  string
	Name    string
	Decimal uint64
	Price   decimal.Decimal
}

func (token *Token) AmountUi(amount uint64) decimal.Decimal {
	amountUi := decimal.NewFromInt(int64(amount))
	amountUi = amountUi.Div(decimal.NewFromInt(int64(token.Decimal)))
	return amountUi
}

func (e *Env) loadTokens() {
	infoJson, err := os.ReadFile(config.TokensFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(infoJson, &e.tokens)
	if err != nil {
		panic(err)
	}
}

func (e *Env) Token(key solana.PublicKey) *Token {
	if item, ok := e.tokens[key]; ok {
		return item
	}
	return nil
}
