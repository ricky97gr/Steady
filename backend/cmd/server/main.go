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
	"quant-system/backend/internal/model"
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

	// 3.5 自动迁移新增表（task_run / notify_config / app_config / morning_brief，与 init.sql 同构幂等）
	if err := db.AutoMigrate(
		&model.TaskRun{}, &model.NotifyConfig{}, &model.AppConfig{}, &model.MorningBrief{}); err != nil {
		log.Fatal("自动迁移失败", zap.Error(err))
	}

	// 4. 交易/通知/执行服务 + 调度器（Sprint 5：19:35 自动下单 / 21:05 净值快照 / 21:15 对账校验）
	tradingSvc := service.NewTradingService(db, cfg.Account)
	navSvc := service.NewNavService(db, cfg.Account)
	taskRunSvc := service.NewTaskRunService(db)
	notifySvc := service.NewNotifyService(db)
	executeSvc := service.NewExecuteService(db, tradingSvc, navSvc, taskRunSvc, notifySvc)
	briefSvc := service.NewMorningBriefService(db)
	consistencySvc := service.NewConsistencyService(db, taskRunSvc, notifySvc)

	sched := service.NewScheduler(log)
	accountRepo := repository.NewAccountRepository(db)
	dailyRepo := repository.NewDailyRepository(db)
	// 三个每日任务均声明补跑语义：重启后若当天触发时刻已过且当日未执行 → 启动时立即补跑。
	// 注册顺序即补跑顺序（auto-trade → nav-snapshot → consistency-check），保持数据依赖链。
	sched.RegisterCatchUp("auto-trade", 19, 35, func() error {
		return runAutoTrade(log, taskRunSvc, tradingSvc, accountRepo, dailyRepo)
	}, catchUpDaily(taskRunSvc, dailyRepo, "auto_trade"))
	sched.RegisterCatchUp("nav-snapshot", 21, 5, func() error {
		return runNavSnapshot(log, taskRunSvc, db, navSvc, accountRepo, dailyRepo)
	}, catchUpDaily(taskRunSvc, dailyRepo, "nav_snapshot"))
	sched.RegisterCatchUp("consistency-check", 21, 15, func() error {
		return runConsistencyCheck(log, taskRunSvc, consistencySvc, dailyRepo)
	}, catchUpDaily(taskRunSvc, dailyRepo, "consistency_check"))
	go sched.Start()

	// 5. 注册路由并启动服务
	router := api.SetupRouter(db, tradingSvc, navSvc, cfg.Account.InitialCash,
		taskRunSvc, notifySvc, executeSvc, briefSvc)
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

// recordTask 写任务账本（best-effort，失败仅记日志，不影响业务）
func recordTask(log *zap.Logger, svc *service.TaskRunService, name string, td time.Time,
	status, msg string, detail interface{}) {
	if err := svc.Record(name, td, status, msg, detail); err != nil {
		log.Warn("记录任务执行状态失败", zap.String("task", name), zap.Error(err))
	}
}

// catchUpDaily 启动补跑判定：仅当日补跑。最近交易日 == 今天（交易日）且当日该任务
// 无执行记录 → 需补跑；非交易日/周末（最近交易日 < 今天）返回 false，避免对历史
// 交易日做无谓的幂等重跑（那会盖掉当日原始台账 detail）。与对应 run 函数同源
// （GetLatestTradeDate）；无行情数据返回 false（run 函数也会跳过）。
func catchUpDaily(taskRunSvc *service.TaskRunService, dailyRepo *repository.DailyRepository,
	taskName string) func() (bool, error) {
	sh := time.FixedZone("CST", 8*3600)
	return func() (bool, error) {
		latest, err := dailyRepo.GetLatestTradeDate()
		if err != nil {
			return false, err
		}
		if latest == nil {
			return false, nil // 无行情数据 → 对应 run 函数也会跳过，无需补跑
		}
		now := time.Now().In(sh)
		latestLocal := latest.In(sh)
		if latestLocal.Year() != now.Year() || latestLocal.YearDay() != now.YearDay() {
			return false, nil // 最近交易日不是今天（周末/节假日）→ 当日无执行期望
		}
		done, err := taskRunSvc.HasRun(taskName, *latest)
		if err != nil {
			return false, err
		}
		return !done, nil
	}
}

