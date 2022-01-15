package tokenswap

import (
	"context"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/egaotan/solana-arbitrage/system"
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
	program := NewProgram(ctx, backend, program.TokenSwap, splTokenProgram, systemProgram, cb)
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
