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
	gBalance uint64
)

type BalanceListen struct {
	ctx         context.Context
	wg          sync.WaitGroup
	spltoken    *spltoken.Program
	usdcAccount string
	dsdk        *dingsdk.DingSdk
}

func NewBalanceListen(ctx context.Context, spltoken *spltoken.Program, usdcAccount string, dsdk *dingsdk.DingSdk) *BalanceListen {
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
	usdcAccount := solana.MustPublicKeyFromBase58(bl.usdcAccount)
	timer2 := time.NewTicker(time.Second * 10)
	for {
		select {
		case <-timer2.C:
			balance, err := bl.spltoken.GetBalance(usdcAccount)
			if err != nil {
				continue
			}
			bl.notify(balance)
		case <-bl.ctx.Done():
			return
		}
	}
}

func (bl *BalanceListen) notify(balance uint64) {
	if balance == gBalance {
		return
	}
	oldBalance := decimal.NewFromInt(int64(gBalance)).Div(decimal.NewFromInt(1000000))
	newBalance := decimal.NewFromInt(int64(balance)).Div(decimal.NewFromInt(1000000))
	ttStr := time.Now().Format("2006-01-02 15:04:05")
	diff := decimal.NewFromInt(0)
	if gBalance != uint64(0) {
		diff = newBalance.Sub(oldBalance)
	}
	dingNotify := &dingsdk.DingNotify{
		MsgType: "text",
		Text: dingsdk.DingContent{
			Content: fmt.Sprintf("arbitrage account balance update: %s -> %s (%s);\ntime: %s;",
				oldBalance.StringFixed(2), newBalance.StringFixed(2),
				diff.StringFixed(2), ttStr),
		},
		At: dingsdk.DingAt{
			IsAtAll: false,
		},
	}
	bl.dsdk.Notify(dingNotify)
	gBalance = balance
}
