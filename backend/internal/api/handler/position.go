package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/repository"
	"quant-system/backend/pkg/response"
)

// GetPositions 持仓列表（GET /positions）
func GetPositions(positionRepo *repository.PositionRepository,
	accountRepo *repository.AccountRepository,
	stockRepo *repository.StockRepository) gin.HandlerFunc {

	return func(c *gin.Context) {
		acc, err := accountRepo.GetPrimary()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		items, err := positionRepo.ListByAccount(acc.ID)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]positionDTO, 0, len(items))
		for _, p := range items {
			dto := positionDTO{
				Code:         p.Code,
				Quantity:     p.Quantity,
				AvailableQty: p.AvailableQty,
				CostPrice:    p.CostPrice,
				CurrentPrice: p.CurrentPrice,
				MarketValue:  p.MarketValue,
				Profit:       p.Profit,
				ProfitRate:   p.ProfitRate,
			}
			if stock, err := stockRepo.GetByCode(p.Code); err == nil && stock != nil {
				dto.Name = stock.Name
			}
			out = append(out, dto)
		}
		response.OK(c, gin.H{"items": out})
	}
}
