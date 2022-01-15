package orca

import (
	"github.com/gagliardetto/solana-go"
)

type Fees struct {
	TradeFeeNumerator           uint64
	TradeFeeDenominator         uint64
	OwnerTradeFeeNumerator      uint64
	OwnerTradeFeeDenominator    uint64
	OwnerWithdrawFeeNumerator   uint64
	OwnerWithdrawFeeDenominator uint64
	HostFeeNumerator            uint64
	HostFeeDenominator          uint64
}

type Calculator struct {
	Data1 uint64
	Data2 uint64
	Data3 uint64
	Data4 uint64
}

const (
	ConstantProduct = 0
	ConstantPrice   = 1
	Stable          = 2
	Offset          = 3
)

type SwapCurve struct {
	CurveType  int8
	Calculator Calculator
}

var (
	SwapLayoutSize = 324
)

type SwapLayout struct {
	Version        int8
	IsInitialized  int8
	BumpSeed       int8
	TokenProgramId solana.PublicKey
	SwapA          solana.PublicKey
	SwapB          solana.PublicKey
	PoolToken      solana.PublicKey
	TokenA         solana.PublicKey
	TokenB         solana.PublicKey
	PoolFeeAccount solana.PublicKey
	Fees           Fees
	SwapCurve      SwapCurve
}

type KeyedSwap struct {
	Key    solana.PublicKey
	Height uint64
	SwapLayout
}
