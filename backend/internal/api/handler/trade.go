package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/repository"
	"quant-system/backend/pkg/response"
)

// GetTrades 成交列表（GET /trades?page=&page_size=）
func GetTrades(tradeRepo *repository.TradeRepository,
	accountRepo *repository.AccountRepository) gin.HandlerFunc {

	return func(c *gin.Context) {
		acc, err := accountRepo.GetPrimary()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if pageSize < 1 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}

		items, total, err := tradeRepo.GetList(acc.ID, page, pageSize)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]tradeDTO, 0, len(items))
		for _, t := range items {
			out = append(out, tradeDTO{
				TradeID:    t.TradeID,
				OrderID:    t.OrderID,
				Code:       t.Code,
				Direction:  t.Direction,
				Price:      t.Price,
				Quantity:   t.Quantity,
				Amount:     t.Amount,
				Commission: t.Commission,
				Tax:        t.Tax,
				NetAmount:  t.NetAmount,
				TradeDate:  formatDate(t.TradeDate),
			})
		}
		response.OK(c, gin.H{"items": out, "total": total, "page": page, "page_size": pageSize})
	}
}
