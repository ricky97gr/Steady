package repository

import (
	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// StockRepository 股票数据访问层
type StockRepository struct {
	db *gorm.DB
}

func NewStockRepository(db *gorm.DB) *StockRepository {
	return &StockRepository{db: db}
}

// GetList 分页查询股票列表，支持行业过滤
func (r *StockRepository) GetList(page, pageSize int, industry string) ([]model.StockBasic, int64, error) {
	var stocks []model.StockBasic
	var total int64

	q := r.db.Model(&model.StockBasic{})
	if industry != "" {
		q = q.Where("industry = ?", industry)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.Order("code").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&stocks).Error
	return stocks, total, err
}
