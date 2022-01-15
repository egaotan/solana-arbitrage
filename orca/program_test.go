package orca

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/egaotan/solana-arbitrage/system"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"testing"
)

type ProgramCallback struct {
}

func (pc *ProgramCallback) OnModelInit(model program.Model) error {
	return nil
}

func startProgram() *Program {
	ctx := context.Background()
	backend := backend.NewBackend(ctx, rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS)
	splTokenProgram := spltoken.NewProgram(ctx, backend)
	systemProgram := system.NewProgram(ctx, backend)
	cb := &ProgramCallback{}
	program := NewProgram(ctx, backend, program.OrcaV2, splTokenProgram, systemProgram, cb)
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
	startProgram()
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
	fmt.Printf("local: %s", result.State)
}

func TestProgram_Simulate(t *testing.T) {
	program1 := startProgram()
	parameter := make(map[string]interface{})
	parameter["market"] = solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U")
	parameter["token"] = solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	parameter["amount"] = uint64(1000000000)
	result, err := program1.Simulate(parameter)
	if err != nil {
		panic(err)
	}
	fmt.Printf("simulate txs: %s", result.Txs)
	fmt.Printf("simulate logs: %s", result.Logs)
	fmt.Printf("simulate state: %s", result.State)
}
