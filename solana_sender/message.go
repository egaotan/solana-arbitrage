package sender

import (
	"encoding/binary"
	"github.com/gagliardetto/solana-go"
)

var (
	CommandLen = 1400
)

type Command struct {
	Hash   solana.Signature
	Id     uint64
	Tx     []byte
	Status int
}

func (cmd *Command) Encode() []byte {
	data := make([]byte, CommandLen)
	index := 0
	binary.LittleEndian.PutUint16(data[index:index+2], uint16(len(cmd.Hash)))
	index += 2
	copy(data[index:index+len(cmd.Hash.String())], cmd.Hash.String())
	index += len(cmd.Hash)
	binary.LittleEndian.PutUint16(data[index:index+2], uint16(len(cmd.Tx)))
	index += 2
	copy(data[index:index+len(cmd.Tx)], cmd.Tx)
	index += len(cmd.Tx)
	binary.LittleEndian.PutUint64(data[index:index+8], cmd.Id)
	return data
}

func (cmd *Command) Decode(data []byte) {
	index := 0
	hashLen := int(binary.LittleEndian.Uint16(data[index : index+2]))
	index += 2
	hash := make([]byte, hashLen)
	copy(hash, data[index:index+hashLen])
	cmd.Hash = solana.MustSignatureFromBase58(string(hash))
	//cmd.Hash = string(hash)
	index += hashLen
	txLen := int(binary.LittleEndian.Uint16(data[index : index+2]))
	index += 2
	tx := make([]byte, txLen)
	copy(tx, data[index:index+txLen])
	cmd.Tx = tx
	index += txLen
	cmd.Id = binary.LittleEndian.Uint64(data[index : index+8])
}
