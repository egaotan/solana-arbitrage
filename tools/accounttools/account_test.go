package accounttools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/serumv2"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/egaotan/solana-arbitrage/system"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"os"
	"testing"
	"time"
)

var (
	//Owner = solana.MustPublicKeyFromBase58("FrJZ4DP12Tg7r8rpjMqknkpCbJihqbEhfEBBQkpFimaS")
	Player = solana.MustPublicKeyFromBase58("FrJZ4DP12Tg7r8rpjMqknkpCbJihqbEhfEBBQkpFimaS")
)

var (
	//OwnerKey = solana.MustPrivateKeyFromBase58("")
	PlayerKey = ""
)

func CreateSplTokenAccount(mint solana.PublicKey) solana.PublicKey {
	ctx := context.Background()
	backend := backend.NewBackend(ctx, []*config.Node{{rpc.MainNetBeta_RPC, rpc.MainNetBeta_WS, []string{}, true}}, true, []*config.Node{{rpc.MainNetBeta_RPC, rpc.MainNetBeta_WS, []string{}, true}})
	backend.ImportWallet(PlayerKey)
	backend.SetPlayer(Player)
	env := env.NewEnv(ctx)
	systemProgram := system.NewProgram(ctx, backend)
	tokenProgram := spltoken.NewProgram(ctx, backend, nil)
	backend.Start()
	env.Start()
	systemProgram.Start()
	tokenProgram.Start()
	backend.SubscribeSlot(nil)
	time.Sleep(time.Second * 5)

	// create a new private key
	wallet := solana.NewWallet()
	newTokenAccount := wallet.PublicKey()
	fmt.Printf("spl token account: %s\n", newTokenAccount.String())
	fmt.Printf("spl token account pri: %s\n", wallet.PrivateKey.String())
	backend.ImportWallet(wallet.PrivateKey.String())

	backend.SaveWallet()
	// create account
	in1, err := systemProgram.InstructionCreateAccount(Player, newTokenAccount, uint64(spltoken.TokenLayoutSize), program.Token)
	if err != nil {
		panic(err)
	}
	// init
	in2, err := tokenProgram.InstructionInitUser(newTokenAccount, mint, Player)
	if err != nil {
		panic(err)
	}
	backend.Commit(0, uint64(time.Now().UnixNano()/1000), []solana.Instruction{in1, in2}, false, nil)
	return newTokenAccount
}

func Test_CreateSplTokenAccountSingle(t *testing.T) {
	mint := solana.MustPublicKeyFromBase58("Basis9oJw9j8cw53oMV7iqsgo6ihi9ALw4QR31rcjUJa")
	CreateSplTokenAccount(mint)
	time.Sleep(time.Second * 10)
}

func Test_CreateSplTokenAccount(t *testing.T) {
	userJson, err := os.ReadFile("./config/users.json")
	if err != nil {
		panic(err)
	}
	users := make(map[solana.PublicKey]string)
	err = json.Unmarshal(userJson, &users)
	if err != nil {
		panic(err)
	}
	for k, v := range users {
		if v != "" {
			continue
		}
		v1 := CreateSplTokenAccount(k)
		users[k] = v1.String()
		break
	}
}

func CreateMarketOpenOrders(market solana.PublicKey) solana.PublicKey {
	ctx := context.Background()
	backend := backend.NewBackend(ctx, []*config.Node{{rpc.MainNetBeta_RPC, rpc.MainNetBeta_WS, []string{}, true}}, true, []*config.Node{{rpc.MainNetBeta_RPC, rpc.MainNetBeta_WS, []string{}, true}})
	backend.ImportWallet(PlayerKey)
	backend.SetPlayer(Player)
	env := env.NewEnv(ctx)
	systemProgram := system.NewProgram(ctx, backend)
	tokenProgram := spltoken.NewProgram(ctx, backend, nil)
	serumProgram := serumv2.NewProgram(program.SerumV22, ctx, config.MarketFromConfig, env, backend, tokenProgram, systemProgram, nil)
	backend.Start()
	env.Start()
	systemProgram.Start()
	tokenProgram.Start()
	serumProgram.Start()
	backend.SubscribeSlot(nil)
	time.Sleep(time.Second * 5)

	// create a private key
	wallet := solana.NewWallet()
	openorder := wallet.PublicKey()
	fmt.Printf("new open order: %s\n", openorder.String())
	fmt.Printf("new open order pri: %s\n", wallet.PrivateKey.String())
	backend.ImportWallet(wallet.PrivateKey.String())
	//
	backend.SaveWallet()

	// create account
	in1, err := systemProgram.InstructionCreateAccount(Player, openorder, uint64(serumv2.OpenOrdersLayoutSize), program.SerumV22)
	if err != nil {
		panic(err)
	}
	in2, err := serumProgram.InstructionInitOpenOrders(market, openorder, Player, false)
	if err != nil {
		panic(err)
	}
	backend.Commit(0, uint64(time.Now().UnixNano()/1000), []solana.Instruction{in1, in2}, false, nil)
	return openorder
}

func Test_CreateOpenOrders4UserSingle(t *testing.T) {
	market := solana.MustPublicKeyFromBase58("HCWgghHfDefcGZsPsLAdMP3NigJwBrptZnXemeQchZ69")
	CreateMarketOpenOrders(market)
	time.Sleep(time.Second * 10)
}

func Test_CreateOpenOrders4User(t *testing.T) {
	userJson, err := os.ReadFile("./../config/markets_openorder.json")
	if err != nil {
		panic(err)
	}
	users := make(map[solana.PublicKey]string)
	err = json.Unmarshal(userJson, &users)
	if err != nil {
		panic(err)
	}
	for k, v := range users {
		if v != "" {
			continue
		}
		v1 := CreateMarketOpenOrders(k)
		users[k] = v1.String()
		break
	}
}
