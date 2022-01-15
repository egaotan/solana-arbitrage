package tokenswap

import (
	"fmt"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/gagliardetto/solana-go"
	"math/big"
)

func (m *Model) swapConstantPrice(token solana.PublicKey, amount uint64) (*program.SwapResult, error) {
	if token != m.TokenSwap.TokenA && token != m.TokenSwap.TokenB {
		return nil, fmt.Errorf("token is not in this pool")
	}
	tradeFee := m.tradingFeeConstantPrice(amount)
	ownerFee := m.ownerTradingFeeConstantPrice(amount)
	totalFee := new(big.Int).Add(tradeFee, ownerFee)
	sourceAmountLessFees := new(big.Int).Sub(new(big.Int).SetUint64(amount), totalFee)
	if sourceAmountLessFees.Cmp(new(big.Int).SetUint64(0)) <= 0 {
		return nil, fmt.Errorf("amount is insufficient")
	}
	sr := m.swapWithoutFeesConstantPrice(token, sourceAmountLessFees)
	sr.AmountIn = sr.AmountIn + totalFee.Uint64()
	return sr, nil
}

func (m *Model) tradingFeeConstantPrice(amount uint64) *big.Int {
	if m.TokenSwap.Fees.TradeFeeNumerator == 0 || amount == 0 {
		return new(big.Int).SetUint64(0)
	}
	fee := new(big.Int).Div(
		new(big.Int).Mul(
			new(big.Int).SetUint64(amount),
			new(big.Int).SetUint64(m.TokenSwap.Fees.TradeFeeNumerator),
		),
		new(big.Int).SetUint64(m.TokenSwap.Fees.TradeFeeDenominator),
	)
	if fee.Cmp(new(big.Int).SetUint64(0)) == 0 {
		return new(big.Int).SetUint64(1)
	} else {
		return fee
	}
}

func (m *Model) ownerTradingFeeConstantPrice(amount uint64) *big.Int {
	if m.TokenSwap.Fees.OwnerTradeFeeNumerator == 0 || amount == 0 {
		return new(big.Int).SetUint64(0)
	}
	fee := new(big.Int).Div(
		new(big.Int).Mul(
			new(big.Int).SetUint64(amount),
			new(big.Int).SetUint64(m.TokenSwap.Fees.OwnerTradeFeeNumerator),
		),
		new(big.Int).SetUint64(m.TokenSwap.Fees.OwnerTradeFeeDenominator),
	)
	if fee.Cmp(new(big.Int).SetUint64(0)) == 0 {
		return new(big.Int).SetUint64(1)
	} else {
		return fee
	}
}

func (m *Model) swapWithoutFeesConstantPrice(token solana.PublicKey, amountLessFees *big.Int) *program.SwapResult {
	bPrice := m.TokenSwap.SwapCurve.Calculator.Data1
	tokenBPrice := new(big.Int).SetUint64(bPrice)
	swappedSourceAmount := new(big.Int).SetUint64(0)
	swappedDestinationAmount := new(big.Int).SetUint64(0)
	sourceToken := m.TokenSwap.TokenA
	destinationToken := m.TokenSwap.TokenB
	if token == m.TokenSwap.TokenB {
		sourceToken = m.TokenSwap.TokenB
		destinationToken = m.TokenSwap.TokenA
		swappedSourceAmount = amountLessFees
		swappedDestinationAmount = new(big.Int).Mul(amountLessFees, tokenBPrice)
	} else {
		sourceToken = m.TokenSwap.TokenA
		destinationToken = m.TokenSwap.TokenB
		swappedDestinationAmount = new(big.Int).Div(amountLessFees, tokenBPrice)
		swappedSourceAmount = amountLessFees
	}
	return &program.SwapResult{
		TokenIn:   sourceToken,
		AmountIn:  swappedSourceAmount.Uint64(),
		TokenOut:  destinationToken,
		AmountOut: swappedDestinationAmount.Uint64(),
	}
}
