package model

import (
	"time"
)

// BacktestJob 回测任务（引擎侧消费：pending → running → done/failed）
type BacktestJob struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyName string     `gorm:"size:64;not null;default:multi_factor" json:"strategy_name"`
	StartDate    time.Time  `gorm:"not null" json:"start_date"`
	EndDate      time.Time  `gorm:"not null" json:"end_date"`
	TopN         int        `gorm:"not null;default:20" json:"top_n"`
	Status       string     `gorm:"size:16;not null;default:pending" json:"status"`
	Error        string     `gorm:"type:text" json:"error"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at"`

	// 关联结果（ListJobs/GetDetail 预加载）
	Result *BacktestResult `gorm:"foreignKey:JobID;references:ID" json:"result,omitempty"`
}

// TableName 指定表名
func (BacktestJob) TableName() string { return "backtest_job" }

// BacktestResult 回测结果（nav 为 JSONB 序列：[{"date","nav","benchmark"}]）
type BacktestResult struct {
	JobID            uint64    `gorm:"primaryKey" json:"job_id"`
	TotalReturn      float64   `json:"total_return"`
	AnnualizedReturn float64   `json:"annualized_return"`
	MaxDrawdown      float64   `json:"max_drawdown"`
	Sharpe           float64   `json:"sharpe"`
	TradingDays      int       `json:"trading_days"`
	FinalValue       float64   `json:"final_value"`
	Trades           int       `json:"trades"`
	Positions        int       `json:"positions"`
	BenchmarkReturn  float64   `json:"benchmark_return"`
	ExcessReturn     float64   `json:"excess_return"`
	Nav              string    `gorm:"type:jsonb" json:"nav"` // 原始 JSON 字符串，handler 转结构体
	CreatedAt        time.Time `json:"created_at"`
}

// TableName 指定表名
func (BacktestResult) TableName() string { return "backtest_result" }
