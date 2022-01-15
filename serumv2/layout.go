package serumv2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/badgerodon/collections/stack"
	"github.com/gagliardetto/solana-go"
)

var (
	MarketLayoutSize = 388
)

type AccountFlagLayout struct {
	IsInitialized  uint8
	IsMarket       uint8
	IsOpenOrders   uint8
	IsRequestQueue uint8
	IsEventQueue   uint8
	IsBids         uint8
	IsAsks         uint8
	_              uint8
}

type MarketLayout struct {
	Data1                  [5]byte
	AccountFlag            AccountFlagLayout
	OwnAddress             solana.PublicKey
	VaultSignerNonce       uint64
	BaseToken              solana.PublicKey
	QuoteToken             solana.PublicKey
	BaseVault              solana.PublicKey
	BaseDepositsTotal      uint64
	BaseFeesAccrued        uint64
	QuoteVault             solana.PublicKey
	QuoteDepositsTotal     uint64
	QuoteFeesAccrued       uint64
	QuoteDustThreshold     uint64
	RequestQueue           solana.PublicKey
	EventQueue             solana.PublicKey
	Bids                   solana.PublicKey
	Asks                   solana.PublicKey
	BaseLotSize            uint64
	QuoteLotSize           uint64
	FeeRateBps             uint64
	ReferrerRebatesAccrued uint64
	Data2                  [7]byte
}

type KeyedMarket struct {
	Key    solana.PublicKey
	Height uint64
	MarketLayout
}

type U128 struct {
	Id [16]uint8
}

type Order struct {
	U128
}

type Client struct {
	Id uint64
}

var (
	OpenOrdersLayoutSize = 3228
)

type OpenOrdersLayout struct {
	Data1                  [5]byte
	AccountFlag            AccountFlagLayout
	Market                 solana.PublicKey
	Owner                  solana.PublicKey
	BaseTokenFree          uint64
	BaseTokenTotal         uint64
	QuoteTokenFree         uint64
	QuoteTokenTotal        uint64
	FreeSlotBits           U128
	IsBidBits              U128
	Orders                 [128]Order
	Clients                [128]Client
	ReferrerRebatesAccrued uint64
	Data2                  [7]byte
}

type KeyedOpenOrders struct {
	Key    solana.PublicKey
	Height uint64
	OpenOrdersLayout
}

var (
	SlabHeaderLayoutSize = 32
)

type SlabHeaderLayout struct {
	BumpIndex    uint32
	Zero0        uint32
	FreeListLen  uint32
	Zero1        uint32
	FreeListHead uint32
	Root         uint32
	LeafCount    uint32
	Zero2        uint32
}

var (
	UninitializedNodeType = uint32(0)
	InnerNodeType         = uint32(1)
	LeafNodeType          = uint32(2)
	FreeNodeType          = uint32(3)
	LastFreeNodeType      = uint32(4)
)

var (
	UninitializedNodeSize = 0
	InnerNodeSize         = 28
	LeafNodeSize          = 68
	FreeNodeSize          = 4
	LastFreeNodeSize      = 0
)

type UninitializedNode struct {
}

type InnerNode struct {
	PrefixLen uint32
	Key       U128
	Children  [2]uint32
}

type LeafNode struct {
	OwnerSlot     uint8
	FeeTier       uint8
	Data1         [2]uint8
	Key           U128
	Owner         solana.PublicKey
	Quantity      uint64
	ClientOrderId uint64
}

type FreeNode struct {
	Next uint32
}

type LastFreeNode struct {
}

type SlabNodeLayout struct {
	Tag  uint32
	Node interface{}
}

var (
	SlabNodeNativeLayoutSize = 72
)

/*
type SlabNodeNativeLayout struct {
	Tag uint32
	Nodes [68]uint8
}

*/

type SlabLayout struct {
	Header SlabHeaderLayout
	Nodes  []*SlabNodeLayout
}

type OrderBookLayout struct {
	Data1       [5]byte
	AccountFlag AccountFlagLayout
	Slab        SlabLayout
	Data2       [7]byte
}

