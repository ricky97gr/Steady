package model

import "time"

// 委托状态
const (
	OrderPending   = "PENDING"   // 待成交
	OrderFilled    = "FILLED"    // 全部成交
	OrderPartial   = "PARTIAL"   // 部分成交
	OrderCancelled = "CANCELLED" // 已撤销
	OrderRejected  = "REJECTED"  // 已拒绝
)

// 买卖方向
const (
	DirectionBuy  = "BUY"
	DirectionSell = "SELL"
)

// Order 委托单（对应表 "order"，order 为 SQL 关键字需加引号）
type Order struct {
	ID           uint64    `gorm:"primaryKey"`
	OrderID      string    `gorm:"uniqueIndex;size:36;not null"` // 委托编号（UUID）
	AccountID    uint64    `gorm:"not null"`
	Code         string    `gorm:"size:10;not null"`
	Direction    string    `gorm:"size:10;not null"`                 // BUY / SELL
	OrderType    string    `gorm:"size:10;default:LIMIT"`            // LIMIT / MARKET
	Price        float64   `gorm:"type:decimal(10,2)"`               // 委托价格
	Quantity     int       `gorm:"not null"`                         // 委托数量
	FilledQty    int       `gorm:"default:0"`                        // 已成交数量
	AvgFillPrice float64   `gorm:"type:decimal(10,2);default:0"`     // 成交均价
	Status       string    `gorm:"size:20;default:PENDING"`          // 见上方常量
	Reason       string    `gorm:"type:text"`                        // 委托原因（策略信号）
	Source       string    `gorm:"size:20;default:strategy"`         // 来源（strategy/manual）
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (Order) TableName() string { return `"order"` }

// Trade 成交记录（对应表 trade）
type Trade struct {
	ID         uint64    `gorm:"primaryKey"`
	TradeID    string    `gorm:"uniqueIndex;size:36;not null"` // 成交编号（UUID）
	OrderID    string    `gorm:"size:36;not null"`
	AccountID  uint64    `gorm:"not null"`
	Code       string    `gorm:"size:10;not null"`
	Direction  string    `gorm:"size:10;not null"`
	Price      float64   `gorm:"type:decimal(10,2)"`      // 成交价
	Quantity   int       `gorm:"not null"`                // 成交数量
	Amount     float64   `gorm:"type:decimal(15,2)"`      // 成交金额
	Commission float64   `gorm:"type:decimal(10,2)"`      // 手续费
	Tax        float64   `gorm:"type:decimal(10,2)"`      // 印花税
	NetAmount  float64   `gorm:"type:decimal(15,2)"`      // 净金额
	TradeDate  time.Time `gorm:"type:date;not null"`
	CreatedAt  time.Time
}

func (Trade) TableName() string { return "trade" }
