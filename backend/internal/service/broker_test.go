package service

import (
	"math"
	"testing"

	"quant-system/backend/internal/config"
)

func testBroker() *Broker {
	return NewBroker(config.AccountConfig{
		InitialCash:    100000,
		CommissionRate: 0.00025,
		MinCommission:  5.0,
		StampTaxRate:   0.0005,
		Slippage:       0.001,
	})
}

func TestLimitRatioOf(t *testing.T) {
	cases := []struct {
		code string
		want float64
	}{
		{"600519", 0.10}, // 主板
		{"000001", 0.10},
		{"300750", 0.20}, // 创业板
		{"301001", 0.20},
		{"688981", 0.20}, // 科创板
		{"689009", 0.20},
		{"830799", 0.30}, // 北交所
		{"430047", 0.30},
		{"920001", 0.30}, // 北交所新代码段
	}
	for _, c := range cases {
		if got := LimitRatioOf(c.code); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("LimitRatioOf(%s) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestBrokerCommission(t *testing.T) {
	b := testBroker()
	// 金额大 → 万2.5；金额小 → 最低 5 元
	if got := b.Commission(100000); got != 25.0 {
		t.Errorf("Commission(100000) = %v, want 25", got)
	}
	if got := b.Commission(1000); got != 5.0 {
		t.Errorf("Commission(1000) = %v, want 5（下限）", got)
	}
}

func TestBrokerTaxAndSlippage(t *testing.T) {
	b := testBroker()
	if got := b.Tax(100000); got != 50.0 {
		t.Errorf("Tax(100000) = %v, want 50（万5）", got)
	}
	if got := b.BuyExecPrice(10.0); got != 10.01 {
		t.Errorf("BuyExecPrice(10.0) = %v, want 10.01", got)
	}
	if got := b.SellExecPrice(10.5); got != 10.49 {
		t.Errorf("SellExecPrice(10.5) = %v, want 10.49", got)
	}
}

func TestCheckPriceLimit(t *testing.T) {
	b := testBroker()

	// 涨停买不进：收盘价 ≥ 前收×(1+幅度)
	if b.CheckPriceLimit("600519", 11.0, 10.0, "BUY") {
		t.Error("涨停价买入应被拒绝")
	}
	if !b.CheckPriceLimit("600519", 10.99, 10.0, "BUY") {
		t.Error("限价内买入应允许")
	}
	// 跌停卖不出：收盘价 ≤ 前收×(1−幅度)
	if b.CheckPriceLimit("600519", 9.0, 10.0, "SELL") {
		t.Error("跌停价卖出应被拒绝")
	}
	if !b.CheckPriceLimit("600519", 9.01, 10.0, "SELL") {
		t.Error("限价内卖出应允许")
	}
	// 创业板 20%：11.5 未超限
	if !b.CheckPriceLimit("300750", 11.5, 10.0, "BUY") {
		t.Error("创业板 15% 涨幅不应被拒")
	}
	// 无前收（上市首日）不拦截
	if !b.CheckPriceLimit("600519", 100.0, 0, "BUY") {
		t.Error("无前收不应拦截")
	}
}
