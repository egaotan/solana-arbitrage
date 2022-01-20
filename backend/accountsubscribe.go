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
	backend.subscribes[pubkey] = cb
	return nil
}

func (backend *Backend) StartSubscribe() error {
	for pubkey, cb := range backend.subscribes {
		for _, wsClient := range backend.wsClients {
			sub, err := wsClient.AccountSubscribeWithOpts(pubkey, rpc.CommitmentProcessed, solana.EncodingBase64)
			if err != nil {
				return err
			}
			backend.accountSubs = append(backend.accountSubs, sub)
			backend.wg.Add(1)
			go backend.RecvAccount(pubkey, cb, sub)
		}
	}
	go backend.FetchAccount()
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

func (backend *Backend) FetchAccount() {
	defer backend.wg.Done()
	accounts := make([]solana.PublicKey, 0, len(backend.subscribes))
	for pubkey, _ := range backend.subscribes {
		accounts = append(accounts, pubkey)
	}
	//ticker := time.NewTicker(time.Second * 2)
	for {
		select {
		case <-backend.updateAccount:
		L:
			for {
				select {
				case <-backend.updateAccount:
				default:
					break L
				}
			}
			//backend.logger.Printf("current solt: %d, chan size: %d", program.GlobalSlot, len(backend.updateAccount))
			getAccountResult, err := backend.rpcClient.GetAccountInfoWithOpts(
				backend.ctx, accounts[0], &rpc.GetAccountInfoOpts{
					Encoding:   solana.EncodingBase64,
					Commitment: rpc.CommitmentProcessed,
				})
			if err != nil {
				continue
			}
			backend.logger.Printf("fetch account slot: %d", getAccountResult.Context.Slot)
			/*
				account := &Account{
					PubKey:  accounts[0],
					Account: getAccountResult.Value,
					Height:  getAccountResult.Context.Slot,
				}
				cb, ok := backend.subscribes[account.PubKey]
				if !ok || cb == nil {
					continue
				}
				cb.OnAccountUpdate(account)
			*/
			/*
				backend.logger.Printf("current solt: %d", program.GlobalSlot)
				getMultipleAccountsResult, err := backend.rpcClient.GetMultipleAccountsWithOpts(
					backend.ctx, accounts[0:1], &rpc.GetMultipleAccountsOpts{
						Encoding:   solana.EncodingBase64,
						Commitment: rpc.CommitmentProcessed,
					})
				if err != nil {
					continue
				}
				backend.logger.Printf("fetch account slot: %d", getMultipleAccountsResult.Context.Slot)
				for i, updateAccount := range getMultipleAccountsResult.Value {
					account := &Account{
						PubKey:  accounts[i],
						Account: updateAccount,
						Height:  getMultipleAccountsResult.Context.Slot,
					}
					cb, ok := backend.subscribes[account.PubKey]
					if !ok || cb == nil {
						continue
					}
					cb.OnAccountUpdate(account)
				}
			*/
		case <-backend.ctx.Done():
			backend.logger.Printf("fetch account exit")
			return
		}
	}
}