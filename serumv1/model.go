package serumv1

import (
	"encoding/binary"
	"fmt"
	"github.com/egaotan/solana-arbitrage/program"
	"github.com/egaotan/solana-arbitrage/spltoken"
	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
)

type Model struct {
	ProgramId  solana.PublicKey
	Market     *KeyedMarket
	Request    *KeyedRequest
	Event      *KeyedEvent
	Ask        *KeyedOrderBook
	Bid        *KeyedOrderBook
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
	return []solana.PublicKey{m.Market.BaseMint, m.Market.QuoteMint}
}

func (m *Model) PoolPair() []solana.PublicKey {
	return []solana.PublicKey{m.Market.BaseVault, m.Market.QuoteVault}
}

func (m *Model) CurrentSlot() uint64 {
	return m.Market.Height
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

func (m *Model) Price(d uint64) decimal.Decimal {
	bidItems := m.Bid.Slab.items(true)
	askItems := m.Ask.Slab.items(false)
	if len(askItems) < 1 {
		return decimal.NewFromInt(0)
	}
	if len(bidItems) < 1 {
		return decimal.NewFromInt(0)
	}
	askPrice := m.priceLotsToNumber(binary.LittleEndian.Uint64(askItems[0].Key.Id[8:16]))
	bidPrice := m.priceLotsToNumber(binary.LittleEndian.Uint64(bidItems[0].Key.Id[8:16]))
	price := askPrice.Add(bidPrice)
	return price.Div(decimal.NewFromInt(2))
}

func (m *Model) Type() string {
	return program.OrderBook
}

type Tick struct {
	Quantity decimal.Decimal
	Price    decimal.Decimal
}

func (m *Model) bids(size int) []*Tick {
	ticks := make([]*Tick, 0)
	bidItems := m.Bid.Slab.items(true)
	for i, bidItem := range bidItems {
		if i >= size {
			break
		}
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
	ticks := make([]*Tick, 0)
	askItems := m.Ask.Slab.items(false)
	for i, askItem := range askItems {
		if i >= size {
			break
		}
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
	bidItems := m.Bid.Slab.items(true)
	for i, bidItem := range bidItems {
		if i >= size {
			break
		}
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
	askItems := m.Ask.Slab.items(false)
	for i, askItem := range askItems {
		if i >= size {
			break
		}
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
	bidItems := m.Bid.Slab.items(true)
	askItems := m.Ask.Slab.items(false)
	if token == m.Market.BaseMint {
		if len(askItems) < 1 {
			return nil, fmt.Errorf("no ask in this market")
		}
		price := m.priceLotsToNumber(binary.LittleEndian.Uint64(askItems[0].Key.Id[8:16]))
		sourceAmount := m.baseSizeLotsToNumber(askItems[0].Quantity)
		destinationAmount := price.Mul(sourceAmount)
		sr := &program.SwapResult{
			TokenIn:    m.Market.BaseMint,
			AmountIn:   sourceAmount.BigInt().Uint64(),
			TokenOut:   m.Market.QuoteMint,
			AmountOut:  destinationAmount.BigInt().Uint64(),
			NewSwapSrc: m.Market.BaseDepositsTotal,
			NewSwapDst: m.Market.QuoteDepositsTotal,
		}
		return sr, nil
	} else if token == m.Market.QuoteMint {
		if len(bidItems) < 1 {
			return nil, fmt.Errorf("no ask in this market")
		}
		price := m.priceLotsToNumber(binary.LittleEndian.Uint64(bidItems[0].Key.Id[8:16]))
		destinationAmount := m.baseSizeLotsToNumber(bidItems[0].Quantity)
		sourceAmount := price.Mul(destinationAmount)
		sr := &program.SwapResult{
			TokenIn:    m.Market.QuoteMint,
			AmountIn:   sourceAmount.BigInt().Uint64(),
			TokenOut:   m.Market.BaseMint,
			AmountOut:  destinationAmount.BigInt().Uint64(),
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

func (m *Model) priceNumberToLots(price uint64) uint64 {
	dPrice := decimal.NewFromInt(int64(price))
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
