package backend

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"sync/atomic"
)

func (backend *Backend) CacheRecentBlockHash() {
	defer backend.wg.Done()
	//ticker := time.NewTicker(time.Second * 2)
	rpcClients := make([]*rpc.Client, 0)
	for _, x := range backend.blockHash {
		rpcClients = append(rpcClients, rpc.New(x))
	}
	index := 0
	for {
		select {
		case slot := <-backend.updateBlockHash:
		L:
			for {
				select {
				case slot = <-backend.updateBlockHash:
				default:
					break L
				}
			}
			/*
				getRecentBlockHashResult, err := rpcClient.GetRecentBlockhash(backend.ctx, rpc.CommitmentFinalized)
				if err != nil {
					backend.logger.Printf("GetRecentBlockhash, err: %s", err.Error())
					continue
				}
							backend.logger.Printf("get recent block hash. (%s, %d)",
						getRecentBlockHashResult.Value.Blockhash.String(), getRecentBlockHashResult.Context.Slot)
					if backend.cachedBlockHash[2] == getRecentBlockHashResult.Value.Blockhash {
						continue
					}
					for !atomic.CompareAndSwapInt32(&backend.lock, 0, 1) {
						continue
					}
					backend.cachedBlockHash = append(backend.cachedBlockHash, getRecentBlockHashResult.Value.Blockhash)
					backend.cachedBlockHash = backend.cachedBlockHash[1:]
					atomic.StoreInt32(&backend.lock, 0)
			*/
			slot = slot / 5 * 5
			var getRecentBlockHashResult *rpc.GetRecentBlockhashResult
			var err error
			for i := 0;i < len(rpcClients);i ++ {
				getRecentBlockHashResult, err = rpcClients[index].GetRecentBlockhash(backend.ctx, rpc.CommitmentFinalized)
				if err != nil {
					backend.logger.Printf("GetRecentBlockhash, %d err: %s", index, err.Error())
					index++
					index = index % len(rpcClients)
				} else {
					break
				}
			}
			if err != nil {
				backend.logger.Printf("GetBlock, all err: %s", err.Error())
				continue
			}
			backend.logger.Printf("get recent block hash. (%s, %d, %d)",
				getRecentBlockHashResult.Value.Blockhash.String(), getRecentBlockHashResult.Context.Slot)
			if backend.cachedBlockHash[2] == getRecentBlockHashResult.Value.Blockhash {
				continue
			}
			for !atomic.CompareAndSwapInt32(&backend.lock, 0, 1) {
				continue
			}
			backend.cachedBlockHash = append(backend.cachedBlockHash, getRecentBlockHashResult.Value.Blockhash)
			backend.cachedBlockHash = backend.cachedBlockHash[1:]
			atomic.StoreInt32(&backend.lock, 0)
			backend.logger.Printf("receive block hash, %s", getRecentBlockHashResult.Value.Blockhash)
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
	return backend.cachedBlockHash[2-level]
}
