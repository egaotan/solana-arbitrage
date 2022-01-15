package orca

import (
	"fmt"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/gagliardetto/solana-go"
	"math/big"
)

func (m *Model) swapConstantProduct(token solana.PublicKey, amount uint64) (*program.SwapResult, error) {
	if token != m.TokenSwap.TokenA && token != m.TokenSwap.TokenB {
		return nil, fmt.Errorf("token is not in this pool")
	}
	tradeFee := m.tradingFeeConstantProduct(amount)
	ownerFee := m.ownerTradingFeeConstantProduct(amount)
	totalFee := new(big.Int).Add(tradeFee, ownerFee)
	sourceAmountLessFees := new(big.Int).Sub(new(big.Int).SetUint64(amount), totalFee)
	if sourceAmountLessFees.Cmp(new(big.Int).SetUint64(0)) <= 0 {
		return nil, fmt.Errorf("amount is too small")
	}
	sr := m.swapWithoutFeesConstantProduct(token, sourceAmountLessFees)
	sr.AmountIn = sr.AmountIn + totalFee.Uint64()
	return sr, nil
}

func (m *Model) tradingFeeConstantProduct(amount uint64) *big.Int {
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

func (m *Model) ownerTradingFeeConstantProduct(amount uint64) *big.Int {
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

func (m *Model) swapWithoutFeesConstantProduct(token solana.PublicKey, amountLessFees *big.Int) *program.SwapResult {
	sourceToken := m.TokenSwap.TokenA
	sourceAmount := new(big.Int).SetUint64(m.SwapA.Amount)
	sourceSlot := m.SwapA.Height
	destinationToken := m.TokenSwap.TokenB
	destinationAmount := new(big.Int).SetUint64(m.SwapB.Amount)
	destinationSlot := m.SwapB.Height
	if token == m.TokenSwap.TokenB {
		sourceToken = m.TokenSwap.TokenB
		sourceAmount = new(big.Int).SetUint64(m.SwapB.Amount)
		sourceSlot = m.SwapB.Height
		destinationToken = m.TokenSwap.TokenA
		destinationAmount = new(big.Int).SetUint64(m.SwapA.Amount)
		destinationSlot = m.SwapA.Height
	}
	invariant := new(big.Int).Mul(sourceAmount, destinationAmount)
	newSourceAmount := new(big.Int).Add(sourceAmount, amountLessFees)
	newDestinationAmount := new(big.Int).Div(invariant, newSourceAmount)
	swappedSourceAmount := new(big.Int).Sub(newSourceAmount, sourceAmount)
	swappedDestinationAmount := new(big.Int).Sub(destinationAmount, newDestinationAmount)
	return &program.SwapResult{
		TokenIn:    sourceToken,
		AmountIn:   swappedSourceAmount.Uint64(),
		SlotIn:     sourceSlot,
		TokenOut:   destinationToken,
		AmountOut:  swappedDestinationAmount.Uint64(),
		SlotOut:    destinationSlot,
		NewSwapSrc: newSourceAmount.Uint64(),
		NewSwapDst: newDestinationAmount.Uint64(),
	}
}
