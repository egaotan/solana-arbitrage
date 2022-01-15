package store

import (
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Dao struct {
	db *gorm.DB
}

func NewDao(url, scheme, user, passwd string) *Dao {
	dao := &Dao{}
	Logger := logger.Default
	Logger = Logger.LogMode(logger.Info)
	db, err := gorm.Open(mysql.Open(user+":"+passwd+"@tcp("+url+")/"+
		scheme+"?charset=utf8"), &gorm.Config{Logger: Logger})
	if err != nil {
		panic(err)
	}
	err = db.Debug().AutoMigrate(&LocalArbitrage{}, &LocalArbitrageStep{}, &CommittedArbitrage{}, &CommittedArbitrageStep{}, &ExecutedArbitrage{})
	if err != nil {
		panic(err)
	}
	dao.db = db
	return dao
}

func (dao *Dao) SaveLocalArbitrage(arb *LocalArbitrage) error {
	return dao.db.Create(arb).Error
}

func (dao *Dao) SaveCommittedArbitrage(arb *CommittedArbitrage) error {
	return dao.db.Create(arb).Error
}

func (dao *Dao) SaveExecutedArbitrage(arb *ExecutedArbitrage) error {
	return dao.db.Create(arb).Error
}

func (dao *Dao) SelectLocalArbitrage(id uint64) ([]*LocalArbitrage, error) {
	localArbitrage := make([]*LocalArbitrage, 0)
	res := dao.db.Where("id = ?", id).Preload("LocalArbitrageSteps").Find(&localArbitrage)
	return localArbitrage, res.Error
}

func (dao *Dao) SelectCommittedArbitrage(id uint64) ([]*CommittedArbitrage, error) {
	committedArbitrage := make([]*CommittedArbitrage, 0)
	res := dao.db.Where("id = ?", id).Preload("CommittedArbitrageSteps").Find(&committedArbitrage)
	return committedArbitrage, res.Error
}

func (dao *Dao) SelectExecutedArbitrage(id uint64) ([]*ExecutedArbitrage, error) {
	executedArbitrage := make([]*ExecutedArbitrage, 0)
	res := dao.db.Where("id = ?", id).Find(&executedArbitrage)
	return executedArbitrage, res.Error
}