func unpackOrderBookLayout(buf []byte, orderBook *OrderBookLayout) error {
	index := 0
	{
		data1Reader := bytes.NewReader(buf[index : index+5])
		err := binary.Read(data1Reader, binary.LittleEndian, &orderBook.Data1)
		if err != nil {
			return err
		}
		index += 5
	}
	{
		accountFlagReader := bytes.NewReader(buf[index : index+8])
		err := binary.Read(accountFlagReader, binary.LittleEndian, &orderBook.AccountFlag)
		if err != nil {
			return err
		}
		index += 8
	}
	{
		slabHeaderReader := bytes.NewReader(buf[index : index+SlabHeaderLayoutSize])
		err := binary.Read(slabHeaderReader, binary.LittleEndian, &orderBook.Slab.Header)
		if err != nil {
			return err
		}
		index += SlabHeaderLayoutSize
	}
	orderBook.Slab.Nodes = make([]*SlabNodeLayout, 0)
	for i := 0; i < int(orderBook.Slab.Header.BumpIndex); i++ {
		slabNode := &SlabNodeLayout{}
		slabNodeTagReader := bytes.NewReader(buf[index : index+4])
		err := binary.Read(slabNodeTagReader, binary.LittleEndian, &slabNode.Tag)
		if err != nil {
			return err
		}
		switch slabNode.Tag {
		case UninitializedNodeType:
			node := &UninitializedNode{}
			nodeReader := bytes.NewReader(buf[index+4 : index+4+UninitializedNodeSize])
			err = binary.Read(nodeReader, binary.LittleEndian, node)
			if err != nil {
				return err
			}
			slabNode.Node = node
		case InnerNodeType:
			node := &InnerNode{}
			nodeReader := bytes.NewReader(buf[index+4 : index+4+InnerNodeSize])
			err = binary.Read(nodeReader, binary.LittleEndian, node)
			if err != nil {
				return err
			}
			slabNode.Node = node
		case LeafNodeType:
			node := &LeafNode{}
			nodeReader := bytes.NewReader(buf[index+4 : index+4+LeafNodeSize])
			err = binary.Read(nodeReader, binary.LittleEndian, node)
			if err != nil {
				return err
			}
			slabNode.Node = node
		case FreeNodeType:
			node := &FreeNode{}
			nodeReader := bytes.NewReader(buf[index+4 : index+4+FreeNodeSize])
			err = binary.Read(nodeReader, binary.LittleEndian, node)
			if err != nil {
				return err
			}
			slabNode.Node = node
		case LastFreeNodeType:
			node := &LastFreeNode{}
			nodeReader := bytes.NewReader(buf[index+4 : index+4+LastFreeNodeSize])
			err = binary.Read(nodeReader, binary.LittleEndian, node)
			if err != nil {
				return err
			}
			slabNode.Node = node
		default:
			return fmt.Errorf("unknow node in order book slab node")
		}
		orderBook.Slab.Nodes = append(orderBook.Slab.Nodes, slabNode)
		index += SlabNodeNativeLayoutSize
	}
	{
		data2Reader := bytes.NewReader(buf[index : index+7])
		err := binary.Read(data2Reader, binary.LittleEndian, &orderBook.Data2)
		if err != nil {
			return err
		}
		index += 7
	}
	return nil
}

type KeyedOrderBook struct {
	Key    solana.PublicKey
	Height uint64
	OrderBookLayout
}

type RequestFlagLayout struct {
	NewOrder    uint8
	CancelOrder uint8
	Bid         uint8
	PostOnly    uint8
	Ioc         uint8
	_           uint8
	_           uint8
	_           uint8
}

var (
	RequestNodeLayoutSize = 87
)

type RequestNodeLayout struct {
	RequestFlag               RequestFlagLayout
	OpenOrdersSlot            uint8
	FeeTier                   uint8
	Data1                     [5]byte
	MaxBaseSizeOrCancelId     uint64
	NativeQuoteQuantityLocked uint64
	Order                     Order
	OpenOrders                solana.PublicKey
	ClientOrderId             uint64
}

var (
	RequestHeaderLayoutSize = 37
)

type RequestHeaderLayout struct {
	Data1       [5]byte
	AccountFlag AccountFlagLayout
	Head        uint32
	Zero0       uint32
	Count       uint32
	Zero1       uint32
	NextSeqNum  uint32
	Zero2       uint32
}

type RequestLayout struct {
	Header RequestHeaderLayout
	Nodes  []*RequestNodeLayout
}

func unpackRequestLayout(buf []byte, request *RequestLayout) error {
	{
		requestHeaderReader := bytes.NewReader(buf[0:RequestHeaderLayoutSize])
		err := binary.Read(requestHeaderReader, binary.LittleEndian, &request.Header)
		if err != nil {
			return err
		}
	}
	nodeSize := (len(buf) - RequestHeaderLayoutSize) / RequestNodeLayoutSize
	request.Nodes = make([]*RequestNodeLayout, 0)
	for i := 0; i < int(request.Header.Count); i++ {
		index := (int(request.Header.Head) + i) % nodeSize
		start := RequestHeaderLayoutSize + RequestNodeLayoutSize*index
		requestNode := &RequestNodeLayout{}
		requestReader := bytes.NewReader(buf[start : start+RequestNodeLayoutSize])
		err := binary.Read(requestReader, binary.LittleEndian, requestNode)
		if err != nil {
			return err
		}
		request.Nodes = append(request.Nodes, requestNode)
	}
	return nil
}

