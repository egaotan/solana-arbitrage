package server

import "encoding/binary"

var (
	CommandLen = 4096
)

type Command struct {
	Hash string
	Id   uint64
	Tx   []byte
}

func (cmd *Command) Encode() []byte {
	data := make([]byte, 4096)
	index := 0
	data[index] = byte(len(cmd.Hash))
	index += 1
	copy(data[index:index+len(cmd.Hash)], cmd.Hash)
	index += len(cmd.Hash)
	data[index] = byte(len(cmd.Tx))
	index += 1
	copy(data[index:index+len(cmd.Tx)], cmd.Tx)
	index += len(cmd.Tx)
	binary.LittleEndian.PutUint64(data[index:index+8], cmd.Id)
	return data
}

func (cmd *Command) Decode(data []byte) {
	index := 0
	hashLen := int(data[index])
	index += 1
	copy([]byte(cmd.Hash), data[index:index+hashLen])
	index += hashLen
	txLen := int(data[index])
	index += 1
	copy(cmd.Tx, data[index:index+txLen])
	index += txLen
	cmd.Id = binary.LittleEndian.Uint64(data[index : index+8])
}
