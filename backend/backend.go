package backend

import (
	"context"
	"github.com/egaotan/solana-arbitrage/config"
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
	updateBlockHash chan bool
	transaction     bool
	store           *store.Store
	commandChans    []chan *Command
	clients         []*rpc.Client
	blockHash string
}

func NewBackend(ctx context.Context, nodes []*config.Node, transaction bool, transactionNodes []*config.Node, blockHash string) *Backend {
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
		accountSubs:     make([]*ws.AccountSubscription, 0),
		slotSubs:        make([]*ws.SlotSubscription, 0),
		updateBlockHash: make(chan bool, 1024),
		cachedBlockHash:make([]solana.Hash, 0, 3),
		transaction:     transaction,
		blockHash: blockHash,
	}
	commandChans := make([]chan *Command, 0, len(transactionNodes))
	clients := make([]*rpc.Client, 0, len(transactionNodes))
	for _, node := range transactionNodes {
		commandChans = append(commandChans, make(chan *Command, 1024))
		clients = append(clients, rpc.New(node.Rpc))
	}
	backend.commandChans = commandChans
	backend.clients = clients
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
	backend.cachedBlockHash = append(backend.cachedBlockHash, []solana.Hash{{},{},{}}...)
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
	backend.wg.Wait()
}

func (backend *Backend) SetStore(store *store.Store) {
	backend.store = store
}
