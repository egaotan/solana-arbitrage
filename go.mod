module github.com/egaotan/solana-arbitrage

go 1.16

require (
	github.com/badgerodon/collections v0.0.0-20130729185459-604e922904d3
	github.com/gagliardetto/solana-go v1.0.2
	github.com/gin-gonic/gin v1.7.7
	github.com/go-ping/ping v0.0.0-20211130115550-779d1e919534
	github.com/shopspring/decimal v1.3.1
	github.com/stretchr/testify v1.7.0 // indirect
	golang.org/x/net v0.0.0-20210510120150-4163338589ed
	gorm.io/driver/mysql v1.2.2
	gorm.io/gorm v1.22.4
)

replace github.com/gagliardetto/solana-go v1.0.2 => github.com/egaotan/solana-go v1.0.4
