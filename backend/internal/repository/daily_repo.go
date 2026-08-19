package repository

import (
	"time"

	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// DailyRepository 日行情数据访问层（只读）
type DailyRepository struct {
	db *gorm.DB
}

func NewDailyRepository(db *gorm.DB) *DailyRepository {
	return &DailyRepository{db: db}
}

// GetRange 区间行情，start/end 为 nil 时不过滤，按 trade_date 升序
func (r *DailyRepository) GetRange(code string, start, end *time.Time) ([]model.DailyPrice, error) {
	var bars []model.DailyPrice
	q := r.db.Where("code = ?", code)
	if start != nil {
		q = q.Where("trade_date >= ?", *start)
	}
	if end != nil {
		q = q.Where("trade_date <= ?", *end)
	}
	err := q.Order("trade_date ASC").Find(&bars).Error
	return bars, err
}

// GetLatest 最近一个交易日的行情，无数据返回 (nil, nil)
func (r *DailyRepository) GetLatest(code string) (*model.DailyPrice, error) {
	var bar model.DailyPrice
	err := r.db.Where("code = ?", code).Order("trade_date DESC").First(&bar).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &bar, nil
}

// GetLatestValuation 最近一日估值（详情页日度 PE/PB），无数据返回 (nil, nil)
func (r *DailyRepository) GetLatestValuation(code string) (*model.DailyValuation, error) {
	var v model.DailyValuation
	err := r.db.Where("code = ?", code).Order("trade_date DESC").First(&v).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetByDate 指定交易日行情；当日停牌/无数据返回 (nil, nil)
func (r *DailyRepository) GetByDate(code string, tradeDate time.Time) (*model.DailyPrice, error) {
	var bar model.DailyPrice
	err := r.db.Where("code = ? AND trade_date = ?", code, tradeDate).First(&bar).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &bar, nil
}

// GetPrevClose 前一交易日收盘价（涨跌停判断），无数据返回 (0, false, nil)
func (r *DailyRepository) GetPrevClose(code string, tradeDate time.Time) (float64, bool, error) {
	var bar model.DailyPrice
	err := r.db.Select("close").
		Where("code = ? AND trade_date < ?", code, tradeDate).
		Order("trade_date DESC").
		First(&bar).Error
	if err == gorm.ErrRecordNotFound {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return bar.Close, true, nil
}

// GetLatestTradeDate 全市场最近交易日（自动下单日期基准）；无数据返回 (nil, nil)
func (r *DailyRepository) GetLatestTradeDate() (*time.Time, error) {
	var bar model.DailyPrice
	err := r.db.Order("trade_date DESC").First(&bar).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &bar.TradeDate, nil
}

// GetLatestFactor 该股全量最新非空复权因子（前复权锚点，跨查询区间），无数据返回 (0, false, nil)
func (r *DailyRepository) GetLatestFactor(code string) (float64, bool, error) {
	var bar model.DailyPrice
	err := r.db.Select("adj_factor").
		Where("code = ? AND adj_factor IS NOT NULL", code).
		Order("trade_date DESC").
		First(&bar).Error
	if err == gorm.ErrRecordNotFound {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return bar.AdjFactor, true, nil
}
