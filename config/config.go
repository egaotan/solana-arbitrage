package config

import "github.com/gagliardetto/solana-go"

var (
	SOL_USDC  = solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U")
	USDC_USER = solana.MustPublicKeyFromBase58("C5dr2fk9zLvjuSjxe3tdvFurcZHKbfU1mHbpdv9Bfr7T")
)

var (
//validYield = int64(-50)
)

var (
	USDC_AMOUNT = uint64(0)
)

const (
	MarketFromChain = iota
	MarketFromConfig
)

var (
	CachePath                    = "./cache/"
	MarketUsableTokensFile       = CachePath + "market_usable_tokens.json"
	MarketUsablePoolAccounts     = CachePath + "market_usable_pool_accounts.json"
	ConfigPath                   = "./config/"
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
	LogPath                      = "./logs/"
	BackendLog                   = "backend"
	ExecutorLog                  = "executor"
	NetworkLog = "network"
)

type Node struct {
	Rpc    string `json:"rpc"`
	Ws     string `json:"ws"`
	Wss    []string `json:"wss"`
	Usable bool   `json:"usable"`
}

type Config struct {
	Nodes            []*Node            `json:"nodes"`
	TransactionNodes []*Node            `json:"transaction_nodes"`
	BlochHash string `json:"block_hash"`
	TransactionNodeSize int `json:"transaction_node_size"`
	NodeId int `json:"node_id"`
	Programs         []solana.PublicKey `json:"programs"`
	User             solana.PublicKey   `json:"user"`
	Key              string             `json:"key"`
	Calculators      []string           `json:"calculators"`
	ValidYield       int64              `json:"valid-yield"`
	Which            int                `json:"which"`
	Local            bool               `json:"local"`
	Simulate         bool               `json:"simulate"`
	WorkSpace        string             `json:"workspace"`
	DingUrl          string             `json:"ding-url"`
	DBUrl            string             `json:"db_url"`
	DBScheme         string             `json:"db_scheme"`
	DBUser           string             `json:"db_user"`
	DBPasswd         string             `json:"db_passwd"`
	Listen           string             `json:"listen"`
	Usdc             uint64             `json:"usdc"`
}
