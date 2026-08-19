package handler

import (
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/model"
	"quant-system/backend/internal/repository"
	"quant-system/backend/pkg/response"
)

// 复权模式
const (
	adjustNone = "none" // 不复权
	adjustQFQ  = "qfq"  // 前复权
	adjustHFQ  = "hfq"  // 后复权
)

// GetKline K线数据（period=day，adjust=none|qfq|hfq）
func GetKline(dailyRepo *repository.DailyRepository, stockRepo *repository.StockRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Param("code")
		if !validCode(code) {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "股票代码格式错误")
			return
		}

		period := c.DefaultQuery("period", "day")
		if period != "day" {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "暂仅支持 period=day")
			return
		}

		adjust := c.DefaultQuery("adjust", adjustNone)
		if adjust != adjustNone && adjust != adjustQFQ && adjust != adjustHFQ {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "adjust 仅支持 none/qfq/hfq")
			return
		}

		var start, end *time.Time
		if s := c.Query("start"); s != "" {
			t, err := parseDate(s)
			if err != nil {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "start 格式应为 YYYY-MM-DD")
				return
			}
			start = &t
		}
		if e := c.Query("end"); e != "" {
			t, err := parseDate(e)
			if err != nil {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "end 格式应为 YYYY-MM-DD")
				return
			}
			end = &t
		}
		if start != nil && end != nil && start.After(*end) {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "start 不能晚于 end")
			return
		}

		exists, err := stockRepo.Exists(code)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		if !exists {
			response.Fail(c, http.StatusNotFound, response.CodeResourceMissing, "股票不存在")
			return
		}

		bars, err := dailyRepo.GetRange(code, start, end)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}

		// 前复权锚点：该股全量最新非空复权因子（缺失时退化为不复权，见 adjustPrices）
		anchor := 0.0
		if adjust == adjustQFQ {
			if f, ok, err := dailyRepo.GetLatestFactor(code); err != nil {
				response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
				return
			} else if ok {
				anchor = f
			}
		}

		response.OK(c, gin.H{
			"code":   code,
			"period": period,
			"adjust": adjust,
			"items":  adjustPrices(bars, adjust, anchor),
		})
	}
}

// adjustPrices 复权计算纯函数（adj_factor 为东财后复权收盘÷不复权收盘的累计因子）：
//   - none: 原样返回
//   - qfq:  前复权 = 价格 × 因子 / 锚点因子（锚点=该股全量最新非空因子）
//   - hfq:  后复权 = 价格 × 因子（绝对后复权，锚定上市日）
//
// 单行因子缺失/非法（<=0）时无法计算复权价，该行按未复权价返回；
// 锚点缺失（qfq 且 anchorFactor<=0）时整体退化为不复权。
func adjustPrices(items []model.DailyPrice, mode string, anchorFactor float64) []klineItem {
	out := make([]klineItem, 0, len(items))
	useQFQ := mode == adjustQFQ && anchorFactor > 0
	useHFQ := mode == adjustHFQ

	for _, it := range items {
		f := it.AdjFactor
		if f <= 0 {
			// 因子缺失/脏数据：返回未复权价
			out = append(out, toKlineItem(it))
			continue
		}
		item := toKlineItem(it)
		switch {
		case useQFQ:
			item.Open = round2(it.Open * f / anchorFactor)
			item.High = round2(it.High * f / anchorFactor)
			item.Low = round2(it.Low * f / anchorFactor)
			item.Close = round2(it.Close * f / anchorFactor)
		case useHFQ:
			item.Open = round2(it.Open * f)
			item.High = round2(it.High * f)
			item.Low = round2(it.Low * f)
			item.Close = round2(it.Close * f)
		}
		out = append(out, item)
	}
	return out
}

// round2 四舍五入保留两位小数（价格精度）
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
