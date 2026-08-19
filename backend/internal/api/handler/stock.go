package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/repository"
	"quant-system/backend/pkg/response"
)

// GetStockList 股票列表（分页 + 行业过滤）
func GetStockList(stockRepo *repository.StockRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 20
		}

		industry := c.Query("industry")
		stocks, total, err := stockRepo.GetList(page, pageSize, industry)
		if err != nil {
			response.Fail(c, 500, response.CodeInternalError, "查询失败")
			return
		}

		response.OK(c, gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"items":     stocks,
		})
	}
}
