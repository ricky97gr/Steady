package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	"quant-system/backend/internal/model"
	"quant-system/backend/internal/repository"
)

// ConsistencyService 每日对账校验（防御层）。
//
// ExecuteDay 在单事务内用内存 Ledger 演化，cash/持仓/净值天然一致且幂等，
// 但对账是**独立复算**：单独从落库状态核对账户恒等式与订单/成交/T+1/净值约束，
// 用于捕获手动改库、脚本异常、并发漏跑、T+1 误判等造成的漂移。
// 结果写 task_run 台账（幂等 upsert）；通过推绿色卡片，偏差推红色告警卡片。
// 定时由 Scheduler 注册（21:15，晚于 21:05 净值快照）。
type ConsistencyService struct {
	db     *gorm.DB
	taskRun *TaskRunService
	notify *NotifyService

	accountRepo *repository.AccountRepository
	tradeRepo   *repository.TradeRepository
}

// ConsistencyResult CheckDay 对账结果
type ConsistencyResult struct {
	TradeDate  time.Time
	Passed     bool
	Idle       bool // 当日无交易活动（空日守卫命中），未做实质性检查
	Violations []string
}

func NewConsistencyService(db *gorm.DB, taskRun *TaskRunService, notify *NotifyService) *ConsistencyService {
	return &ConsistencyService{
		db:          db,
		taskRun:     taskRun,
		notify:      notify,
		accountRepo: repository.NewAccountRepository(db),
		tradeRepo:   repository.NewTradeRepository(db),
	}
}

// tolerance：账户/持仓/净值均为 decimal(15,2)，按分容差比较
func within(a, b float64) bool { return math.Abs(a-b) <= 0.01 }

