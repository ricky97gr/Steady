package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

var testDB *gorm.DB

// setupTestDB 惰性初始化测试库：
//   - 未设 TEST_DB_DSN 时跳过（本地/CI 设置后自动启用集成测试）
//   - 拒绝连接生产库 quant_system，必须用独立测试库（如 quant_system_test）
//   - 每次运行重建 schema（先删后建），保证可重复执行
func setupTestDB(t *testing.T) *gorm.DB {
	if testDB != nil {
		return testDB
	}
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
	if err := db.Exec("DROP TABLE IF EXISTS daily_price, financial_indicator, stock_basic CASCADE").Error; err != nil {
		t.Fatalf("清理测试表失败: %v", err)
	}
	if err := db.AutoMigrate(&model.StockBasic{}, &model.DailyPrice{}, &model.FinancialIndicator{}); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	seedTestData(t, db)
	testDB = db
	return testDB
}

// newTestRouter 组装被测路由（集成测试入口）
func newTestRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	return SetupRouter(setupTestDB(t), zap.NewNop())
}

// seedTestData 确定性种子数据（手算断言用；AutoMigrate 无唯一约束兜底，仅 setup 时调用一次）
func seedTestData(t *testing.T, db *gorm.DB) {
	day := func(s string) time.Time {
		tm, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("种子日期解析失败 %q: %v", s, err)
		}
		return tm
	}

	stocks := []model.StockBasic{
		{Code: "600519", Name: "贵州茅台", Market: "SH", Industry: "白酒", ListDate: day("2001-08-27"), Status: "L", Universe: "hs300"},
		{Code: "000001", Name: "平安银行", Market: "SZ", Industry: "银行", ListDate: day("1991-04-03"), Status: "L", Universe: "hs300"},
	}
	if err := db.Create(&stocks).Error; err != nil {
		t.Fatalf("种子股票插入失败: %v", err)
	}
	// 688111 无行情/无财务/无上市日期；用原生 SQL 省略 list_date 列以真正产生 NULL
	// （GORM 会把零值 time.Time 插成 0001-01-01，无法用于 NULL 排序回归）
	if err := db.Exec(
		"INSERT INTO stock_basic (code, name, market, industry, status) VALUES (?, ?, ?, ?, ?)",
		"688111", "金山办公", "SH", "计算机", "L",
	).Error; err != nil {
		t.Fatalf("种子股票 688111 插入失败: %v", err)
	}

	// 600519：5 根日线，因子 1.0 → 1.2（qfq/hfq 手算断言）
	bars := []model.DailyPrice{
		{Code: "600519", TradeDate: day("2026-08-13"), Open: 1550, High: 1570, Low: 1540, Close: 1560, Volume: 100000, Amount: 156000000, AdjFactor: 1.0},
		{Code: "600519", TradeDate: day("2026-08-14"), Open: 1560, High: 1590, Low: 1555, Close: 1580, Volume: 120000, Amount: 188000000, AdjFactor: 1.05},
		{Code: "600519", TradeDate: day("2026-08-17"), Open: 1580, High: 1600, Low: 1570, Close: 1590, Volume: 90000, Amount: 141000000, AdjFactor: 1.1},
		{Code: "600519", TradeDate: day("2026-08-18"), Open: 1590, High: 1610, Low: 1585, Close: 1600, Volume: 110000, Amount: 175000000, AdjFactor: 1.15},
		{Code: "600519", TradeDate: day("2026-08-19"), Open: 1600, High: 1620, Low: 1590, Close: 1610, Volume: 130000, Amount: 210000000, AdjFactor: 1.2},
		// 000001：首行因子缺失（NULL），测单行回退
		{Code: "000001", TradeDate: day("2026-08-17"), Open: 9.9, High: 10.1, Low: 9.85, Close: 10.0, Volume: 500000, Amount: 5000000, AdjFactor: 0},
		{Code: "000001", TradeDate: day("2026-08-18"), Open: 10.0, High: 10.3, Low: 9.95, Close: 10.2, Volume: 600000, Amount: 6120000, AdjFactor: 1.0},
		{Code: "000001", TradeDate: day("2026-08-19"), Open: 10.2, High: 10.6, Low: 10.15, Close: 10.5, Volume: 700000, Amount: 7350000, AdjFactor: 1.1},
	}
	if err := db.Create(&bars).Error; err != nil {
		t.Fatalf("种子行情插入失败: %v", err)
	}

	fin := []model.FinancialIndicator{
		{Code: "600519", ReportDate: day("2026-03-31"), AnnounceDate: day("2026-04-28"), PE: 25.5, PB: 6.2, ROE: 0.0542, ProfitGrowth: 0.1531, RevenueGrowth: 0.1218, DebtRatio: 0.201, GrossMargin: 0.913},
		{Code: "600519", ReportDate: day("2025-12-31"), AnnounceDate: day("2026-03-30"), PE: 22.1, PB: 5.9, ROE: 0.0487, ProfitGrowth: 0.132, RevenueGrowth: 0.1105, DebtRatio: 0.198, GrossMargin: 0.909},
		{Code: "600519", ReportDate: day("2025-09-30"), AnnounceDate: day("2025-10-30"), PE: 20.8, PB: 5.6, ROE: 0.0452, ProfitGrowth: 0.1186, RevenueGrowth: 0.1052, DebtRatio: 0.195, GrossMargin: 0.905},
	}
	if err := db.Create(&fin).Error; err != nil {
		t.Fatalf("种子财务插入失败: %v", err)
	}
}

