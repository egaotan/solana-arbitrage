package backend

import (
	"github.com/gagliardetto/solana-go/rpc/ws"
	"sync/atomic"
	"syscall"
	"time"
)

type Slot struct {
	Number uint64
}

type SlotCallback interface {
	OnSlotUpdate(slot *Slot) error
}

func (backend *Backend) SubscribeSlot(cb SlotCallback) error {
	for _, wsClient := range backend.wsClients {
		sub, err := wsClient.SlotSubscribe()
		if err != nil {
			return err
		}
		backend.slotSubs = append(backend.slotSubs, sub)
		//
		tt := int64(0)
		backend.wg.Add(1)
		go backend.RecvSlot(cb, sub, &tt)
		backend.wg.Add(1)
		go backend.AdditionalSlot(cb, &tt)
	}
	return nil
}

func (backend *Backend) RecvSlot(cb SlotCallback, sub *ws.SlotSubscription, tt *int64) {
	defer backend.wg.Done()
	for {
		got, err := sub.Recv()
		if err != nil {
			backend.logger.Printf("RecvSlot err: %v", err)
			syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
			return
		}
		if got == nil {
			backend.logger.Printf("RecvSlot exit")
			return
		}
		backend.logger.Printf("receive slot, %d", got.Slot)
		atomic.StoreInt64(tt, time.Now().UnixNano())
		backend.updateBlockHash <- true
		backend.tpu.CommitSlot(got.Slot)
		data := got
		slot := &Slot{
			Number: data.Slot,
		}
		if cb != nil {
			cb.OnSlotUpdate(slot)
		}
	}
}

func (backend *Backend) AdditionalSlot(cb SlotCallback, tt *int64) {
	defer backend.wg.Done()
	timer2 := time.NewTicker(time.Millisecond * 100)
	for {
		select {
		case <-timer2.C:
			newTime := time.Now().UnixNano()
			oldTime := atomic.SwapInt64(tt, newTime)
			if newTime-oldTime >= time.Millisecond.Nanoseconds()*90 {
				slot := &Slot{
					Number: 0,
				}
				if cb != nil {
					cb.OnSlotUpdate(slot)
				}
			}
		case <-backend.ctx.Done():
			backend.logger.Printf("AdditionalSlot exit")
			return
		}
	}
}
