package program

import (
	"github.com/gagliardetto/solana-go"
)

var (
	StateUsed       = "used"
	StateCustomUsed = "custom_used"
)

type SwapResult struct {
	TokenIn    solana.PublicKey
	AmountIn   uint64
	SlotIn     uint64
	TokenOut   solana.PublicKey
	AmountOut  uint64
	SlotOut    uint64
	NewSwapSrc uint64
	NewSwapDst uint64
}

type Model interface {
	Program() solana.PublicKey
	Id() solana.PublicKey
	Type() string
	TokenPair() []solana.PublicKey
	PoolPair() []solana.PublicKey
	CurrentSlot() uint64
	SetState(key string, value interface{}) error
	HasState(key string) bool
	State(key string) interface{}
	Swap(token solana.PublicKey, amount uint64) (*SwapResult, error)
}
