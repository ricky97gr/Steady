package handler

import (
	"time"

	"quant-system/backend/internal/model"
)

// StockBasicDTO 股票基础信息（列表/详情通用，日期输出 YYYY-MM-DD）
type StockBasicDTO struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Market   string `json:"market"`
	Industry string `json:"industry"`
	ListDate string `json:"list_date"`
	Status   string `json:"status"`
	Universe string `json:"universe"`
}

// klineItem K线单条（对齐技术准备文档 §6.3.1 响应示例）
type klineItem struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
	Amount float64 `json:"amount"`
}

// financialDTO 财务指标（列表与详情摘要共用）
type financialDTO struct {
	ReportDate    string  `json:"report_date"`
	AnnounceDate  string  `json:"announce_date"`
	PE            float64 `json:"pe"`
	PB            float64 `json:"pb"`
	ROE           float64 `json:"roe"`
	ProfitGrowth  float64 `json:"profit_growth"`
	RevenueGrowth float64 `json:"revenue_growth"`
	DebtRatio     float64 `json:"debt_ratio"`
	GrossMargin   float64 `json:"gross_margin"`
}

// valuationDTO 每日估值（日度 PE(TTM)/PB，未回填时 detail.Valuation 为 null）
type valuationDTO struct {
	TradeDate string  `json:"trade_date"`
	PeTtm     float64 `json:"pe_ttm"`
	Pb        float64 `json:"pb"`
}

// stockDetailDTO 股票详情：基本信息 + 最新行情 + 财务摘要 + 日度估值（未回填时为 null）
type stockDetailDTO struct {
	StockBasicDTO
	LatestBar        *klineItem    `json:"latest_bar"`
	FinancialSummary *financialDTO `json:"financial_summary"`
	Valuation        *valuationDTO `json:"valuation"`
}

// accountDTO 模拟账户卡（Sprint 5）
type accountDTO struct {
	AccountID   uint64  `json:"account_id"`
	Name        string  `json:"name"`
	Cash        float64 `json:"cash"`
	MarketValue float64 `json:"market_value"`
	TotalAsset  float64 `json:"total_asset"`
	Profit      float64 `json:"profit"`
	ProfitRate  float64 `json:"profit_rate"`
	MaxDrawdown float64 `json:"max_drawdown"`
	InitialCash float64 `json:"initial_cash"`
}

// navItemDTO 净值快照单条
type navItemDTO struct {
	TradeDate   string  `json:"trade_date"`
	TotalAsset  float64 `json:"total_asset"`
	Nav         float64 `json:"nav"`
	DailyReturn float64 `json:"daily_return"`
	Drawdown    float64 `json:"drawdown"`
}

// positionDTO 持仓单条
type positionDTO struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Quantity     int     `json:"quantity"`
	AvailableQty int     `json:"available_qty"` // < quantity 表示 T+1 冻结
	CostPrice    float64 `json:"cost_price"`
	CurrentPrice float64 `json:"current_price"`
	MarketValue  float64 `json:"market_value"`
	Profit       float64 `json:"profit"`
	ProfitRate   float64 `json:"profit_rate"`
}

// orderDTO 委托单条
type orderDTO struct {
	OrderID      string  `json:"order_id"`
	Code         string  `json:"code"`
	Direction    string  `json:"direction"`
	OrderType    string  `json:"order_type"`
	Price        float64 `json:"price"`
	Quantity     int     `json:"quantity"`
	FilledQty    int     `json:"filled_qty"`
	AvgFillPrice float64 `json:"avg_fill_price"`
	Status       string  `json:"status"`
	Reason       string  `json:"reason"`
	Source       string  `json:"source"`
	CreatedAt    string  `json:"created_at"`
}

// tradeDTO 成交单条
type tradeDTO struct {
	TradeID    string  `json:"trade_id"`
	OrderID    string  `json:"order_id"`
	Code       string  `json:"code"`
	Direction  string  `json:"direction"`
	Price      float64 `json:"price"`
	Quantity   int     `json:"quantity"`
	Amount     float64 `json:"amount"`
	Commission float64 `json:"commission"`
	Tax        float64 `json:"tax"`
	NetAmount  float64 `json:"net_amount"`
	TradeDate  string  `json:"trade_date"`
}

// formatDate 日期格式化为 YYYY-MM-DD，零值返回空串
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// validCode 股票代码格式校验：6 位数字
func validCode(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseDate 解析 YYYY-MM-DD
func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func toStockBasicDTO(m model.StockBasic) StockBasicDTO {
	return StockBasicDTO{
		Code:     m.Code,
		Name:     m.Name,
		Market:   m.Market,
		Industry: m.Industry,
		ListDate: formatDate(m.ListDate),
		Status:   m.Status,
		Universe: m.Universe,
	}
}

func toStockBasicDTOs(items []model.StockBasic) []StockBasicDTO {
	out := make([]StockBasicDTO, 0, len(items))
	for _, it := range items {
		out = append(out, toStockBasicDTO(it))
	}
	return out
}

func toKlineItem(m model.DailyPrice) klineItem {
	return klineItem{
		Date:   formatDate(m.TradeDate),
		Open:   m.Open,
		High:   m.High,
		Low:    m.Low,
		Close:  m.Close,
		Volume: m.Volume,
		Amount: m.Amount,
	}
}

func toFinancialDTO(m model.FinancialIndicator) financialDTO {
	return financialDTO{
		ReportDate:    formatDate(m.ReportDate),
		AnnounceDate:  formatDate(m.AnnounceDate),
		PE:            m.PE,
		PB:            m.PB,
		ROE:           m.ROE,
		ProfitGrowth:  m.ProfitGrowth,
		RevenueGrowth: m.RevenueGrowth,
		DebtRatio:     m.DebtRatio,
		GrossMargin:   m.GrossMargin,
	}
}
