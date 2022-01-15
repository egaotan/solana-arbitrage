package networkdetect

import (
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/egaotan/solana-arbitrage/dingsdk"
	"github.com/egaotan/solana-arbitrage/utils"
	"github.com/go-ping/ping"
	"log"
	"math"
	"strings"
	"time"
)

type NetworkDetector struct {
	peer   string
	ttl    []int64
	avg    []int64
	pinger *ping.Pinger
	logger *log.Logger
	dsdk *dingsdk.DingSdk
}

func NewNetworkDetector(peer string, dsdk *dingsdk.DingSdk) *NetworkDetector {
	index := strings.Index(peer, ":")
	address := peer[index+3:]
	logger := utils.NewLog(config.LogPath, fmt.Sprintf("%s", config.NetworkLog))
	nd := &NetworkDetector{
		peer: address,
		ttl: make([]int64, 0),
		logger: logger,
		dsdk: dsdk,
	}
	return nd
}

func DetectPeers(peers []string) (string, int64) {
	detect := func(peer string) int64 {
		index := strings.Index(peer, ":")
		address := peer[index+3:]
		index = strings.LastIndex(address, ":")
		address = address[:index]
		pinger, err := ping.NewPinger(address)
		if err != nil {
			panic(err)
		}

		pinger.Count = 3
		pinger.Run() // blocks until finished
		stats := pinger.Statistics()
		return stats.AvgRtt.Nanoseconds()
	}
	minttl := int64(math.MaxInt64)
	index := -1
	for i, peer := range peers {
		ttl := detect(peer)
		if ttl < minttl {
			minttl = ttl
			index = i
		}
	}
	return peers[index], minttl
}

func (nd *NetworkDetector) ping() {
	pinger, err := ping.NewPinger(nd.peer)
	if err != nil {
		return
	}
	nd.pinger = pinger
	notifyTime := time.Now().Unix()
	pinger.OnRecv = func(pkt *ping.Packet) {
		nd.ttl = append(nd.ttl, pkt.Rtt.Nanoseconds())
		sum := int64(0)
		for _, x := range nd.ttl {
			sum += x
		}
		avg := sum / int64(len(nd.ttl))
		nd.avg = append(nd.avg, avg)
		if len(nd.ttl) > 300 {
			nd.ttl = nd.ttl[len(nd.ttl) - 300:]
		}
		if len(nd.avg) > 300 {
			nd.avg = nd.avg[len(nd.avg) - 300:]
		}
		isLow := false
		for _, avgx := range nd.avg {
			if avgx < 20 * 1000 * 1000 {
				isLow = true
			}
		}
		xx := time.Now().Unix()
		nd.logger.Printf("ping ttl: %d", avg / 1000000)
		if !isLow {
			nd.logger.Printf("network latenct is too large, restart")
			if xx - notifyTime > 5 * 60 {
				nd.notify(nd.avg[len(nd.avg)-1])
				notifyTime = xx
			}
		}
	}
	pinger.Run()
}

func (nd *NetworkDetector) notify(ttl int64) {
	ttStr := time.Now().Format("2006-01-02 15:04:05")
	dingNotify := &dingsdk.DingNotify{
		MsgType: "text",
		Text: dingsdk.DingContent{
			Content: fmt.Sprintf("arbitrage server network ttl: %d;\ntime: %s;",
				ttl/1000000, ttStr),
		},
		At: dingsdk.DingAt{
			IsAtAll: false,
		},
	}
	nd.dsdk.Notify(dingNotify)
}

func (nd *NetworkDetector) Start() {
	go nd.ping()
}

func (nd *NetworkDetector) Stop() {
	nd.pinger.Stop()
}

