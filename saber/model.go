package saber

import (
	"fmt"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/gagliardetto/solana-go"
	"math/big"
)

var (
	NCoins        = 2
	NCoinsSquared = 4
	Iterations    = 20
)

type Model struct {
	ProgramId  solana.PublicKey
	StableSwap *KeyedStableSwap
	SwapA      *spltoken.KeyedUser
	SwapB      *spltoken.KeyedUser
	States     map[string]interface{}
}

func (m *Model) Program() solana.PublicKey {
	return m.ProgramId
}

func (m *Model) Id() solana.PublicKey {
	return m.StableSwap.Key
}

func (m *Model) TokenPair() []solana.PublicKey {
	return []solana.PublicKey{m.StableSwap.TokenA, m.StableSwap.TokenB}
}

func (m *Model) PoolPair() []solana.PublicKey {
	return []solana.PublicKey{m.StableSwap.SwapA, m.StableSwap.SwapB}
}

func (m *Model) CurrentSlot() uint64 {
	return m.SwapA.Height
}

func (m *Model) SetState(key string, value interface{}) error {
	m.States[key] = value
	return nil
}

func (m *Model) HasState(key string) bool {
	if _, ok := m.States[key]; ok {
		return true
	}
	return false
}

func (m *Model) State(key string) interface{} {
	if item, ok := m.States[key]; ok {
		return item
	}
	return nil
}

func (m *Model) Type() string {
	return program.AMM
}

func (m *Model) Swap(token solana.PublicKey, amount uint64) (*program.SwapResult, error) {
	if amount == 0 {
		return nil, fmt.Errorf("swap in amount is zero")
	}
	if m.StableSwap.IsPaused != 0 {
		return nil, fmt.Errorf("swap is paused")
	}
	return m.stableSwap(token, amount)
}

func (m *Model) computeAmpFactor() *big.Int {
	return nil
}

func (m *Model) tradeFee(dy *big.Int) *big.Int {
	return muldivimbalanced(dy, m.StableSwap.Fees.TradeFeeNumerator, m.StableSwap.Fees.TradeFeeDenominator)
}

func (m *Model) adminTradeFee(dyFee *big.Int) *big.Int {
	return muldivimbalanced(dyFee, m.StableSwap.Fees.AdminTradeFeeNumerator, m.StableSwap.Fees.TradeFeeDenominator)
}

func (m *Model) computeD(amp *big.Int, sourceAmount *big.Int, destinationAmount *big.Int) *big.Int {
	ann := new(big.Int).Mul(amp, new(big.Int).SetUint64(uint64(NCoins)))
	s := new(big.Int).Add(sourceAmount, destinationAmount)
	dPrev := new(big.Int).SetUint64(0)
	d := s
	for i := 0; i < 20; i++ {
		dPrev = d
		dp := d
		dp = new(big.Int).Div(
			new(big.Int).Mul(dp, d),
			new(big.Int).Mul(sourceAmount, new(big.Int).SetUint64(uint64(NCoins))))
		dp = new(big.Int).Div(
			new(big.Int).Mul(dp, d),
			new(big.Int).Mul(destinationAmount, new(big.Int).SetUint64(uint64(NCoins))))
		dNumerator := new(big.Int).Mul(
			d,
			new(big.Int).Add(
				new(big.Int).Mul(ann, s),
				new(big.Int).Mul(dp, new(big.Int).SetUint64(uint64(NCoins)))))
		dDenominator := new(big.Int).Add(
			new(big.Int).Mul(
				d,
				new(big.Int).Sub(ann, new(big.Int).SetUint64(1))),
			new(big.Int).Mul(
				dp,
				new(big.Int).Add(new(big.Int).SetUint64(uint64(NCoins)), new(big.Int).SetUint64(1))))
		d = new(big.Int).Div(dNumerator, dDenominator)
		xx := new(big.Int).Abs(new(big.Int).Sub(d, dPrev))
		if xx.Cmp(new(big.Int).SetUint64(1)) < 0 {
			return d
		}
	}
	return d
}

func (m *Model) computeY(amp *big.Int, x *big.Int, d *big.Int) *big.Int {
	ann := new(big.Int).Mul(amp, new(big.Int).SetUint64(uint64(NCoins)))
	b := new(big.Int).Sub(
		new(big.Int).Add(x,
			new(big.Int).Div(d, ann)),
		d)
	c := new(big.Int).Div(
		new(big.Int).Mul(new(big.Int).Mul(d, d), d),
		new(big.Int).Mul(
			new(big.Int).SetUint64(uint64(NCoins)),
			new(big.Int).Mul(
				new(big.Int).SetUint64(uint64(NCoins)),
				new(big.Int).Mul(x, ann))))
	yPrev := new(big.Int).SetUint64(0)
	y := d
	for i := 0; i < Iterations; i++ {
		yPrev = y
		y = new(big.Int).Div(
			new(big.Int).Add(new(big.Int).Mul(y, y), c),
			new(big.Int).Add(new(big.Int).Mul(new(big.Int).SetUint64(uint64(NCoins)), y), b))
		xx := new(big.Int).Abs(new(big.Int).Sub(y, yPrev))
		if xx.Cmp(new(big.Int).SetUint64(1)) < 0 {
			return y
		}
	}
	return y
}

