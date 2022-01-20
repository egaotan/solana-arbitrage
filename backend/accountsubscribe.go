package backend

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"syscall"
	"time"
)

type AccountCallback interface {
	OnAccountUpdate(account *Account) error
}

type AccountSubscribe struct {
	cb AccountCallback
	t int
}

func (backend *Backend) SubscribeAccount(pubkey solana.PublicKey, cb AccountCallback, t int) error {
	backend.subscribes[pubkey] = &AccountSubscribe{
		cb: cb,
		t:  t,
	}
	return nil
}

func (backend *Backend) StartSubscribe() error {
	/*
	for pubkey, as := range backend.subscribes {
		for _, wsClient := range backend.wsClients {
			sub, err := wsClient.AccountSubscribeWithOpts(pubkey, rpc.CommitmentProcessed, solana.EncodingBase64)
			if err != nil {
				return err
			}
			backend.accountSubs = append(backend.accountSubs, sub)
			backend.wg.Add(1)
			go backend.RecvAccount(pubkey, as.cb, sub)
		}
	}
	*/
	spltokenAccounts := make([]solana.PublicKey, 0)
	var spltokenCb AccountCallback
	serumAccounts := make([]solana.PublicKey, 0)
	var serumCb AccountCallback
	for pubkey, as := range backend.subscribes {
		if as.t == 0 {
			spltokenAccounts = append(spltokenAccounts, pubkey)
			spltokenCb = as.cb
		} else if as.t == 1 {
			serumAccounts = append(serumAccounts, pubkey)
			serumCb = as.cb
		}
	}
	go backend.FetchAccount(spltokenAccounts, spltokenCb, 0)
	go backend.FetchAccount(serumAccounts, serumCb, 1)
	return nil
}

/*
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
*/

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
		data := got
		account := &Account{
			PubKey:  key,
			Account: &data.Value.Account,
			Height:  data.Context.Slot,
		}
		backend.logger.Printf("receive account, slot %d, %s", account.Height, account.PubKey.String())
		if cb != nil {
			cb.OnAccountUpdate(account)
		}
	}
}

func (backend *Backend) FetchAccount(keys []solana.PublicKey, cb AccountCallback, t int) {
	defer backend.wg.Done()
	ticker := time.NewTicker(time.Millisecond * 100)
	currentSolt := uint64(0)
	for {
		select {
		case <- ticker.C:
			backend.logger.Printf("try to fetch account (%d)", t)
			getMultipleAccountsResult, err := backend.rpcClient.GetMultipleAccountsWithOpts(
				backend.ctx, keys, &rpc.GetMultipleAccountsOpts{
					Encoding:   solana.EncodingBase64,
					Commitment: rpc.CommitmentProcessed,
				})
			if err != nil {
				continue
			}
			backend.logger.Printf("fetch account (%d, %d)", getMultipleAccountsResult.Context.Slot, t)
			if getMultipleAccountsResult.Context.Slot <= currentSolt {
				continue
			}
			backend.logger.Printf("receive account (%d, %d)", getMultipleAccountsResult.Context.Slot, t)
			currentSolt = getMultipleAccountsResult.Context.Slot
			for i, updateAccount := range getMultipleAccountsResult.Value {
				account := &Account{
					PubKey:  keys[i],
					Account: updateAccount,
					Height:  getMultipleAccountsResult.Context.Slot,
				}
				if cb != nil {
					cb.OnAccountUpdate(account)
				}
			}
		case <-backend.ctx.Done():
			backend.logger.Printf("fetch account exit")
			return
		}
	}
}