package handler

import (
	"math"
	"testing"
	"time"

	"quant-system/backend/internal/model"
)

func testBar(date string, open, high, low, closePrice, factor float64) model.DailyPrice {
	t, _ := time.Parse("2006-01-02", date)
	return model.DailyPrice{
		TradeDate: t, Open: open, High: high, Low: low, Close: closePrice,
		Volume: 1000, Amount: 100000, AdjFactor: factor,
	}
}

func TestAdjustPrices(t *testing.T) {
	// 600519 种子行情：因子 1.0 → 1.2
	bars := []model.DailyPrice{
		testBar("2026-08-13", 1550, 1570, 1540, 1560, 1.0),
		testBar("2026-08-14", 1560, 1590, 1555, 1580, 1.05),
		testBar("2026-08-17", 1580, 1600, 1570, 1590, 1.1),
		testBar("2026-08-18", 1590, 1610, 1585, 1600, 1.15),
		testBar("2026-08-19", 1600, 1620, 1590, 1610, 1.2),
	}

	t.Run("none 原样返回", func(t *testing.T) {
		items := adjustPrices(bars, adjustNone, 0)
		if len(items) != 5 {
			t.Fatalf("条数不符: got %d want 5", len(items))
		}
		if items[0].Close != 1560 || items[4].Close != 1610 {
			t.Fatalf("none 模式不应改价: %+v", items[0])
		}
	})

	t.Run("qfq 前复权", func(t *testing.T) {
		items := adjustPrices(bars, adjustQFQ, 1.2)
		want := []float64{1300, 1382.5, 1457.5, 1533.33, 1610}
		for i, w := range want {
			if math.Abs(items[i].Close-w) > 1e-6 {
				t.Fatalf("第 %d 根 qfq 收盘不符: got %v want %v", i, items[i].Close, w)
			}
		}
	})

	t.Run("hfq 后复权", func(t *testing.T) {
		items := adjustPrices(bars, adjustHFQ, 0)
		want := []float64{1560, 1659, 1749, 1840, 1932}
		for i, w := range want {
			if math.Abs(items[i].Close-w) > 1e-6 {
				t.Fatalf("第 %d 根 hfq 收盘不符: got %v want %v", i, items[i].Close, w)
			}
		}
	})

	t.Run("qfq 锚点缺失退化为不复权", func(t *testing.T) {
		items := adjustPrices(bars, adjustQFQ, 0)
		if items[0].Close != 1560 || items[4].Close != 1610 {
			t.Fatalf("锚点缺失时应原样返回: %+v", items)
		}
	})

	t.Run("单行因子缺失按未复权价返回", func(t *testing.T) {
		broken := []model.DailyPrice{
			testBar("2026-08-18", 10, 10.3, 9.95, 10.2, 1.0),
			testBar("2026-08-19", 10.2, 10.6, 10.15, 10.5, 1.1),
			testBar("2026-08-20", 10.5, 10.8, 10.4, 10.7, 0), // 因子缺失
		}
		items := adjustPrices(broken, adjustHFQ, 0)
		if items[0].Close != 10.2 { // 10.2 × 1.0
			t.Fatalf("hfq 第 1 根不符: got %v", items[0].Close)
		}
		if math.Abs(items[1].Close-11.55) > 1e-6 { // 10.5 × 1.1
			t.Fatalf("hfq 第 2 根不符: got %v", items[1].Close)
		}
		if items[2].Close != 10.7 { // 因子缺失 → 未复权价
			t.Fatalf("因子缺失行应返回未复权价: got %v", items[2].Close)
		}
	})

	t.Run("因子为负按未复权价返回", func(t *testing.T) {
		neg := []model.DailyPrice{testBar("2026-08-19", 10, 10.5, 9.9, 10.2, -1)}
		items := adjustPrices(neg, adjustHFQ, 0)
		if items[0].Close != 10.2 {
			t.Fatalf("负因子应按未复权价返回: got %v", items[0].Close)
		}
	})

	t.Run("volume/amount 不受复权影响", func(t *testing.T) {
		items := adjustPrices(bars, adjustQFQ, 1.2)
		if items[4].Volume != 1000 || items[4].Amount != 100000 {
			t.Fatalf("成交数据不应被复权修改: %+v", items[4])
		}
	})

	t.Run("round2 四舍五入", func(t *testing.T) {
		if round2(1533.3333333) != 1533.33 {
			t.Fatalf("round2 出错: %v", round2(1533.3333333))
		}
		if round2(1291.6666666) != 1291.67 {
			t.Fatalf("round2 出错: %v", round2(1291.6666666))
		}
	})
}

func TestParseDate(t *testing.T) {
	if _, err := parseDate("2026-08-19"); err != nil {
		t.Fatalf("合法日期应通过: %v", err)
	}
	for _, bad := range []string{"2026/08/19", "2026-13-01", "2026-08-32", "", "2026-8-1"} {
		if _, err := parseDate(bad); err == nil {
			t.Fatalf("非法日期 %q 应被拒绝", bad)
		}
	}
}

func TestValidCode(t *testing.T) {
	valid := map[string]bool{
		"600519": true, "000001": true, "688111": true,
		"60051": false, "1234567": false, "abc123": false, "60051a": false, "": false,
	}
	for code, want := range valid {
		if got := validCode(code); got != want {
			t.Fatalf("validCode(%q) = %v, want %v", code, got, want)
		}
	}
}
