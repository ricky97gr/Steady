package handler

import (
	"math"
	"testing"
	"time"

	"quant-system/backend/internal/model"
	"quant-system/backend/internal/service"
)

func TestValidIndexCode(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"sh000300", true},
		{"000300", true},
		{"sz399001", true},
		{"600519", true}, // 裸 6 位（自动补 sh）也接受
		{"sh0003001", false},
		{"shabc", false},
		{"", false},
		{"SH000300", false}, // 仅小写前缀
		{"60051", false},
	}
	for _, c := range cases {
		if got := validIndexCode(c.code); got != c.want {
			t.Errorf("validIndexCode(%q) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestNormalizeIndexCode(t *testing.T) {
	if got := normalizeIndexCode("000300"); got != "sh000300" {
		t.Fatalf("000300 → %q, want sh000300", got)
	}
	if got := normalizeIndexCode("sh000300"); got != "sh000300" {
		t.Fatalf("sh000300 → %q, want sh000300", got)
	}
	if got := normalizeIndexCode("abc"); got != "" {
		t.Fatalf("abc → %q, want 空串", got)
	}
}

func TestIndexNavItems(t *testing.T) {
	bars := []model.DailyPrice{
		testBar("2026-08-17", 0, 0, 0, 4900, 0),
		testBar("2026-08-18", 0, 0, 0, 4949, 0), // 4900 × 1.01 = 4949
		testBar("2026-08-19", 0, 0, 0, 0, 0),    // close 缺失日跳过
		testBar("2026-08-20", 0, 0, 0, 4802, 0), // 4900 × 0.98 = 4802
	}
	items := indexNavItems(bars)
	if len(items) != 3 {
		t.Fatalf("条数 = %d, want 3（缺失日跳过）", len(items))
	}
	want := []float64{1.0, 1.01, 0.98}
	for i, w := range want {
		if math.Abs(items[i].Nav-w) > 1e-9 {
			t.Fatalf("第 %d 点 nav = %v, want %v", i, items[i].Nav, w)
		}
	}
	if items[0].TradeDate != "2026-08-17" {
		t.Fatalf("首点日期 = %s, want 2026-08-17", items[0].TradeDate)
	}
}

// TestBacktestServiceValidation 校验链在 repo 调用前触发（nil repo 足够测错误分支）
func TestBacktestServiceValidation(t *testing.T) {
	svc := service.NewBacktestService(nil)
	parse := func(s string) time.Time {
		tm, _ := time.Parse("2006-01-02", s)
		return tm
	}

	t.Run("起始晚于结束", func(t *testing.T) {
		_, err := svc.CreateJob(parse("2026-08-19"), parse("2026-08-01"), 20)
		if err != service.ErrBacktestRange {
			t.Fatalf("err = %v, want ErrBacktestRange", err)
		}
	})

	t.Run("区间超5年", func(t *testing.T) {
		_, err := svc.CreateJob(parse("2020-01-01"), parse("2026-08-01"), 20)
		if err != service.ErrBacktestSpan {
			t.Fatalf("err = %v, want ErrBacktestSpan", err)
		}
	})

	t.Run("topN 越界", func(t *testing.T) {
		if _, err := svc.CreateJob(parse("2026-01-01"), parse("2026-08-01"), 0); err != service.ErrBacktestTopN {
			t.Fatalf("topN=0 err = %v, want ErrBacktestTopN", err)
		}
		if _, err := svc.CreateJob(parse("2026-01-01"), parse("2026-08-01"), 51); err != service.ErrBacktestTopN {
			t.Fatalf("topN=51 err = %v, want ErrBacktestTopN", err)
		}
	})
}
