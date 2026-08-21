package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/service"
	"quant-system/backend/pkg/response"
)

// tushareTokenDTO 保存 / 测试入参（token 可空：PUT 空=清空，test 空=测已存 token）
type tushareTokenDTO struct {
	Token string `json:"token"`
}

// GetTushareConfig 查询数据源配置（GET /config/tushare）：只回 configured + 掩码
func GetTushareConfig(svc *service.TushareConfigService) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, err := svc.Get()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询数据源配置失败")
			return
		}
		response.OK(c, cfg)
	}
}

// UpdateTushareConfig 保存数据源配置（PUT /config/tushare）
func UpdateTushareConfig(svc *service.TushareConfigService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var d tushareTokenDTO
		if err := c.ShouldBindJSON(&d); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		if err := svc.Update(d.Token); err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "保存数据源配置失败")
			return
		}
		response.OK(c, gin.H{"updated": true})
	}
}

// TestTushare 测试连接（POST /config/tushare/test）；token 为空时用已存 token
func TestTushare(svc *service.TushareConfigService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var d tushareTokenDTO
		_ = c.ShouldBindJSON(&d)
		if err := svc.TestConnection(d.Token); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, err.Error())
			return
		}
		response.OK(c, gin.H{"ok": true})
	}
}
