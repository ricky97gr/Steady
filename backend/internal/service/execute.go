package service

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"quant-system/backend/internal/model"
	"quant-system/backend/internal/repository"
)

// ErrNoMarketData 无行情数据（手动执行时返回 400）
var ErrNoMarketData = errors.New("无行情数据，无法执行（请先完成当日行情同步）")

// ExecuteService 手动执行（用户主动触发）：ExecuteDay + SnapshotDay + 账本 + 结果卡片。
// 与定时 runAutoTrade/runNavSnapshot 的区别：同步执行、立即推送结果卡片。
// 用户确认需要的场景：定时 19:35 已过、当日信号无自动成交入口时补执行。
type ExecuteService struct {
	db          *gorm.DB
	trading     *TradingService
	nav         *NavService
	taskRun     *TaskRunService
	notify      *NotifyService
	accountRepo *repository.AccountRepository
	dailyRepo   *repository.DailyRepository
}

func NewExecuteService(db *gorm.DB, trading *TradingService, nav *NavService,
	taskRun *TaskRunService, notify *NotifyService) *ExecuteService {
	return &ExecuteService{
		db:          db,
		trading:     trading,
		nav:         nav,
		taskRun:     taskRun,
		notify:      notify,
		accountRepo: repository.NewAccountRepository(db),
		dailyRepo:   repository.NewDailyRepository(db),
	}
}

// ExecuteResult 手动执行结果
type ExecuteResult struct {
	TradeDate  time.Time
	Skipped    bool // ExecuteDay 幂等跳过（当日净值已存在 = 已完整执行）
	BuyCount   int
	SellCount  int
	Manual     int
	Rejected   int
	Nav        float64
	NavSkipped bool // 净值快照幂等跳过
}

// ExecuteDayManual 执行最近交易日的自动交易 + 净值快照，并记录账本 + 推送结果卡片。
func (s *ExecuteService) ExecuteDayManual() (*ExecuteResult, error) {
	acc, err := s.accountRepo.GetPrimary()
	if err != nil {
		return nil, fmt.Errorf("查询主账户失败: %w", err)
	}
	latest, err := s.dailyRepo.GetLatestTradeDate()
	if err != nil {
		return nil, fmt.Errorf("查询最近交易日失败: %w", err)
	}
	if latest == nil {
		return nil, ErrNoMarketData
	}

	tradeRes, err := s.trading.ExecuteDay(acc.ID, *latest)
	if err != nil {
		_ = s.taskRun.Record("auto_trade", *latest, "failed", "自动交易异常", nil)
		return nil, err
	}
	navRes, err := s.nav.SnapshotDay(acc.ID, *latest)
	if err != nil {
		_ = s.taskRun.Record("nav_snapshot", *latest, "failed", "净值快照异常", nil)
		return nil, err
	}

	res := &ExecuteResult{
		TradeDate: *latest, Skipped: tradeRes.Skipped,
		BuyCount: tradeRes.BuyCount, SellCount: tradeRes.SellCount,
		Manual: tradeRes.Manual, Rejected: tradeRes.Rejected,
		Nav: navRes.Nav, NavSkipped: navRes.Skipped,
	}
	s.recordLedger(acc.ID, *latest, tradeRes)
	s.pushExecuteCard(res)
	return res, nil
}

// recordLedger 写 auto_trade / nav_snapshot 账本（best-effort，失败仅记日志）
func (s *ExecuteService) recordLedger(accountID uint64, tradeDate time.Time,
	tradeRes *ExecResult) {
	if tradeRes.Skipped {
		_ = s.taskRun.Record("auto_trade", tradeDate, "success", "当日已执行，幂等跳过",
			map[string]interface{}{"trade_date": tradeDate.Format("2006-01-02"), "skipped": true})
	} else {
		_ = s.taskRun.Record("auto_trade", tradeDate, "success",
			fmt.Sprintf("买入 %d / 卖出 %d / 手动 %d / 拒绝 %d",
				tradeRes.BuyCount, tradeRes.SellCount, tradeRes.Manual, tradeRes.Rejected),
			map[string]interface{}{
				"trade_date": tradeDate.Format("2006-01-02"), "skipped": false,
				"buy_count": tradeRes.BuyCount, "sell_count": tradeRes.SellCount,
				"manual": tradeRes.Manual, "rejected": tradeRes.Rejected,
			})
	}
	var navRow model.AccountNav
	if err := s.db.Where("account_id = ? AND trade_date = ?", accountID, tradeDate).
		Order("id desc").First(&navRow).Error; err == nil {
		_ = s.taskRun.Record("nav_snapshot", tradeDate, "success",
			fmt.Sprintf("净值 %v", navRow.Nav),
			map[string]interface{}{
				"trade_date": tradeDate.Format("2006-01-02"), "skipped": tradeRes.Skipped,
				"nav": navRow.Nav, "daily_return": navRow.DailyReturn,
				"drawdown": navRow.Drawdown, "total_asset": navRow.TotalAsset,
			})
	}
}

// pushExecuteCard 手动执行结果卡片（发送失败不阻断主流程）
func (s *ExecuteService) pushExecuteCard(res *ExecuteResult) {
	date := res.TradeDate.Format("2006-01-02")
	if res.Skipped {
		content := fmt.Sprintf("**执行日期** %s\n\n当日已完整执行（幂等跳过），无需重复操作。", date)
		_ = s.notify.SendCard("💹 Steady · 手动执行结果", content, "blue", "手动触发 ExecuteDay")
		return
	}
	content := fmt.Sprintf(
		"**执行日期** %s\n买入 **%d** 笔 · 卖出 **%d** 笔\n手动 **%d** 笔 · 拒绝 **%d** 笔",
		date, res.BuyCount, res.SellCount, res.Manual, res.Rejected)
	_ = s.notify.SendCard("💹 Steady · 手动执行结果", content, "blue", "手动触发 ExecuteDay")
}
