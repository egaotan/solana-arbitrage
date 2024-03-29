package main

import (
	"context"
	"encoding/json"
	"fmt"
	sender "github.com/egaotan/solana-arbitrage/solana_sender"
	"github.com/egaotan/solana-arbitrage/solana_sender/config"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	//
	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGABRT)
	go shutdown(cancel, quit)
	//
	if len(os.Args) != 2 {
		panic("args is invalid")
	}
	configFile := os.Args[1]
	//
	infoJson, err := os.ReadFile(configFile)
	if err != nil {
		panic(err)
	}
	var cfg config.Config
	err = json.Unmarshal(infoJson, &cfg)
	if err != nil {
		panic(err)
	}
	at := sender.NewSender(ctx, &cfg)
	at.Service()
}

func shutdown(cancel context.CancelFunc, quit <-chan os.Signal) {
	osCall := <-quit
	fmt.Printf("System call: %v, solana sender is shutting down......\n", osCall)
	cancel()
}