func (m *Model) stableSwap(token solana.PublicKey, amount uint64) (*program.SwapResult, error) {
	sourceToken := m.StableSwap.TokenA
	sourceAmount := new(big.Int).SetUint64(m.SwapA.Amount)
	sourceSlot := m.SwapA.Height
	destinationToken := m.StableSwap.TokenB
	destinationAmount := new(big.Int).SetUint64(m.SwapB.Amount)
	destinationSlot := m.SwapB.Height
	if token == m.StableSwap.TokenB {
		sourceToken = m.StableSwap.TokenB
		sourceAmount = new(big.Int).SetUint64(m.SwapB.Amount)
		sourceSlot = m.SwapB.Height
		destinationToken = m.StableSwap.TokenA
		destinationAmount = new(big.Int).SetUint64(m.SwapA.Amount)
		destinationSlot = m.SwapA.Height
	}
	inAmount := new(big.Int).SetUint64(amount)
	amp := new(big.Int).SetUint64(m.StableSwap.InitialAmpFactor)
	d := m.computeD(amp, sourceAmount, destinationAmount)
	y := m.computeY(amp, new(big.Int).Add(sourceAmount, inAmount), d)
	amountBeforeFees := new(big.Int).Sub(destinationAmount, y)
	fee := muldivimbalanced(amountBeforeFees, m.StableSwap.Fees.TradeFeeNumerator, m.StableSwap.Fees.TradeFeeDenominator)
	/*
		tradeFraction := new(big.Int).SetUint64(m.StableSwap.Fees.TradeFeeNumerator)
		fee := new(big.Int).Mul(tradeFraction, amountBeforeFees)
		adminFraction := new(big.Int).SetUint64(m.StableSwap.Fees.AdminTradeFeeNumerator)
		adminFee := new(big.Int).Mul(adminFraction, fee)
		lpFee := new(big.Int).Sub(fee, adminFee)
	*/

	newSourceAmount := new(big.Int).Add(sourceAmount, inAmount)
	newDestinationAmount := new(big.Int).Sub(destinationAmount, amountBeforeFees)
	swappedSourceAmount := new(big.Int).Sub(newSourceAmount, sourceAmount)
	swappedDestinationAmount := new(big.Int).Sub(amountBeforeFees, fee)
	return &program.SwapResult{
		TokenIn:    sourceToken,
		AmountIn:   swappedSourceAmount.Uint64(),
		SlotIn:     sourceSlot,
		TokenOut:   destinationToken,
		AmountOut:  swappedDestinationAmount.Uint64(),
		SlotOut:    destinationSlot,
		NewSwapSrc: newSourceAmount.Uint64(),
		NewSwapDst: newDestinationAmount.Uint64(),
	}, nil
}

func muldivimbalanced(amount *big.Int, numerator uint64, denominator uint64) *big.Int {
	if amount.Uint64() == 0 {
		return new(big.Int).SetUint64(0)
	}
	if numerator == 0 {
		return new(big.Int).SetUint64(0)
	}
	fee := new(big.Int).Div(
		new(big.Int).Mul(
			amount,
			new(big.Int).SetUint64(numerator),
		),
		new(big.Int).SetUint64(denominator),
	)
	if fee.Cmp(new(big.Int).SetUint64(0)) == 0 {
		return new(big.Int).SetUint64(1)
	} else {
		return fee
	}
}

func (m *Model) getSwapAccounts(token solana.PublicKey) (solana.PublicKey, solana.PublicKey, solana.PublicKey, solana.PublicKey, solana.PublicKey, error) {
	if token != m.StableSwap.TokenA && token != m.StableSwap.TokenB {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("token is not the swap token pair - (%s %s)", m.StableSwap.Key, token)
	}
	// src and dst
	mintSrc := m.StableSwap.TokenA
	tokenSrc := m.StableSwap.SwapA
	mintDst := m.StableSwap.TokenB
	tokenDst := m.StableSwap.SwapB
	feeAdmin := m.StableSwap.AdminFeeKeyB
	if token == mintDst {
		mintSrc = m.StableSwap.TokenB
		tokenSrc = m.StableSwap.SwapB
		mintDst = m.StableSwap.TokenA
		tokenDst = m.StableSwap.SwapA
		feeAdmin = m.StableSwap.AdminFeeKeyA
	}
	return mintSrc, tokenSrc, mintDst, tokenDst, feeAdmin, nil
}