// doJSON 发送 GET 请求并解析统一响应信封，返回 (HTTP 状态码, body)
func doJSON(t *testing.T, r *gin.Engine, path string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s 响应不是合法 JSON: %v（原始: %s）", path, err, w.Body.String())
	}
	return w.Code, body
}

// dataOf 取响应信封的 data 字段
func dataOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	d, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data 字段缺失或类型错误: %v", body)
	}
	return d
}

func itemsOf(d map[string]any) []map[string]any {
	raw, _ := d["items"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func assertOK(t *testing.T, status int, body map[string]any) {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("HTTP 状态不符: got %d want 200（body: %v）", status, body)
	}
	if code, _ := body["code"].(float64); code != 0 {
		t.Fatalf("业务码不符: got %v want 0（body: %v）", code, body)
	}
	if ts, _ := body["timestamp"].(string); ts == "" {
		t.Fatal("timestamp 缺失")
	}
}

// assertFail 断言失败响应的业务码（HTTP 状态码由调用方断言）
func assertFail(t *testing.T, _ int, wantCode float64, body map[string]any) {
	t.Helper()
	code, _ := body["code"].(float64)
	if code != wantCode {
		t.Fatalf("业务码不符: got %v want %v（body: %v）", code, wantCode, body)
	}
}

func assertClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("数值不符: got %v want %v", got, want)
	}
}

// ---------------- 股票列表 ----------------

func TestGetStockList(t *testing.T) {
	r := newTestRouter(t)

	t.Run("默认分页与日期格式化", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks")
		assertOK(t, status, body)
		d := dataOf(t, body)
		if d["total"].(float64) != 3 {
			t.Fatalf("total 不符: %v", d["total"])
		}
		items := itemsOf(d)
		if len(items) != 3 {
			t.Fatalf("items 条数不符: %d", len(items))
		}
		// 默认按 code 升序，首条为 000001；日期必须输出 YYYY-MM-DD 而非 RFC3339（Sprint 2 格式化回归点）
		if items[0]["code"] != "000001" {
			t.Fatalf("默认排序首条应为 000001: %v", items[0]["code"])
		}
		if listDate, _ := items[0]["list_date"].(string); listDate != "1991-04-03" {
			t.Fatalf("list_date 格式不符: %v", items[0]["list_date"])
		}
	})

	t.Run("keyword 名称/代码模糊", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks?keyword=茅台")
		assertOK(t, status, body)
		if d := dataOf(t, body); d["total"].(float64) != 1 {
			t.Fatalf("keyword=茅台 total 不符: %v", d["total"])
		}

		status, body = doJSON(t, r, "/api/v1/stocks?keyword=600")
		assertOK(t, status, body)
		if d := dataOf(t, body); d["total"].(float64) != 1 {
			t.Fatalf("keyword=600 total 不符: %v", d["total"])
		}
	})

	t.Run("market/universe 过滤", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks?market=SZ")
		assertOK(t, status, body)
		if d := dataOf(t, body); d["total"].(float64) != 1 {
			t.Fatalf("market=SZ total 不符: %v", d["total"])
		}
		if it := itemsOf(dataOf(t, body)); it[0]["code"] != "000001" {
			t.Fatalf("market=SZ 首条不符: %v", it[0]["code"])
		}

		status, body = doJSON(t, r, "/api/v1/stocks?universe=hs300")
		assertOK(t, status, body)
		if d := dataOf(t, body); d["total"].(float64) != 2 {
			t.Fatalf("universe=hs300 total 不符: %v", d["total"])
		}
	})

	t.Run("排序与 page_size 上限", func(t *testing.T) {
		// 688111 list_date 为 NULL，应排最后（NULLS LAST），不占首位
		status, body := doJSON(t, r, "/api/v1/stocks?sort=list_date&order=desc")
		assertOK(t, status, body)
		items := itemsOf(dataOf(t, body))
		if items[0]["code"] != "600519" || items[2]["code"] != "688111" {
			t.Fatalf("list_date desc 顺序不符: %v %v", items[0]["code"], items[2]["code"])
		}

		status, body = doJSON(t, r, "/api/v1/stocks?sort=list_date&order=asc")
		assertOK(t, status, body)
		items = itemsOf(dataOf(t, body))
		if items[0]["code"] != "000001" || items[2]["code"] != "688111" {
			t.Fatalf("list_date asc 顺序不符: %v %v", items[0]["code"], items[2]["code"])
		}

		// page_size 超限静默修正为 20（回显）
		status, body = doJSON(t, r, "/api/v1/stocks?page_size=500")
		assertOK(t, status, body)
		if ps := dataOf(t, body)["page_size"].(float64); ps != 20 {
			t.Fatalf("page_size 应被 clamp 为 20: %v", ps)
		}
	})

	t.Run("非法 market 返回 40001", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks?market=HK")
		if status != http.StatusBadRequest {
			t.Fatalf("HTTP 状态不符: got %d want 400", status)
		}
		assertFail(t, status, 40001, body)
	})
}

