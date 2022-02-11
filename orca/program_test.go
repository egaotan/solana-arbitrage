package orca

import (
	"context"
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

type ProgramCallback struct {
}

func (pc *ProgramCallback) OnModelInit(model program.Model) error {
	return nil
}

func startProgram() *Program {
	ctx := context.Background()
	backend := backend.NewBackend(
		ctx,
		[]*config.Node{{rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS, nil, true}},
		false,
		[]*config.Node{{rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS, nil, true}},
		rpc.MainNetBetaSerum_RPC,
		rpc.MainNetBetaSerum_RPC,
		1,
		)
	splTokenProgram := spltoken.NewProgram(ctx, backend, nil)
	systemProgram := system.NewProgram(ctx, backend)
	env := env.NewEnv(ctx)
	programId := solana.MustPublicKeyFromBase58("9W959DqEETiGZocYWCQPaJ6sBmUzgfxXfqGeTEdp3aQP")
	program := NewProgram(programId, ctx, 0, env, backend, splTokenProgram, systemProgram, nil)
	//
	backend.Start()
	env.Start()
	systemProgram.Start()
	splTokenProgram.Start()

	//
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
