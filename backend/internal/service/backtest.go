package service

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"quant-system/backend/internal/model"
	"quant-system/backend/internal/repository"
)

// 回测任务校验错误
var (
	ErrBacktestRange     = errors.New("回测起始日不能晚于结束日")
	ErrBacktestSpan      = errors.New("回测区间不能超过5年")
	ErrBacktestTopN      = errors.New("目标持仓数应在1-50之间")
	ErrBacktestNotFound  = errors.New("回测任务不存在")
)

// maxBacktestSpanYears 区间上限（内存重放预热 120 天，超长区间耗时线性增长）
const maxBacktestSpanYears = 5

// BacktestService 回测任务服务：参数校验 + 幂等创建 + 查询
type BacktestService struct {
	repo *repository.BacktestRepository
}

func NewBacktestService(repo *repository.BacktestRepository) *BacktestService {
	return &BacktestService{repo: repo}
}

// CreateJob 校验并创建任务（幂等：同参数返回已有任务）
func (s *BacktestService) CreateJob(start, end time.Time, topN int) (*model.BacktestJob, error) {
	if !start.Before(end) {
		return nil, ErrBacktestRange
	}
	if end.Sub(start) > maxBacktestSpanYears*365*24*time.Hour {
		return nil, ErrBacktestSpan
	}
	if topN < 1 || topN > 50 {
		return nil, ErrBacktestTopN
	}
	j := &model.BacktestJob{
		StrategyName: "multi_factor",
		StartDate:    start,
		EndDate:      end,
		TopN:         topN,
	}
	return s.repo.CreateJob(j)
}

// List 最近任务列表
func (s *BacktestService) List(limit int) ([]model.BacktestJob, error) {
	return s.repo.ListJobs(limit)
}

// Get 任务详情；不存在返回 ErrBacktestNotFound
func (s *BacktestService) Get(id uint64) (*model.BacktestJob, error) {
	j, err := s.repo.GetJobDetail(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrBacktestNotFound
	}
	if err != nil {
		return nil, err
	}
	return j, nil
}
