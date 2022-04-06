package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/random_serum_orca/app"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGABRT)
	go shutdown(cancel, quit)

	if len(os.Args) != 2 {
		panic("args is invalid")
	}
	workSpace := os.Args[1]
	os.Chdir(workSpace)

	infoJson, err := os.ReadFile(config.ConfigFile)
	if err != nil {
		panic(err)
	}
	var cfg config.Config
	err = json.Unmarshal(infoJson, &cfg)
	if err != nil {
		panic(err)
	}

	//
	config.USDC_AMOUNT = cfg.Usdc * 1000000
	config.Bomb = cfg.Bomb

	cfg.WorkSpace = workSpace
	workspace, _ := os.Getwd()
	fmt.Printf("work space: %s\n", workspace)

	//
	infoJson, err = os.ReadFile(config.ValidatorFile)
	if err != nil {
		panic(err)
	}
	validatorNodes := make([]*config.Node, 0)
	err = json.Unmarshal(infoJson, &validatorNodes)
	if err != nil {
		panic(err)
	}

	//
	oldNodes := cfg.Nodes
	usableNodes := make([]*config.Node, 0)
	for _, node := range oldNodes {
		if node.Usable {
			usableNodes = append(usableNodes, node)
		}
	}
	cfg.Nodes = usableNodes
	oldValidators := cfg.TransactionNodes
	usableValidators := make([]*config.Node, 0)
	for _, oldValidator := range oldValidators {
		if oldValidator.Usable {
			usableValidators = append(usableValidators, oldValidator)
		}
	}
	cfg.TransactionNodes = usableValidators
	//
	usableValidators1 := make([]*config.Node, 0)
	for _, validator := range validatorNodes {
		if validator.Usable {
			usableValidators1 = append(usableValidators1, validator)
		}
	}
	//
	selectedValidators := make([]*config.Node, 0)
	for i := 0; i < len(usableValidators1); i++ {
		selectedValidators = append(selectedValidators, usableValidators1[i])
		if len(selectedValidators) > cfg.TransactionNodeSize {
			break
		}
	}
	cfg.TransactionNodes = append(cfg.TransactionNodes, selectedValidators...)

	//
	t := time.Now()
	t_str := t.Format("2006-01-02")
	dir := fmt.Sprintf("./%s_log/", t_str)
	os.Mkdir(dir, os.ModePerm)
	config.LogPath = dir

	at := app.NewArbitrage(ctx, &cfg)
	at.Service()
}

func shutdown(cancel context.CancelFunc, quit <-chan os.Signal) {
	osCall := <-quit
	fmt.Printf("System call: %v, auto trader is shutting down......\n", osCall)
	cancel()
}
