package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/repository"
	"quant-system/backend/pkg/response"
)

// GetStrategies 策略列表（活跃策略）
func GetStrategies(signalRepo *repository.SignalRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := signalRepo.GetStrategies()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]gin.H, 0, len(items))
		for _, s := range items {
			out = append(out, gin.H{
				"name":           s.Name,
				"description":    s.Description,
				"factor_weights": s.FactorWeights,
				"params":         s.Params,
				"status":         s.Status,
			})
		}
		response.OK(c, gin.H{"items": out})
	}
}

// GetSignals 策略信号列表
// ?strategy=multi_factor&date=2026-08-19&action=BUY&limit=100
// date 缺省 = 最近一期；无信号返回空 items（仍 200）
func GetSignals(signalRepo *repository.SignalRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		strategy := c.DefaultQuery("strategy", "multi_factor")
		action := c.Query("action")
		if action != "" && action != "BUY" && action != "SELL" && action != "HOLD" {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "action 参数错误")
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
		if limit < 1 {
			limit = 100
		}
		if limit > 500 {
			limit = 500
		}

		var date time.Time
		if dateStr := c.Query("date"); dateStr != "" {
			d, err := parseDate(dateStr)
			if err != nil {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "date 格式应为 YYYY-MM-DD")
				return
			}
			date = d
		} else {
			latest, err := signalRepo.GetLatestSignalDate(strategy)
			if err != nil {
				response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
				return
			}
			if latest == nil {
				response.OK(c, gin.H{
					"strategy": strategy, "trade_date": "", "items": []gin.H{},
				})
				return
			}
			date = *latest
		}

		items, err := signalRepo.GetSignals(strategy, date, action, limit)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]gin.H, 0, len(items))
		for _, it := range items {
			out = append(out, gin.H{
				"code": it.Code, "name": it.Name, "score": it.Score,
				"action": it.Action, "reason": it.Reason,
			})
		}
		response.OK(c, gin.H{
			"strategy": strategy, "trade_date": formatDate(date), "items": out,
		})
	}
}

// GetSignalsByCode 个股信号历史（倒序，limit 默认 50 上限 200）
func GetSignalsByCode(signalRepo *repository.SignalRepository,
	stockRepo *repository.StockRepository) gin.HandlerFunc {

	return func(c *gin.Context) {
		code := c.Param("code")
		if !validCode(code) {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "股票代码格式错误")
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if limit < 1 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
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

		items, err := signalRepo.GetSignalsByCode(code, limit)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]gin.H, 0, len(items))
		for _, it := range items {
			out = append(out, gin.H{
				"trade_date": formatDate(it.TradeDate), "score": it.Score,
				"action": it.Action, "reason": it.Reason,
			})
		}
		response.OK(c, gin.H{"code": code, "items": out})
	}
}
