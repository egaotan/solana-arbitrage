package tpu

import (
	"context"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"log"
	"sync/atomic"
)

var (
	UPCOMING_SLOT_SEARCH = uint64(50)
	PAST_SLOT_SEARCH     = uint64(4)
)

type LeaderScheduleService struct {
	ctx       context.Context
	client    *rpc.Client
	firstSlot uint64
	leaders   map[uint64]solana.PublicKey
	newFresh  chan uint64
	lock      int32
	logger *log.Logger
}

func NewLeaderScheduleService(ctx context.Context, client *rpc.Client, logger *log.Logger) *LeaderScheduleService {
	lss := &LeaderScheduleService{
		ctx:      ctx,
		client:   client,
		logger: logger,
		leaders:  make(map[uint64]solana.PublicKey),
		newFresh: make(chan uint64, 1024),
	}
	return lss
}

func (ans *LeaderScheduleService) Start() {
	go ans.Refresh()
}

func (ans *LeaderScheduleService) fetchLeaders(slot uint64, counter uint64) {
	leaders, err := ans.client.GetSlotLeaders(ans.ctx, slot, counter)
	if err != nil {
		ans.logger.Printf("GetSlotLeaders err: %s", err.Error())
		return
	}
	ans.logger.Printf("GetSlotLeaders, slot: %d, count: %d", slot, counter)
	for !atomic.CompareAndSwapInt32(&ans.lock, 0, 1) {
		continue
	}
	defer atomic.StoreInt32(&ans.lock, 0)
	for i, leader := range leaders {
		ans.leaders[slot+uint64(i)] = leader
	}
}

func (ans *LeaderScheduleService) GetFirstSlot() uint64 {
	return ans.firstSlot
}

func (ans *LeaderScheduleService) GetLastSlot() uint64 {
	return ans.firstSlot + uint64(len(ans.leaders))
}

func (ans *LeaderScheduleService) GetCheckPoint() uint64 {
	return ans.firstSlot + UPCOMING_SLOT_SEARCH
}

func (ans *LeaderScheduleService) GetSlotLeader(slot uint64) solana.PublicKey {
	if slot > ans.GetCheckPoint() {
		ans.newFresh <- slot
	}
	for !atomic.CompareAndSwapInt32(&ans.lock, 0, 1) {
		continue
	}
	defer atomic.StoreInt32(&ans.lock, 0)
	if slot >= ans.firstSlot && slot <= ans.GetLastSlot() {
		return ans.leaders[slot]
	}
	return solana.PublicKey{}
}

func (ans *LeaderScheduleService) Refresh() {
	for {
		select {
		case slot := <-ans.newFresh:
		L:
			for {
				select {
				case slot = <-ans.newFresh:
				default:
					break L
				}
			}
			ans.refresh(slot)
		}
	}
}

func (ans *LeaderScheduleService) refresh(slot uint64) {
	firstSlot := slot - PAST_SLOT_SEARCH
	counter :=  UPCOMING_SLOT_SEARCH*2
	ans.fetchLeaders(firstSlot, counter)
	ans.firstSlot = firstSlot
}
