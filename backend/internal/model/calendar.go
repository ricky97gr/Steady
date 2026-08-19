package model

import "time"

// TradeCalendar 交易日历（对应表 trade_calendar，A股节假日多于周末）
type TradeCalendar struct {
	CalDate  time.Time `gorm:"primaryKey;type:date"`
	IsOpen   bool      `gorm:"not null"`
	Exchange string    `gorm:"size:10;default:SSE"` // SSE / SZSE / BSE
}

func (TradeCalendar) TableName() string { return "trade_calendar" }
