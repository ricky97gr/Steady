package service

import (
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// 每日对账校验集成测试（TEST_DB_DSN 门控，模式对齐 api/router_test.go）：
//   - 未设 TEST_DB_DSN 时跳过（本地/CI 设置后自动启用）
//   - 拒绝连接生产库 quant_system，必须用独立测试库（如 quant_system_test）
//   - 每次 freshDB 先删后建表，保证可重复执行

func freshDB(t *testing.T) *gorm.DB {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN 未设置，跳过集成测试（本地/CI 设置后自动启用）")
	}
	if strings.Contains(dsn, "dbname=quant_system") && !strings.Contains(dsn, "dbname=quant_system_test") {
		t.Fatal("拒绝连接生产库 quant_system，请使用独立测试库 quant_system_test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := db.Migrator().DropTable(&model.Account{}, &model.Position{}, &model.Order{},
		&model.Trade{}, &model.AccountNav{}, &model.TaskRun{}, &model.StrategySignal{},
		&model.AppConfig{}); err != nil {
		t.Fatalf("清理测试表失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Account{}, &model.Position{}, &model.Order{},
		&model.Trade{}, &model.AccountNav{}, &model.TaskRun{}, &model.StrategySignal{},
		&model.AppConfig{}); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	return db
}

func consistencyDay(t *testing.T, s string) time.Time {
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("日期解析失败 %q: %v", s, err)
	}
	return tm
}

// seedConsistencyBaseline 一致的基线态：
//   现金 95000 + 持仓 500股×10.00=5000 = 总资产 100000（对账通过前提）
//   + 当日净值（活动基线） + auto_trade 非幂等成功台账
func seedConsistencyBaseline(t *testing.T, db *gorm.DB, td time.Time) uint64 {
	acc := model.Account{Name: "主账户", Cash: 95000, MarketValue: 5000,
		TotalAsset: 100000, Profit: 0, ProfitRate: 0}
	if err := db.Create(&acc).Error; err != nil {
		t.Fatalf("种子账户失败: %v", err)
	}
	if err := db.Create(&model.Position{
		AccountID: acc.ID, Code: "600000", Quantity: 500, AvailableQty: 500,
		CostPrice: 10, CurrentPrice: 10, MarketValue: 5000,
	}).Error; err != nil {
		t.Fatalf("种子持仓失败: %v", err)
	}
	if err := db.Create(&model.AccountNav{
		AccountID: acc.ID, TradeDate: td, TotalAsset: 100000,
		Cash: 95000, MarketValue: 5000, Nav: 1, DailyReturn: 0, Drawdown: 0,
	}).Error; err != nil {
		t.Fatalf("种子净值失败: %v", err)
	}
	detail := datatypes.JSON([]byte(`{"skipped":false,"trade_date":"` + td.Format("2006-01-02") + `"}`))
	if err := db.Create(&model.TaskRun{
		TaskName: "auto_trade", RunDate: td, Status: "success",
		Message: "买入 0 / 卖出 0", Detail: detail,
	}).Error; err != nil {
		t.Fatalf("种子台账失败: %v", err)
	}
	return acc.ID
}

func newConsistencySvc(db *gorm.DB) *ConsistencyService {
	return NewConsistencyService(db, NewTaskRunService(db), NewNotifyService(db))
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// ---------- 空日守卫 ----------

func TestConsistencyIdleDay(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	if err := db.Create(&model.Account{Name: "主账户", Cash: 100000, TotalAsset: 100000}).Error; err != nil {
		t.Fatalf("种子账户失败: %v", err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !res.Idle || !res.Passed || len(res.Violations) != 0 {
		t.Errorf("空日应直接通过: %+v", res)
	}
	var run model.TaskRun
	if err := db.Where("task_name = ? AND run_date = ?", "consistency_check", td).
		First(&run).Error; err != nil {
		t.Fatalf("空日应写台账: %v", err)
	}
	if run.Status != "success" {
		t.Errorf("空日台账状态 = %s, want success", run.Status)
	}
}

// ---------- 检查 1：账户对账 ----------

func TestConsistencyAccountMismatch(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	// 现金 95000 + 市值 5000 = 100000，把总资产改成 99999 → 对账不平
	if err := db.Model(&model.Account{}).Where("id = ?", accID).
		Update("total_asset", 99999).Error; err != nil {
		t.Fatal(err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if res.Passed {
		t.Fatal("应检测到账户对账不平")
	}
	if !contains(res.Violations, "账户对账不平") {
		t.Errorf("缺账户对账违规: %v", res.Violations)
	}
}

func TestConsistencyPositionMarketValueMismatch(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	// 500×10.00=5000，改成 5100 → 市值 ≠ 数量×现价
	if err := db.Model(&model.Position{}).Where("account_id = ?", accID).
		Update("market_value", 5100).Error; err != nil {
		t.Fatal(err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !contains(res.Violations, "市值") {
		t.Errorf("缺市值违规: %v", res.Violations)
	}
}

// ---------- 检查 2+3：订单/成交一致 + 撤单状态 ----------

func TestConsistencyOrderTradeMismatch(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	// FILLED 订单 500 股，但成交只有 400 → 数量不一致
	if err := db.Create(&model.Order{
		OrderID: "ord-1", AccountID: accID, Code: "600000", Direction: "BUY",
		OrderType: "MARKET", Price: 10, Quantity: 500, FilledQty: 500,
		AvgFillPrice: 10, Status: "FILLED", Source: "strategy", CreatedAt: td,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Trade{
		TradeID: "trd-1", OrderID: "ord-1", AccountID: accID, Code: "600000",
		Direction: "BUY", Price: 10, Quantity: 400, Amount: 4000,
		Commission: 5, TradeDate: td,
	}).Error; err != nil {
		t.Fatal(err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !contains(res.Violations, "成交数量不一致") {
		t.Errorf("缺订单-成交不一致违规: %v", res.Violations)
	}
}

func TestConsistencyCancelledWithTrade(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	// 已撤销的委托仍带成交 → 撤单状态错误
	if err := db.Create(&model.Order{
		OrderID: "ord-2", AccountID: accID, Code: "600000", Direction: "BUY",
		OrderType: "LIMIT", Price: 10, Quantity: 300, FilledQty: 0,
		Status: "CANCELLED", Source: "manual", CreatedAt: td,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Trade{
		TradeID: "trd-2", OrderID: "ord-2", AccountID: accID, Code: "600000",
		Direction: "BUY", Price: 10, Quantity: 300, Amount: 3000,
		Commission: 5, TradeDate: td,
	}).Error; err != nil {
		t.Fatal(err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !contains(res.Violations, "已撤销但仍有") {
		t.Errorf("缺撤单违规: %v", res.Violations)
	}
}

// ---------- 检查 4：T+1 可卖数量 ----------

func TestConsistencyT1FrozenMismatch(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	// 持仓 500 股但可用只有 100（冻结 400），当日买入仅 200 → 冻结超出买入
	if err := db.Model(&model.Position{}).Where("account_id = ?", accID).
		Update("available_qty", 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Trade{
		TradeID: "trd-t1", OrderID: "ord-t1", AccountID: accID, Code: "600000",
		Direction: "BUY", Price: 10, Quantity: 200, Amount: 2000,
		Commission: 5, TradeDate: td,
	}).Error; err != nil {
		t.Fatal(err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !contains(res.Violations, "T+1 数量不符") {
		t.Errorf("缺 T+1 违规: %v", res.Violations)
	}
}

// ---------- 检查 5：净值快照 ----------

func TestConsistencyNavMissing(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	// 删净值 + 补一笔成交 → 有活动但无净值
	if err := db.Where("account_id = ? AND trade_date = ?", accID, td).
		Delete(&model.AccountNav{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Trade{
		TradeID: "trd-nav", OrderID: "ord-nav", AccountID: accID, Code: "600000",
		Direction: "BUY", Price: 10, Quantity: 100, Amount: 1000,
		Commission: 5, TradeDate: td,
	}).Error; err != nil {
		t.Fatal(err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !contains(res.Violations, "净值快照缺失") {
		t.Errorf("缺净值缺失违规: %v", res.Violations)
	}
}

func TestConsistencyNavInconsistent(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	// 净值内部 现金+市值=100000，总资产改 99999 → 内部不一致
	if err := db.Model(&model.AccountNav{}).Where("account_id = ? AND trade_date = ?", accID, td).
		Update("total_asset", 99999).Error; err != nil {
		t.Fatal(err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !contains(res.Violations, "净值快照内部不一致") {
		t.Errorf("缺净值内部违规: %v", res.Violations)
	}
}

// ---------- 检查 6：自动交易不重复 ----------

func TestConsistencyAutoTradeDuplicate(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	seedConsistencyBaseline(t, db, td)
	// 产品侧 uq_task_run 唯一索引本就防重复；此处模拟「索引被绕过」的防御场景：
	// 删索引后造 2 条非幂等成功 → 应检测到重复执行
	if err := db.Exec("DROP INDEX IF EXISTS uq_task_run").Error; err != nil {
		t.Fatalf("删索引失败: %v", err)
	}
	detail := datatypes.JSON([]byte(`{"skipped":false,"trade_date":"` + td.Format("2006-01-02") + `"}`))
	for i, msg := range []string{"买入 0 / 卖出 0", "买入 1 / 卖出 0"} {
		if err := db.Create(&model.TaskRun{
			TaskName: "auto_trade", RunDate: td, Status: "success",
			Message: msg, Detail: detail,
		}).Error; err != nil {
			t.Fatalf("种子台账 %d 失败: %v", i, err)
		}
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !contains(res.Violations, "自动交易重复执行") {
		t.Errorf("缺重复执行违规: %v", res.Violations)
	}
}

func TestConsistencySignalNotExecuted(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	// 信号已生成但自动交易未执行且无净值 → 执行链断
	if err := db.Where("account_id = ? AND trade_date = ?", accID, td).
		Delete(&model.AccountNav{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("task_name = ? AND run_date = ?", "auto_trade", td).
		Delete(&model.TaskRun{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StrategySignal{
		StrategyName: "multi_factor", Code: "600000", TradeDate: td,
		Score: 80, Action: "BUY", Reason: "测试",
	}).Error; err != nil {
		t.Fatal(err)
	}
	res, err := newConsistencySvc(db).CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !contains(res.Violations, "策略信号已生成 1 条但自动交易未执行且无净值快照") {
		t.Errorf("缺执行链断裂违规: %v", res.Violations)
	}
}

// ---------- 台账记录（通过 success / 偏差 failed） ----------

func TestConsistencyRecordsLedger(t *testing.T) {
	db := freshDB(t)
	td := consistencyDay(t, "2026-08-21")
	accID := seedConsistencyBaseline(t, db, td)
	svc := newConsistencySvc(db)

	res, err := svc.CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if !res.Passed || len(res.Violations) != 0 {
		t.Fatalf("基线应对账通过: %+v", res)
	}
	var run model.TaskRun
	if err := db.Where("task_name = ? AND run_date = ?", "consistency_check", td).
		First(&run).Error; err != nil {
		t.Fatalf("应对账写台账: %v", err)
	}
	if run.Status != "success" {
		t.Errorf("通过时台账状态 = %s, want success", run.Status)
	}
	// 基线 + 改坏 → failed
	if err := db.Model(&model.Account{}).Where("id = ?", accID).
		Update("total_asset", 99999).Error; err != nil {
		t.Fatal(err)
	}
	res, err = svc.CheckDay(td)
	if err != nil {
		t.Fatalf("CheckDay 报错: %v", err)
	}
	if res.Passed {
		t.Fatal("改坏后应对账失败")
	}
	if err := db.Where("task_name = ? AND run_date = ?", "consistency_check", td).
		First(&run).Error; err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" {
		t.Errorf("偏差时台账状态 = %s, want failed", run.Status)
	}
}
