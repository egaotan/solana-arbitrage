package program

import "github.com/gagliardetto/solana-go"

type Instruction struct {
	IsAccounts  []*solana.AccountMeta
	IsData      []byte
	IsProgramID solana.PublicKey
}

func (i *Instruction) Accounts() []*solana.AccountMeta {
	return i.IsAccounts
}

func (i *Instruction) ProgramID() solana.PublicKey {
	return i.IsProgramID
}

func (i *Instruction) Data() ([]byte, error) {
	return i.IsData, nil
}
