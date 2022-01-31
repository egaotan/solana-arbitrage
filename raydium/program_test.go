package raydium

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/egaotan/solana-arbitrage/system"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"testing"
)

func startProgram() *Program {

	ctx := context.Background()
	backend := backend.NewBackend(ctx,
		[]*config.Node{{rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS, nil, true}},
		false,
		[]*config.Node{{rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS, nil, true}},
		rpc.MainNetBetaSerum_RPC,
		rpc.MainNetBetaSerum_RPC,
		1)
	splTokenProgram := spltoken.NewProgram(ctx, backend, nil)
	systemProgram := system.NewProgram(ctx, backend)
	env := env.NewEnv(ctx)
	//
	program := NewProgram(program.Raydium, ctx, config.MarketFromChain, env, backend, splTokenProgram, systemProgram, nil)
	//
	backend.Start()
	systemProgram.Start()
	splTokenProgram.Start()
	err := program.Start()
	if err != nil {
		panic(err)
	}
	return program
}

func TestProgram_Start(t *testing.T) {
	program := startProgram()
	program.Stop()
}

func TestProgram_Local(t *testing.T) {
	program1 := startProgram()
	parameter := make(map[string]interface{})
	parameter["market"] = solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U")
	parameter["token"] = solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	parameter["amount"] = uint64(1000000000)
	result, err := program1.Local(parameter)
	if err != nil {
		panic(err)
	}
	fmt.Printf("local: %v", result)
}

func TestProgram_Simulate(t *testing.T) {
}
