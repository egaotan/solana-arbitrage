package marketanalysis

import (
	"context"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/egaotan/solana-arbitrage/system"
	"github.com/gagliardetto/solana-go/rpc"
)

func parseBlock(result rpc.blo) {
	ctx := context.Background()
	backend := backend.NewBackend(ctx, rpc.MainNetBeta_RPC, rpc.MainNetBeta_WS)
	env := env.NewEnv(ctx)
	systemProgram := system.NewProgram(ctx, backend)
	tokenProgram := spltoken.NewProgram(ctx, backend)
	backend.Start()
	env.Start()
	systemProgram.Start()
	tokenProgram.Start()
}
