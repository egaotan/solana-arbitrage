package program

import "github.com/gagliardetto/solana-go"

var (
	SerumV11  = solana.MustPublicKeyFromBase58("4ckmDgGdxQoPDLUkDT3vHgSAkzA3QRdNq5ywwY4sUSJn")
	OrcaV2    = solana.MustPublicKeyFromBase58("9W959DqEETiGZocYWCQPaJ6sBmUzgfxXfqGeTEdp3aQP")
	SerumV22  = solana.MustPublicKeyFromBase58("9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin")
	SerumV12  = solana.MustPublicKeyFromBase58("BJ3jrUzddfuSrZHXSCxMUUQsjKEyLmuuyZebkcaFp2fg")
	OrcaV1    = solana.MustPublicKeyFromBase58("DjVE6JNiYqPL2QXyCUUh8rNjHrbz9hXHNYt99MQ59qw1")
	SerumV21  = solana.MustPublicKeyFromBase58("EUqojwWA2rd19FZrzeBncJsm38Jm1hEhE3zsmX3bRc2o")
	Saber     = solana.MustPublicKeyFromBase58("SSwpkEEcbUqx4vtoEByFjSkhKdCT862DNVb52nZg1UZ")
	TokenSwap = solana.MustPublicKeyFromBase58("SwaPpA9LAaLfeLi3a68M4DjnLqgtticKg6CnyNwgAC8")
	Raydium = solana.MustPublicKeyFromBase58("675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8")
	Token     = solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	System    = solana.MustPublicKeyFromBase58("11111111111111111111111111111111")
	SysClock  = solana.MustPublicKeyFromBase58("SysvarC1ock11111111111111111111111111111111")
	SysRent   = solana.MustPublicKeyFromBase58("SysvarRent111111111111111111111111111111111")
)
var (
	USDT      = solana.MustPublicKeyFromBase58("Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB")
	USDC      = solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	SOL       = solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	Exchange  = solana.MustPublicKeyFromBase58("HhUVfHYvGby6k7zHrAcmA52YQLB7sWD41wkcb1WyUw8Z")
	Arbitrage = solana.MustPublicKeyFromBase58("7H4ShpibmzrKS8yPJX9wi1ZyrRYzw5tLym7RjWvAxcHA")
)
var (
	Orca_Sol_mSol   = solana.MustPublicKeyFromBase58("9EQMEzJdE2LDAY1hw1RytpufdwAXzatYfQ3M2UuT9b88")
	Orca_Sol_Usdt   = solana.MustPublicKeyFromBase58("Dqk7mHQBx2ZWExmyrR2S8X6UG75CrbbpK2FSBZsNYsw6")
	Orca_Sol_Usdc   = solana.MustPublicKeyFromBase58("EGZ7tiLeH62TPV1gL8WwbXGzEPa9zmcpVnnkPKKnrE2U")
	Saber_Sol_mSol  = solana.MustPublicKeyFromBase58("Lee1XZJfJ9Hm2K1qTyeCz1LXNc1YBZaKZszvNY4KCDw")
	Saber_Usdt_Usdc = solana.MustPublicKeyFromBase58("YAkoNb6HKmSxQN9L8hiBE5tPJRsniSSMzND1boHmZxe")
	Serum_Sol_Usdc  = solana.MustPublicKeyFromBase58("9wFFyRfZBsuAha4YcuxcXLKwMxJR43S7fPfQLusDBzvT")
	Serum_Sol_Usdt  = solana.MustPublicKeyFromBase58("HWHvQhFmJB3NUcu1aihKmrKegfVxBEHzwVX6yZCKEsi1")
	Serum_mSol_Usdc = solana.MustPublicKeyFromBase58("6oGsL2puUgySccKzn9XA9afqF217LfxP5ocq4B3LWsjy")
	Serum_mSol_Usdt = solana.MustPublicKeyFromBase58("HxkQdUnrPdHwXP5T9kewEXs3ApgvbufuTfdw9v1nApFd")
)

const (
	AMM       = "AMM"
	OrderBook = "OrderBook"
)

type Callback interface {
	OnModelInit(model Model) error
	OnStateUpdate(slot uint64) error
}

type LocalState struct {
	TokenIn   solana.PublicKey
	AmountIn  uint64
	SlotIn    uint64
	TokenOut  solana.PublicKey
	AmountOut uint64
	SlotOut   uint64
}

type SimulateState struct {
	SourceToken       solana.PublicKey
	SourceAmount      uint64
	DestinationToken  solana.PublicKey
	DestinationAmount uint64
	Txs               []byte
	Logs              []byte
	UnitsConsumed     uint64
	State             []byte
}

type Program interface {
	Start() error
	Stop() error
	Flash() error
	Name() string
	Id() solana.PublicKey
	Type() string
	GetMarket(key solana.PublicKey) Model
	Markets() []Model
	RetrieveState(market solana.PublicKey) (string, error)
	Local(parameter map[string]interface{}) (*LocalState, error)
	//Simulate(parameter map[string]interface{}) (*SimulateState, error)
	ArbitrageStep(parameter map[string]interface{}) ([]solana.Instruction, error)
}
