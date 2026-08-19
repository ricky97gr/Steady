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

// stockDetailDTO 股票详情：基本信息 + 最新行情 + 财务摘要（未回填时为 null）
type stockDetailDTO struct {
	StockBasicDTO
	LatestBar        *klineItem    `json:"latest_bar"`
	FinancialSummary *financialDTO `json:"financial_summary"`
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
