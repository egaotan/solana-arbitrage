package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

/*
AccountNotFound : account balance is insufficient
*/

func (backend *Backend) Simulate(is []solana.Instruction, pubkeys []solana.PublicKey) ([]*rpc.Account, []byte, []byte, uint64, error) {
	ctx := context.Background()
	getRecentBlockHashResult, err := backend.rpcClient.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, []byte{}, []byte{}, 0, err
	}
	blockHash := getRecentBlockHashResult.Value.Blockhash

	builder := solana.NewTransactionBuilder()
	for _, i := range is {
		builder.AddInstruction(i)
	}

	builder.SetRecentBlockHash(blockHash)
	builder.SetFeePayer(backend.player)
	trx, err := builder.Build()
	if err != nil {
		return nil, []byte{}, []byte{}, 0, err
	}
	trx.Sign(backend.getWallet)

	trxJson, _ := json.MarshalIndent(trx, "", "    ")

	/*
		txData, err := trx.MarshalBinary()
		if err != nil {
			return nil, err
		}
		base64Data := base64.StdEncoding.EncodeToString(txData)
		fmt.Printf("tx data: %s\n", base64Data)
	*/

	response, err := backend.rpcClient.SimulateTransactionWithOpts(ctx, trx, &rpc.SimulateTransactionOpts{
		SigVerify:  false,
		Commitment: rpc.CommitmentFinalized,
		Accounts: &rpc.SimulateTransactionAccountsOpts{
			Encoding:  solana.EncodingBase64,
			Addresses: pubkeys,
		},
	})
	if err != nil {
		return nil, trxJson, []byte{}, 0, err
	}
	simulateTransactionResponse := response.Value
	if simulateTransactionResponse.Logs == nil {
		return nil, trxJson, []byte{}, 0, fmt.Errorf("log is nil, simulate failed before the transaction was able to executed, such as signature verification failure or invalid blockhash")
	}
	logsJson, _ := json.MarshalIndent(simulateTransactionResponse.Logs, "", "    ")

	if simulateTransactionResponse.Err != nil {
		return nil, trxJson, logsJson, 0, fmt.Errorf("%v", simulateTransactionResponse.Err)
	}
	unitConsumed := uint64(0)
	if simulateTransactionResponse.UnitsConsumed != nil {
		unitConsumed = *simulateTransactionResponse.UnitsConsumed
	}
	return simulateTransactionResponse.Accounts, trxJson, logsJson, unitConsumed, nil
}
