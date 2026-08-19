package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/repository"
	"quant-system/backend/pkg/response"
)

// GetStockList 股票列表（分页 + 行业/关键词/市场/股票池过滤 + 白名单排序）
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

		market := c.Query("market")
		if market != "" && market != "SH" && market != "SZ" && market != "BJ" {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "market 仅支持 SH/SZ/BJ")
			return
		}

		query := repository.StockListQuery{
			Page:     page,
			PageSize: pageSize,
			Industry: c.Query("industry"),
			Keyword:  strings.TrimSpace(c.Query("keyword")),
			Market:   market,
			Universe: c.Query("universe"),
			Sort:     c.Query("sort"),
			Order:    c.Query("order"),
		}

		stocks, total, err := stockRepo.GetList(query)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}

		response.OK(c, gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"items":     toStockBasicDTOs(stocks),
		})
	}
}
