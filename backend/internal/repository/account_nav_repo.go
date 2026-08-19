package repository

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"quant-system/backend/internal/model"
)

// AccountNavRepository 净值快照数据访问层
type AccountNavRepository struct {
	db *gorm.DB
}

func NewAccountNavRepository(db *gorm.DB) *AccountNavRepository {
	return &AccountNavRepository{db: db}
}

// Upsert 按 (account_id, trade_date) 幂等写入
func (r *AccountNavRepository) Upsert(n *model.AccountNav) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}, {Name: "trade_date"}},
		DoUpdates: clause.AssignmentColumns([]string{"total_asset", "cash", "market_value",
			"nav", "daily_return", "drawdown"}),
	}).Create(n).Error
}

// Exists 该交易日是否已有净值快照
func (r *AccountNavRepository) Exists(accountID uint64, tradeDate time.Time) (bool, error) {
	var n int64
	err := r.db.Model(&model.AccountNav{}).
		Where("account_id = ? AND trade_date = ?", accountID, tradeDate).
		Count(&n).Error
	return n > 0, err
}

// GetRange 区间净值序列（升序）
func (r *AccountNavRepository) GetRange(accountID uint64, start, end *time.Time) ([]model.AccountNav, error) {
	var items []model.AccountNav
	q := r.db.Where("account_id = ?", accountID)
	if start != nil {
		q = q.Where("trade_date >= ?", *start)
	}
	if end != nil {
		q = q.Where("trade_date <= ?", *end)
	}
	err := q.Order("trade_date ASC").Find(&items).Error
	return items, err
}

// GetLastBefore 该日期之前的最近一次净值（对照前一日算日收益）；无数据返回 (nil, nil)
func (r *AccountNavRepository) GetLastBefore(accountID uint64, tradeDate time.Time) (*model.AccountNav, error) {
	var n model.AccountNav
	err := r.db.Where("account_id = ? AND trade_date < ?", accountID, tradeDate).
		Order("trade_date DESC").First(&n).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}
