package raydium

import (
	"bytes"
	"encoding/binary"
	"github.com/gagliardetto/solana-go"
)

var (
	AmmInfoLayoutSize = 752
)

type FeesLayout struct {
	MinSeparateNumerator   uint64
	MinSeparateDenominator uint64
	TradeFeeNumerator      uint64
	TradeFeeDenominator    uint64
	PnlNumerator           uint64
	PnlDenominator         uint64
	SwapFeeNumerator       uint64
	SwapFeeDenominator     uint64
}

type OutputDataLayout struct {
	NeedTakePnlCoin      uint64
	NeedTakePnlPc        uint64
	TotalPnlPc           uint64
	TotalPnlCoin         uint64
	PoolTotalDepositPc   [2]uint64
	PoolTotalDepositCoin [2]uint64
	SwapCoinInAmount     [2]uint64
	SwapPcOutAmount      [2]uint64
	SwapCoin2PcFee       uint64
	SwapPcInAmount       [2]uint64
	SwapCoinOutAmount    [2]uint64
	SwapPc2CoinFee       uint64
}

type AmmInfoLayout struct {
	Status             uint64
	Nonce              uint64
	OrderNum           uint64
	Depth              uint64
	CoinDecimals       uint64
	PcDecimals         uint64
	State              uint64
	ResetFlag          uint64
	MinSize            uint64
	VolMaxCutRatio     uint64
	AmountWave         uint64
	CoinLotSize        uint64
	PcLotSize          uint64
	MinPriceMultiplier uint64
	MaxPriceMultiplier uint64
	SysDecimalValue    uint64
	Fees               FeesLayout
	Output             OutputDataLayout
	TokenCoin          solana.PublicKey
	TokenPc            solana.PublicKey
	CoinMint           solana.PublicKey
	PcMint             solana.PublicKey
	LpMint             solana.PublicKey
	OpenOrders         solana.PublicKey
	Market             solana.PublicKey
	SerumDex           solana.PublicKey
	TargetOrders       solana.PublicKey
	WithdrawQueue      solana.PublicKey
	TokenTempLp        solana.PublicKey
	AmmOwner           solana.PublicKey
	PnlOwner           solana.PublicKey
}

func (ammInfo *AmmInfoLayout) unpack(data []byte) error {
	buf := bytes.NewReader(data)
	return binary.Read(buf, binary.LittleEndian, ammInfo)
}

type KeyedAmmInfo struct {
	AmmInfoLayout
	Height uint64
	Key    solana.PublicKey
}
