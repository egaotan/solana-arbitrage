package backend

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"syscall"
)

type AccountCallback interface {
	OnAccountUpdate(account *Account) error
}

func (backend *Backend) SubscribeAccount(pubkey solana.PublicKey, cb AccountCallback) error {
	for _, wsClient := range backend.wsClients {
		sub, err := wsClient.AccountSubscribeWithOpts(pubkey, rpc.CommitmentProcessed, solana.EncodingBase64)
		if err != nil {
			return err
		}
		backend.accountSubs = append(backend.accountSubs, sub)
		backend.wg.Add(1)
		go backend.RecvAccount(pubkey, cb, sub)
	}
	return nil
}

func (backend *Backend) RecvAccount(key solana.PublicKey, cb AccountCallback, sub *ws.AccountSubscription) {
	defer backend.wg.Done()
	for {
		got, err := sub.Recv()
		if err != nil {
			backend.logger.Printf("RecvAccount err: %v", err)
			syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
			return
		}
		if got == nil {
			backend.logger.Printf("RecvAccount exit")
			return
		}
		backend.logger.Printf("receive account, %d", got.Context.Slot)
		data := got
		account := &Account{
			PubKey:  key,
			Account: &data.Value.Account,
			Height:  data.Context.Slot,
		}
		cb.OnAccountUpdate(account)
	}
}
