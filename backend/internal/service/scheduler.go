package service

import (
	"time"

	"go.uber.org/zap"
)

// Scheduler 定时任务调度：goroutine + ticker，无外部依赖。
// 任务按 Asia/Shanghai 时区的时:分触发，每天最多一次；
// 失败后每 30s 重试（错误日志 10 分钟节流），重启当天不补跑。
type Scheduler struct {
	loc  *time.Location
	log  *zap.Logger
	jobs []*schedJob
}

type schedJob struct {
	name      string
	hour      int
	minute    int
	lastRun   time.Time // 日期部分：当天已尝试过（含失败）
	lastLog   time.Time // 错误日志节流
	run       func() error
}

// NewScheduler 调度器（Asia/Shanghai）
func NewScheduler(log *zap.Logger) *Scheduler {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &Scheduler{loc: loc, log: log}
}

// Register 注册每日任务。
// 重启当天：若注册时当天触发时刻已过 → 当天不补跑（lastRun=今天，次日恢复）；
// 若注册时尚未到触发时刻 → 当天照常执行（lastRun=昨天，避免整日跳过）。
func (s *Scheduler) Register(name string, hour, minute int, fn func() error) {
	now := time.Now().In(s.loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	lastRun := today
	triggerToday := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, s.loc)
	if now.Before(triggerToday) {
		// 当天触发时刻尚未到 → 当天照常执行（lastRun 置为昨天，避免整日跳过）
		lastRun = today.AddDate(0, 0, -1)
	}
	s.jobs = append(s.jobs, &schedJob{
		name: name, hour: hour, minute: minute, lastRun: lastRun, run: fn,
	})
}

// Start 启动调度循环（阻塞，应放入 goroutine）
func (s *Scheduler) Start() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().In(s.loc)
		for _, j := range s.jobs {
			if now.Hour()*60+now.Minute() < j.hour*60+j.minute {
				continue // 未到触发时间
			}
			if sameDate(j.lastRun, now) {
				continue // 今天已尝试
			}
			j.lastRun = now
			if err := j.run(); err != nil {
				j.lastRun = time.Time{} // 失败：下一 tick 重试
				if now.Sub(j.lastLog) > 10*time.Minute {
					s.log.Error("定时任务执行失败", zap.String("job", j.name), zap.Error(err))
					j.lastLog = now
				}
				continue
			}
			s.log.Info("定时任务执行完成", zap.String("job", j.name))
		}
	}
}

func sameDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
