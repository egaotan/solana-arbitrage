package backend

import (
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/store"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"log"
	"time"
)

const (
	ExecutorSize = 8
	Try          = 0
)

type Command struct {
	Id       uint64
	Trx      *solana.Transaction
	Simulate bool
	Accounts []solana.PublicKey
}

func (backend *Backend) Executor(id int, commandChan chan *Command, client *rpc.Client) {
	i := id / 1000
	j := id % 1000
	logger := utils.NewLog(config.LogPath, fmt.Sprintf("%s_%d_%d", config.ExecutorLog, i, j))
	defer func() {
		backend.logger.Printf("executor %d exit", id)
		logger.Printf("executor %d exit", id)
	}()
	logger.Printf("executor %d start", id)
	for {
		select {
		case command := <-commandChan:
			backend.Execute(command, client, id, logger)
		case <-backend.ctx.Done():
			return
		}
	}
}

func (backend *Backend) Execute(command *Command, client *rpc.Client, id int, logger *log.Logger) {
	defer func() {
		logger.Printf("end execute command: %d", command.Id)
	}()
	logger.Printf("start execute command: %d, time: %s", command.Id,
		time.Unix(int64(command.Id)/1000000, int64(command.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"))
	if command.Simulate {
		return
	}
	executedArbitrage := &store.ExecutedArbitrage{
		Id:           command.Id,
		ExecuteId:    id,
		SendTime:     0,
		ResponseTime: 0,
		Signature:    "",
	}
	defer func() {
		if backend.store != nil && id / 1000 == 1 {
			backend.store.StoreExecutedArbitrage(executedArbitrage)
		}
	}()
	trx := command.Trx
	send := func() solana.Signature {
		signature, err := client.SendTransactionWithOpts(backend.ctx, trx, true, rpc.CommitmentFinalized)
		if err != nil {
			logger.Printf("SendTransactionWithOpts err: %s", err.Error())
		}
		return signature
	}
	check := func(signature solana.Signature) error {
		if signature.IsZero() {
			return fmt.Errorf("no transaction hash")
		}
		_, err := client.GetTransaction(backend.ctx, signature, &rpc.GetTransactionOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: rpc.CommitmentConfirmed,
		})
		if err != nil {
			return err
		}
		tt := time.Now().UnixNano() / time.Microsecond.Nanoseconds()
		executedArbitrage.FinishTime = uint64(tt)
		return nil
	}
	//
	tt := time.Now().UnixNano() / time.Microsecond.Nanoseconds()
	if uint64(tt)-command.Id > 20000000 {
		logger.Printf("the arbitrage command is too old")
		return
	}
	executedArbitrage.SendTime = uint64(tt)
	logger.Printf("trying %d......", 1)
	signature := send()
	tt2 := time.Now().UnixNano() / time.Microsecond.Nanoseconds()
	executedArbitrage.ResponseTime = uint64(tt2)
	executedArbitrage.Signature = signature.String()
	executedArbitrage.ExecuteCounter = 1
	counter := 1
	finished := false
	for !finished && counter < Try {
		counter++
		err := check(signature)
		if err == nil {
			finished = true
			logger.Printf("transaction success")
			break
		}
		logger.Printf("check err: %s", err.Error())
		tt = time.Now().UnixNano() / time.Microsecond.Nanoseconds()
		if uint64(tt)-command.Id > 20000000 {
			logger.Printf("the arbitrage command is too old")
			return
		}
		logger.Printf("trying %d......", counter)
		signature = send()
		if !signature.IsZero() {
			executedArbitrage.Signature = signature.String()
		}
		time.Sleep(time.Millisecond * 100)
	}
	executedArbitrage.ExecuteCounter = counter
	counter = 0
	for !finished && counter < Try {
		counter++
		err := check(signature)
		if err == nil {
			finished = true
			logger.Printf("transaction success")
			break
		}
		logger.Printf("check err: %s", err.Error())
		time.Sleep(time.Millisecond * 500)
	}
	trxJson, _ := json.MarshalIndent(trx, "", "    ")
	logger.Printf("transaction: %s", trxJson)
}

func (backend *Backend) startExecutor() {
	for i := 0; i < len(backend.commandChans); i++ {
		for j := 0; j < ExecutorSize; j++ {
			id := (i+1)*1000 + (j + 1)
			go backend.Executor(id, backend.commandChans[i], backend.clients[i])
		}
	}
}

func (backend *Backend) Commit(level int, id uint64, ins []solana.Instruction, simulate bool, accounts []solana.PublicKey) {
	// build transaction
	builder := solana.NewTransactionBuilder()
	for _, i := range ins {
		builder.AddInstruction(i)
	}
	builder.SetRecentBlockHash(backend.GetRecentBlockHash(level))
	builder.SetFeePayer(backend.player)
	trx, err := builder.Build()
	if err != nil {
		backend.logger.Printf("build err: %s", err.Error())
		return
	}
	trx.Sign(backend.getWallet)
	//
	command := &Command{
		Id:       id,
		Trx:      trx,
		Simulate: simulate,
		Accounts: accounts,
	}
	//
	txData, err := trx.MarshalBinary()
	if err != nil {
		backend.logger.Printf("trx.MarshalBinary err: %s", err.Error())
	}

	if backend.transactionSend == 2 || backend.transactionSend == 3 {
		backend.tpu.CommitTransaction(txData)
	}
	if backend.transactionSend == 1 || backend.transactionSend == 3 {
		for i := 0; i < len(backend.commandChans); i++ {
			backend.commandChans[i] <- command
		}
	}
}