// dateOnly 截断到日期（用于与 tradeDate 同量级比较）
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// CheckDay 对指定交易日的落库状态做一致性校验；写台账并推送结果卡片。
// 空日（无信号/无当日委托/无成交/无净值）→ 直接通过，只记台账不推卡片。
func (s *ConsistencyService) CheckDay(tradeDate time.Time) (*ConsistencyResult, error) {
	acc, err := s.accountRepo.GetPrimary()
	if err != nil {
		return nil, fmt.Errorf("查询主账户失败: %w", err)
	}

	// ---- 装载落库状态 ----
	var positions []model.Position
	if err := s.db.Where("account_id = ?", acc.ID).Find(&positions).Error; err != nil {
		return nil, fmt.Errorf("查询持仓失败: %w", err)
	}
	var orders []model.Order
	if err := s.db.Where("account_id = ?", acc.ID).Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("查询委托失败: %w", err)
	}
	var trades []model.Trade
	if err := s.db.Where("account_id = ?", acc.ID).Find(&trades).Error; err != nil {
		return nil, fmt.Errorf("查询成交失败: %w", err)
	}
	var navs []model.AccountNav
	if err := s.db.Where("account_id = ? AND trade_date = ?", acc.ID, tradeDate).
		Find(&navs).Error; err != nil {
		return nil, fmt.Errorf("查询净值快照失败: %w", err)
	}

	// 当日活动基线（决定是否命中空日守卫 / 是否有执行期望）
	var sigCount int64
	if err := s.db.Model(&model.StrategySignal{}).
		Where("strategy_name = ? AND trade_date = ?", "multi_factor", tradeDate).
		Count(&sigCount).Error; err != nil {
		return nil, fmt.Errorf("查询策略信号失败: %w", err)
	}
	var todayOrders int64
	if err := s.db.Model(&model.Order{}).
		Where("account_id = ? AND DATE(created_at) = ?", acc.ID, tradeDate).
		Count(&todayOrders).Error; err != nil {
		return nil, fmt.Errorf("查询当日委托失败: %w", err)
	}
	hasTrades, err := s.tradeRepo.ExistsOn(acc.ID, tradeDate)
	if err != nil {
		return nil, fmt.Errorf("查询当日成交失败: %w", err)
	}

	// 空日守卫：当日无信号、无委托、无成交、无净值 → 无交易活动，直接通过
	if sigCount == 0 && todayOrders == 0 && !hasTrades && len(navs) == 0 {
		res := &ConsistencyResult{TradeDate: tradeDate, Passed: true, Idle: true}
		_ = s.taskRun.Record("consistency_check", tradeDate, "success", "当日无交易活动，无需对账",
			map[string]interface{}{
				"trade_date": tradeDate.Format("2006-01-02"), "idle": true,
				"passed": true, "violations": []string{}, "card_sent": false,
			})
		return res, nil
	}

	var violations []string

	// 1. 账户对账：现金 + 持仓市值 = 总资产；逐仓 市值 = 数量 × 现价
	sumMV := 0.0
	for i := range positions {
		p := &positions[i]
		if mv := Round2(float64(p.Quantity) * p.CurrentPrice); !within(p.MarketValue, mv) {
			violations = append(violations, fmt.Sprintf(
				"持仓 %s 市值 %.2f ≠ 数量×现价 %.2f", p.Code, p.MarketValue, mv))
		}
		sumMV += p.MarketValue
	}
	if !within(acc.Cash+sumMV, acc.TotalAsset) {
		violations = append(violations, fmt.Sprintf(
			"账户对账不平：现金 %.2f + 持仓市值 %.2f = %.2f ≠ 总资产 %.2f",
			acc.Cash, sumMV, acc.Cash+sumMV, acc.TotalAsset))
	}

	// 2+3. 订单/成交数量一致 + 撤单/拒单状态正确
	filledByOrder := map[string]int{}
	for i := range trades {
		filledByOrder[trades[i].OrderID] += trades[i].Quantity
	}
	for i := range orders {
		o := &orders[i]
		tq := filledByOrder[o.OrderID]
		switch o.Status {
		case model.OrderFilled, model.OrderPartial:
			if tq != o.FilledQty {
				violations = append(violations, fmt.Sprintf(
					"委托 %s(%s %s) 成交数量不一致：订单 %d ≠ 成交 %d",
					o.OrderID, o.Code, o.Direction, o.FilledQty, tq))
			}
			if o.Status == model.OrderFilled && o.FilledQty != o.Quantity {
				violations = append(violations, fmt.Sprintf(
					"委托 %s 状态 FILLED 但未全部成交：%d/%d", o.OrderID, o.FilledQty, o.Quantity))
			}
			if o.FilledQty > o.Quantity {
				violations = append(violations, fmt.Sprintf(
					"委托 %s 成交数量 %d 超过委托数量 %d", o.OrderID, o.FilledQty, o.Quantity))
			}
		case model.OrderCancelled:
			if tq != 0 {
				violations = append(violations, fmt.Sprintf(
					"委托 %s 已撤销但仍有 %d 股成交", o.OrderID, tq))
			}
			if o.FilledQty != 0 {
				violations = append(violations, fmt.Sprintf(
					"委托 %s 已撤销但 filled_qty = %d", o.OrderID, o.FilledQty))
			}
		case model.OrderRejected:
			if tq != 0 {
				violations = append(violations, fmt.Sprintf(
					"委托 %s 已拒绝但仍有 %d 股成交", o.OrderID, tq))
			}
			if o.FilledQty != 0 {
				violations = append(violations, fmt.Sprintf(
					"委托 %s 已拒绝但 filled_qty = %d", o.OrderID, o.FilledQty))
			}
		case model.OrderPending:
			if tq != 0 {
				violations = append(violations, fmt.Sprintf(
					"委托 %s 仍待成交但已有 %d 股成交", o.OrderID, tq))
			}
			if dateOnly(o.CreatedAt).Before(dateOnly(tradeDate)) {
				violations = append(violations, fmt.Sprintf(
					"委托 %s(%s %s) 停留待成交超过一个交易日（%s 创建）",
					o.OrderID, o.Code, o.Direction, o.CreatedAt.Format("2006-01-02")))
			}
		}
	}

	// 4. T+1 可卖数量：0 ≤ available ≤ quantity；冻结股须由当日买入覆盖
	buyOnDate := map[string]int{}
	for i := range trades {
		t := &trades[i]
		if t.Direction == model.ActionBuy && dateOnly(t.TradeDate).Equal(dateOnly(tradeDate)) {
			buyOnDate[t.Code] += t.Quantity
		}
	}
	for i := range positions {
		p := &positions[i]
		if p.AvailableQty < 0 || p.AvailableQty > p.Quantity {
			violations = append(violations, fmt.Sprintf(
				"持仓 %s T+1 可用数量异常：available=%d quantity=%d",
				p.Code, p.AvailableQty, p.Quantity))
			continue
		}
		if frozen := p.Quantity - p.AvailableQty; frozen > 0 && buyOnDate[p.Code] < frozen {
			violations = append(violations, fmt.Sprintf(
				"持仓 %s 冻结 %d 股但当日买入仅 %d 股（T+1 数量不符）",
				p.Code, frozen, buyOnDate[p.Code]))
		}
	}

	// 5. 净值快照：有活动须存在；幂等（≤1 行）；内部一致
	if len(navs) == 0 {
		violations = append(violations, "净值快照缺失（当日有交易活动但无 account_nav 记录）")
	} else if len(navs) > 1 {
		violations = append(violations, fmt.Sprintf("净值快照重复：%d 行（应 ≤1）", len(navs)))
	} else if !within(navs[0].Cash+navs[0].MarketValue, navs[0].TotalAsset) {
		violations = append(violations, fmt.Sprintf(
			"净值快照内部不一致：现金 %.2f + 市值 %.2f ≠ 总资产 %.2f",
			navs[0].Cash, navs[0].MarketValue, navs[0].TotalAsset))
	}

	// 6. 自动交易不重复：非幂等成功至多 1 次；有信号且整条执行链未跑 → 违规
	var nonSkipped int64
	if err := s.db.Model(&model.TaskRun{}).
		Where("task_name = ? AND run_date = ? AND status = ? AND detail->>'skipped' = ?",
			"auto_trade", tradeDate, "success", "false").
		Count(&nonSkipped).Error; err != nil {
		return nil, fmt.Errorf("查询自动交易台账失败: %w", err)
	}
	if nonSkipped > 1 {
		violations = append(violations, fmt.Sprintf("自动交易重复执行 %d 次（应 ≤1）", nonSkipped))
	}
	if sigCount > 0 && nonSkipped == 0 && len(navs) == 0 {
		violations = append(violations, fmt.Sprintf(
			"策略信号已生成 %d 条但自动交易未执行且无净值快照", sigCount))
	}

	res := &ConsistencyResult{TradeDate: tradeDate, Passed: len(violations) == 0, Violations: violations}
	s.recordAndNotify(tradeDate, res)
	return res, nil
}

