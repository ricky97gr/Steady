package model

import "time"

// DailyValuation 每日估值（对应表 daily_valuation，东财日度 PE/PB/市值）
type DailyValuation struct {
	ID        uint64    `gorm:"primaryKey"`
	Code      string    `gorm:"size:10;not null"`
	TradeDate time.Time `gorm:"type:date;not null"`
	Close     float64   `gorm:"type:decimal(10,2)"`
	TotalMv   float64   `gorm:"type:decimal(18,2)"` // 总市值（元）
	FloatMv   float64   `gorm:"type:decimal(18,2)"` // 流通市值（元）
	PeTtm     float64   `gorm:"type:decimal(12,4)"` // 市盈率 TTM
	PeStatic  float64   `gorm:"type:decimal(12,4)"` // 市盈率（静态）
	Pb        float64   `gorm:"type:decimal(12,4)"` // 市净率
}

func (DailyValuation) TableName() string { return "daily_valuation" }
