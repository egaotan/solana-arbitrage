package serumv2

import (
	"encoding/binary"
	"fmt"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
)

type Tick struct {
	Quantity decimal.Decimal
	Price    decimal.Decimal
}

func (t *Tick) Copy(src *Tick) {
	t.Quantity = src.Quantity
	t.Price = src.Price
}

type Model struct {
	ProgramId solana.PublicKey
	Market    *KeyedMarket
	//Request    *KeyedRequest
	//Event      *KeyedEvent
	Ask        *KeyedOrderBook
	AskTick0   [5]Tick
	Bid        *KeyedOrderBook
	BidTick0   [5]Tick
	BaseVault  *spltoken.KeyedUser
	QuoteVault *spltoken.KeyedUser
	States     map[string]interface{}
}

func (m *Model) Program() solana.PublicKey {
	return m.ProgramId
}

func (m *Model) Id() solana.PublicKey {
	return m.Market.Key
}

func (m *Model) TokenPair() []solana.PublicKey {
	return []solana.PublicKey{m.Market.BaseToken, m.Market.QuoteToken}
}

func (m *Model) PoolPair() []solana.PublicKey {
	return []solana.PublicKey{m.Market.BaseVault, m.Market.QuoteVault}
}

func (m *Model) CurrentSlot() uint64 {
	return m.Ask.Height
}

func (m *Model) SetState(key string, value interface{}) error {
	m.States[key] = value
	return nil
}

func (m *Model) HasState(key string) bool {
	if _, ok := m.States[key]; ok {
		return true
	}
	return false
}

func (m *Model) State(key string) interface{} {
	if item, ok := m.States[key]; ok {
		return item
	}
	return nil
}

func (m *Model) Type() string {
	return program.OrderBook
}

func (m *Model) bids(size int) []*Tick {
	ticks := make([]*Tick, 0, size)
	bidItems := m.Bid.Slab.items(true, size)
	for _, bidItem := range bidItems {
		price := m.priceLotsToNumber(binary.LittleEndian.Uint64(bidItem.Key.Id[8:16]))
		quantity := m.baseSizeLotsToNumber(bidItem.Quantity)
		ticks = append(ticks, &Tick{
			Quantity: quantity,
			Price:    price,
		})
	}
	return ticks
}

func (m *Model) asks(size int) []*Tick {
	ticks := make([]*Tick, 0, size)
	askItems := m.Ask.Slab.items(false, size)
	for _, askItem := range askItems {
		price := m.priceLotsToNumber(binary.LittleEndian.Uint64(askItem.Key.Id[8:16]))
		quantity := m.baseSizeLotsToNumber(askItem.Quantity)
		ticks = append(ticks, &Tick{
			Quantity: quantity,
			Price:    price,
		})
	}
	return ticks
}

func (m *Model) bidsRaw(size int) []*Tick {
	ticks := make([]*Tick, 0)
	bidItems := m.Bid.Slab.items(true, 1)
	for _, bidItem := range bidItems {
		price := binary.LittleEndian.Uint64(bidItem.Key.Id[8:16])
		quantity := bidItem.Quantity
		ticks = append(ticks, &Tick{
			Quantity: decimal.NewFromInt(int64(quantity)),
			Price:    decimal.NewFromInt(int64(price)),
		})
	}
	return ticks
}

func (m *Model) asksRaw(size int) []*Tick {
	ticks := make([]*Tick, 0)
	askItems := m.Ask.Slab.items(false, 1)
	for _, askItem := range askItems {
		price := binary.LittleEndian.Uint64(askItem.Key.Id[8:16])
		quantity := askItem.Quantity
		ticks = append(ticks, &Tick{
			Quantity: decimal.NewFromInt(int64(quantity)),
			Price:    decimal.NewFromInt(int64(price)),
		})
	}
	return ticks
}