// recordAndNotify 写台账 + 推卡片（best-effort，失败不阻断主流程）。
// 通过：绿色「对账通过」（同日已推过则跳过，避免周末对上周五重复推送）；
// 偏差：红色「对账校验未通过」逐条列违规（重复推送是刻意的——状态持续异常应持续告警）。
func (s *ConsistencyService) recordAndNotify(tradeDate time.Time,
	res *ConsistencyResult) {
	detail := map[string]interface{}{
		"trade_date": tradeDate.Format("2006-01-02"),
		"passed":     res.Passed,
		"violations": res.Violations,
	}
	date := tradeDate.Format("2006-01-02")
	if res.Passed {
		alreadySent := s.cardSent(tradeDate)
		detail["card_sent"] = !alreadySent
		_ = s.taskRun.Record("consistency_check", tradeDate, "success", "对账通过", detail)
		if !alreadySent {
			content := fmt.Sprintf("**交易日** %s\n\n7 项检查全部通过：\n"+
				"账户对账 · 订单成交一致 · 撤单状态 · T+1 可卖 · 净值幂等 · 自动交易唯一 · 空日守卫", date)
			_ = s.notify.SendCard("✅ Steady · 对账校验通过", content, "green", "每日对账 · 21:15")
		}
		return
	}
	detail["card_sent"] = false
	_ = s.taskRun.Record("consistency_check", tradeDate, "failed",
		fmt.Sprintf("对账失败：%d 项偏差", len(res.Violations)), detail)
	lines := []string{"**交易日** " + date, "", fmt.Sprintf("发现 **%d** 项偏差：", len(res.Violations))}
	for _, v := range res.Violations {
		lines = append(lines, "• "+v)
	}
	_ = s.notify.SendCard("❌ Steady · 对账校验未通过", "**对账校验**\n\n"+strings.Join(lines, "\n"),
		"red", "每日对账 · 21:15")
}

// cardSent 当日绿卡是否已推送过（读台账 detail.card_sent；无记录/解析失败按未推送）
func (s *ConsistencyService) cardSent(tradeDate time.Time) bool {
	var row model.TaskRun
	if err := s.db.Where("task_name = ? AND run_date = ?", "consistency_check", tradeDate).
		Limit(1).Find(&row).Error; err != nil || row.ID == 0 {
		return false
	}
	var d map[string]interface{}
	if err := json.Unmarshal(row.Detail, &d); err != nil {
		return false
	}
	sent, _ := d["card_sent"].(bool)
	return sent
}
