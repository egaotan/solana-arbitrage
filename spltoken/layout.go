package spltoken

import (
	"github.com/gagliardetto/solana-go"
)

var (
	TokenLayoutSize = 165
	MintLayoutSize  = 82
)

type UserLayout struct {
	Mint                 solana.PublicKey
	Owner                solana.PublicKey
	Amount               uint64
	DelegateOption       [4]byte
	Delegate             solana.PublicKey
	State                uint8
	IsNativeOption       [4]byte
	IsNative             uint64
	DelegatedAmount      uint64
	CloseAuthorityOption [4]byte
	CloseAuthority       solana.PublicKey
}

type TokenLayout struct {
	MintAuthorityOption   [4]byte
	MintAuthority         solana.PublicKey
	Supply                uint64
	Decimals              byte
	IsInitialized         uint8
	FreezeAuthorityOption [4]byte
	FreezeAuthority       solana.PublicKey
}

type KeyedUser struct {
	Key    solana.PublicKey
	Height uint64
	UserLayout
}

type KeyedToken struct {
	Key    solana.PublicKey
	Height uint64
	TokenLayout
}
