package service

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"quant-system/backend/internal/model"
)

// TestRegisterLastRunOnRestart 验证重启当天 lastRun 的初始化语义：
// - 注册时当天触发时刻尚未到 → lastRun 为昨天，当天照常执行（不整日跳过）
// - 注册时当天触发时刻已过 → lastRun 为今天，当天不补跑，次日恢复
func TestRegisterLastRunOnRestart(t *testing.T) {
	s := NewScheduler(nil)
	now := time.Now().In(s.loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)

	cases := []struct {
		name      string
		delta     time.Duration
		wantToday bool // 期望 lastRun 与 today 同一天（= 当天不执行）
	}{
		{"trigger-still-coming-fire-today", time.Hour, false},
		{"trigger-already-passed-skip-today", -time.Hour, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trigger := now.Add(tc.delta)
			if trigger.Year() != now.Year() || trigger.YearDay() != now.YearDay() {
				t.Skipf("触发时刻跨天（now=%s delta=%s），跳过该边界场景", now.Format("15:04"), tc.delta)
			}
			s.jobs = nil
			s.Register("job", trigger.Hour(), trigger.Minute(), func() error { return nil })
			gotToday := sameDate(s.jobs[0].lastRun, today)
			if gotToday != tc.wantToday {
				t.Errorf("Register trigger=%02d:%02d (now=%s): lastRun sameDate(today)=%v, want %v",
					trigger.Hour(), trigger.Minute(), now.Format("15:04"), gotToday, tc.wantToday)
			}
		})
	}
}

// ---------- 启动补跑（catchUpToday） ----------

func cstTime(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, time.FixedZone("CST", 8*3600))
}

func pinnedScheduler(now time.Time) *Scheduler {
	s := NewScheduler(zap.NewNop())
	s.now = func() time.Time { return now.In(s.loc) }
	return s
}

func TestCatchUpTodayRunsMissedJob(t *testing.T) {
	now := cstTime(2026, 8, 21, 20, 0) // 触发 19:35 已过
	s := pinnedScheduler(now)
	ran := false
	s.RegisterCatchUp("auto-trade", 19, 35, func() error { ran = true; return nil },
		func() (bool, error) { return true, nil })
	s.catchUpToday()
	if !ran {
		t.Fatal("触发时刻已过且判定未执行 → 应补跑")
	}
	if !sameDate(s.jobs[0].lastRun, now) {
		t.Error("补跑后 lastRun 应为今天，防 ticker 重复触发")
	}
	ran = false
	s.catchUpToday()
	if ran {
		t.Error("第二次 catchUpToday 不应重复补跑")
	}
}

func TestCatchUpTodaySkipsWhenAlreadyRun(t *testing.T) {
	now := cstTime(2026, 8, 21, 20, 0)
	s := pinnedScheduler(now)
	ran := false
	s.RegisterCatchUp("auto-trade", 19, 35, func() error { ran = true; return nil },
		func() (bool, error) { return false, nil }) // 当日已执行（如手动 ExecuteDay）
	s.catchUpToday()
	if ran {
		t.Error("判定当日已执行 → 不应补跑")
	}
	if !sameDate(s.jobs[0].lastRun, now) {
		t.Error("跳过补跑也应标记今日已尝试，防 ticker 重复触发")
	}
}

func TestCatchUpTodaySkipsPlainRegister(t *testing.T) {
	now := cstTime(2026, 8, 21, 20, 0)
	s := pinnedScheduler(now)
	ran := false
	s.Register("legacy", 19, 35, func() error { ran = true; return nil }) // 未声明补跑语义
	s.catchUpToday()
	if ran {
		t.Error("未声明补跑语义 → 维持重启当天不补跑")
	}
	if !sameDate(s.jobs[0].lastRun, now) {
		t.Error("普通任务 lastRun 应为今天（注册时已置 today）")
	}
}

