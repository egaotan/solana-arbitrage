package calculator

import (
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/gagliardetto/solana-go"
)

var (
	CyclicArb = "cyclic"
	SerumArb  = "serum"
)

var (
	SBF = "sbf"
)

type Callback interface {
	OnArbitrage(result *Result) error
}

type Calculator interface {
	Start() error
	Stop() error
	Name() string
	Algorithm() string
	Algorithms() []string
	AddModel(model program.Model) error
	Calculate() error
}

type Result struct {
	Id         uint64
	Calculator string
	TokenIn    solana.PublicKey
	AmountIn   uint64
	UsdcAmount uint64
	TokenOut   solana.PublicKey
	AmountOut  uint64
	Models     []program.Model
}
