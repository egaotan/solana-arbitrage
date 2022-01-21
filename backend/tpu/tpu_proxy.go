package tpu

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

type Proxy struct {
	ctx          context.Context
	client       *rpc.Client
	curSlot      uint64
	ans          *AvailableNodesService
	lss          *LeaderScheduleService
	tpuConns     map[string]net.Conn
	latestSlots  chan uint64
	transactions chan []byte
	lock         int32
	logger *log.Logger
}

func NewLog(dir, name string) *log.Logger {
	fileName := fmt.Sprintf("%s%s.log", dir, name)
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	log := log.New(file, "", log.LstdFlags|log.Lmicroseconds)
	return log
}

func NewProxy(ctx context.Context, client *rpc.Client) *Proxy {
	proxy := &Proxy{
		ctx:          ctx,
		client:       client,
		latestSlots:  make(chan uint64, 1024),
		transactions: make(chan []byte, 1024),
		tpuConns: make(map[string]net.Conn),
	}
	proxy.logger = NewLog("./", "tpu_proxy")
	proxy.ans = NewAvailableNodesService(proxy.ctx, proxy.client, proxy.logger)
	proxy.lss = NewLeaderScheduleService(proxy.ctx, proxy.client, proxy.logger)
	return proxy
}

func (proxy *Proxy) Start() {
	proxy.ans.Start()
	proxy.lss.Start()
	go proxy.newSlot()
	go proxy.SendTransactions()
}

func (proxy *Proxy) RefreshConnection() {
	startSlot := proxy.curSlot - PAST_SLOT_SEARCH
	endSlot := proxy.curSlot + UPCOMING_SLOT_SEARCH
	leaderAddress := make(map[solana.PublicKey]bool)
	tpuAddress := make(map[string]uint64)
	for slot := startSlot; slot < endSlot; slot++ {
		leader := proxy.lss.GetSlotLeader(slot)
		if !leader.IsZero() && !leaderAddress[leader] {
			leaderAddress[leader] = true
			tpu := proxy.ans.GetNode(leader)
			if tpu != "" {
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
		con, err := net.Dial("UDP", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			proxy.logger.Printf("cannot dial udp, address: %s, slot: %d", tpu, slot)
			continue
		}
		tpuConnctions[tpu] = con
	}
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

func (proxy *Proxy) CommitTransaction(tx []byte) {
	proxy.transactions <- tx
}

func (proxy *Proxy) SendTransactions() {
	for {
		select {
		case tx := <-proxy.transactions:
			proxy.SendTransaction(tx)
		}
	}
}

func (proxy *Proxy) SendTransaction(tx []byte) {
	proxy.logger.Printf("send transaction......")
	for !atomic.CompareAndSwapInt32(&proxy.lock, 0, 1) {
		continue
	}
	tpuConnections := proxy.tpuConns
	atomic.StoreInt32(&proxy.lock, 0)
	for _, conn := range tpuConnections {
		conn.Write(tx)
	}
}
