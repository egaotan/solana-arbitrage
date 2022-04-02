package server

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/solana_sender/server/config"
	"github.com/gin-gonic/gin"
	"log"
	"net"
	"net/http"
	"time"
)

type Sender struct {
	cfg        *config.Config
	ctx        context.Context
	logger     *log.Logger
	tpu        *Proxy
	httpServer *http.Server
}

func NewSender(ctx context.Context, cfg *config.Config) *Sender {
	//
	logger := log.Default()
	// client
	var usedClient *config.Node
	for _, client := range cfg.SlotNodes {
		if client.Usable == true {
			usedClient = client
			break
		}
	}
	if usedClient == nil {
		logger.Printf("there is no usable client")
		return nil
	}
	// tpu clients
	clients := make([]string, 0)
	for _, senderClient := range cfg.LssNodes {
		clients = append(clients, senderClient.Rpc)
	}
	//
	tpu, err := NewProxy(ctx, usedClient.Ws, clients, cfg.Bomb)
	if err != nil {
		logger.Printf("new proxy err: %s", err.Error())
		return nil
	}
	sender := &Sender{
		ctx:    ctx,
		logger: logger,
		tpu:    tpu,
	}
	return sender
}

func (sender *Sender) Service() {
	sender.Start()
	go sender.StartTCP()
	<-sender.ctx.Done()
	sender.StopTCP()
	sender.Stop()
}

func (sender *Sender) Start() {
	sender.tpu.Start()
}

func (sender *Sender) Stop() {

}

func (sender *Sender) StartTCP() {
	sender.logger.Printf("Server Running...")
	//
	listener, err := net.Listen("tcp", "0.0.0.0:8089")
	if err != nil {
		panic(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			sender.logger.Printf("accept connection err: %s", err.Error())
			continue
		}
		sender.logger.Printf("accept connection")
		go sender.doProcess(conn)
	}
}

func (sender *Sender) doProcess(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 0)
	for {
		temp := make([]byte, CommandLen)
		n, err := conn.Read(temp)
		if err != nil {
			sender.logger.Printf("read err: %s", err.Error())
			return
		}
		buf = append(buf, temp[0:n]...)
		if len(buf) >= CommandLen {
			sender.handleMsg(buf[0:CommandLen])
			buf = buf[CommandLen:]
		}
	}
}

func (sender *Sender) handleMsg(data []byte) {
	if len(data) != CommandLen {
		sender.logger.Printf("msg size is not right")
		panic(fmt.Errorf("msg size is not right"))
	}
	var command Command
	command.Decode(data)
	sender.tpu.CommitTransaction(&command)
}

func (sender *Sender) StopTCP() {
}

func (sender *Sender) StartRPC() {
	router := gin.New()
	g := router.Group("/api")
	g.POST("/sendtransaction", sender.sendTransaction4RPC)
	sender.httpServer = &http.Server{
		Addr:    "0.0.0.0:8089",
		Handler: router,
	}
	sender.logger.Printf("start rpc server......")
	go func() {
		if err := sender.httpServer.ListenAndServe(); err != nil {
			sender.logger.Printf("ListenAndServe: %s", err.Error())
		}
	}()
}

func (sender *Sender) StopRPC() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := sender.httpServer.Shutdown(ctx); err != nil {
		panic(err)
	}
	sender.logger.Printf("rpc server has stopped......")
}

type SendTransactionResponse struct {
}

func (sender *Sender) sendTransaction4RPC(c *gin.Context) {
	var command Command
	if err := c.ShouldBindJSON(&command); err != nil {
		sender.logger.Printf("err: %s", err.Error())
		c.JSON(500, err)
		return
	}
	sender.tpu.CommitTransaction(&command)
	c.JSON(http.StatusOK, &SendTransactionResponse{})
}