func (m *Model) Swap(token solana.PublicKey, amount uint64) (*program.SwapResult, error) {
	if token == m.Market.BaseToken {
		if m.AskTick0[0].Quantity.IsZero() {
			return nil, fmt.Errorf("no ask in this market")
		}
		sourceAmount := decimal.NewFromInt(0)
		destinationAmount := decimal.NewFromInt(0)
		for i := 0; i < 5; i++ {
			sourceAmount = sourceAmount.Add(m.AskTick0[i].Quantity)
			destinationAmount = destinationAmount.Add(m.AskTick0[i].Price.Mul(m.AskTick0[i].Quantity))
			usdcAmount := sourceAmount.Mul(decimal.NewFromInt(int64(amount))).Div(decimal.NewFromInt(1000000)).BigInt().Uint64()
			if usdcAmount > 500*1000000 {
				break
			}
		}
		// fee
		//destinationAmount = destinationAmount.Mul(decimal.NewFromFloat(0.998))
		sr := &program.SwapResult{
			TokenIn:    m.Market.BaseToken,
			AmountIn:   sourceAmount.BigInt().Uint64(),
			SlotIn:     m.Ask.Height,
			TokenOut:   m.Market.QuoteToken,
			AmountOut:  destinationAmount.BigInt().Uint64(),
			SlotOut:    m.Bid.Height,
			NewSwapSrc: m.Market.BaseDepositsTotal,
			NewSwapDst: m.Market.QuoteDepositsTotal,
		}
		return sr, nil
	} else if token == m.Market.QuoteToken {
		if m.BidTick0[0].Quantity.IsZero() {
			return nil, fmt.Errorf("no bid in this market")
		}
		destinationAmount := decimal.NewFromInt(0)
		sourceAmount := decimal.NewFromInt(0)
		for i := 0; i < 5; i++ {
			destinationAmount = destinationAmount.Add(m.BidTick0[i].Quantity)
			sourceAmount = sourceAmount.Add(m.BidTick0[i].Price.Mul(m.BidTick0[i].Quantity))
			usdcAmount := sourceAmount.Mul(decimal.NewFromInt(int64(amount))).Div(decimal.NewFromInt(1000000)).BigInt().Uint64()
			if usdcAmount > 500*1000000 {
				break
			}
		}
		// fee
		//sourceAmount = sourceAmount.Mul(decimal.NewFromFloat(1.002))
		sr := &program.SwapResult{
			TokenIn:    m.Market.QuoteToken,
			AmountIn:   sourceAmount.BigInt().Uint64(),
			SlotIn:     m.Bid.Height,
			TokenOut:   m.Market.BaseToken,
			AmountOut:  destinationAmount.BigInt().Uint64(),
			SlotOut:    m.Ask.Height,
			NewSwapSrc: m.Market.QuoteDepositsTotal,
			NewSwapDst: m.Market.BaseDepositsTotal,
		}
		return sr, nil
	} else {
		return nil, fmt.Errorf("token is not in this market")
	}
}

func (m *Model) Swap1(token solana.PublicKey, amount uint64) (*program.SwapResult, error) {
	if token == m.Market.BaseToken {
		if m.AskTick0[0].Quantity.IsZero() {
			return nil, fmt.Errorf("no ask in this market")
		}

		leftAmount := decimal.NewFromInt(int64(amount))
		gotAmount := decimal.NewFromInt(0)
		for i := 0; i < 5; i++ {
			if leftAmount.Cmp(m.AskTick0[i].Quantity) <= 0 {
				gotAmount = gotAmount.Add(leftAmount.Mul(m.AskTick0[i].Price))
				leftAmount = decimal.NewFromInt(0)
				break
			} else {
				gotAmount = gotAmount.Add(m.AskTick0[i].Quantity.Mul(m.AskTick0[i].Price))
				leftAmount = leftAmount.Sub(m.AskTick0[i].Quantity)
			}
		}
		// fee
		//destinationAmount = destinationAmount.Mul(decimal.NewFromFloat(0.998))
		sr := &program.SwapResult{
			TokenIn:    m.Market.BaseToken,
			AmountIn:   amount,
			SlotIn:     m.Ask.Height,
			TokenOut:   m.Market.QuoteToken,
			AmountOut:  gotAmount.BigInt().Uint64(),
			SlotOut:    m.Bid.Height,
			NewSwapSrc: m.Market.BaseDepositsTotal,
			NewSwapDst: m.Market.QuoteDepositsTotal,
		}
		return sr, nil
	} else if token == m.Market.QuoteToken {
		if m.BidTick0[0].Quantity.IsZero() {
			return nil, fmt.Errorf("no bid in this market")
		}

		leftAmount := decimal.NewFromInt(int64(amount))
		gotAmount := decimal.NewFromInt(0)
		for i := 0; i < 5; i++ {
			tickQ := m.BidTick0[i].Price.Mul(m.BidTick0[i].Quantity)
			if tickQ.Cmp(leftAmount) >= 0 {
				gotAmount = gotAmount.Add(leftAmount.Div(m.BidTick0[i].Price))
				leftAmount = decimal.NewFromInt(0)
				break
			} else {
				gotAmount = gotAmount.Add(m.BidTick0[i].Quantity)
				leftAmount = leftAmount.Sub(tickQ)
			}
		}
		// fee
		//sourceAmount = sourceAmount.Mul(decimal.NewFromFloat(1.002))
		sr := &program.SwapResult{
			TokenIn:    m.Market.QuoteToken,
			AmountIn:   amount,
			SlotIn:     m.Bid.Height,
			TokenOut:   m.Market.BaseToken,
			AmountOut:  gotAmount.BigInt().Uint64(),
			SlotOut:    m.Ask.Height,
			NewSwapSrc: m.Market.QuoteDepositsTotal,
			NewSwapDst: m.Market.BaseDepositsTotal,
		}
		return sr, nil
	} else {
		return nil, fmt.Errorf("token is not in this market")
	}
}

