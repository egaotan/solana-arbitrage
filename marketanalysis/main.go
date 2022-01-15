package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/marketanalysis/app"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGABRT)
	go shutdown(cancel, quit)

	if len(os.Args) != 5 {
		panic("args is invalid")
	}
	workSpace := os.Args[1]
	os.Chdir(workSpace)
	market := os.Args[2]
	slotStart, err := strconv.ParseUint(os.Args[3], 10, 64)
	if err != nil {
		panic(err)
	}
	slotEnd, err := strconv.ParseUint(os.Args[4], 10, 64)
	if err != nil {
		panic(err)
	}

	infoJson, err := os.ReadFile(config.ConfigFile)
	if err != nil {
		panic(err)
	}
	var config1 config.Config
	err = json.Unmarshal(infoJson, &config1)
	if err != nil {
		panic(err)
	}
	config1.WorkSpace = workSpace
	workspace, _ := os.Getwd()
	fmt.Printf("work space: %s\n", workspace)

	//
	t := time.Now()
	t_str := t.Format("2006-01-02")
	dir := fmt.Sprintf("./%s_log/", t_str)
	os.Mkdir(dir, os.ModePerm)
	config.LogPath = dir

	at := app.NewMarketAnalysis(ctx, &config1, market, slotStart, slotEnd)
	at.Service()
}

func shutdown(cancel context.CancelFunc, quit <-chan os.Signal) {
	osCall := <-quit
	fmt.Printf("System call: %v, auto trader is shutting down......\n", osCall)
	cancel()
}
