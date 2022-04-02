package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/backend/tpu"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/store"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"log"
	"net/http"
	"time"
)

const (
	ExecutorSize = 8
	Try          = 0
	Test         = false
)

type Callback interface {
	OnCommandExecuted(account []*rpc.Account) error
}

type Command struct {
	Id       uint64
	Trx      *solana.Transaction
	Simulate bool
	Accounts []solana.PublicKey
	Callback Callback
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
		if backend.store != nil && id/1000 == 1 {
			backend.store.StoreExecutedArbitrage(executedArbitrage)
		}
	}()
	trx := command.Trx
	send := func() solana.Signature {
		if !Test {
			signature, err := client.SendTransactionWithOpts(backend.ctx, trx, true, rpc.CommitmentFinalized)
			if err != nil {
				logger.Printf("SendTransactionWithOpts err: %s", err.Error())
			}
			return signature
		}
		if Test {
			response, err := backend.rpcClient.SimulateTransactionWithOpts(backend.ctx, trx, &rpc.SimulateTransactionOpts{
				SigVerify:              false,
				Commitment:             rpc.CommitmentFinalized,
				ReplaceRecentBlockhash: true,
				Accounts:               &rpc.SimulateTransactionAccountsOpts{solana.EncodingBase64, command.Accounts},
			})
			if err != nil {
				logger.Printf("SimulateTransactionWithOpts err: %s", err.Error())
				return solana.Signature{}
			}
			simulateTransactionResponse := response.Value
			if simulateTransactionResponse.Logs == nil {
				logger.Printf("log is nil, simulate failed before the transaction was able to executed, such as signature verification failure or invalid blockhash")
				return solana.Signature{}
			}
			logsJson, _ := json.MarshalIndent(simulateTransactionResponse.Logs, "", "    ")
			logger.Printf("logs: %s", string(logsJson))
			if simulateTransactionResponse.Err != nil {
				logger.Printf("SimulateTransactionWithOpts err: %s", simulateTransactionResponse.Err)
				return solana.Signature{}
			}
			if command.Callback != nil {
				command.Callback.OnCommandExecuted(response.Value.Accounts)
			}
			return solana.Signature{}
		}
		return solana.Signature{}
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

func (backend *Backend) Commit(level int, id uint64, ins []solana.Instruction, simulate bool, accounts []solana.PublicKey, callback Callback) {
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

	if Test {
		txJson, _ := json.MarshalIndent(trx, "", "    ")
		backend.logger.Printf("%s", txJson)
	}

	trx.Sign(backend.getWallet)

	backend.txLogger.Printf("%s;%d;%s", trx.Signatures[0].String(), id,
		time.Unix(int64(id)/1000000, int64(id)%1000000*1000).Format("2006-01-02 15:04:05.000000"))

	//
	if (backend.transactionSend == 2 || backend.transactionSend == 3) && !Test {
		backend.logger.Printf("send transaction to tpu")
		command := &tpu.Command{
			Id: id,
			Hash: trx.Signatures[0].String(),
		}
		command.Tx, err = trx.MarshalBinary()
		if err != nil {
			backend.logger.Printf("trx.MarshalBinary err: %s", err.Error())
		}
		backend.tpu.CommitTransaction(command)
	}
	if backend.transactionSend == 1 || backend.transactionSend == 3 {
		backend.logger.Printf("sent transaction to rpc")
		command := &Command{
			Id:       id,
			Trx:      trx,
			Simulate: simulate,
			Accounts: accounts,
			Callback: callback,
		}
		for i := 0; i < len(backend.commandChans); i++ {
			backend.commandChans[i] <- command
		}
	}
	if backend.transactionSend == 4 || backend.transactionSend == 3 {
		backend.logger.Printf("send transaction to sender")
		//
		command := &tpu.Command{
			Id: id,
			Hash: trx.Signatures[0].String(),
		}
		command.Tx, err = trx.MarshalBinary()
		if err != nil {
			backend.logger.Printf("trx.MarshalBinary err: %s", err.Error())
			return
		}
		//
		commandJson, err := json.Marshal(command)
		if err != nil {
			backend.logger.Printf("mashal command err: %s", err.Error())
			return
		}
		//
		client := &http.Client{}
		for _, node := range backend.senderNodes {
			req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/sendtransaction", node.Rpc), bytes.NewBuffer(commandJson))
			if err != nil {
				backend.logger.Printf("sender err, NewRequest: %s", err.Error())
				continue
			}
			req.Header.Set("Accepts", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				backend.logger.Printf("sender err, Do: %s", err.Error())
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				backend.logger.Printf("sender err, status code: %d, %s", resp.StatusCode, resp.Status)
				continue
			}
			resp.Body.Close()
		}
		backend.logger.Printf("sender successful!")
	}
}
