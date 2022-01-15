package serumv2

import (
	"context"
	"encoding/binary"
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

type ProgramCallback struct {
}

func (pc *ProgramCallback) OnModelInit(model program.Model) error {
	return nil
}

func (pc *ProgramCallback) OnStateUpdate(slot uint64) error {
	return nil
}

func startProgram() *Program {
	ctx := context.Background()
	backend := backend.NewBackend(ctx, []*config.Node{{rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS, nil, true}}, false, []*config.Node{{rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS, nil, true}})
	splTokenProgram := spltoken.NewProgram(ctx, backend, nil)
	systemProgram := system.NewProgram(ctx, backend)
	env := env.NewEnv(ctx)
	cb := &ProgramCallback{}
	programId := solana.MustPublicKeyFromBase58("9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin")
	program := NewProgram(programId, ctx, 1, env, backend, splTokenProgram, systemProgram, cb)
	//
	backend.Start()
	env.Start()
	systemProgram.Start()
	splTokenProgram.Start()
	err := program.Start()
	if err != nil {
		panic(err)
	}
	market := solana.MustPublicKeyFromBase58("Di66GTLsV64JgCCYGVcY21RZ173BHkjJVgPyezNN7P1K")
	state, err := program.RetrieveState(market)
	if err != nil {
		panic(err)
	}
	fmt.Printf("state: %s\n", state)
	return program
}

func TestProgram_Start(t *testing.T) {
	startProgram()
}


func TestProgram_ErrorCode(t *testing.T) {
	code := 0x100086d
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, uint32(code))
	file := data[3]
	line := binary.LittleEndian.Uint16(data[0:2])
	fmt.Printf("file: %d, line: %d\n", file, line)
}