func (m *Model) priceLotsToNumber(price uint64) decimal.Decimal {
	dPrice := decimal.NewFromInt(int64(price))
	dPrice = dPrice.Mul(decimal.NewFromInt(int64(m.Market.QuoteLotSize)))
	dPrice = dPrice.Div(decimal.NewFromInt(int64(m.Market.BaseLotSize)))
	return dPrice
}

func (m *Model) baseSizeLotsToNumber(quantity uint64) decimal.Decimal {
	dQuantity := decimal.NewFromInt(int64(quantity))
	dQuantity = dQuantity.Mul(decimal.NewFromInt(int64(m.Market.BaseLotSize)))
	return dQuantity
}

func (m *Model) priceNumberToLots(price decimal.Decimal) uint64 {
	dPrice := price
	dPrice = dPrice.Mul(decimal.NewFromInt(int64(m.Market.BaseLotSize)))
	dPrice = dPrice.Div(decimal.NewFromInt(int64(m.Market.QuoteLotSize)))
	return dPrice.BigInt().Uint64()
}

func (m *Model) baseSizeNumberToLots(quantity uint64) uint64 {
	dQuantity := decimal.NewFromInt(int64(quantity))
	dQuantity = dQuantity.Div(decimal.NewFromInt(int64(m.Market.BaseLotSize)))
	return dQuantity.BigInt().Uint64()
}

func priceNumberToUi(price decimal.Decimal, baseTokenMultiplier, quoteTokenMultiplier uint64) decimal.Decimal {
	dPrice := price.Mul(decimal.NewFromInt(int64(baseTokenMultiplier)))
	dPrice = dPrice.Div(decimal.NewFromInt(int64(quoteTokenMultiplier)))
	return dPrice
}

func baseSizeNumberToUi(quantity decimal.Decimal, baseTokenMultiplier, quoteTokenMultiplier uint64) decimal.Decimal {
	dQuantity := quantity.Div(decimal.NewFromInt(int64(baseTokenMultiplier)))
	return dQuantity
}

func priceUiToNumber(price decimal.Decimal, baseTokenMultiplier, quoteTokenMultiplier uint64) decimal.Decimal {
	dPrice := price.Mul(decimal.NewFromInt(int64(quoteTokenMultiplier)))
	dPrice = dPrice.Div(decimal.NewFromInt(int64(baseTokenMultiplier)))
	return dPrice
}

func (m *Model) getMarketAccounts(token solana.PublicKey) (solana.PublicKey, solana.PublicKey, error) {
	if token == m.Market.BaseToken {
		return m.Market.BaseToken, m.Market.QuoteToken, nil
	} else if token == m.Market.QuoteToken {
		return m.Market.QuoteToken, m.Market.BaseToken, nil
	} else {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("token is not the swap token pair - (%s %s)", m.Market.Key, token)
	}
}
