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
	"time"
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
	backend := backend.NewBackend(ctx,
		[]*config.Node{{rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS, nil, true}},
		true,
		[]*config.Node{{rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS, nil, true}},
		[]string{"https://free.rpcpool.com"}, []string{"https://free.rpcpool.com"}, 2,
	)
	splTokenProgram := spltoken.NewProgram(ctx, backend, nil)
	systemProgram := system.NewProgram(ctx, backend)
	env := env.NewEnv(ctx)
	cb := &ProgramCallback{}
	programId := solana.MustPublicKeyFromBase58("9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin")
	program := NewProgram(programId, ctx, 1, env, backend, splTokenProgram, systemProgram, cb)
	//
	backend.Start()
	backend.SetPlayer(solana.MustPublicKeyFromBase58("5t695wLY2FPfx2MpGscG3YM3yhGVNPKCQC5d2qmtMibd"))
	backend.ImportWallet("2oNrHdcEgWCnbfraEkD1kV4Ytv3HBgJBaQBbSDng1d24hnCrNkTx7K9VC3ehZks8Kk5e4qpt5x1Ea6n9vQhMjm3y")
	backend.SubscribeSlot(nil)
	env.Start()
	systemProgram.Start()
	splTokenProgram.Start()
	err := program.Start()
	if err != nil {
		panic(err)
	}
	time.Sleep(time.Second * 15)
	market := solana.MustPublicKeyFromBase58("HWHvQhFmJB3NUcu1aihKmrKegfVxBEHzwVX6yZCKEsi1")
	state, err := program.RetrieveState(market)
	if err != nil {
		panic(err)
	}
	fmt.Printf("state: %s\n", state)
	ins, err := program.InstructionNewOrder(market, solana.MustPublicKeyFromBase58("Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"), 1000000000, false)
	if err != nil {
		panic(err)
	}
	backend.Commit(0, uint64(time.Now().UnixNano()/1000), []solana.Instruction{ins}, false, nil)
	time.Sleep(time.Second * 15)
	return program
}

func TestProgram_Start(t *testing.T) {
	startProgram()
}

func TestProgram_ErrorCode(t *testing.T) {
	code := 0x1000879
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, uint32(code))
	file := data[3]
	line := binary.LittleEndian.Uint16(data[0:2])
	fmt.Printf("file: %d, line: %d\n", file, line)
}