type KeyedRequest struct {
	Key    solana.PublicKey
	Height uint64
	RequestLayout
}

type EventFlagLayout struct {
	Fill  uint8
	Out   uint8
	Bid   uint8
	Maker uint8
	_     uint8
	_     uint8
	_     uint8
	_     uint8
}

var (
	EventNodeLayoutSize = 95
)

type EventNodeLayout struct {
	EventFlag              EventFlagLayout
	OpenOrdersSlot         uint8
	FeeTier                uint8
	Data1                  [5]byte
	NativeQuantityReleased uint64
	NativeQuantityPaid     uint64
	NativeFeeOrRebate      uint64
	Order                  Order
	OpenOrders             solana.PublicKey
	ClientOrderId          uint64
}

var (
	EventHeaderLayoutSize = 37
)

type EventHeaderLayout struct {
	Data1       [5]byte
	AccountFlag AccountFlagLayout
	Head        uint32
	Zero0       uint32
	Count       uint32
	Zero1       uint32
	SeqNum      uint32
	Zero2       uint32
}

type EventLayout struct {
	Header EventHeaderLayout
	Nodes  []*EventNodeLayout
}

func unpackEventLayout(buf []byte, event *EventLayout) error {
	{
		eventHeaderReader := bytes.NewReader(buf[0:EventHeaderLayoutSize])
		err := binary.Read(eventHeaderReader, binary.LittleEndian, &event.Header)
		if err != nil {
			return err
		}
	}
	nodeSize := (len(buf) - EventHeaderLayoutSize) / EventNodeLayoutSize
	event.Nodes = make([]*EventNodeLayout, 0)
	for i := 0; i < int(event.Header.Count); i++ {
		index := (int(event.Header.Head) + i) % nodeSize
		start := EventHeaderLayoutSize + EventNodeLayoutSize*index
		eventNode := &EventNodeLayout{}
		eventNodeReader := bytes.NewReader(buf[start : start+EventNodeLayoutSize])
		err := binary.Read(eventNodeReader, binary.LittleEndian, eventNode)
		if err != nil {
			return err
		}
		event.Nodes = append(event.Nodes, eventNode)
	}
	return nil
}

type KeyedEvent struct {
	Key    solana.PublicKey
	Height uint64
	EventLayout
}

type SerumMarketV3Layout struct {
	Options1               [5]byte
	IsInitialized          uint8
	IsMarket               uint8
	IsOpenOrders           uint8
	IsRequestQueue         uint8
	IsEventQueue           uint8
	IsBids                 uint8
	IsAsks                 uint8
	_                      uint8
	OwnAddress             solana.PublicKey
	VaultSignerNonce       uint64
	BaseMint               solana.PublicKey
	QuoteMint              solana.PublicKey
	BaseVault              solana.PublicKey
	BaseDepositsTotal      uint64
	BaseFeesAccrued        uint64
	QuotaValue             solana.PublicKey
	QuoteDepositsTotal     uint64
	QuoteFeesAccrued       uint64
	QuoteDustThreshold     uint64
	RequestQueue           solana.PublicKey
	EventQueue             solana.PublicKey
	Bids                   solana.PublicKey
	Asks                   solana.PublicKey
	BaseLotSize            uint64
	QuoteLotSize           uint64
	FeeRateBps             uint64
	ReferrerRebatesAccrued uint64
	OpenOrdersAuthority    solana.PublicKey
	PruneAuthority         solana.PublicKey
	ConsumeEventsAuthority solana.PublicKey
	Data1                  [992]byte
	Data2                  [7]byte
}

func (slab *SlabLayout) items(bids bool, size int) []*LeafNode {
	leaves := make([]*LeafNode, 0)
	if slab.Header.LeafCount == 0 {
		return nil
	}
	stack := stack.New()
	stack.Push(slab.Header.Root)
	for stack.Len() > 0 {
		index := stack.Pop().(uint32)
		node := slab.Nodes[index]
		if node.Tag == LeafNodeType {
			leafNode := node.Node.(*LeafNode)
			leaves = append(leaves, leafNode)
			if len(leaves) >= size {
				return leaves
			}
		} else if node.Tag == InnerNodeType {
			innerNode := node.Node.(*InnerNode)
			if bids {
				stack.Push(innerNode.Children[1])
				stack.Push(innerNode.Children[0])
			} else {
				stack.Push(innerNode.Children[0])
				stack.Push(innerNode.Children[1])
			}
		}
	}
	return leaves
}
