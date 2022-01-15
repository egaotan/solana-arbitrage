package tokenswap

import (
	"fmt"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/gagliardetto/solana-go"
	"math/big"
)

var (
	NCoins        = 2
	NCoinsSquared = 4
)

func (m *Model) swapStable(token solana.PublicKey, amount uint64) (*program.SwapResult, error) {
	if token != m.TokenSwap.TokenA && token != m.TokenSwap.TokenB {
		return nil, fmt.Errorf("token is not in this pool")
	}
	tradeFee := m.tradingStable(amount)
	ownerFee := m.ownerTradingStable(amount)
	totalFee := new(big.Int).Add(tradeFee, ownerFee)
	sourceAmountLessFees := new(big.Int).Sub(new(big.Int).SetUint64(amount), totalFee)
	if sourceAmountLessFees.Cmp(new(big.Int).SetUint64(0)) <= 0 {
		return nil, fmt.Errorf("amount is insufficient")
	}
	sr := m.swapWithoutFeesStable(token, sourceAmountLessFees)
	sr.AmountIn = sr.AmountIn + totalFee.Uint64()
	return sr, nil
}

func (m *Model) tradingStable(amount uint64) *big.Int {
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

func (m *Model) ownerTradingStable(amount uint64) *big.Int {
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

func (m *Model) swapWithoutFeesStable(token solana.PublicKey, amountLessFees *big.Int) *program.SwapResult {
	amp := m.TokenSwap.SwapCurve.Calculator.Data1
	leverage := new(big.Int).Mul(new(big.Int).SetUint64(amp), new(big.Int).SetUint64(uint64(NCoins)))
	sourceToken := m.TokenSwap.TokenA
	sourceAmount := new(big.Int).SetUint64(m.SwapA.Amount)
	destinationToken := m.TokenSwap.TokenB
	destinationAmount := new(big.Int).SetUint64(m.SwapB.Amount)
	if token == m.TokenSwap.TokenB {
		sourceToken = m.TokenSwap.TokenB
		sourceAmount = new(big.Int).SetUint64(m.SwapB.Amount)
		destinationToken = m.TokenSwap.TokenA
		destinationAmount = new(big.Int).SetUint64(m.SwapA.Amount)
	}
	newSourceAmount := new(big.Int).Add(sourceAmount, amountLessFees)
	dVal := m.computeD(leverage, sourceAmount, destinationAmount)
	newDestinationAmount := m.computeNewDestinationAmount(leverage, newSourceAmount, dVal)
	swapDestinationAmount := new(big.Int).Sub(destinationAmount, newDestinationAmount)
	return &program.SwapResult{
		TokenIn:   sourceToken,
		AmountIn:  amountLessFees.Uint64(),
		TokenOut:  destinationToken,
		AmountOut: swapDestinationAmount.Uint64(),
	}
}

func (m *Model) calculateStep(d *big.Int, leverage *big.Int, sumX *big.Int, dProduct *big.Int) *big.Int {
	leverageMul := new(big.Int).Mul(leverage, sumX)
	ncoins := new(big.Int).SetUint64(uint64(NCoins))
	dpMul := new(big.Int).Mul(dProduct, ncoins)
	lVal := new(big.Int).Mul(new(big.Int).Add(leverageMul, dpMul), d)
	leverageSub := new(big.Int).Mul(d, new(big.Int).Sub(leverage, new(big.Int).SetUint64(1)))
	nCoinsSum := new(big.Int).Mul(dProduct, new(big.Int).Add(ncoins, new(big.Int).SetUint64(1)))
	rVal := new(big.Int).Add(leverageSub, nCoinsSum)
	return new(big.Int).Div(lVal, rVal)
}

func (m *Model) computeD(leverage *big.Int, sourceAmount *big.Int, destinationAmount *big.Int) *big.Int {
	ncoins := new(big.Int).SetUint64(uint64(NCoins))
	sourceAmountTimesCoins := new(big.Int).Add(new(big.Int).Mul(sourceAmount, ncoins), new(big.Int).SetUint64(1))
	destinationAmountTimesCoins := new(big.Int).Add(new(big.Int).Mul(destinationAmount, ncoins), new(big.Int).SetUint64(1))
	sumX := new(big.Int).Add(sourceAmount, destinationAmount)
	if sumX.Cmp(new(big.Int).SetUint64(0)) == 0 {
		return new(big.Int).SetUint64(0)
	}
	dPrevious := new(big.Int)
	d := sumX
	for i := 0; i < 32; i++ {
		dProduct := d
		dProduct = new(big.Int).Div(new(big.Int).Mul(dProduct, d), sourceAmountTimesCoins)
		dProduct = new(big.Int).Div(new(big.Int).Mul(dProduct, d), destinationAmountTimesCoins)
		dPrevious = d
		d = m.calculateStep(d, leverage, sumX, dProduct)
		if d.Cmp(dPrevious) == 0 {
			break
		}
	}
	return d
}

func (m *Model) computeNewDestinationAmount(leverage *big.Int, newSourceAmount *big.Int, dVal *big.Int) *big.Int {
	ncoins := new(big.Int).SetUint64(uint64(NCoins))
	ncoinsSquared := new(big.Int).SetUint64(uint64(NCoinsSquared))
	c := new(big.Int).Div(
		new(big.Int).Exp(dVal, new(big.Int).Add(ncoins, new(big.Int).SetUint64(1)), nil),
		new(big.Int).Mul(new(big.Int).Mul(newSourceAmount, ncoinsSquared), leverage))
	b := new(big.Int).Add(newSourceAmount, new(big.Int).Div(dVal, leverage))
	yPrev := new(big.Int)
	y := dVal
	for i := 0; i < 32; i++ {
		yPrev = y
		y = new(big.Int).Div(
			new(big.Int).Add(new(big.Int).Exp(y, new(big.Int).SetUint64(2), nil), c),
			new(big.Int).Sub(new(big.Int).Add(new(big.Int).Mul(y, new(big.Int).SetUint64(2)), b), dVal),
		)
		if y.Cmp(yPrev) == 0 {
			break
		}
	}
	return y
}
