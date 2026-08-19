package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"quant-system/backend/internal/model"
)

// BacktestRepository 回测任务/结果数据访问层（状态迁移由引擎侧执行）
type BacktestRepository struct {
	db *gorm.DB
}

func NewBacktestRepository(db *gorm.DB) *BacktestRepository {
	return &BacktestRepository{db: db}
}

// CreateJob 创建任务（幂等：同参数已存在直接返回；failed 任务重置为 pending 重跑，
// 与引擎 backtest_service.create_job 语义一致）
func (r *BacktestRepository) CreateJob(j *model.BacktestJob) (*model.BacktestJob, error) {
	err := r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "strategy_name"}, {Name: "start_date"},
			{Name: "end_date"}, {Name: "top_n"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":      "pending",
			"error":       nil,
			"finished_at": nil,
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "backtest_job.status = 'failed'"},
		}},
	}).Create(j).Error
	if err != nil {
		return nil, err
	}
	// 冲突时不返回新行，按参数重新查询
	return r.GetByParams(j.StrategyName, j.StartDate, j.EndDate, j.TopN)
}

// GetByParams 按唯一键查询
func (r *BacktestRepository) GetByParams(strategy string, start, end interface{}, topN int) (*model.BacktestJob, error) {
	var j model.BacktestJob
	err := r.db.Where("strategy_name = ? AND start_date = ? AND end_date = ? AND top_n = ?",
		strategy, start, end, topN).First(&j).Error
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// ListJobs 任务列表（含结果，按创建时间倒序）
func (r *BacktestRepository) ListJobs(limit int) ([]model.BacktestJob, error) {
	var jobs []model.BacktestJob
	err := r.db.Preload("Result").Order("id DESC").Limit(limit).Find(&jobs).Error
	return jobs, err
}

// GetJobDetail 任务详情（含结果）；不存在返回 gorm.ErrRecordNotFound
func (r *BacktestRepository) GetJobDetail(id uint64) (*model.BacktestJob, error) {
	var j model.BacktestJob
	err := r.db.Preload("Result").First(&j, id).Error
	if err != nil {
		return nil, err
	}
	return &j, nil
}
