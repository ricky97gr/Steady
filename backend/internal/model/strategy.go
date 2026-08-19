package model

import (
	"time"

	"gorm.io/datatypes"
)

// 信号动作
const (
	ActionBuy  = "BUY"
	ActionSell = "SELL"
	ActionHold = "HOLD"
)

// FactorDefinition 因子定义（对应表 factor_definition）
type FactorDefinition struct {
	ID          uint64    `gorm:"primaryKey"`
	Name        string    `gorm:"uniqueIndex;size:50;not null"`
	Category    string    `gorm:"size:20"` // trend / value / quality / risk
	Description string    `gorm:"type:text"`
	Formula     string    `gorm:"type:text"`
	Weight      float64   `gorm:"type:decimal(5,4)"`
	CreatedAt   time.Time
}

func (FactorDefinition) TableName() string { return "factor_definition" }

// FactorValue 因子值（对应表 factor_value，横截面排名）
type FactorValue struct {
	ID         uint64    `gorm:"primaryKey"`
	Code       string    `gorm:"size:10;not null"`
	FactorName string    `gorm:"size:50;not null"`
	TradeDate  time.Time `gorm:"type:date;not null"`
	Value      float64   `gorm:"type:decimal(15,6)"`
	Rank       int       // 因子排名
	Normalized float64   `gorm:"type:decimal(8,6)"` // 归一化值（0-1）
}

func (FactorValue) TableName() string { return "factor_value" }

// Strategy 策略（对应表 strategy，factor_weights/params 为 JSONB）
type Strategy struct {
	ID            uint64         `gorm:"primaryKey"`
	Name          string         `gorm:"uniqueIndex;size:50;not null"`
	Description   string         `gorm:"type:text"`
	FactorWeights datatypes.JSON `gorm:"type:jsonb"`
	Params        datatypes.JSON `gorm:"type:jsonb"`
	Status        string         `gorm:"size:10;default:active"`
	CreatedAt     time.Time
}

func (Strategy) TableName() string { return "strategy" }

// StrategySignal 策略信号（对应表 strategy_signal）
type StrategySignal struct {
	ID           uint64    `gorm:"primaryKey"`
	StrategyName string    `gorm:"size:50;not null"`
	Code         string    `gorm:"size:10;not null"`
	TradeDate    time.Time `gorm:"type:date;not null"`
	Score        float64   `gorm:"type:decimal(8,4)"` // 综合评分（0-100）
	Action       string    `gorm:"size:10"`           // BUY / SELL / HOLD
	Reason       string    `gorm:"type:text"`
	CreatedAt    time.Time
}

func (StrategySignal) TableName() string { return "strategy_signal" }
