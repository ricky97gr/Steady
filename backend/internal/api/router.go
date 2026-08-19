package api

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"quant-system/backend/internal/api/handler"
	"quant-system/backend/internal/repository"
)

// SetupRouter 注册全部路由
func SetupRouter(db *gorm.DB, log *zap.Logger) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	stockRepo := repository.NewStockRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	orderRepo := repository.NewOrderRepository(db)

	// 基础路径 /api/v1
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", handler.HealthCheck(db))

		// 股票相关
		v1.GET("/stocks", handler.GetStockList(stockRepo))
	}

	// 占位引用，避免 Sprint 0 未使用告警
	_ = accountRepo
	_ = orderRepo
	_ = log

	return r
}
