package repository

import (
	"database/sql"
	"time"

	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// SignalRepository 策略与信号数据访问层（只读）
type SignalRepository struct {
	db *gorm.DB
}

func NewSignalRepository(db *gorm.DB) *SignalRepository {
	return &SignalRepository{db: db}
}

// SignalItem 信号 + 股票名称（join stock_basic）
type SignalItem struct {
	Code   string
	Name   string
	Score  float64
	Action string
	Reason string
}

// GetStrategies 活跃策略列表
func (r *SignalRepository) GetStrategies() ([]model.Strategy, error) {
	var items []model.Strategy
	err := r.db.Where("status = ?", "active").Find(&items).Error
	return items, err
}

// GetStrategy 按名称查询策略（ExecuteDay 读取 top_n / max_position_pct），不存在返回 (nil, nil)
func (r *SignalRepository) GetStrategy(name string) (*model.Strategy, error) {
	var s model.Strategy
	err := r.db.Where("name = ?", name).First(&s).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetLatestSignalDate 策略最近一次信号日期；无信号返回 (nil, nil)
// 注意：MAX 在空表上返回 NULL，需 Scan 进 sql.NullTime（time.Time 无法接收 NULL）
func (r *SignalRepository) GetLatestSignalDate(strategy string) (*time.Time, error) {
	var t sql.NullTime
	err := r.db.Table("strategy_signal").
		Select("MAX(trade_date)").
		Where("strategy_name = ?", strategy).
		Scan(&t).Error
	if err != nil {
		return nil, err
	}
	if !t.Valid {
		return nil, nil
	}
	return &t.Time, nil
}

// GetSignals 指定策略+日期的信号（评分降序；date 零值表示不限定；action 可选过滤）
func (r *SignalRepository) GetSignals(strategy string, date time.Time,
	action string, limit int) ([]SignalItem, error) {
	q := r.db.Table("strategy_signal").
		Select("strategy_signal.code, stock_basic.name, strategy_signal.score, "+
			"strategy_signal.action, strategy_signal.reason").
		Joins("LEFT JOIN stock_basic ON stock_basic.code = strategy_signal.code").
		Where("strategy_signal.strategy_name = ?", strategy)
	if !date.IsZero() {
		q = q.Where("strategy_signal.trade_date = ?", date)
	}
	if action != "" {
		q = q.Where("strategy_signal.action = ?", action)
	}
	var items []SignalItem
	err := q.Order("strategy_signal.score DESC").Limit(limit).Scan(&items).Error
	return items, err
}

// GetSignalsByCode 个股信号历史（按日期倒序，limit 条）
func (r *SignalRepository) GetSignalsByCode(code string, limit int) ([]model.StrategySignal, error) {
	var items []model.StrategySignal
	err := r.db.Where("code = ?", code).
		Order("trade_date DESC, id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}
