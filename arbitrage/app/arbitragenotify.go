package app

import (
	"context"
	"fmt"
	"github.com/egaotan/solana-arbitrage/dingsdk"
	"github.com/egaotan/solana-arbitrage/env"
	"github.com/shopspring/decimal"
	"strings"
	"sync"
	"time"
)

type Notify struct {
	ctx  context.Context
	wg   sync.WaitGroup
	env  *env.Env
	data chan *ArbitrageData
	dsdk *dingsdk.DingSdk
}

func NewNotify(ctx context.Context, env *env.Env, dsdk *dingsdk.DingSdk) *Notify {
	bl := &Notify{
		ctx:  ctx,
		env:  env,
		dsdk: dsdk,
		data: make(chan *ArbitrageData, 32),
	}
	return bl
}

func (notify *Notify) Start() {
	notify.wg.Add(1)
	go notify.listen()
}

func (notify *Notify) Commit(data *ArbitrageData) {
	notify.data <- data
}

func (notify *Notify) listen() {
	defer notify.wg.Done()
	for {
		select {
		case data := <-notify.data:
			notify.tryNotify(data)
		case <-notify.ctx.Done():
			return
		}
	}
}

func (notify *Notify) tryNotify(data *ArbitrageData) {
	items := make([]string, 0)
	tt := int64(data.id)
	ttStr := time.Unix(tt/1000000, 0).Format("2006-01-02 15:04:05")
	items = append(items, "arbitrage: ")
	items = append(items, fmt.Sprintf("id: %d;", tt))
	items = append(items, fmt.Sprintf("time: %s;", ttStr))
	items = append(items, fmt.Sprintf("amount: %s;", decimal.NewFromInt(int64(data.amount)).Div(decimal.NewFromInt(1000000)).StringFixed(2)))
	items = append(items, fmt.Sprintf("local yield: %s%%;", decimal.NewFromInt(data.yield).Div(decimal.NewFromInt(100)).StringFixed(2)))
	for _, step := range data.steps {
		tokenIn := notify.env.Token(step.tokenIn)
		tokenOut := notify.env.Token(step.tokenOut)
		amountIn := decimal.NewFromInt(int64(step.amountIn)).Sub(tokenIn.Price).Div(decimal.NewFromInt(int64(tokenIn.Decimal)))
		amountOut := decimal.NewFromInt(int64(step.amountOut)).Sub(tokenOut.Price).Div(decimal.NewFromInt(int64(tokenOut.Decimal)))
		items = append(items, fmt.Sprintf("%s:%s(%s)->%s(%s);", step.program.Name(),
			tokenIn.Symbol, amountIn.StringFixed(2), tokenOut.Symbol, amountOut.StringFixed(2)))
	}
	text := strings.Join(items, "\n")
	dingNotify := &dingsdk.DingNotify{
		MsgType: "text",
		Text: dingsdk.DingContent{
			Content: text,
		},
		At: dingsdk.DingAt{
			IsAtAll: false,
		},
	}
	notify.dsdk.Notify(dingNotify)
}
