package raydium

import (
	"fmt"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/gagliardetto/solana-go"
)

type Model struct {
	ProgramId solana.PublicKey
	AmmInfo   *KeyedAmmInfo
	SwapA     *spltoken.KeyedUser
	SwapB     *spltoken.KeyedUser
	States    map[string]interface{}
}

func (m *Model) Program() solana.PublicKey {
	return m.ProgramId
}

func (m *Model) Id() solana.PublicKey {
	return m.AmmInfo.Key
}

func (m *Model) TokenPair() []solana.PublicKey {
	return []solana.PublicKey{m.AmmInfo.CoinMint, m.AmmInfo.PcMint}
}

func (m *Model) PoolPair() []solana.PublicKey {
	return []solana.PublicKey{m.AmmInfo.TokenCoin, m.AmmInfo.TokenPc}
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
	return nil, nil
}

func (m *Model) getSwapAccounts(token solana.PublicKey) (solana.PublicKey, solana.PublicKey, solana.PublicKey, solana.PublicKey, error) {
	if token != m.AmmInfo.CoinMint && token != m.AmmInfo.PcMint {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("token is not the swap token pair - (%s %s)", m.AmmInfo.Key, token)
	}
	// src and dst
	mintSrc := m.AmmInfo.CoinMint
	tokenSrc := m.AmmInfo.TokenCoin
	mintDst := m.AmmInfo.PcMint
	tokenDst := m.AmmInfo.TokenPc
	if token == mintDst {
		mintSrc = m.AmmInfo.PcMint
		tokenSrc = m.AmmInfo.TokenPc
		mintDst = m.AmmInfo.CoinMint
		tokenDst = m.AmmInfo.TokenCoin
	}
	return mintSrc, tokenSrc, mintDst, tokenDst, nil
}
