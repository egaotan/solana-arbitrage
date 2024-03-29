package sender

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"log"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type Proxy1 struct {
	ctx          context.Context
	logger       *log.Logger
	slotClient   *ws.Client
	lssClients   []*rpc.Client
	ans          *AvailableNodesService
	lss          *LeaderScheduleService
	latestSlots  chan uint64
	transactions chan *Command
	tpuConns     map[string]net.Conn
	curSlot      uint64
	bomb         int
	lock         int32
}

func NewProxy1(ctx context.Context, slotClientUrl string, lssClientUrls []string, bomb int, logPath string) (*Proxy1, error) {
	logger := utils.NewLog(logPath, "tpu_executor")
	// client
	slotClient, err := ws.Connect(ctx, slotClientUrl)
	if err != nil {
		return nil, err
	}
	// tpu clients
	lssClients := make([]*rpc.Client, 0)
	for _, lssClientUrl := range lssClientUrls {
		lssClients = append(lssClients, rpc.New(lssClientUrl))
	}
	proxy := &Proxy1{
		ctx:          ctx,
		logger:       logger,
		slotClient:   slotClient,
		lssClients:   lssClients,
		ans:          NewAvailableNodesService(ctx, lssClients, logger),
		lss:          NewLeaderScheduleService(ctx, lssClients, logger),
		latestSlots:  make(chan uint64, 1024),
		transactions: make(chan *Command, 1024),
		tpuConns:     make(map[string]net.Conn),
		bomb:         bomb,
	}
	return proxy, nil
}

func (proxy *Proxy1) Start() {
	proxy.ans.Start()
	proxy.lss.Start()
	go proxy.refreshConnection()
	proxy.slotSubscribe()
	for i := 0; i < 8; i++ {
		go proxy.sendTransaction()
	}
}

func (proxy *Proxy1) Stop() {

}

func (proxy *Proxy1) refresh() {
	startSlot := proxy.curSlot - PAST_SLOT_SEARCH
	endSlot := proxy.curSlot + UPCOMING_SLOT_SEARCH
	leaderAddress := make(map[solana.PublicKey]bool)
	tpuAddress := make(map[string]uint64)
	proxy.logger.Printf("refresh connection, slot (%d, %d)", startSlot, endSlot)
	for slot := startSlot; slot < endSlot; slot++ {
		leader := proxy.lss.GetSlotLeader(slot)
		//proxy.logger.Printf("slot leader (%d, %s)", slot, leader.String())
		if leader.IsZero() {
			proxy.logger.Printf("no leader in the slot: %d", slot)
			continue
		}
		_, ok := leaderAddress[leader]
		if ok {
			continue
		}
		leaderAddress[leader] = true
		tpu := proxy.ans.GetNode(leader)
		if tpu != "" {
			//proxy.logger.Printf("leader tpu (%s, %s)", leader.String(), tpu)
			tpuAddress[tpu] = slot
		} else {
			proxy.logger.Printf("tpu address is invalid, slot: %d, leader: %s", slot, leader.String())
		}
	}
	tpuConnctions := make(map[string]net.Conn)
	for tpu, slot := range tpuAddress {
		datas := strings.Split(tpu, ":")
		host := datas[0]
		port, _ := strconv.ParseUint(datas[1], 10, 64)
		con, ok := proxy.tpuConns[tpu]
		var err error
		n := 0
		if ok {
			n, err = con.Write([]byte{1})
			if err != nil || n != 1 {
				proxy.logger.Printf("connect has invalid, reconnect, tpu: %s", tpu)
				ok = false
			} else {
				tpuConnctions[tpu] = con
			}
		}
		if !ok {
			con, err = net.Dial("udp", fmt.Sprintf("%s:%d", host, port))
			if err != nil {
				proxy.logger.Printf("cannot dial udp, err: %s, address: %s, slot: %d", err.Error(), tpu, slot)
				continue
			}
			n, err = con.Write([]byte{1})
			if err != nil || n != 1 {
				proxy.logger.Printf("can not connect, tpu: %s, err: %s", tpu, err.Error())
				continue
			}
			tpuConnctions[tpu] = con
		}
	}
	proxy.logger.Printf("there are %d connections", len(tpuConnctions))
	for !atomic.CompareAndSwapInt32(&proxy.lock, 0, 1) {
		continue
	}
	defer atomic.StoreInt32(&proxy.lock, 0)
	proxy.tpuConns = tpuConnctions
}

func (proxy *Proxy1) refreshConnection() {
	for {
		select {
		case slot := <-proxy.latestSlots:
			{
			L:
				for {
					select {
					case slot = <-proxy.latestSlots:
					default:
						break L
					}
				}
			}
			if slot-proxy.curSlot < 5 {
				continue
			}
			proxy.curSlot = slot
			proxy.refresh()
		case <-proxy.ctx.Done():
			proxy.logger.Printf("refreshConnection exit!")
			return
		}
	}
}

func (proxy *Proxy1) slotSubscribe() {
	sub, err := proxy.slotClient.SlotSubscribe()
	if err != nil {
		proxy.logger.Printf("slot subscribe error: %s", err.Error())
		return
	}
	go proxy.recvSlot(sub)
}

func (proxy *Proxy1) recvSlot(sub *ws.SlotSubscription) {
	for {
		got, err := sub.Recv()
		if err != nil {
			proxy.logger.Printf("RecvSlot error: %s", err.Error())
			syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
			return
		}
		if got == nil {
			proxy.logger.Printf("RecvSlot exit")
			return
		}
		proxy.logger.Printf("receive slot, %d", got.Slot)
		if got.Slot%5 == 0 {
			proxy.latestSlots <- got.Slot
		}
	}
}

func (proxy *Proxy1) CommitTransaction(command *Command) {
	proxy.transactions <- command
}

func (proxy *Proxy1) sendTransaction() {
	defer func() {
		proxy.logger.Printf("tpu exit")
	}()
	for {
		select {
		case tx := <-proxy.transactions:
			proxy.send(tx)
		}
	}
}

func (proxy *Proxy1) send(tx *Command) {
	for !atomic.CompareAndSwapInt32(&proxy.lock, 0, 1) {
		continue
	}
	tpuConnections := proxy.tpuConns
	atomic.StoreInt32(&proxy.lock, 0)
	//
	proxy.logger.Printf("begin send tx (%s)(%d)", tx.Hash, tx.Id)
	proxy.logger.Printf("tx time: %s, send time: %s",
		time.Unix(int64(tx.Id)/1000000, int64(tx.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"),
		time.Now().Format("2006-01-02 15:04:05.000000"),
	)
	defer func() {
		proxy.logger.Printf("end send tx (%d) (%d)", tx.Id, tx.Counter)
	}()

	for addr, conn := range tpuConnections {
		proxy.logger.Printf("send tx (%d) to %s", tx.Id, addr)
		n, err := conn.Write(tx.Tx)
		if err != nil {
			proxy.logger.Printf("send tx (%d) err: %s, %d", tx.Id, err.Error())
		} else {
			proxy.logger.Printf("send tx (%d) (%d, %d)", tx.Id, n, len(tx.Tx))
		}
		tx.Counter ++
	}
	for i := 0;i < proxy.bomb;i ++ {
		for _, conn := range tpuConnections {
			//proxy.logger.Printf("send tx to %s", addr)
			_, err := conn.Write(tx.Tx)
			if err != nil {
				proxy.logger.Printf("send tx (%d) err: %s", tx.Id, err.Error())
			} else {
				//proxy.logger.Printf("send tx (%d) (%d, %d)", tx.Id, n, len(tx.Tx))
			}
		}
		tx.Counter ++
	}
}
