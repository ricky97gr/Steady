package service

import (
	"math"
	"testing"
)

// 一致性对照测试（Go 端）：与 quant-engine/tests/test_broker_consistency.py 共用同一 fixture，
// 逐步断言现金/持仓/成交价，保证两端交易规则数值一致。
//
// Fixture（2 个交易日，top_n=10, max_position_pct=0.20，初始资金 100000）：
// 注意：d2 卖出价刻意选 20.00 → 成交价 19.98、金额 19980.00、印花税 9.99 均为精确两位小数，
// 避免 Python 端不逐笔舍入导致的边界分歧。
//
//   d1 收盘 600000=10.00, 000001=20.00（前收 9.50/19.00，无涨跌停干扰）
//       BUY 600000: 预算=min(100000/10, 100000×0.2)=10000 → 1000 股
//                   成交价=Round2(10.00×1.001)=10.01，金额=10010.00，佣金=Round2(max(10010×0.00025,5))=5.00（下限）
//                   → 现金 89985.00，持仓 1000@10.01（available=0，T+1 冻结）
//       BUY 000001: 总资产=89985.00+10010=99995.00 → 预算=9999.50 → 400 股
//                   成交价=Round2(20.00×1.001)=20.02，金额=8008.00，佣金=5.00（下限）
//                   → 现金 81972.00，持仓 400@20.02
//   d2 解冻 T+1；收盘 600000=20.00
//       SELL 600000 1000 股：成交价=Round2(20.00×0.999)=19.98，金额=19980.00
//                    佣金=5.00（下限），印花税=Round2(19980×0.0005)=9.99，回款=19980-5-9.99=19965.01
//                    → 现金 101937.01，仅剩 000001 400@20.02
//   d2 总资产 = 101937.01 + 400×20.02 = 109945.01
func TestTradingConsistencyFixture(t *testing.T) {
	b := testBroker()
	l := NewLedger(100000, 100000)

	// ---- d1：BUY 600000 ----
	qty := l.SizeBuyQty(10.00, 10, 0.20)
	if qty != 1000 {
		t.Fatalf("d1 qty = %d, want 1000", qty)
	}
	exec := b.BuyExecPrice(10.00)
	if exec != 10.01 {
		t.Fatalf("d1 exec = %v, want 10.01", exec)
	}
	amount := Round2(exec * float64(qty))
	commission := Round2(b.Commission(amount))
	if amount != 10010.00 || commission != 5.00 {
		t.Fatalf("d1 amount/comm = %v/%v, want 10010.00/5.00", amount, commission)
	}
	if err := l.Buy("600000", exec, qty, commission); err != nil {
		t.Fatal(err)
	}
	if math.Abs(l.cash-89985.00) > 1e-9 {
		t.Fatalf("d1 cash = %v, want 89985.00", l.cash)
	}
	pos := l.positions["600000"]
	if pos.AvailableQty != 0 || pos.CostPrice != 10.01 {
		t.Fatalf("d1 T+1/成本不符: %+v", pos)
	}

	// ---- d1：BUY 000001（资金逐笔演化）----
	qty = l.SizeBuyQty(20.00, 10, 0.20)
	if qty != 400 {
		t.Fatalf("d1 第二笔 qty = %d, want 400", qty)
	}
	exec = b.BuyExecPrice(20.00)
	if exec != 20.02 {
		t.Fatalf("d1 第二笔 exec = %v, want 20.02", exec)
	}
	amount = Round2(exec * float64(qty))
	commission = Round2(b.Commission(amount))
	if amount != 8008.00 || commission != 5.00 {
		t.Fatalf("d1 第二笔 amount/comm = %v/%v, want 8008.00/5.00", amount, commission)
	}
	if err := l.Buy("000001", exec, qty, commission); err != nil {
		t.Fatal(err)
	}
	if math.Abs(l.cash-81972.00) > 1e-9 {
		t.Fatalf("d1 第二笔后 cash = %v, want 81972.00", l.cash)
	}

	// ---- d2：解冻 + SELL 600000 ----
	l.UnfreezeT1()
	pos = l.positions["600000"]
	if pos.AvailableQty != 1000 {
		t.Fatalf("d2 解冻后 available = %d, want 1000", pos.AvailableQty)
	}
	exec = b.SellExecPrice(20.00)
	if exec != 19.98 {
		t.Fatalf("d2 exec = %v, want 19.98", exec)
	}
	amount = Round2(exec * float64(pos.AvailableQty))
	commission = Round2(b.Commission(amount))
	tax := Round2(b.Tax(amount))
	if amount != 19980.00 || commission != 5.00 || tax != 9.99 {
		t.Fatalf("d2 amount/comm/tax = %v/%v/%v, want 19980.00/5.00/9.99", amount, commission, tax)
	}
	if err := l.Sell("600000", exec, pos.AvailableQty, commission, tax); err != nil {
		t.Fatal(err)
	}
	if math.Abs(l.cash-101937.01) > 1e-9 {
		t.Fatalf("d2 cash = %v, want 101937.01", l.cash)
	}
	if _, ok := l.positions["600000"]; ok {
		t.Fatal("d2 清仓后 600000 应移除")
	}
	if pos := l.positions["000001"]; pos == nil || pos.Quantity != 400 || pos.CostPrice != 20.02 {
		t.Fatalf("d2 000001 持仓不符: %+v", pos)
	}

	total := Round2(l.TotalAsset())
	if total != 109945.01 {
		t.Fatalf("d2 总资产 = %v, want 109945.01", total)
	}
}
