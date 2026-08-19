package model

import "time"

// Account 模拟账户（对应表 account）
type Account struct {
	ID          uint64    `gorm:"primaryKey"`
	Name        string    `gorm:"size:50;default:主账户"`
	Cash        float64   `gorm:"type:decimal(15,2)"` // 可用资金
	TotalAsset  float64   `gorm:"type:decimal(15,2)"` // 总资产
	MarketValue float64   `gorm:"type:decimal(15,2)"` // 持仓市值
	Profit      float64   `gorm:"type:decimal(15,2)"`
	ProfitRate  float64   `gorm:"type:decimal(8,4)"`
	MaxDrawdown float64   `gorm:"type:decimal(8,4)"`
	Status      string    `gorm:"size:10;default:active"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Account) TableName() string { return "account" }

// Position 持仓（对应表 position，available_qty 实现 T+1）
type Position struct {
	ID           uint64    `gorm:"primaryKey"`
	AccountID    uint64    `gorm:"not null"`
	Code         string    `gorm:"size:10;not null"`
	Quantity     int       `gorm:"not null"` // 持仓数量（股）
	AvailableQty int       `gorm:"not null"` // 可用数量（T+1：当日买入不计入）
	CostPrice    float64   `gorm:"type:decimal(10,2)"`
	CurrentPrice float64   `gorm:"type:decimal(10,2)"`
	MarketValue  float64   `gorm:"type:decimal(15,2)"`
	Profit       float64   `gorm:"type:decimal(15,2)"`
	ProfitRate   float64   `gorm:"type:decimal(8,4)"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (Position) TableName() string { return "position" }

// AccountNav 每日净值快照（对应表 account_nav，收益曲线数据源）
type AccountNav struct {
	ID          uint64    `gorm:"primaryKey"`
	AccountID   uint64    `gorm:"not null"`
	TradeDate   time.Time `gorm:"type:date;not null"`
	TotalAsset  float64   `gorm:"type:decimal(15,2)"`
	Cash        float64   `gorm:"type:decimal(15,2)"`
	MarketValue float64   `gorm:"type:decimal(15,2)"`
	Nav         float64   `gorm:"type:decimal(10,6)"`
	DailyReturn float64   `gorm:"type:decimal(8,4)"`
	Drawdown    float64   `gorm:"type:decimal(8,4)"`
	CreatedAt   time.Time
}

func (AccountNav) TableName() string { return "account_nav" }
