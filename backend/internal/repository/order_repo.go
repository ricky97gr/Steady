package repository

import (
	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// OrderRepository 委托数据访问层（模拟交易在 Sprint 5 完整实现）
type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// GetList 分页查询委托列表，支持状态过滤
func (r *OrderRepository) GetList(accountID uint64, status string, page, pageSize int) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	q := r.db.Model(&model.Order{}).Where("account_id = ?", accountID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&orders).Error
	return orders, total, err
}
