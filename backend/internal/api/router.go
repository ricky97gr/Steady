package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"quant-system/backend/internal/api/handler"
	"quant-system/backend/internal/repository"
	"quant-system/backend/internal/service"
)

// SetupRouter 注册全部路由
func SetupRouter(db *gorm.DB, tradingSvc *service.TradingService,
	navSvc *service.NavService, initialCash float64) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	stockRepo := repository.NewStockRepository(db)
	dailyRepo := repository.NewDailyRepository(db)
	financialRepo := repository.NewFinancialRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	signalRepo := repository.NewSignalRepository(db)
	positionRepo := repository.NewPositionRepository(db)
	tradeRepo := repository.NewTradeRepository(db)

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

		// 模拟交易（Sprint 5）
		v1.GET("/account", handler.GetAccount(accountRepo, initialCash))
		v1.GET("/account/nav", handler.GetAccountNav(navSvc, accountRepo))
		v1.GET("/positions", handler.GetPositions(positionRepo, accountRepo, stockRepo))
		v1.GET("/orders", handler.GetOrders(orderRepo, accountRepo))
		v1.POST("/orders", handler.PlaceOrder(tradingSvc, accountRepo))
		v1.DELETE("/orders/:id", handler.CancelOrder(tradingSvc, accountRepo))
		v1.GET("/trades", handler.GetTrades(tradeRepo, accountRepo))
	}

	return r
}
