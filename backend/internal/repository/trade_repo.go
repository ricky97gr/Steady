package repository

import (
	"time"

	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// TradeRepository 成交数据访问层
type TradeRepository struct {
	db *gorm.DB
}

func NewTradeRepository(db *gorm.DB) *TradeRepository {
	return &TradeRepository{db: db}
}

// Create 写入成交记录
func (r *TradeRepository) Create(t *model.Trade) error {
	return r.db.Create(t).Error
}

// GetList 分页查询成交列表（按 id 倒序）
func (r *TradeRepository) GetList(accountID uint64, page, pageSize int) ([]model.Trade, int64, error) {
	var items []model.Trade
	var total int64

	q := r.db.Model(&model.Trade{}).Where("account_id = ?", accountID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	return items, total, err
}

// ExistsOn 某交易日是否已有成交（幂等判断用）
func (r *TradeRepository) ExistsOn(accountID uint64, tradeDate time.Time) (bool, error) {
	var n int64
	err := r.db.Model(&model.Trade{}).
		Where("account_id = ? AND trade_date = ?", accountID, tradeDate).
		Count(&n).Error
	return n > 0, err
}
