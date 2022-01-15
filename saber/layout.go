package saber

import (
	"github.com/gagliardetto/solana-go"
)

var (
	StableSwapLayoutSize = 395
)

type StableSwapLayout struct {
	IsInitialized       int8
	IsPaused            int8
	Nonce               int8
	InitialAmpFactor    uint64
	TargetAmpFactor     uint64
	StartRampTs         int64
	StopRampTs          int64
	FutureAdminDeadline int64
	FutureAdminKey      solana.PublicKey
	AdminKey            solana.PublicKey
	SwapA               solana.PublicKey
	SwapB               solana.PublicKey
	PoolMint            solana.PublicKey
	TokenA              solana.PublicKey
	TokenB              solana.PublicKey
	AdminFeeKeyA        solana.PublicKey
	AdminFeeKeyB        solana.PublicKey
	Fees                Fees
}

type Fees struct {
	AdminTradeFeeNumerator      uint64
	AdminTradeFeeDenominator    uint64
	AdminWithdrawFeeNumerator   uint64
	AdminWithdrawFeeDenominator uint64
	TradeFeeNumerator           uint64
	TradeFeeDenominator         uint64
	WithdrawFeeNumerator        uint64
	WithdrawFeeDenominator      uint64
}

type KeyedStableSwap struct {
	Key    solana.PublicKey
	Height uint64
	StableSwapLayout
}
