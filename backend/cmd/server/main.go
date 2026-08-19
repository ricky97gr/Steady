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

	// 4. 注册路由并启动服务
	router := api.SetupRouter(db, log)
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

// 确保 gin release 模式（非 debug 日志噪音）
var _ = gin.ReleaseMode
