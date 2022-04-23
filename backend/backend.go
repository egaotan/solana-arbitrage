package backend

import (
	"context"
	"github.com/egaotan/solana-arbitrage/config"
	sender "github.com/egaotan/solana-arbitrage/solana_sender"
	"github.com/egaotan/solana-arbitrage/store"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"log"
	"sync"
)

type Backend struct {
	logger          *log.Logger
	txLogger        *log.Logger
	rpcClient       *rpc.Client
	wsClients       []*ws.Client
	ctx             context.Context
	wg              sync.WaitGroup
	accountSubs     []*ws.AccountSubscription
	slotSubs        []*ws.SlotSubscription
	wallets         []*Wallet
	player          solana.PublicKey
	lock            int32
	cachedBlockHash []solana.Hash
	updateBlockHash chan uint64
	transaction     bool
	store           *store.Store
	commandChans    []chan *Command
	clients         []*rpc.Client
	blockHash       []string
	blockHashTime   uint64
	tpu             *sender.Proxy1
	senderNodes     []*config.Node
	commandData     []chan []byte
	transactionSend int
	preExecute      bool
}

func NewBackend(ctx context.Context, nodes []*config.Node, transaction bool, transactionNodes []*config.Node, blockHash []string, tpuclient []string, senderNodes []*config.Node, transactionSend int, preExecute bool) *Backend {
	rpcClient := rpc.New(nodes[0].Rpc)
	wsClients := make([]*ws.Client, 0, len(nodes))
	for _, node := range nodes {
		wsClient, err := ws.Connect(ctx, node.Ws)
		if err != nil {
			panic(err)
		}
		wsClients = append(wsClients, wsClient)
	}
	backend := &Backend{
		rpcClient:       rpcClient,
		wsClients:       wsClients,
		ctx:             ctx,
		logger:          utils.NewLog(config.LogPath, config.BackendLog),
		txLogger:        utils.NewLog(config.LogPath, config.SentTxHash),
		accountSubs:     make([]*ws.AccountSubscription, 0),
		slotSubs:        make([]*ws.SlotSubscription, 0),
		updateBlockHash: make(chan uint64, 1024),
		cachedBlockHash: make([]solana.Hash, 0, 3),
		transaction:     transaction,
		blockHash:       blockHash,
		transactionSend: transactionSend,
		senderNodes:     senderNodes,
		preExecute:      preExecute,
	}
	commandChans := make([]chan *Command, 0, len(transactionNodes))
	clients := make([]*rpc.Client, 0, len(transactionNodes))
	for _, node := range transactionNodes {
		backend.logger.Printf("transaction node (%s)", node.Rpc)
		commandChans = append(commandChans, make(chan *Command, 1024))
		clients = append(clients, rpc.New(node.Rpc))
	}
	backend.commandChans = commandChans
	backend.clients = clients

	tpu, err := sender.NewProxy1(ctx, nodes[0].Ws, tpuclient, config.Bomb, config.LogPath)
	if err != nil {
		panic(err)
	}
	backend.tpu = tpu
	return backend
}

/*
func (backend *Backend) RpcClient() *rpc.Client {
	return backend.rpcClient
}

func (backend *Backend) WsClient() *ws.Client {
	return backend.wsClient
}
*/

func (backend *Backend) Start() {
	if !backend.transaction {
		return
	}
	//
	backend.startExecutor()
	// start recent block hash cache
	backend.wg.Add(1)
	go backend.CacheRecentBlockHash()
	//backend.updateBlockHash <- true
	backend.cachedBlockHash = append(backend.cachedBlockHash, []solana.Hash{{}, {}, {}}...)
	backend.tpu.Start()
	//
	backend.commandData = make([]chan []byte, 0)
	for i, senderNode := range backend.senderNodes {
		backend.commandData = append(backend.commandData, make(chan []byte, 64))
		go backend.sender(i, senderNode.Rpc)
	}
}

func (backend *Backend) Stop() {
	if !backend.transaction {
		return
	}
	for _, slotSub := range backend.slotSubs {
		slotSub.Unsubscribe()
	}
	for _, accountSub := range backend.accountSubs {
		accountSub.Unsubscribe()
	}
	backend.tpu.Stop()
	backend.wg.Wait()
}

func (backend *Backend) SetStore(store *store.Store) {
	backend.store = store
}
