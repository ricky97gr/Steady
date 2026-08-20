package model

import (
	"time"

	"gorm.io/datatypes"
)

// MorningBrief 早盘简报（对应表 morning_brief，Issue #4）。
// quant-engine 09:10 组装落库；Sections 结构见 quant-engine/app/morning_brief.py
// assemble_brief：{brief_date, trade_date, is_open_today, market, yesterday, today}。
type MorningBrief struct {
	BriefDate time.Time      `gorm:"type:date;primaryKey"`
	Sections  datatypes.JSON `gorm:"type:jsonb;not null"`
	CreatedAt time.Time
}

func (MorningBrief) TableName() string { return "morning_brief" }
