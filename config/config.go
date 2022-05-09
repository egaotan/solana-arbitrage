package config

import (
	"github.com/gagliardetto/solana-go"
)

var (
//validYield = int64(-50)
)

var (
	USDC_AMOUNT = uint64(0)
)

var (
	Bomb = 2000
)

const (
	MarketFromChain = iota
	MarketFromConfig
)

var (
	CachePath                    = "./cache/"
	MarketUsableTokensFile       = CachePath + "market_usable_tokens.json"
	MarketUsablePoolAccounts     = CachePath + "market_usable_pool_accounts.json"
	ConfigPath                   = "./config3/"
	TokensFile                   = ConfigPath + "tokens.json"
	UsersFile                    = ConfigPath + "tokens_user.json"
	UsersSimulateFile            = ConfigPath + "tokens_user_simulate.json"
	UsersOwnerFile               = ConfigPath + "users_owner.json"
	UserOwnerSimulateFile        = ConfigPath + "users_owner_simulate.json"
	MarketOpenOrdersFile         = ConfigPath + "markets_openorder.json"
	MarketOpenOrdersSimulateFile = ConfigPath + "markets_openorder_simulate.json"
	MarketsFile                  = ConfigPath + "markets.json"
	ConfigFile                   = ConfigPath + "config.json"
	ValidatorFile                = ConfigPath + "validator.json"
	RandomCaseFile               = ConfigPath + "random_case.json"
	LogPath                      = "./logs/"
	BackendLog                   = "backend"
	ExecutorLog                  = "executor"
	TPULog                       = "tpu_executor"
	NetworkLog                   = "network"
	SentTxHash                   = "sent_tx"
)

type Node struct {
	Rpc    string `json:"rpc"`
	Ws     string `json:"ws"`
	Usable bool   `json:"usable"`
}

type Config struct {
	Nodes               []*Node            `json:"nodes"`
	TransactionNodes    []*Node            `json:"transaction_nodes"`
	BlochHash           []string           `json:"block_hash"`
	TpuClient           []string           `json:"tpu_client"`
	Senders             []*Node            `json:"senders"`
	TransactionNodeSize int                `json:"transaction_node_size"`
	TransactionSend     int                `json:"transaction_send"`
	PreExecute          bool               `json:"pre_execute"`
	NodeId              int                `json:"node_id"`
	Programs            []solana.PublicKey `json:"programs"`
	User                solana.PublicKey   `json:"user"`
	Key                 string             `json:"key"`
	ArbitrageContract   solana.PublicKey   `json:"arbitrage_contract"`
	ExchangeContract    solana.PublicKey   `json:"exchange_contract"`
	Calculators         []string           `json:"calculators"`
	ValidYield          int64              `json:"valid-yield"`
	Which               int                `json:"which"`
	Bomb                int                `json:"bomb"`
	Local               bool               `json:"local"`
	DumpState           bool               `json:"dump_state"`
	DumpLog             bool               `json:"dump_log"`
	NetStatus           bool               `json:"net_status"`
	Simulate            bool               `json:"simulate"`
	WorkSpace           string             `json:"workspace"`
	DingUrl             string             `json:"ding-url"`
	DBUrl               string             `json:"db_url"`
	DBScheme            string             `json:"db_scheme"`
	DBUser              string             `json:"db_user"`
	DBPasswd            string             `json:"db_passwd"`
	Listen              string             `json:"listen"`
	Usdc                uint64             `json:"usdc"`
	UsdcAccount         []string           `json:"usdc_account"`
	TokenA              solana.PublicKey   `json:"token_a"`
	TokenB              solana.PublicKey   `json:"token_b"`
	InstructionSize     int                `json:"instruction_size"`
	InstructionSize2    int                `json:"instruction_size2"`
	RandomTicker        uint64             `json:"random_ticker"`
	USTUSDC             bool               `json:"ust_usdc"`
	SOLUSDC             bool               `json:"sol_usdc"`
	GSTUSDC             bool               `json:"gst_usdc"`
	GMTUSDC             bool               `json:"gmt_usdc"`
}

type Path struct {
	Program solana.PublicKey `json:"program"`
	Market  solana.PublicKey `json:"market"`
	In      solana.PublicKey `json:"in"`
}
type Case struct {
	Amount uint64  `json:"amount"`
	Paths  []*Path `json:"path"`
}
type RandomCase struct {
	Cases []*Case `json:"case"`
}
