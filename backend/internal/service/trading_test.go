package service

import (
	"errors"
	"testing"
)

func TestLedgerBuyT1(t *testing.T) {
	l := NewLedger(100000, 100000)
	if err := l.Buy("600000", 10.01, 500, 5.0); err != nil {
		t.Fatalf("买入失败: %v", err)
	}
	// 100000 - 5005(金额) - 5(佣金) = 94990
	if l.cash != 94990 {
		t.Errorf("现金 = %v, want 94990", l.cash)
	}
	pos := l.positions["600000"]
	if pos == nil || pos.Quantity != 500 || pos.AvailableQty != 0 {
		t.Errorf("T+1 冻结错误: %+v", pos)
	}
	if pos.CostPrice != 10.01 {
		t.Errorf("成本价 = %v, want 10.01", pos.CostPrice)
	}

	// 当日买入不可卖
	if err := l.Sell("600000", 10.5, 100, 5.0, 0.5); !errors.Is(err, ErrT1Unavailable) {
		t.Errorf("当日买入卖出应报 T+1 错误, got %v", err)
	}

	// 解冻后可卖：回款 = 10.49×500 - 5(佣金) - 2.62(印花税) = 5237.38
	l.UnfreezeT1()
	if pos.AvailableQty != 500 {
		t.Errorf("解冻后可用 = %v, want 500", pos.AvailableQty)
	}
	if err := l.Sell("600000", 10.49, 500, 5.0, 2.62); err != nil {
		t.Fatalf("卖出失败: %v", err)
	}
	if l.cash != 100227.38 {
		t.Errorf("卖出后现金 = %v, want 100227.38", l.cash)
	}
	if _, ok := l.positions["600000"]; ok {
		t.Error("清仓后持仓应移除")
	}
}

func TestLedgerWeightedCost(t *testing.T) {
	l := NewLedger(100000, 100000)
	// 买入 100 股 @10.00 → 再买入 100 股 @12.00，加权成本 11.00
	if err := l.Buy("600000", 10.00, 100, 0); err != nil {
		t.Fatal(err)
	}
	if err := l.Buy("600000", 12.00, 100, 0); err != nil {
		t.Fatal(err)
	}
	pos := l.positions["600000"]
	if pos.Quantity != 200 || pos.CostPrice != 11.00 {
		t.Errorf("加权成本错误: qty=%d cost=%v", pos.Quantity, pos.CostPrice)
	}
}

func TestLedgerValidation(t *testing.T) {
	l := NewLedger(100000, 1000) // 现金只有 1000

	if err := l.Buy("600000", 10.00, 150, 0); !errors.Is(err, ErrNotLotSize) {
		t.Errorf("非整手应报错, got %v", err)
	}
	// 100 股 × 10.01 = 1001 > 1000 → 资金不足
	if err := l.Buy("600000", 10.01, 100, 0); !errors.Is(err, ErrInsufficientCash) {
		t.Errorf("资金不足应报错, got %v", err)
	}
	if err := l.Sell("600000", 10.00, 100, 0, 0); !errors.Is(err, ErrNoPosition) {
		t.Errorf("无持仓卖出应报错, got %v", err)
	}
}

func TestSizeBuyQty(t *testing.T) {
	l := NewLedger(100000, 100000)
	// 等权预算 = min(100000/20, 100000×0.2) = 5000，股价 10 → 500 股
	if qty := l.SizeBuyQty(10.0, 20, 0.20); qty != 500 {
		t.Errorf("SizeBuyQty(10, 20) = %d, want 500", qty)
	}
	// 股价 30：5000/30 = 166 → 100 股（等权约束更紧）
	if qty := l.SizeBuyQty(30.0, 20, 0.20); qty != 100 {
		t.Errorf("SizeBuyQty(30, 20) = %d, want 100", qty)
	}
	// 大资金下单股上限 20% 封顶：min(30000, 120000) = 30000，股价 30 → 1000 股
	l2 := NewLedger(600000, 600000)
	if qty := l2.SizeBuyQty(30.0, 20, 0.20); qty != 1000 {
		t.Errorf("SizeBuyQty(30, 20) 大资金 = %d, want 1000", qty)
	}
	// 股价过高，买不起一手 → 0
	if qty := l.SizeBuyQty(100000.0, 20, 0.20); qty != 0 {
		t.Errorf("SizeBuyQty 应返回 0, got %d", qty)
	}
	// 总资产随买入演化：现金减少后预算随之缩小
	if err := l.Buy("600000", 10.01, 500, 5.0); err != nil {
		t.Fatal(err)
	}
	if qty := l.SizeBuyQty(20.0, 20, 0.20); qty != 200 {
		t.Errorf("演化后 SizeBuyQty(20) = %d, want 200", qty)
	}
}
