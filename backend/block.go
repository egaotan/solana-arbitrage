package backend

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

type Block struct {
	Hash         solana.Hash
	Time         int64
	Transactions []*Transaction
}

type Transaction struct {
	Signature solana.Signature
	Status    bool
	Accounts  []solana.PublicKey
	Ins       []*Instruction
}

type Instruction struct {
	Accounts          []solana.PublicKey
	Program           solana.PublicKey
	Data              []byte
	DataLen           uint16
	InnerInstructions []*Instruction
}

func (backend *Backend) GetBlock(slot uint64) (*Block, error) {
	response, err := backend.rpcClient.GetBlock(backend.ctx, slot)
	if err != nil {
		return nil, err
	}
	block := &Block{
		Hash: response.Blockhash,
		Time: *response.BlockTime,
	}
	transactions := make([]*Transaction, 0)
	for _, transaction := range response.Transactions {
		transactions = append(transactions, backend.parserTransaction(&transaction))
	}
	block.Transactions = transactions
	return block, nil
}

func (backend *Backend) parserTransaction(transaction *rpc.TransactionWithMeta) *Transaction {
	tx := &Transaction{
		Status:    false,
		Ins:       nil,
		Signature: transaction.Transaction.Signatures[0],
		Accounts:  transaction.Transaction.Message.AccountKeys,
	}
	if transaction.Meta.Err == nil {
		tx.Status = true
	}
	ins := make([]*Instruction, 0)
	accounts := make(map[uint16]solana.PublicKey)
	for i, account := range transaction.Transaction.Message.AccountKeys {
		accounts[uint16(i)] = account
	}
	for _, i := range transaction.Transaction.Message.Instructions {
		in := &Instruction{
			Accounts:          make([]solana.PublicKey, 0),
			InnerInstructions: make([]*Instruction, 0),
		}
		in.Data = i.Data
		in.DataLen = uint16(len(i.Data))
		in.Program = accounts[i.ProgramIDIndex]
		inAccounts := make([]solana.PublicKey, 0)
		for _, index := range i.Accounts {
			inAccounts = append(inAccounts, accounts[index])
		}
		in.Accounts = inAccounts
		ins = append(ins, in)
	}
	for _, i := range transaction.Meta.InnerInstructions {
		in := ins[i.Index]
		interins := make([]*Instruction, 0)
		for _, j := range i.Instructions {
			interin := &Instruction{
				Accounts:          make([]solana.PublicKey, 0),
				InnerInstructions: make([]*Instruction, 0),
			}
			interin.Data = j.Data
			interin.DataLen = uint16(len(j.Data))
			interin.Program = accounts[j.ProgramIDIndex]
			interinAccounts := make([]solana.PublicKey, 0)
			for _, index := range j.Accounts {
				interinAccounts = append(interinAccounts, accounts[index])
			}
			interin.Accounts = interinAccounts
			interins = append(interins, interin)
		}
		in.InnerInstructions = interins
	}
	//
	tx.Ins = ins
	return tx
}
