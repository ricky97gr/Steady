package model

import (
	"time"

	"gorm.io/datatypes"
)

// TaskRun 任务执行记录（对应表 task_run）：监控/对账数据源。
// 与 quant-engine SQLAlchemy 侧同构，DDL 以 deploy/postgres/init.sql 为准。
type TaskRun struct {
	ID        uint64         `gorm:"primaryKey"`
	TaskName  string         `gorm:"size:64;not null;uniqueIndex:uq_task_run"`
	RunDate   time.Time      `gorm:"type:date;not null;uniqueIndex:uq_task_run"` // 业务交易日
	Status    string         `gorm:"size:16;not null"`                           // success/skipped/failed
	Message   string         `gorm:"type:text"`
	Detail    datatypes.JSON `gorm:"type:jsonb"` // 结构化明细（供页面 / 后续大模型消费）
	CreatedAt time.Time
}

func (TaskRun) TableName() string { return "task_run" }

// NotifyConfig 通知事件配置（对应表 notify_config）：页面可改的开关 / 调度规则
type NotifyConfig struct {
	EventKey     string    `gorm:"primaryKey"`
	Name         string    `gorm:"size:50;not null"`
	Enabled      bool      `gorm:"not null;default:true"`
	ScheduleType string  `gorm:"size:16;not null;default:trading_day"` // weekday/trading_day/event
	Weekdays     string  `gorm:"type:text"`                            // '1,2,3,4,5'（1=周一..7=周日）
	SendAt       *string `gorm:"type:time"`                            // HH:MM:SS；event 型为 NULL
	Template     string  `gorm:"size:16;not null;default:blue"`
	UpdatedAt    time.Time
}

func (NotifyConfig) TableName() string { return "notify_config" }

// AppConfig 应用配置键值表（对应表 app_config）：飞书 + 大模型预留，页面可改
type AppConfig struct {
	Key         string    `gorm:"primaryKey"`
	Value       string    `gorm:"type:text"`
	ValueType   string    `gorm:"size:16;not null;default:string"` // bool/int/string/secret
	Description string    `gorm:"type:text"`
	UpdatedAt   time.Time
}

func (AppConfig) TableName() string { return "app_config" }
