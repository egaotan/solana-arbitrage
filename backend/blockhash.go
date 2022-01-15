package backend

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"sync/atomic"
)

func (backend *Backend) CacheRecentBlockHash() {
	defer backend.wg.Done()
	//ticker := time.NewTicker(time.Second * 2)
	for {
		select {
		case <-backend.updateBlockHash:
			getRecentBlockHashResult, err := backend.rpcClient.GetRecentBlockhash(backend.ctx, rpc.CommitmentFinalized)
			if err != nil {
				continue
			}
			for !atomic.CompareAndSwapInt32(&backend.lock, 0, 1) {
				continue
			}
			backend.cachedBlockHash = append(backend.cachedBlockHash, getRecentBlockHashResult.Value.Blockhash)
			backend.cachedBlockHash = backend.cachedBlockHash[1:]
			atomic.StoreInt32(&backend.lock, 0)
		case <-backend.ctx.Done():
			backend.logger.Printf("recent blockhash cacher exit")
			return
		}
	}
}

func (backend *Backend) GetRecentBlockHash(level int) solana.Hash {
	defer atomic.StoreInt32(&backend.lock, 0)
	for !atomic.CompareAndSwapInt32(&backend.lock, 0, 1) {
		continue
	}
	return backend.cachedBlockHash[2 - level]
}
