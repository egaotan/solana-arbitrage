package server

import (
	"context"
	"github.com/egaotan/solana-arbitrage/solana_sender/server/config"
	"github.com/gin-gonic/gin"
	"log"
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
	sender.StartRPC()
	<-sender.ctx.Done()
	sender.StopRPC()
	sender.Stop()
}

func (sender *Sender) Start() {
	sender.tpu.Start()
}

func (sender *Sender) Stop() {

}

func (sender *Sender) StartRPC() {
	router := gin.New()
	g := router.Group("/api")
	g.POST("/sendtransaction", sender.sendTransaction)
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

func (sender *Sender) sendTransaction(c *gin.Context) {
	var command Command
	if err := c.ShouldBindJSON(&command); err != nil {
		sender.logger.Printf("err: %s", err.Error())
		c.JSON(500, err)
		return
	}
	sender.tpu.CommitTransaction(&command)
	c.JSON(http.StatusOK, &SendTransactionResponse{})
}
