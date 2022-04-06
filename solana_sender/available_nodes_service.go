package sender

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
	client         []*rpc.Client
	availableNodes map[solana.PublicKey]string
	lock           int32
	logger         *log.Logger
}

func NewAvailableNodesService(ctx context.Context, client []*rpc.Client, logger *log.Logger) *AvailableNodesService {
	ans := &AvailableNodesService{
		ctx:            ctx,
		client:         client,
		availableNodes: make(map[solana.PublicKey]string),
		logger:         logger,
	}
	return ans
}

func (ans *AvailableNodesService) Start() {
	go ans.refresh()
}

func (ans *AvailableNodesService) fetchAvailableNodes() {
	var clusterNodes []*rpc.GetClusterNodesResult
	var err error
	availableNodes := make(map[solana.PublicKey]string)
	for i := 0; i < len(ans.client); i++ {
		clusterNodes, err = ans.client[i].GetClusterNodes(ans.ctx)
		if err != nil {
			ans.logger.Printf("GetClusterNodes err (%d): %s", i, err.Error())
		} else {
			ans.logger.Printf("GetClusterNodes (%d)......", i)
			for _, node := range clusterNodes {
				if node.TPU != nil {
					availableNodes[node.Pubkey] = *node.TPU
				} else {
					ans.logger.Printf("tpu is unavailable, (%s)", node.Pubkey.String())
				}
			}
		}
	}
	ans.logger.Printf("update cluster nodes......")
	for !atomic.CompareAndSwapInt32(&ans.lock, 0, 1) {
		continue
	}
	for k, v := range availableNodes {
		ans.availableNodes[k] = v
	}
	atomic.StoreInt32(&ans.lock, 0)
}

func (ans *AvailableNodesService) GetNode(key solana.PublicKey) string {
	for !atomic.CompareAndSwapInt32(&ans.lock, 0, 1) {
		continue
	}
	defer atomic.StoreInt32(&ans.lock, 0)
	return ans.availableNodes[key]
}

func (ans *AvailableNodesService) refresh() {
	ans.fetchAvailableNodes()
	ticker := time.NewTicker(time.Minute * 5)
	for {
		select {
		case <-ticker.C:
			ans.fetchAvailableNodes()
		case <-ans.ctx.Done():
			ans.logger.Printf("AvailableNodesService::refresh exit!")
			return
		}
	}
}
