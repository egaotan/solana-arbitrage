package tokenswap

import (
	"fmt"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/gagliardetto/solana-go"
)

type Model struct {
	ProgramId solana.PublicKey
	TokenSwap *KeyedSwap
	SwapA     *spltoken.KeyedUser
	SwapB     *spltoken.KeyedUser
	States    map[string]interface{}
}

func (m *Model) Program() solana.PublicKey {
	return m.ProgramId
}

func (m *Model) Id() solana.PublicKey {
	return m.TokenSwap.Key
}

func (m *Model) TokenPair() []solana.PublicKey {
	return []solana.PublicKey{m.TokenSwap.TokenA, m.TokenSwap.TokenB}
}

func (m *Model) CurrentSlot() uint64 {
	return m.TokenSwap.Height
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

func (m *Model) PoolPair() []solana.PublicKey {
	return []solana.PublicKey{m.TokenSwap.SwapA, m.TokenSwap.SwapB}
}

func (m *Model) Type() string {
	return program.AMM
}

func (m *Model) Swap(token solana.PublicKey, amount uint64) (*program.SwapResult, error) {
	if m.TokenSwap.SwapCurve.CurveType == ConstantProduct {
		return m.swapConstantProduct(token, amount)
	} else if m.TokenSwap.SwapCurve.CurveType == ConstantPrice {
		return m.swapConstantPrice(token, amount)
	} else if m.TokenSwap.SwapCurve.CurveType == Stable {
		return m.swapStable(token, amount)
	} else {
		return nil, fmt.Errorf("curve type %d is not supported", m.TokenSwap.SwapCurve.CurveType)
	}
}

func (m *Model) getSwapAccounts(token solana.PublicKey) (solana.PublicKey, solana.PublicKey, solana.PublicKey, solana.PublicKey, error) {
	if token != m.TokenSwap.TokenA && token != m.TokenSwap.TokenB {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("token is not the swap token pair - (%s %s)", m.TokenSwap.Key, token)
	}
	// src and dst
	mintSrc := m.TokenSwap.TokenA
	tokenSrc := m.TokenSwap.SwapA
	mintDst := m.TokenSwap.TokenB
	tokenDst := m.TokenSwap.SwapB
	if token == mintDst {
		mintSrc = m.TokenSwap.TokenB
		tokenSrc = m.TokenSwap.SwapB
		mintDst = m.TokenSwap.TokenA
		tokenDst = m.TokenSwap.SwapA
	}
	return mintSrc, tokenSrc, mintDst, tokenDst, nil
}
