package backend

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"sync/atomic"
)

func (backend *Backend) CacheRecentBlockHash() {
	defer backend.wg.Done()
	//ticker := time.NewTicker(time.Second * 2)
	rpcClient := rpc.New(backend.blockHash)
	for {
		select {
		case <-backend.updateBlockHash:
		L:
			for {
				select {
				case <-backend.updateBlockHash:
				default:
					break L
				}
			}
			getRecentBlockHashResult, err := rpcClient.GetRecentBlockhash(backend.ctx, rpc.CommitmentFinalized)
			if err != nil {
				backend.logger.Printf("GetRecentBlockhash, err: %s", err.Error())
				continue
			}
			if backend.cachedBlockHash[2] == getRecentBlockHashResult.Value.Blockhash {
				continue
			}
			for !atomic.CompareAndSwapInt32(&backend.lock, 0, 1) {
				continue
			}
			backend.cachedBlockHash = append(backend.cachedBlockHash, getRecentBlockHashResult.Value.Blockhash)
			backend.cachedBlockHash = backend.cachedBlockHash[1:]
			atomic.StoreInt32(&backend.lock, 0)
			backend.logger.Printf("receive block hash, %s", getRecentBlockHashResult.Value.Blockhash.String())
		case <-backend.ctx.Done():
			backend.logger.Printf("recent block hash cache exit")
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