func TestCatchUpTodayBeforeTrigger(t *testing.T) {
	now := cstTime(2026, 8, 21, 10, 0) // 未到 19:35
	s := pinnedScheduler(now)
	ran := false
	s.RegisterCatchUp("auto-trade", 19, 35, func() error { ran = true; return nil },
		func() (bool, error) { return true, nil })
	s.catchUpToday()
	if ran {
		t.Error("未到触发时刻 → 不应补跑")
	}
	if !sameDate(s.jobs[0].lastRun, now.AddDate(0, 0, -1)) {
		t.Error("未到点 → lastRun 保持昨天，交给 ticker 正常触发")
	}
}

func TestCatchUpTodayPredicateErrorSkips(t *testing.T) {
	now := cstTime(2026, 8, 21, 20, 0)
	s := pinnedScheduler(now)
	ran := false
	s.RegisterCatchUp("auto-trade", 19, 35, func() error { ran = true; return nil },
		func() (bool, error) { return false, errors.New("db down") })
	s.catchUpToday()
	if ran {
		t.Error("补跑判定出错 → 应保守跳过，不补跑")
	}
	if !sameDate(s.jobs[0].lastRun, now) {
		t.Error("判定出错也应标记今日已尝试，防 ticker 绕过判定直接执行")
	}
}

func TestCatchUpTodayFailedRunRetries(t *testing.T) {
	now := cstTime(2026, 8, 21, 20, 0)
	s := pinnedScheduler(now)
	ran := 0
	s.RegisterCatchUp("auto-trade", 19, 35, func() error {
		ran++
		if ran == 1 {
			return errors.New("first fail")
		}
		return nil
	}, func() (bool, error) { return true, nil })
	s.catchUpToday()
	if ran != 1 {
		t.Fatalf("首次执行失败后 ran=%d, want 1", ran)
	}
	if !s.jobs[0].lastRun.IsZero() {
		t.Error("补跑失败 → lastRun 应置零，交 ticker 每 30s 重试")
	}
	s.catchUpToday()
	if ran != 2 {
		t.Errorf("失败后应允许再次补跑（模拟 ticker 重试），ran=%d, want 2", ran)
	}
}

// ---------- 集成：TaskRunService.HasRun + 端到端补跑（TEST_DB_DSN 门控） ----------

func mustHasRun(t *testing.T, svc *TaskRunService, name string, td time.Time) bool {
	t.Helper()
	ok, err := svc.HasRun(name, td)
	if err != nil {
		t.Fatalf("HasRun 查询失败: %v", err)
	}
	return ok
}

func TestSchedulerCatchUpIntegration(t *testing.T) {
	db := freshDB(t)
	td := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	svc := NewTaskRunService(db)

	if ok := mustHasRun(t, svc, "auto_trade", td); ok {
		t.Fatal("空表 HasRun 应为 false")
	}
	if err := svc.Record("auto_trade", td, "success", "买入 1 / 卖出 0", nil); err != nil {
		t.Fatalf("Record 失败: %v", err)
	}
	if ok := mustHasRun(t, svc, "auto_trade", td); !ok {
		t.Fatal("有记录 HasRun 应为 true")
	}

	// 场景 1：当日无台账 → 补跑并写台账
	if err := db.Where("task_name = ? AND run_date = ?", "auto_trade", td).
		Delete(&model.TaskRun{}).Error; err != nil {
		t.Fatalf("清理台账失败: %v", err)
	}
	ran := false
	catchUp := func() (bool, error) { return !mustHasRun(t, svc, "auto_trade", td), nil }
	s := pinnedScheduler(cstTime(2026, 8, 21, 20, 0))
	s.RegisterCatchUp("auto-trade", 19, 35, func() error {
		ran = true
		return svc.Record("auto_trade", td, "success", "补跑", nil)
	}, catchUp)
	s.catchUpToday()
	if !ran {
		t.Fatal("无台账 → 应补跑")
	}
	if ok := mustHasRun(t, svc, "auto_trade", td); !ok {
		t.Fatal("补跑应写台账")
	}

	// 场景 2：已有台账 → 不补跑
	ran = false
	s2 := pinnedScheduler(cstTime(2026, 8, 21, 21, 0))
	s2.RegisterCatchUp("auto-trade", 19, 35, func() error { ran = true; return nil }, catchUp)
	s2.catchUpToday()
	if ran {
		t.Error("已有台账 → 不应补跑")
	}
}
