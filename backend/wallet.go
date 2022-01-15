package backend

import (
	"fmt"
	"github.com/gagliardetto/solana-go"
	"os"
)

type Wallet struct {
	pubkey solana.PublicKey
	prikey solana.PrivateKey
}

func (backend *Backend) ImportWallet(priKey string) {
	pri := solana.MustPrivateKeyFromBase58(priKey)
	pub := pri.PublicKey()
	backend.wallets = append(backend.wallets, &Wallet{
		pubkey: pub,
		prikey: pri,
	})
}

func (backend *Backend) getWallet(key solana.PublicKey) *solana.PrivateKey {
	for _, wallet := range backend.wallets {
		if wallet.pubkey == key {
			return &wallet.prikey
		}
	}
	return &solana.PrivateKey{}
}

func (backend *Backend) SetPlayer(player solana.PublicKey) {
	backend.player = player
}

func (backend *Backend) SaveWallet() {
	for _, wallet := range backend.wallets {
		file := fmt.Sprintf("%s", wallet.pubkey.String())
		err := os.WriteFile(file, []byte(wallet.prikey.String()), 0644)
		if err != nil {
			panic(err)
		}
	}
}
