package tpu

import (
	"context"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"log"
	"sync/atomic"
	"time"
)

type AvailableNodesService struct {
	ctx            context.Context
	client         *rpc.Client
	availableNodes map[solana.PublicKey]string
	lock           int32
	logger *log.Logger
}

func NewAvailableNodesService(ctx context.Context, client *rpc.Client, logger *log.Logger) *AvailableNodesService {
	ans := &AvailableNodesService{
		ctx:            ctx,
		client:         client,
		availableNodes: make(map[solana.PublicKey]string),
		logger: logger,
	}
	return ans
}

func (ans *AvailableNodesService) Start() {
	go ans.refresh()
}

func (ans *AvailableNodesService) fetchAvailableNodes() {
	clusterNodes, err := ans.client.GetClusterNodes(ans.ctx)
	if err != nil {
		ans.logger.Printf("GetClusterNodes err: %s", err.Error())
		return
	}
	ans.logger.Printf("get GetClusterNodes......")
	for _, node := range clusterNodes {
		if node.TPU != nil {
			for !atomic.CompareAndSwapInt32(&ans.lock, 0, 1) {
				continue
			}
			ans.availableNodes[node.Pubkey] = *node.TPU
			atomic.StoreInt32(&ans.lock, 0)
		}
	}
}

func (ans *AvailableNodesService) GetNode(key solana.PublicKey) string {
	for !atomic.CompareAndSwapInt32(&ans.lock, 0, 1) {
		continue
	}
	defer atomic.StoreInt32(&ans.lock, 0)
	return ans.availableNodes[key]
}

func (ans *AvailableNodesService) refresh() {
	ticker := time.NewTicker(time.Minute * 5)
	for {
		select {
		case <-ticker.C:
			ans.fetchAvailableNodes()
		}
	}
}
