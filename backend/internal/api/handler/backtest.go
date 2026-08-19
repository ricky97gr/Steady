package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/model"
	"quant-system/backend/internal/repository"
	"quant-system/backend/internal/service"
	"quant-system/backend/pkg/response"
)

// ---- DTO ----

// navPointDTO 净值序列单点（nav 与 benchmark 均需前端归一化对比）
type navPointDTO struct {
	Date      string   `json:"date"`
	Nav       float64  `json:"nav"`
	Benchmark *float64 `json:"benchmark"` // 指数缺失日 null
}

// backtestJobDTO 回测任务单条（列表/详情共用；Result 为 nil 时指标字段不输出）
type backtestJobDTO struct {
	ID           uint64      `json:"id"`
	StrategyName string      `json:"strategy_name"`
	StartDate    string      `json:"start_date"`
	EndDate      string      `json:"end_date"`
	TopN         int         `json:"top_n"`
	Status       string      `json:"status"`
	Error        string      `json:"error"`
	CreatedAt    string      `json:"created_at"`
	FinishedAt   string      `json:"finished_at"`
	// 结果指标（Result 为 nil 时省略）
	TotalReturn      float64      `json:"total_return,omitempty"`
	AnnualizedReturn float64      `json:"annualized_return,omitempty"`
	MaxDrawdown      float64      `json:"max_drawdown,omitempty"`
	Sharpe           float64      `json:"sharpe,omitempty"`
	TradingDays      int          `json:"trading_days,omitempty"`
	FinalValue       float64      `json:"final_value,omitempty"`
	Trades           int          `json:"trades,omitempty"`
	Positions        int          `json:"positions,omitempty"`
	BenchmarkReturn  float64      `json:"benchmark_return,omitempty"`
	ExcessReturn     float64      `json:"excess_return,omitempty"`
	Nav              []navPointDTO `json:"nav,omitempty"`
}

// indexNavItemDTO 指数归一化净值单条（close/区间首日 close）
type indexNavItemDTO struct {
	TradeDate string  `json:"trade_date"`
	Nav       float64 `json:"nav"`
}

func toBacktestJobDTO(j *model.BacktestJob) backtestJobDTO {
	d := backtestJobDTO{
		ID:           j.ID,
		StrategyName: j.StrategyName,
		StartDate:    formatDate(j.StartDate),
		EndDate:      formatDate(j.EndDate),
		TopN:         j.TopN,
		Status:       j.Status,
		Error:        j.Error,
		CreatedAt:    formatDate(j.CreatedAt),
		FinishedAt:   formatDateTime(j.FinishedAt),
	}
	if j.Result != nil {
		d.TotalReturn = j.Result.TotalReturn
		d.AnnualizedReturn = j.Result.AnnualizedReturn
		d.MaxDrawdown = j.Result.MaxDrawdown
		d.Sharpe = j.Result.Sharpe
		d.TradingDays = j.Result.TradingDays
		d.FinalValue = j.Result.FinalValue
		d.Trades = j.Result.Trades
		d.Positions = j.Result.Positions
		d.BenchmarkReturn = j.Result.BenchmarkReturn
		d.ExcessReturn = j.Result.ExcessReturn
		// nav JSONB 原始文本解析为序列（仅详情需要，列表处 Nav 为 nil）
		if j.Result.Nav != "" {
			_ = json.Unmarshal([]byte(j.Result.Nav), &d.Nav)
		}
	}
	return d
}

// ---- handlers ----

// GetBacktests 回测任务列表（GET /backtests?limit=）
func GetBacktests(btSvc *service.BacktestService) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 20
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}
		jobs, err := btSvc.List(limit)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]backtestJobDTO, 0, len(jobs))
		for _, j := range jobs {
			jj := j
			out = append(out, toBacktestJobDTO(&jj))
		}
		response.OK(c, gin.H{"items": out})
	}
}

