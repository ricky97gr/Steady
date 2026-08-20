package service

import (
	"testing"
	"time"
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
