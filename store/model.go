package store

type LocalArbitrageStep struct {
	Program          string `gorm:"type:varchar(48);not null"`
	Market           string `gorm:"type:varchar(48);not null"`
	TokenIn          string `gorm:"type:varchar(48);not null"`
	AmountIn         uint64 `gorm:"type:bigint(20);not null"`
	SlotIn           uint64 `gorm:"type:bigint(20);not null"`
	TokenOut         string `gorm:"type:varchar(48);not null"`
	AmountOut        uint64 `gorm:"type:bigint(20);not null"`
	SlotOut          uint64 `gorm:"type:bigint(20);not null"`
	LocalArbitrageId uint64 `gorm:"type:bigint(20);not null"`
}

type LocalArbitrage struct {
	Id                  uint64                `gorm:"primaryKey;type:bigint(20);not null"`
	Yield               int64                 `gorm:"type:bigint(20);not null"`
	LocalArbitrageSteps []*LocalArbitrageStep `gorm:"foreignKey:LocalArbitrageId;references:Id"`
}

type CommittedArbitrageStep struct {
	Program              string `gorm:"type:varchar(48);not null"`
	Market               string `gorm:"type:varchar(48);not null"`
	TokenIn              string `gorm:"type:varchar(48);not null"`
	AmountIn             uint64 `gorm:"type:bigint(20);not null"`
	TokenOut             string `gorm:"type:varchar(48);not null"`
	AmountOut        uint64 `gorm:"type:bigint(20);not null"`
	CommittedArbitrageId uint64 `gorm:"type:bigint(20);not null"`
}

type CommittedArbitrage struct {
	Id                      uint64                    `gorm:"primaryKey;type:bigint(20);not null"`
	Amount                  uint64                    `gorm:"type:bigint(20);not null"`
	CommittedArbitrageSteps []*CommittedArbitrageStep `gorm:"foreignKey:CommittedArbitrageId;references:Id"`
}

type ExecutedArbitrage struct {
	Id             uint64 `gorm:"primaryKey;type:bigint(20);not null"`
	ExecuteId      int    `gorm:"primaryKey;type:bigint(20);not null"`
	SendTime       uint64 `gorm:"type:bigint(20);not null"`
	ResponseTime   uint64 `gorm:"type:bigint(20);not null"`
	FinishTime     uint64 `gorm:"type:bigint(20);not null"`
	ExecuteCounter int    `gorm:"type:bigint(20);not null"`
	Signature      string `gorm:"type:varchar(120);not null"`
}
