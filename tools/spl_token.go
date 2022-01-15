package tools

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

var (
	TokenLayoutSize = 165
	MintLayoutSize  = 82
)

type TokenLayout struct {
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

type MintLayout struct {
	MintAuthorityOption   [4]byte
	MintAuthority         solana.PublicKey
	Supply                uint64
	Decimals              byte
	IsInitialized         uint8
	FreezeAuthorityOption [4]byte
	FreezeAuthority       solana.PublicKey
}

func decodeAccount(account *rpc.Account) (*TokenLayout, error) {
	if account.Owner != solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA") {
		return nil, fmt.Errorf("account is not spl token program account")
	}
	tokenData := account.Data.GetBinary()
	if len(tokenData) != TokenLayoutSize {
		return nil, fmt.Errorf("account data size is not valid")
	}
	token := TokenLayout{}
	buf := bytes.NewReader(tokenData)
	err := binary.Read(buf, binary.LittleEndian, &token)
	if err != nil {
		return nil, err
	}
	return &token, nil
}
