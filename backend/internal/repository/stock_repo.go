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

// StockListQuery 股票列表查询条件
type StockListQuery struct {
	Page     int
	PageSize int
	Industry string // 行业精确匹配
	Keyword  string // 代码/名称模糊匹配（ILIKE）
	Market   string // SH / SZ / BJ
	Universe string // hs300 / zz500 / NULL
	Sort     string // 白名单：code / name / list_date / market / industry
	Order    string // asc / desc
}

// GetList 分页查询股票列表，支持行业/关键词/市场/股票池过滤与白名单排序
func (r *StockRepository) GetList(q StockListQuery) ([]model.StockBasic, int64, error) {
	var stocks []model.StockBasic
	var total int64

	query := r.db.Model(&model.StockBasic{})
	if q.Industry != "" {
		query = query.Where("industry = ?", q.Industry)
	}
	if q.Keyword != "" {
		kw := "%" + q.Keyword + "%"
		query = query.Where("name ILIKE ? OR code ILIKE ?", kw, kw)
	}
	if q.Market != "" {
		query = query.Where("market = ?", q.Market)
	}
	if q.Universe != "" {
		query = query.Where("universe = ?", q.Universe)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order(stockSortClause(q.Sort, q.Order)).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&stocks).Error
	return stocks, total, err
}

// GetByCode 按代码查询股票，未找到返回 (nil, nil)
func (r *StockRepository) GetByCode(code string) (*model.StockBasic, error) {
	var stock model.StockBasic
	err := r.db.Where("code = ?", code).First(&stock).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &stock, nil
}

// Exists 股票是否存在
func (r *StockRepository) Exists(code string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.StockBasic{}).Where("code = ?", code).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// 排序白名单：非法值回退 code ASC（值全部来自白名单，拼接 Order 无注入风险）
var stockSortColumns = map[string]string{
	"code":      "code",
	"name":      "name",
	"list_date": "list_date",
	"market":    "market",
	"industry":  "industry",
}

func stockSortClause(sort, order string) string {
	col, ok := stockSortColumns[sort]
	if !ok {
		col = "code"
	}
	// NULLS LAST：真实数据中 list_date 等字段可能为空，空值不应排在有效值前
	if order == "desc" {
		return col + " DESC NULLS LAST"
	}
	return col + " ASC NULLS LAST"
}
