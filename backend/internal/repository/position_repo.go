package repository

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"quant-system/backend/internal/model"
)

// PositionRepository 持仓数据访问层
type PositionRepository struct {
	db *gorm.DB
}

func NewPositionRepository(db *gorm.DB) *PositionRepository {
	return &PositionRepository{db: db}
}

// ListByAccount 账户全部持仓
func (r *PositionRepository) ListByAccount(accountID uint64) ([]model.Position, error) {
	var items []model.Position
	err := r.db.Where("account_id = ?", accountID).Order("code ASC").Find(&items).Error
	return items, err
}

// Get 单只持仓；不存在返回 (nil, nil)
func (r *PositionRepository) Get(accountID uint64, code string) (*model.Position, error) {
	var p model.Position
	err := r.db.Where("account_id = ? AND code = ?", accountID, code).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Upsert 按 (account_id, code) 幂等写入（每日 mark-to-market 用）
func (r *PositionRepository) Upsert(p *model.Position) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}, {Name: "code"}},
		DoUpdates: clause.AssignmentColumns([]string{"quantity", "available_qty", "cost_price",
			"current_price", "market_value", "profit", "profit_rate", "updated_at"}),
	}).Create(p).Error
}

// UnfreezeT1 解冻当日买入份额（available_qty = quantity）
func (r *PositionRepository) UnfreezeT1(accountID uint64) error {
	return r.db.Model(&model.Position{}).
		Where("account_id = ?", accountID).
		Update("available_qty", gorm.Expr("quantity")).Error
}

// Delete 删除持仓（清仓后）
func (r *PositionRepository) Delete(accountID uint64, code string) error {
	return r.db.Where("account_id = ? AND code = ?", accountID, code).
		Delete(&model.Position{}).Error
}
