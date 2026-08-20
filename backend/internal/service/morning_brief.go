package service

import (
	"time"

	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// MorningBriefService 早盘简报只读接口（对应表 morning_brief，Issue #4）。
// 数据由 quant-engine 09:10 组装落库；这里只读，不做写。
type MorningBriefService struct {
	db *gorm.DB
}

func NewMorningBriefService(db *gorm.DB) *MorningBriefService {
	return &MorningBriefService{db: db}
}

// GetByDate 按日期取早报；该日无早报返回 (nil, nil)
func (s *MorningBriefService) GetByDate(briefDate time.Time) (*model.MorningBrief, error) {
	var row model.MorningBrief
	err := s.db.Where("brief_date = ?", briefDate).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Latest 最近一份早报；无数据返回 (nil, nil)
func (s *MorningBriefService) Latest() (*model.MorningBrief, error) {
	var row model.MorningBrief
	err := s.db.Order("brief_date desc").First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