// runAutoTrade 19:35 自动下单：以最近交易日为基准，幂等闸在服务内部（净值已存在 → 跳过）
func runAutoTrade(log *zap.Logger, taskRunSvc *service.TaskRunService,
	tradingSvc *service.TradingService, accountRepo *repository.AccountRepository,
	dailyRepo *repository.DailyRepository) error {

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
		recordTask(log, taskRunSvc, "auto_trade", *latest, "failed", "自动交易异常",
			map[string]interface{}{"trade_date": latest.Format("2006-01-02")})
		return err
	}
	if res.Skipped {
		recordTask(log, taskRunSvc, "auto_trade", *latest, "success", "当日已执行，幂等跳过",
			map[string]interface{}{"trade_date": latest.Format("2006-01-02"), "skipped": true})
		log.Info("当日已执行，跳过自动下单", zap.String("trade_date", latest.Format("2006-01-02")))
		return nil
	}
	recordTask(log, taskRunSvc, "auto_trade", *latest, "success",
		fmt.Sprintf("买入 %d / 卖出 %d / 手动 %d / 拒绝 %d",
			res.BuyCount, res.SellCount, res.Manual, res.Rejected),
		map[string]interface{}{
			"trade_date": latest.Format("2006-01-02"), "skipped": false,
			"buy_count": res.BuyCount, "sell_count": res.SellCount,
			"manual": res.Manual, "rejected": res.Rejected,
		})
	log.Info("自动下单完成",
		zap.String("trade_date", latest.Format("2006-01-02")),
		zap.Int("buy", res.BuyCount), zap.Int("sell", res.SellCount),
		zap.Int("manual", res.Manual), zap.Int("rejected", res.Rejected))
	return nil
}

// runNavSnapshot 21:05 净值快照
func runNavSnapshot(log *zap.Logger, taskRunSvc *service.TaskRunService, db *gorm.DB,
	navSvc *service.NavService, accountRepo *repository.AccountRepository,
	dailyRepo *repository.DailyRepository) error {

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
		recordTask(log, taskRunSvc, "nav_snapshot", *latest, "failed", "净值快照异常",
			map[string]interface{}{"trade_date": latest.Format("2006-01-02")})
		return err
	}
	// 快照详情（日收益/回撤/总资产）从 account_nav 读
	var navRow model.AccountNav
	if err := db.Where("account_id = ? AND trade_date = ?", acc.ID, *latest).
		Order("id desc").First(&navRow).Error; err == nil {
		recordTask(log, taskRunSvc, "nav_snapshot", *latest, "success",
			fmt.Sprintf("净值 %v", navRow.Nav),
			map[string]interface{}{
				"trade_date": latest.Format("2006-01-02"), "skipped": res.Skipped,
				"nav": navRow.Nav, "daily_return": navRow.DailyReturn,
				"drawdown": navRow.Drawdown, "total_asset": navRow.TotalAsset,
			})
	} else {
		recordTask(log, taskRunSvc, "nav_snapshot", *latest, "success",
			fmt.Sprintf("净值 %v", res.Nav),
			map[string]interface{}{"trade_date": latest.Format("2006-01-02"),
				"skipped": res.Skipped, "nav": res.Nav})
	}
	if res.Skipped {
		log.Info("当日净值已存在，跳过快照", zap.String("trade_date", latest.Format("2006-01-02")))
	} else {
		log.Info("净值快照完成", zap.String("trade_date", latest.Format("2006-01-02")),
			zap.Float64("nav", res.Nav))
	}
	return nil
}

// runConsistencyCheck 21:15 每日对账校验（晚于 21:05 净值快照）。
// 检查结果与卡片推送由 ConsistencyService.CheckDay 内部处理（台账幂等 upsert）。
func runConsistencyCheck(log *zap.Logger, taskRunSvc *service.TaskRunService,
	consistencySvc *service.ConsistencyService,
	dailyRepo *repository.DailyRepository) error {

	latest, err := dailyRepo.GetLatestTradeDate()
	if err != nil {
		return fmt.Errorf("查询最近交易日失败: %w", err)
	}
	if latest == nil {
		log.Info("无行情数据，跳过对账校验")
		return nil
	}
	res, err := consistencySvc.CheckDay(*latest)
	if err != nil {
		recordTask(log, taskRunSvc, "consistency_check", *latest, "failed", "对账校验异常",
			map[string]interface{}{"trade_date": latest.Format("2006-01-02")})
		return err
	}
	if res.Passed {
		log.Info("对账校验通过", zap.String("trade_date", latest.Format("2006-01-02")),
			zap.Bool("idle", res.Idle))
	} else {
		log.Warn("对账校验未通过", zap.String("trade_date", latest.Format("2006-01-02")),
			zap.Int("violations", len(res.Violations)))
	}
	return nil
}

// 确保 gin release 模式（非 debug 日志噪音）
var _ = gin.ReleaseMode
