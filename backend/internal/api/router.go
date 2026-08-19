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
	dailyRepo := repository.NewDailyRepository(db)
	financialRepo := repository.NewFinancialRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	signalRepo := repository.NewSignalRepository(db)

	// 基础路径 /api/v1
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", handler.HealthCheck(db))

		// 股票相关
		v1.GET("/stocks", handler.GetStockList(stockRepo))
		v1.GET("/stocks/:code", handler.GetStockDetail(stockRepo, dailyRepo, financialRepo))
		v1.GET("/stocks/:code/financial", handler.GetFinancialList(financialRepo, stockRepo))
		v1.GET("/kline/:code", handler.GetKline(dailyRepo, stockRepo))

		// 策略与信号（Sprint 4）
		v1.GET("/strategies", handler.GetStrategies(signalRepo))
		v1.GET("/signals", handler.GetSignals(signalRepo))
		v1.GET("/signals/:code", handler.GetSignalsByCode(signalRepo, stockRepo))
	}

	// 占位引用，避免 Sprint 0 未使用告警
	_ = accountRepo
	_ = orderRepo
	_ = log

	return r
}