// ---------------- 股票详情 ----------------

func TestGetStockDetail(t *testing.T) {
	r := newTestRouter(t)

	t.Run("基本信息+最新行情+财务摘要", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks/600519")
		assertOK(t, status, body)
		d := dataOf(t, body)
		if d["code"] != "600519" || d["name"] != "贵州茅台" || d["market"] != "SH" {
			t.Fatalf("基本信息不符: %v", d)
		}
		bar := d["latest_bar"].(map[string]any)
		assertClose(t, bar["close"].(float64), 1610)
		if bar["date"] != "2026-08-19" {
			t.Fatalf("最新交易日不符: %v", bar["date"])
		}
		fin := d["financial_summary"].(map[string]any)
		if fin["report_date"] != "2026-03-31" {
			t.Fatalf("财务摘要应为公告日最新的一期: %v", fin["report_date"])
		}
		assertClose(t, fin["pe"].(float64), 25.5)
	})

	t.Run("无行情无财务返回 null 而非 404", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks/688111")
		assertOK(t, status, body)
		d := dataOf(t, body)
		if d["latest_bar"] != nil || d["financial_summary"] != nil {
			t.Fatalf("无数据字段应为 null: %v", d)
		}
	})

	t.Run("股票不存在返回 40004", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks/999999")
		if status != http.StatusNotFound {
			t.Fatalf("HTTP 状态不符: got %d want 404", status)
		}
		assertFail(t, status, 40004, body)
	})

	t.Run("代码格式错误返回 40001", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks/abc")
		if status != http.StatusBadRequest {
			t.Fatalf("HTTP 状态不符: got %d want 400", status)
		}
		assertFail(t, status, 40001, body)
	})
}

// ---------------- K线 ----------------

