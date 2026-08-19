package model

import "time"

// StockBasic 股票基本信息（对应表 stock_basic）
type StockBasic struct {
	Code      string    `gorm:"primaryKey;size:10"`
	Name      string    `gorm:"size:50;not null"`
	Market    string    `gorm:"size:10"`   // SH / SZ / BJ
	Industry  string    `gorm:"size:50"`
	ListDate  time.Time `gorm:"type:date"`
	Status    string    `gorm:"size:10;default:L"` // L=上市 / D=退市
	Universe  string    `gorm:"size:20;index"`     // hs300 / zz500 / NULL
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (StockBasic) TableName() string { return "stock_basic" }

// DailyPrice 日行情（对应表 daily_price）
type DailyPrice struct {
	ID        uint64    `gorm:"primaryKey"`
	Code      string    `gorm:"size:10;not null"`
	TradeDate time.Time `gorm:"type:date;not null"`
	Open      float64   `gorm:"type:decimal(10,2)"`
	High      float64   `gorm:"type:decimal(10,2)"`
	Low       float64   `gorm:"type:decimal(10,2)"`
	Close     float64   `gorm:"type:decimal(10,2)"`
	Volume    int64     // 成交量（手）
	Amount    float64   `gorm:"type:decimal(15,2)"` // 成交额（元）
	AdjFactor float64   `gorm:"type:decimal(10,4)"` // 复权因子
}

func (DailyPrice) TableName() string { return "daily_price" }

// FinancialIndicator 财务指标（对应表 financial_indicator，announce_date 防止未来函数）
type FinancialIndicator struct {
	ID            uint64    `gorm:"primaryKey"`
	Code          string    `gorm:"size:10;not null"`
	ReportDate    time.Time `gorm:"type:date;not null"`
	PE            float64   `gorm:"type:decimal(10,2)"`
	PB            float64   `gorm:"type:decimal(10,2)"`
	ROE           float64   `gorm:"type:decimal(10,4)"`
	ProfitGrowth  float64   `gorm:"type:decimal(10,4)"`
	RevenueGrowth float64   `gorm:"type:decimal(10,4)"`
	DebtRatio     float64   `gorm:"type:decimal(10,4)"`
	GrossMargin   float64   `gorm:"type:decimal(10,4)"`
	AnnounceDate  time.Time `gorm:"type:date;not null"` // 公告日
	CreatedAt     time.Time
}

func (FinancialIndicator) TableName() string { return "financial_indicator" }
