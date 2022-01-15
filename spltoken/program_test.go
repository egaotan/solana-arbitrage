package spltoken

import (
	"context"
	"encoding/json"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"os"
	"testing"
)

func TestProgram_RetrieveMints(t *testing.T) {
	ctx := context.Background()
	fetch := backend.NewBackend(ctx, rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS)
	program := NewProgram(ctx, fetch)
	pubkeys := []solana.PublicKey{
		solana.MustPublicKeyFromBase58("Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"),
		solana.MustPublicKeyFromBase58("CtVjQjExaBVsmJ3WYrjDZvPKYesRTZRSmzQiGj9Tqm7d"),
	}
	err := program.RetrieveTokens(pubkeys, false)
	if err != nil {
		panic(err)
	}
	//
	infoJson, _ := json.MarshalIndent(program.tokens, "", "    ")
	file, _ := os.OpenFile("./spltoken_mints.json", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	defer file.Close()
	file.WriteString(string(infoJson))
}

func TestProgram_RetrieveTokens(t *testing.T) {
	ctx := context.Background()
	fetch := backend.NewBackend(ctx, rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS)
	program := NewProgram(ctx, fetch)
	pubkeys := []solana.PublicKey{
		solana.MustPublicKeyFromBase58("Hnct2T3JmcNKNpBwRQcjBW298PqXFqhuBVbyey8fqy5m"),
		solana.MustPublicKeyFromBase58("7ruSLu3QHNqviyN6tCPReCrDy6XTeZzR8chNRZShM7Zr"),
	}
	err := program.RetrieveUsers(pubkeys, false)
	if err != nil {
		panic(err)
	}
	//
	infoJson, _ := json.MarshalIndent(program.users, "", "    ")
	file, _ := os.OpenFile("./spltoken_tokens.json", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	defer file.Close()
	file.WriteString(string(infoJson))
}

func TestProgram_RetrieveAll(t *testing.T) {
	ctx := context.Background()
	fetch := backend.NewBackend(ctx, rpc.MainNetBetaSerum_RPC, rpc.MainNetBetaSerum_WS)
	program := NewProgram(ctx, fetch)
	err := program.RetrieveAll(false)
	if err != nil {
		panic(err)
	}
	//
	{
		infoJson, _ := json.MarshalIndent(program.tokens, "", "    ")
		file, _ := os.OpenFile("./spltoken_mints.json", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
		defer file.Close()
		file.WriteString(string(infoJson))
	}
	{
		infoJson, _ := json.MarshalIndent(program.users, "", "    ")
		file, _ := os.OpenFile("./spltoken_tokens.json", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
		defer file.Close()
		file.WriteString(string(infoJson))
	}
}
