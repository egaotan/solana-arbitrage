package tpu

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"log"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Proxy struct {
	ctx          context.Context
	client       *rpc.Client
	curSlot      uint64
	ans          *AvailableNodesService
	lss          *LeaderScheduleService
	tpuConns     map[string]net.Conn
	latestSlots  chan uint64
	transactions chan *Command
	lock         int32
	logger       *log.Logger
}

type Command struct {
	Id uint64
	Tx []byte
}

func NewProxy(ctx context.Context, tpuclient string) *Proxy {
	proxy := &Proxy{
		ctx:          ctx,
		client:       rpc.New(tpuclient),
		latestSlots:  make(chan uint64, 1024),
		transactions: make(chan *Command, 1024),
		tpuConns:     make(map[string]net.Conn),
	}
	proxy.logger = utils.NewLog(config.LogPath, config.TPULog)
	proxy.ans = NewAvailableNodesService(proxy.ctx, proxy.client, proxy.logger)
	proxy.lss = NewLeaderScheduleService(proxy.ctx, proxy.client, proxy.logger)
	return proxy
}

func (proxy *Proxy) Start() {
	proxy.ans.Start()
	proxy.lss.Start()
	go proxy.newSlot()
	for i := 0;i < 64;i ++ {
		go proxy.SendTransactions()
	}
}

func (proxy *Proxy) Stop() {

}

func (proxy *Proxy) RefreshConnection() {
	startSlot := proxy.curSlot - PAST_SLOT_SEARCH
	endSlot := proxy.curSlot + UPCOMING_SLOT_SEARCH
	leaderAddress := make(map[solana.PublicKey]bool)
	tpuAddress := make(map[string]uint64)
	proxy.logger.Printf("refresh connection, slot (%d, %d)", startSlot, endSlot)
	for slot := startSlot; slot < endSlot; slot++ {
		leader := proxy.lss.GetSlotLeader(slot)
		//proxy.logger.Printf("slot leader (%d, %s)", slot, leader.String())
		if !leader.IsZero() && !leaderAddress[leader] {
			leaderAddress[leader] = true
			tpu := proxy.ans.GetNode(leader)
			if tpu != "" {
				//proxy.logger.Printf("leader tpu (%s, %s)", leader.String(), tpu)
				tpuAddress[tpu] = slot
			} else {
				proxy.logger.Printf("tpu address is invalid, slot: %d, leader: %s", slot, leader.String())
			}
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

func (proxy *Proxy) CommitSlot(slot uint64) {
	proxy.latestSlots <- slot
}

func (proxy *Proxy) newSlot() {
	for {
		select {
		case slot := <-proxy.latestSlots:
		L:
			for {
				select {
				case slot = <-proxy.latestSlots:
				default:
					break L
				}
			}
			proxy.curSlot = slot
			proxy.RefreshConnection()
		}
	}
}

func (proxy *Proxy) CommitTransaction(command *Command) {
	proxy.transactions <- command
}

func (proxy *Proxy) SendTransactions() {
	for {
		select {
		case tx := <-proxy.transactions:
			proxy.SendTransaction(tx)
		}
	}
}

func (proxy *Proxy) SendTransaction(tx *Command) {
	for !atomic.CompareAndSwapInt32(&proxy.lock, 0, 1) {
		continue
	}
	tpuConnections := proxy.tpuConns
	atomic.StoreInt32(&proxy.lock, 0)

	proxy.logger.Printf("begin send tx: %d, time: %s", tx.Id,
		time.Unix(int64(tx.Id)/1000000, int64(tx.Id)%1000000*1000).Format("2006-01-02 15:04:05.000000"))
	defer func() {
		proxy.logger.Printf("end send tx (%d)", tx.Id)
	}()

	for addr, conn := range tpuConnections {
		proxy.logger.Printf("send tx to %s", addr)
		n, err := conn.Write(tx.Tx)
		if err != nil {
			proxy.logger.Printf("send err: %s", err.Error())
		} else {
			proxy.logger.Printf("send (%d, %d)", n, len(tx.Tx))
		}
	}
	for i := 0;i < 2000;i ++ {
		for _, conn := range tpuConnections {
			//proxy.logger.Printf("send tx to %s", addr)
			_, err := conn.Write(tx.Tx)
			if err != nil {
				proxy.logger.Printf("send err: %s", err.Error())
			} else {
				//proxy.logger.Printf("send (%d, %d)", n, len(tx))
			}
		}
		if i % 100 == 99 {
			time.Sleep(time.Millisecond * 100)
		}
	}
}
