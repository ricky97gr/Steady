package repository

import (
	"time"

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

// Create 写入委托单
func (r *OrderRepository) Create(o *model.Order) error {
	return r.db.Create(o).Error
}

// GetByOrderID 按委托编号查询；不存在返回 (nil, nil)
func (r *OrderRepository) GetByOrderID(orderID string) (*model.Order, error) {
	var o model.Order
	err := r.db.Where("order_id = ?", orderID).First(&o).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetPending 待成交委托（手动单撮合用），按 id 升序（先下先成交）
func (r *OrderRepository) GetPending(accountID uint64) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.Where("account_id = ? AND status = ?", accountID, model.OrderPending).
		Order("id ASC").Find(&orders).Error
	return orders, err
}

// Cancel 撤销委托：仅 PENDING 可撤，原子 UPDATE 防并发
func (r *OrderRepository) Cancel(orderID string) (bool, error) {
	res := r.db.Model(&model.Order{}).
		Where("order_id = ? AND status = ?", orderID, model.OrderPending).
		Update("status", model.OrderCancelled)
	return res.RowsAffected > 0, res.Error
}

// HasStrategyOrderOn 该日是否已有策略单（幂等闸：重跑不重复下单）
// order 表无 trade_date 列，用 created_at 的日期部分比较
func (r *OrderRepository) HasStrategyOrderOn(accountID uint64, tradeDate time.Time) (bool, error) {
	var n int64
	err := r.db.Model(&model.Order{}).
		Where("account_id = ? AND source = ? AND created_at::date = ?",
			accountID, "strategy", tradeDate).
		Count(&n).Error
	return n > 0, err
}

// UpdateFilled 更新成交信息（撮合成功：状态/数量/均价）
func (r *OrderRepository) UpdateFilled(orderID string, filledQty int, avgPrice float64) error {
	return r.db.Model(&model.Order{}).
		Where("order_id = ?", orderID).
		Updates(map[string]interface{}{
			"status":         model.OrderFilled,
			"filled_qty":     filledQty,
			"avg_fill_price": avgPrice,
		}).Error
}

// UpdateRejected 拒绝委托（手动单未成交：状态 + 原因）
func (r *OrderRepository) UpdateRejected(orderID, reason string) error {
	return r.db.Model(&model.Order{}).
		Where("order_id = ? AND status = ?", orderID, model.OrderPending).
		Updates(map[string]interface{}{
			"status": model.OrderRejected,
			"reason": reason,
		}).Error
}