// CreateBacktest 提交回测任务（POST /backtests）
// 请求体 {start_date, end_date, top_n}，成功响应 {job_id, status:"pending"}
func CreateBacktest(btSvc *service.BacktestService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			StartDate string `json:"start_date"`
			EndDate   string `json:"end_date"`
			TopN      int    `json:"top_n"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		start, err := parseDate(req.StartDate)
		if err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "start_date 格式应为 YYYY-MM-DD")
			return
		}
		end, err := parseDate(req.EndDate)
		if err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "end_date 格式应为 YYYY-MM-DD")
			return
		}
		if req.TopN == 0 {
			req.TopN = 20
		}
		job, err := btSvc.CreateJob(start, end, req.TopN)
		if err != nil {
			status, code, msg := backtestError(err)
			response.Fail(c, status, code, msg)
			return
		}
		response.OK(c, gin.H{"job_id": job.ID, "status": job.Status})
	}
}

// GetBacktestDetail 回测任务详情（GET /backtests/:id，含净值序列）
func GetBacktestDetail(btSvc *service.BacktestService) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "任务ID格式错误")
			return
		}
		j, err := btSvc.Get(id)
		if err != nil {
			status, code, msg := backtestError(err)
			response.Fail(c, status, code, msg)
			return
		}
		response.OK(c, toBacktestJobDTO(j))
	}
}

// GetIndexNav 指数归一化净值（GET /index/nav/:code?start=&end=）
// code 支持 sh000300 / 000300（自动补 sh 前缀）；nav = close/区间首日 close
func GetIndexNav(dailyRepo *repository.DailyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := normalizeIndexCode(c.Param("code"))
		if code == "" {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "指数代码格式错误（如 sh000300）")
			return
		}
		var start, end *time.Time
		if v := c.Query("start"); v != "" {
			t, err := parseDate(v)
			if err != nil {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "start 格式应为 YYYY-MM-DD")
				return
			}
			start = &t
		}
		if v := c.Query("end"); v != "" {
			t, err := parseDate(v)
			if err != nil {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "end 格式应为 YYYY-MM-DD")
				return
			}
			end = &t
		}
		bars, err := dailyRepo.GetRange(code, start, end)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		response.OK(c, gin.H{"code": code, "items": indexNavItems(bars)})
	}
}

// ---- 辅助 ----

// indexNavItems 指数归一化：首条非空 close 为 1.0 锚点，后续 close/锚点
func indexNavItems(bars []model.DailyPrice) []indexNavItemDTO {
	items := make([]indexNavItemDTO, 0, len(bars))
	var anchor float64
	for _, b := range bars {
		if b.Close <= 0 {
			continue
		}
		if anchor == 0 {
			anchor = b.Close
		}
		items = append(items, indexNavItemDTO{
			TradeDate: formatDate(b.TradeDate),
			Nav:       service.Round2(b.Close / anchor),
		})
	}
	return items
}

// validIndexCode 指数代码校验：sh/sz 前缀 + 6 位数字（或裸 6 位，自动补 sh）
// 注意：不改全局 validCode（POST /orders 仍需纯 6 位股票代码）
func validIndexCode(s string) bool {
	digits := s
	if strings.HasPrefix(s, "sh") || strings.HasPrefix(s, "sz") {
		digits = s[2:]
	}
	if len(digits) != 6 {
		return false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// normalizeIndexCode 归一化指数代码（000300 → sh000300；sh000300 原样返回）
func normalizeIndexCode(s string) string {
	if !validIndexCode(s) {
		return ""
	}
	if strings.HasPrefix(s, "sh") || strings.HasPrefix(s, "sz") {
		return s
	}
	return "sh" + s
}

// formatDateTime 时间格式化为 YYYY-MM-DD HH:MM，nil 返回空串
func formatDateTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// backtestError 业务错误映射（与 orderError 同模式）
func backtestError(err error) (int, int, string) {
	switch {
	case errors.Is(err, service.ErrBacktestRange):
		return http.StatusBadRequest, response.CodeInvalidParam, err.Error()
	case errors.Is(err, service.ErrBacktestSpan):
		return http.StatusBadRequest, response.CodeInvalidParam, err.Error()
	case errors.Is(err, service.ErrBacktestTopN):
		return http.StatusBadRequest, response.CodeInvalidParam, err.Error()
	case errors.Is(err, service.ErrBacktestNotFound):
		return http.StatusNotFound, response.CodeResourceMissing, err.Error()
	default:
		return http.StatusInternalServerError, response.CodeInternalError, "操作失败"
	}
}
