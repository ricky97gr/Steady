package repository

import (
	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// FinancialRepository 财务指标数据访问层（只读）
type FinancialRepository struct {
	db *gorm.DB
}

func NewFinancialRepository(db *gorm.DB) *FinancialRepository {
	return &FinancialRepository{db: db}
}

// GetByCode 按报告期倒序取财务指标，最多 limit 条
func (r *FinancialRepository) GetByCode(code string, limit int) ([]model.FinancialIndicator, error) {
	var items []model.FinancialIndicator
	err := r.db.Where("code = ?", code).
		Order("report_date DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

// GetLatestByAnnounce 按公告日倒序取最近一条（详情页财务摘要），无数据返回 (nil, nil)
// announce_date 相同时按 report_date 倒序兜底，保证结果稳定
func (r *FinancialRepository) GetLatestByAnnounce(code string) (*model.FinancialIndicator, error) {
	var item model.FinancialIndicator
	err := r.db.Where("code = ?", code).
		Order("announce_date DESC, report_date DESC").
		First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
