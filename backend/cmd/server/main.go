package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"quant-system/backend/internal/api"
	"quant-system/backend/internal/config"
	"quant-system/backend/internal/repository"
	"quant-system/backend/internal/service"
	"quant-system/backend/pkg/logger"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("加载配置失败: %v", err))
	}

	// 2. 初始化日志
	log, err := logger.Init(cfg.Log)
	if err != nil {
		panic(fmt.Sprintf("初始化日志失败: %v", err))
	}
	defer log.Sync()

	// 3. 连接数据库
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User, cfg.Database.Password, cfg.Database.Name,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		log.Fatal("数据库连接失败", zap.Error(err))
	}
	log.Info("数据库连接成功", zap.String("host", cfg.Database.Host))

	// 4. 交易服务 + 调度器（Sprint 5：19:35 自动下单 / 21:05 净值快照）
	tradingSvc := service.NewTradingService(db, cfg.Account)
	navSvc := service.NewNavService(db, cfg.Account)

	sched := service.NewScheduler(log)
	accountRepo := repository.NewAccountRepository(db)
	dailyRepo := repository.NewDailyRepository(db)
	sched.Register("auto-trade", 19, 35, func() error {
		return runAutoTrade(tradingSvc, accountRepo, dailyRepo, log)
	})
	sched.Register("nav-snapshot", 21, 5, func() error {
		return runNavSnapshot(navSvc, accountRepo, dailyRepo, log)
	})
	go sched.Start()

	// 5. 注册路由并启动服务
	router := api.SetupRouter(db, tradingSvc, navSvc, cfg.Account.InitialCash)
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	log.Info("HTTP 服务启动", zap.String("addr", srv.Addr))
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal("HTTP 服务异常退出", zap.Error(err))
	}
}

// runAutoTrade 19:35 自动下单：以最近交易日为基准，幂等闸在服务内部（净值已存在 → 跳过）
func runAutoTrade(tradingSvc *service.TradingService, accountRepo *repository.AccountRepository,
	dailyRepo *repository.DailyRepository, log *zap.Logger) error {

	acc, err := accountRepo.GetPrimary()
	if err != nil {
		return fmt.Errorf("查询主账户失败: %w", err)
	}
	latest, err := dailyRepo.GetLatestTradeDate()
	if err != nil {
		return fmt.Errorf("查询最近交易日失败: %w", err)
	}
	if latest == nil {
		log.Info("无行情数据，跳过自动下单")
		return nil
	}
	res, err := tradingSvc.ExecuteDay(acc.ID, *latest)
	if err != nil {
		return err
	}
	if res.Skipped {
		log.Info("当日已执行，跳过自动下单", zap.String("trade_date", latest.Format("2006-01-02")))
	} else {
		log.Info("自动下单完成",
			zap.String("trade_date", latest.Format("2006-01-02")),
			zap.Int("buy", res.BuyCount), zap.Int("sell", res.SellCount),
			zap.Int("manual", res.Manual), zap.Int("rejected", res.Rejected))
	}
	return nil
}

// runNavSnapshot 21:05 净值快照
func runNavSnapshot(navSvc *service.NavService, accountRepo *repository.AccountRepository,
	dailyRepo *repository.DailyRepository, log *zap.Logger) error {

	acc, err := accountRepo.GetPrimary()
	if err != nil {
		return fmt.Errorf("查询主账户失败: %w", err)
	}
	latest, err := dailyRepo.GetLatestTradeDate()
	if err != nil {
		return fmt.Errorf("查询最近交易日失败: %w", err)
	}
	if latest == nil {
		log.Info("无行情数据，跳过净值快照")
		return nil
	}
	res, err := navSvc.SnapshotDay(acc.ID, *latest)
	if err != nil {
		return err
	}
	if res.Skipped {
		log.Info("当日净值已存在，跳过快照", zap.String("trade_date", latest.Format("2006-01-02")))
	} else {
		log.Info("净值快照完成", zap.String("trade_date", latest.Format("2006-01-02")),
			zap.Float64("nav", res.Nav))
	}
	return nil
}

// 确保 gin release 模式（非 debug 日志噪音）
var _ = gin.ReleaseMode
