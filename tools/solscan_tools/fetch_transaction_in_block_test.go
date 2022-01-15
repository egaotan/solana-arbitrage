package solscan_tools

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"testing"
)

func GetBlocks(startSlot uint64, endSlot uint64) []uint64 {
	client := rpc.New(SolNode)
	response, err := client.GetBlocks(context.Background(), startSlot, &endSlot, rpc.CommitmentFinalized)
	if err != nil {
		panic(err)
	}
	return *response
}

func GetBlock(slot uint64) *rpc.GetBlockResult {
	client := rpc.New(SolNode)
	response, err := client.GetBlock(context.Background(), slot)
	if err != nil {
		return nil
	}
	return response
}

func GetTransactionInBlock(address solana.PublicKey, startSlot uint64, endSlot uint64) {
	i := startSlot
	hashs := make(map[solana.Signature]uint64)
	for i <= endSlot {
		block := GetBlock(i)
		if block == nil {
			i++
			continue
		}
		for _, tx := range block.Transactions {
			if tx.Meta.Err != nil {
				continue
			}
			for _, account := range tx.Transaction.Message.AccountKeys {
				if account == address {
					hashs[tx.Transaction.Signatures[0]] = i
				}
			}
		}
		i++
	}
	for hash, slot := range hashs {
		fmt.Printf("hash: %s, slot: %d\n", hash.String(), slot)
	}
}

func TestGetTransactionInBlock(t *testing.T) {
	address := solana.MustPublicKeyFromBase58("Di66GTLsV64JgCCYGVcY21RZ173BHkjJVgPyezNN7P1K")
	startSlot := uint64(114035006 - 5)
	endSlot := uint64(113909120 + 5)
	GetTransactionInBlock(address, startSlot, endSlot)
}
