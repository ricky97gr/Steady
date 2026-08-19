package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/repository"
	"quant-system/backend/pkg/response"
)

// GetFinancialList 财务数据列表（按报告期倒序，limit 默认 20 上限 100）
func GetFinancialList(financialRepo *repository.FinancialRepository,
	stockRepo *repository.StockRepository) gin.HandlerFunc {

	return func(c *gin.Context) {
		code := c.Param("code")
		if !validCode(code) {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "股票代码格式错误")
			return
		}

		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		if limit < 1 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
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

		items, err := financialRepo.GetByCode(code, limit)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}

		out := make([]financialDTO, 0, len(items))
		for _, it := range items {
			out = append(out, toFinancialDTO(it))
		}
		response.OK(c, gin.H{
			"code":  code,
			"items": out,
		})
	}
}
