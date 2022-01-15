package backend

import (
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	MultipleAccountSliceSize = 100
)

type Account struct {
	PubKey  solana.PublicKey
	Account *rpc.Account
	Height  uint64
}

func (backend *Backend) ProgramAccounts(program solana.PublicKey, dataSizes []uint64) ([]*Account, error) {
	return backend.getProgramAccountsFromChain(program, dataSizes)
}

func (backend *Backend) getProgramAccountsFromChain(program solana.PublicKey, dataSizes []uint64) ([]*Account, error) {
	accounts := make([]*Account, 0)
	filters := make([]rpc.RPCFilter, 0)
	for _, dataSize := range dataSizes {
		filters = append(filters, rpc.RPCFilter{
			//Memcmp:   &rpc.RPCFilterMemcmp{
			//	Offset: 0,
			//	Bytes:  []byte{},
			//},
			DataSize: dataSize,
		})
	}
	getProgramAccountsResult, err := backend.rpcClient.GetProgramAccountsWithOpts(backend.ctx, program,
		&rpc.GetProgramAccountsOpts{
			Encoding: solana.EncodingBase64,
			Filters:  filters,
		})
	if err != nil {
		return nil, err
	}
	for _, account := range getProgramAccountsResult {
		accounts = append(accounts, &Account{
			PubKey:  account.Pubkey,
			Account: account.Account,
			Height:  0,
		})
	}
	return accounts, nil
}

func (backend *Backend) Accounts(pubkeys []solana.PublicKey) ([]*Account, error) {
	return backend.getAccountsFromChain(pubkeys)
}

func (backend *Backend) getAccountsFromChain(pubkeys []solana.PublicKey) ([]*Account, error) {
	accounts := make([]*Account, 0)
	index, end := 0, 0
	for index < len(pubkeys) {
		if end = index + MultipleAccountSliceSize; end > len(pubkeys) {
			end = len(pubkeys)
		}
		getMultipleAccountsRsp, err := backend.rpcClient.GetMultipleAccountsWithOpts(backend.ctx, pubkeys[index:end],
			&rpc.GetMultipleAccountsOpts{Encoding: solana.EncodingBase64})
		if err != nil {
			return nil, err
		}
		if len(getMultipleAccountsRsp.Value) != end-index {
			return nil, fmt.Errorf("get accounts err, some account is missing")
		}
		for i, account := range getMultipleAccountsRsp.Value {
			accounts = append(accounts, &Account{
				PubKey:  pubkeys[index+i],
				Height:  getMultipleAccountsRsp.Context.Slot,
				Account: account,
			})
		}
		index = end
	}
	return accounts, nil
}

func (backend *Backend) Account(pubkey solana.PublicKey) (*Account, error) {
	return backend.getAccountFromChain(pubkey)
}

func (backend *Backend) getAccountFromChain(pubkey solana.PublicKey) (*Account, error) {
	response, err := backend.rpcClient.GetAccountInfo(backend.ctx, pubkey)
	if err != nil {
		return nil, err
	}
	return &Account{
		PubKey:  pubkey,
		Height:  response.Context.Slot,
		Account: response.Value,
	}, nil
}

func (backend *Backend) GetMinimumBalanceForRentExemption(size uint64) (uint64, error) {
	return backend.rpcClient.GetMinimumBalanceForRentExemption(backend.ctx, size, rpc.CommitmentFinalized)
}

func (backend *Backend) HasAccount(pubkey solana.PublicKey) bool {
	getMultipleAccountsRsp, err := backend.rpcClient.GetAccountInfo(backend.ctx, pubkey)
	if err != nil {
		return false
	}
	if getMultipleAccountsRsp.Value == nil {
		return false
	}
	return true
}
