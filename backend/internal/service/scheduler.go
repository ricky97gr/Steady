package service

import (
	"time"

	"go.uber.org/zap"
)

// Scheduler 定时任务调度：goroutine + ticker，无外部依赖。
// 任务按 Asia/Shanghai 时区的时:分触发，每天最多一次；
// 失败后每 30s 重试（错误日志 10 分钟节流）。
// 重启当天：注册了补跑语义（RegisterCatchUp）的任务，若重启时当日触发时刻已过
// 且判定「当日尚未执行」，启动时立即补跑一次；未声明补跑语义（Register）维持
// 「重启当天不补跑」。
type Scheduler struct {
	loc  *time.Location
	log  *zap.Logger
	jobs []*schedJob
	now  func() time.Time // 可注入时钟（测试用）；nil 默认 time.Now
}

type schedJob struct {
	name      string
	hour      int
	minute    int
	lastRun   time.Time // 日期部分：当天已尝试过（含失败）
	lastLog   time.Time // 错误日志节流
	run       func() error
	catchUp   func() (bool, error) // 启动补跑判定：nil = 维持「重启当天不补跑」
}

// NewScheduler 调度器（Asia/Shanghai）
func NewScheduler(log *zap.Logger) *Scheduler {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &Scheduler{loc: loc, log: log}
}

// nowIn 当前时间（Asia/Shanghai）；注入时钟时以其为准
func (s *Scheduler) nowIn(loc *time.Location) time.Time {
	if s.now != nil {
		return s.now().In(loc)
	}
	return time.Now().In(loc)
}

// Register 注册每日任务（不声明补跑语义）。
// 重启当天：若注册时当天触发时刻已过 → 当天不补跑（lastRun=今天，次日恢复）；
// 若注册时尚未到触发时刻 → 当天照常执行（lastRun=昨天，避免整日跳过）。
func (s *Scheduler) Register(name string, hour, minute int, fn func() error) {
	now := s.nowIn(s.loc)
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

// RegisterCatchUp 注册每日任务并声明补跑语义：重启后若当天触发时刻已过，
// 且 catchUp() 判定「当日尚未执行」→ Start 启动时立即补跑一次。
// lastRun 无论注册时是否已过触发时刻都置昨天：已过点由 catchUpToday 补跑，
// 未到点由 ticker 正常触发，两者保证当天各只执行一次。
func (s *Scheduler) RegisterCatchUp(name string, hour, minute int, fn func() error,
	catchUp func() (bool, error)) {
	now := s.nowIn(s.loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	s.jobs = append(s.jobs, &schedJob{
		name: name, hour: hour, minute: minute,
		lastRun: today.AddDate(0, 0, -1), run: fn, catchUp: catchUp,
	})
}

// Start 启动调度循环（阻塞，应放入 goroutine）。
// 先执行 catchUpToday 补跑当天已错过的任务，再进入 30s ticker。
func (s *Scheduler) Start() {
	s.catchUpToday()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := s.nowIn(s.loc)
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

// catchUpToday 启动补跑（只在进程刚启动时跑一次）。
// 判定：当天触发时刻已过 且 尚未尝试 → 有 catchUp 语义则按判定执行；
// 无 catchUp（原 Register）维持「重启当天不补跑」；隔天不补（数据已过期）。
// 失败的任务置 lastRun=time.Time{}，由 ticker 每 30s 重试。
func (s *Scheduler) catchUpToday() {
	now := s.nowIn(s.loc)
	for _, j := range s.jobs {
		triggerToday := time.Date(now.Year(), now.Month(), now.Day(), j.hour, j.minute, 0, 0, s.loc)
		if now.Before(triggerToday) {
			continue // 未到触发时间 → 交给 ticker 正常触发
		}
		if sameDate(j.lastRun, now) {
			continue // 今日已尝试过（Register 已过触发时刻的情形）
		}
		j.lastRun = now // 无论是否补跑都标记今日已尝试，防 ticker 重复触发
		if j.catchUp == nil {
			continue // 未声明补跑语义 → 维持原「重启当天不补跑」
		}
		ok, err := j.catchUp()
		if err != nil {
			s.log.Warn("启动补跑判定失败，跳过", zap.String("job", j.name), zap.Error(err))
			continue
		}
		if !ok {
			continue // 当日已执行过（如手动 ExecuteDay）→ 不重复补跑
		}
		if err := j.run(); err != nil {
			j.lastRun = time.Time{} // 失败：ticker 30s 重试
			s.log.Error("启动补跑执行失败", zap.String("job", j.name), zap.Error(err))
			continue
		}
		s.log.Info("定时任务启动补跑完成", zap.String("job", j.name))
	}
}

func sameDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
