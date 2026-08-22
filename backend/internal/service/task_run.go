package service

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"quant-system/backend/internal/model"
)

// TaskRunService 任务执行账本（对应表 task_run，与 quant-engine app/task_run.py 同构）。
// 用途：通知调度器做「该做没做」检查与失败告警；页面展示最近任务执行状态；
// detail 为结构化明细（LLM-ready）。
type TaskRunService struct {
	db *gorm.DB
}

func NewTaskRunService(db *gorm.DB) *TaskRunService { return &TaskRunService{db: db} }

// Record 幂等写一条任务执行记录：同 (task_name, run_date) 冲突时更新
// status/message/detail，保留首次 created_at。best-effort，失败仅返回 error 由调用方记录。
func (s *TaskRunService) Record(taskName string, runDate time.Time,
	status, message string, detail interface{}) error {

	var detailJSON []byte
	if detail != nil {
		detailJSON, _ = json.Marshal(detail)
	}
	row := model.TaskRun{
		TaskName: taskName,
		RunDate:  runDate,
		Status:   status,
		Message:  message,
		Detail:   detailJSON,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_name"}, {Name: "run_date"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "message", "detail"}),
	}).Create(&row).Error
}

// HasRun 指定任务在指定日是否已有执行记录（Scheduler 启动补跑判定用）。
// best-effort：查询失败返回 (false, err)，由调用方决定是否跳过补跑。
func (s *TaskRunService) HasRun(taskName string, runDate time.Time) (bool, error) {
	var n int64
	err := s.db.Model(&model.TaskRun{}).
		Where("task_name = ? AND run_date = ?", taskName, runDate).
		Count(&n).Error
	return n > 0, err
}

// ListRecent 最近任务执行记录（页面展示）
func (s *TaskRunService) ListRecent(limit int) ([]model.TaskRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var items []model.TaskRun
	err := s.db.Order("run_date desc, created_at desc").Limit(limit).Find(&items).Error
	return items, err
}
