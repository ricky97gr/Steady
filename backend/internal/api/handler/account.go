package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/model"
	"quant-system/backend/internal/repository"
	"quant-system/backend/internal/service"
	"quant-system/backend/pkg/response"
)

// GetAccount 账户卡（GET /account）
func GetAccount(accountRepo *repository.AccountRepository, initialCash float64) gin.HandlerFunc {
	return func(c *gin.Context) {
		acc, err := accountRepo.GetPrimary()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		response.OK(c, toAccountDTO(acc, initialCash))
	}
}

// GetAccountNav 净值曲线（GET /account/nav?start=&end=，区间缺省为全量）
func GetAccountNav(navSvc *service.NavService, accountRepo *repository.AccountRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		acc, err := accountRepo.GetPrimary()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		var start, end *time.Time
		if s := c.Query("start"); s != "" {
			d, err := parseDate(s)
			if err != nil {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "start 格式应为 YYYY-MM-DD")
				return
			}
			start = &d
		}
		if e := c.Query("end"); e != "" {
			d, err := parseDate(e)
			if err != nil {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "end 格式应为 YYYY-MM-DD")
				return
			}
			end = &d
		}
		items, err := navSvc.GetRange(acc.ID, start, end)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]navItemDTO, 0, len(items))
		for _, n := range items {
			out = append(out, navItemDTO{
				TradeDate:   formatDate(n.TradeDate),
				TotalAsset:  n.TotalAsset,
				Nav:         n.Nav,
				DailyReturn: n.DailyReturn,
				Drawdown:    n.Drawdown,
			})
		}
		response.OK(c, gin.H{"items": out})
	}
}

func toAccountDTO(acc *model.Account, initialCash float64) accountDTO {
	return accountDTO{
		AccountID:   acc.ID,
		Name:        acc.Name,
		Cash:        acc.Cash,
		MarketValue: acc.MarketValue,
		TotalAsset:  acc.TotalAsset,
		Profit:      acc.Profit,
		ProfitRate:  acc.ProfitRate,
		MaxDrawdown: acc.MaxDrawdown,
		InitialCash: initialCash,
	}
}