func TestGetKline(t *testing.T) {
	r := newTestRouter(t)

	t.Run("默认全量升序", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/kline/600519")
		assertOK(t, status, body)
		d := dataOf(t, body)
		if d["period"] != "day" || d["adjust"] != "none" {
			t.Fatalf("period/adjust 回显不符: %v", d)
		}
		items := itemsOf(d)
		if len(items) != 5 {
			t.Fatalf("条数不符: %d", len(items))
		}
		if items[0]["date"] != "2026-08-13" || items[4]["date"] != "2026-08-19" {
			t.Fatalf("日期升序不符: %v %v", items[0]["date"], items[4]["date"])
		}
	})

	t.Run("start/end 区间过滤", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/kline/600519?start=2026-08-17&end=2026-08-18")
		assertOK(t, status, body)
		if items := itemsOf(dataOf(t, body)); len(items) != 2 {
			t.Fatalf("区间过滤条数不符: %d", len(items))
		}
	})

	t.Run("qfq 前复权", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/kline/600519?adjust=qfq")
		assertOK(t, status, body)
		d := dataOf(t, body)
		if d["adjust"] != "qfq" {
			t.Fatalf("adjust 回显不符: %v", d["adjust"])
		}
		items := itemsOf(d)
		want := []float64{1300, 1382.5, 1457.5, 1533.33, 1610}
		for i, w := range want {
			assertClose(t, items[i]["close"].(float64), w)
		}
		// 成交额不受复权影响
		assertClose(t, items[4]["amount"].(float64), 210000000)
	})

	t.Run("hfq 后复权", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/kline/600519?adjust=hfq")
		assertOK(t, status, body)
		items := itemsOf(dataOf(t, body))
		want := []float64{1560, 1659, 1749, 1840, 1932}
		for i, w := range want {
			assertClose(t, items[i]["close"].(float64), w)
		}
	})

	t.Run("因子缺失行按未复权价返回", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/kline/000001?adjust=qfq")
		assertOK(t, status, body)
		items := itemsOf(dataOf(t, body))
		// 首行因子 NULL → 未复权价 10.00；其余 qfq：10.2×1.0/1.1、10.5×1.1/1.1
		assertClose(t, items[0]["close"].(float64), 10.0)
		assertClose(t, items[1]["close"].(float64), 9.27)
		assertClose(t, items[2]["close"].(float64), 10.5)
	})

	t.Run("参数校验", func(t *testing.T) {
		cases := []struct {
			path     string
			wantCode float64
		}{
			{"/api/v1/kline/600519?start=2026-09-01&end=2026-01-01", 40001}, // start>end
			{"/api/v1/kline/600519?period=week", 40001},
			{"/api/v1/kline/600519?adjust=bfq", 40001},
			{"/api/v1/kline/600519?start=2026/07/01", 40001},
			{"/api/v1/kline/600519?end=bad", 40001},
			{"/api/v1/kline/999999", 40004},
			{"/api/v1/kline/abc", 40001},
		}
		for _, c := range cases {
			status, body := doJSON(t, r, c.path)
			wantStatus := http.StatusBadRequest
			if c.wantCode == 40004 {
				wantStatus = http.StatusNotFound
			}
			if status != wantStatus {
				t.Fatalf("GET %s HTTP 状态不符: got %d want %d", c.path, status, wantStatus)
			}
			assertFail(t, status, c.wantCode, body)
		}
	})
}

// ---------------- 财务数据 ----------------

func TestGetFinancialList(t *testing.T) {
	r := newTestRouter(t)

	t.Run("默认按报告期倒序", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks/600519/financial")
		assertOK(t, status, body)
		items := itemsOf(dataOf(t, body))
		if len(items) != 3 {
			t.Fatalf("条数不符: %d", len(items))
		}
		if items[0]["report_date"] != "2026-03-31" || items[2]["report_date"] != "2025-09-30" {
			t.Fatalf("报告期倒序不符: %v", items)
		}
	})

	t.Run("limit 参数与 clamp", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks/600519/financial?limit=2")
		assertOK(t, status, body)
		if items := itemsOf(dataOf(t, body)); len(items) != 2 {
			t.Fatalf("limit=2 条数不符: %d", len(items))
		}

		status, body = doJSON(t, r, "/api/v1/stocks/600519/financial?limit=0")
		assertOK(t, status, body)
		if items := itemsOf(dataOf(t, body)); len(items) != 3 { // clamp 回 20 > 全部 3 条
			t.Fatalf("limit=0 应回退默认: %d", len(items))
		}

		status, body = doJSON(t, r, "/api/v1/stocks/600519/financial?limit=500")
		assertOK(t, status, body)
		if items := itemsOf(dataOf(t, body)); len(items) != 3 { // clamp 回 100 > 全部 3 条
			t.Fatalf("limit=500 应 clamp 为 100: %d", len(items))
		}
	})

	t.Run("无财务数据返回空列表", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks/688111/financial")
		assertOK(t, status, body)
		if items := itemsOf(dataOf(t, body)); len(items) != 0 {
			t.Fatalf("应返回空列表: %d", len(items))
		}
	})

	t.Run("股票不存在返回 40004", func(t *testing.T) {
		status, body := doJSON(t, r, "/api/v1/stocks/999999/financial")
		if status != http.StatusNotFound {
			t.Fatalf("HTTP 状态不符: got %d want 404", status)
		}
		assertFail(t, status, 40004, body)
	})
}
