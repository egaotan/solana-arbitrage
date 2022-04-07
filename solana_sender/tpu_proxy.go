package sender

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
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

type Proxy struct {
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
	cache        []*Command
	txSub        map[solana.Signature]*ws.SignatureSubscription
}

func NewProxy(ctx context.Context, slotClientUrl string, lssClientUrls []string, bomb int, logPath string) (*Proxy, error) {
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
	proxy := &Proxy{
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
		cache:        make([]*Command, 0),
		txSub:        make(map[solana.Signature]*ws.SignatureSubscription),
	}
	return proxy, nil
}

func (proxy *Proxy) Start() {
	proxy.ans.Start()
	proxy.lss.Start()
	go proxy.refreshConnection()
	proxy.slotSubscribe()
	//for i := 0; i < 8; i++ {
	go proxy.sendTransaction()
	//}
}

func (proxy *Proxy) Stop() {

}

func (proxy *Proxy) refresh() {
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

func (proxy *Proxy) refreshConnection() {
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

func (proxy *Proxy) slotSubscribe() {
	sub, err := proxy.slotClient.SlotSubscribe()
	if err != nil {
		proxy.logger.Printf("slot subscribe error: %s", err.Error())
		return
	}
	go proxy.recvSlot(sub)
}

func (proxy *Proxy) recvSlot(sub *ws.SlotSubscription) {
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

func (proxy *Proxy) txSubscribe(tx *Command) {
	proxy.logger.Printf("begin subscribe tx (%s) (%d) (%s)", tx.Hash.String(), tx.Id,
		time.Unix(int64(tx.Id)/1000000, int64(tx.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"))

	sub, err := proxy.slotClient.SignatureSubscribe(tx.Hash, rpc.CommitmentProcessed)
	if err != nil {
		proxy.logger.Printf("tx subscribe error: %s", err.Error())
		return
	}
	proxy.txSub[tx.Hash] = sub
	go proxy.recvTx(tx, sub)
}

func (proxy *Proxy) recvTx(tx *Command, sub *ws.SignatureSubscription) {
	for {
		got, err := sub.Recv()
		if err != nil {
			proxy.logger.Printf("recvTx error: %s", err.Error())
			//syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
			return
		}
		if got == nil {
			proxy.logger.Printf("recvTx exit")
			return
		}
		proxy.logger.Printf("receive tx (%s) (%d) (%d)", tx.Hash.String(), got.Context.Slot, tx.Counter)
		tx.Status = 1
	}
}

func (proxy *Proxy) CommitTransaction(command *Command) {
	proxy.transactions <- command
}

func (proxy *Proxy) sendTransaction() {
	defer func() {
		proxy.logger.Printf("tpu exit")
	}()
	ticker := time.NewTicker(time.Microsecond * time.Duration(config.Bomb))
	var tx *Command
	for {
		select {
		case tx = <-proxy.transactions:
			proxy.cache = append(proxy.cache, tx)
			{
			L:
				for {
					select {
					case tx = <-proxy.transactions:
						proxy.cache = append(proxy.cache, tx)
					default:
						break L
					}
				}
			}
			proxy.send()
		case <-ticker.C:
			proxy.send()
		}
	}
}

func (proxy *Proxy) send() {
	for !atomic.CompareAndSwapInt32(&proxy.lock, 0, 1) {
		continue
	}
	tpuConnections := proxy.tpuConns
	atomic.StoreInt32(&proxy.lock, 0)

	// remove old tx
	t := uint64(time.Now().UnixNano() / 1000)
	startIndex := -1
	endIndex := 0
	txCounter := 0
	for i, tx := range proxy.cache {
		if t-tx.Id <= 2*1000000 {
			if startIndex == -1 {
				startIndex = i
			}
			if tx.Status == 0 {
				txCounter++
			}
			endIndex = i
			if txCounter >= 8 {
				break
			}
		} else {
			sub, ok := proxy.txSub[tx.Hash]
			if ok {
				sub.Unsubscribe()
				delete(proxy.txSub, tx.Hash)
			}
			if tx.Status == 1 {
				continue
			} else {
				proxy.logger.Printf("send tx failed. (%s) (%d)", tx.Hash.String(), tx.Counter)
			}
		}
	}
	if startIndex != -1 {
		proxy.cache = proxy.cache[startIndex : endIndex+1]
	} else {
		if len(proxy.cache) > 0 {
			proxy.cache = proxy.cache[:0]
		}
	}

	for _, tx := range proxy.cache {
		/*
			proxy.logger.Printf("begin send tx (%d) time: %s", tx.Id,
				time.Unix(int64(tx.Id)/1000000, int64(tx.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"))
			defer func() {
				proxy.logger.Printf("end send tx (%d)", tx.Id)
			}()
		*/
		if tx.Status == 1 {
			continue
		}
		if t - tx.Id > 2*1000000 {
			continue
		}
		_, ok := proxy.txSub[tx.Hash]
		if !ok {
			proxy.txSubscribe(tx)
		}
		tx.Counter ++
		for _, conn := range tpuConnections {
			//proxy.logger.Printf("send tx (%d) to %s", tx.Id, addr)
			_, err := conn.Write(tx.Tx)
			if err != nil {
				proxy.logger.Printf("send tx (%d) err: %s, %d", tx.Id, err.Error())
			} else {
				//proxy.logger.Printf("send tx (%d) (%d, %d)", tx.Id, n, len(tx.Tx))
			}
		}
	}

	/*
		proxy.logger.Printf("begin send tx (%s)(%d)", tx.Hash, tx.Id)
		proxy.logger.Printf("tx time: %s, send time: %s",
			time.Unix(int64(tx.Id)/1000000, int64(tx.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"),
			time.Now().Format("2006-01-02 15:04:05.000000"),
		)
		defer func() {
			proxy.logger.Printf("end send tx (%d)", tx.Id)
		}()

		for addr, conn := range tpuConnections {
			proxy.logger.Printf("send tx (%d) to %s", tx.Id, addr)
			n, err := conn.Write(tx.Tx)
			if err != nil {
				proxy.logger.Printf("send tx (%d) err: %s, %d", tx.Id, err.Error())
			} else {
				proxy.logger.Printf("send tx (%d) (%d, %d)", tx.Id, n, len(tx.Tx))
			}
		}
		timeRate := 500 / proxy.bomb
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
			time.Sleep(time.Millisecond * time.Duration(timeRate))
		}
	*/

	/*
		for i := 0; i < proxy.bomb; i++ {
			for _, conn := range tpuConnections {
				//proxy.logger.Printf("send tx to %s", addr)
				_, err := conn.Write(tx.Tx)
				if err != nil {
					proxy.logger.Printf("send tx (%d) err: %s", tx.Id, err.Error())
				} else {
					//proxy.logger.Printf("send tx (%d) (%d, %d)", tx.Id, n, len(tx.Tx))
				}
			}
			//proxy.logger.Printf("send tx (%d) (%d, %d)", tx.Id, len(tx.Tx))
			if i%50 == 49 {
				time.Sleep(time.Millisecond * 50)
			}
		}
	*/
}
