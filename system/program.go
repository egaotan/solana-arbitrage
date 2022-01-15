package system

import (
	"context"
	"encoding/binary"
	"github.com/egaotan/solana-arbitrage/backend"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/gagliardetto/solana-go"
	"log"
)

type Program struct {
	backend *backend.Backend
	log     *log.Logger
	cb      program.Callback
	ctx     context.Context
	id      solana.PublicKey
}

func NewProgram(context context.Context, backend *backend.Backend) *Program {
	p := &Program{
		ctx:     context,
		backend: backend,
		log:     log.Default(),
		id:      program.System,
	}
	return p
}

func (p *Program) Name() string {
	return "system"
}

func (p *Program) Id() solana.PublicKey {
	return p.id
}

func (p *Program) Start() error {
	p.log.Printf("start system program: %s......", p.Id())
	return nil
}

func (p *Program) Stop() error {
	p.log.Printf("stop system program......")
	return nil
}

func (p *Program) InstructionCreateAccount(fromKey solana.PublicKey, newKey solana.PublicKey, space uint64, ownerId solana.PublicKey) (solana.Instruction, error) {
	lamports, err := p.backend.GetMinimumBalanceForRentExemption(space)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 52)
	binary.LittleEndian.PutUint32(data[0:], 0)
	binary.LittleEndian.PutUint64(data[4:], lamports)
	binary.LittleEndian.PutUint64(data[12:], space)
	copy(data[20:], ownerId.Bytes())
	instruction := &program.Instruction{
		IsAccounts: []*solana.AccountMeta{
			{PublicKey: fromKey, IsSigner: true, IsWritable: true},
			{PublicKey: newKey, IsSigner: true, IsWritable: true},
		},
		IsData:      data,
		IsProgramID: p.id,
	}
	return instruction, nil
}
