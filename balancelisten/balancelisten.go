package balancelisten

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/dingsdk"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	"sync"
	"time"
)

var (
	gBalance []uint64
)

type BalanceListen struct {
	ctx         context.Context
	wg          sync.WaitGroup
	spltoken    *spltoken.Program
	usdcAccount []string
	dsdk        *dingsdk.DingSdk
}

func NewBalanceListen(ctx context.Context, spltoken *spltoken.Program, usdcAccount []string, dsdk *dingsdk.DingSdk) *BalanceListen {
	bl := &BalanceListen{
		ctx:         ctx,
		spltoken:    spltoken,
		usdcAccount: usdcAccount,
		dsdk:        dsdk,
	}
	return bl
}

func (bl *BalanceListen) Start() {
	bl.wg.Add(1)
	go bl.AccountBalance()
}

func (bl *BalanceListen) AccountBalance() {
	defer bl.wg.Done()
	accounts := make([]solana.PublicKey, 0)
	for _, item := range bl.usdcAccount {
		acc := solana.MustPublicKeyFromBase58(item)
		accounts = append(accounts, acc)
	}
	timer2 := time.NewTicker(time.Second * 10)
	for {
		select {
		case <-timer2.C:
			balances, err := bl.spltoken.GetBalances(accounts)
			if err != nil {
				continue
			}
			if balances == nil {
				continue
			}
			bl.notify(balances)
		case <-bl.ctx.Done():
			return
		}
	}
}

func (bl *BalanceListen) notify(balances []uint64) {
	size := len(balances)
	for len(gBalance) < size {
		gBalance = append(gBalance, 0)
	}
	update := false
	for i := 0;i < size;i ++ {
		if balances[i] != gBalance[i] {
			update = true
			break
		}
	}
	if !update {
		return
	}
	context := "arbitrage account balance update: \n"
	for i := 0;i < size;i ++ {
		oldBalance := decimal.NewFromInt(int64(gBalance[i])).Div(decimal.NewFromInt(1000000))
		newBalance := decimal.NewFromInt(int64(balances[i])).Div(decimal.NewFromInt(1000000))
		ttStr := time.Now().Format("2006-01-02 15:04:05")
		diff := decimal.NewFromInt(0)
		if gBalance[i] != uint64(0) {
			diff = newBalance.Sub(oldBalance)
		}
		context = context + fmt.Sprintf("%s -> %s (%s);\ntime: %s;",
			oldBalance.StringFixed(2), newBalance.StringFixed(2),
			diff.StringFixed(2), ttStr)
	}
	dingNotify := &dingsdk.DingNotify{
		MsgType: "text",
		Text: dingsdk.DingContent{
			Content: context,
		},
		At: dingsdk.DingAt{
			IsAtAll: false,
		},
	}
	bl.dsdk.Notify(dingNotify)
	for i := 0;i < size;i ++ {
		gBalance[i] = balances[i]
	}
}
