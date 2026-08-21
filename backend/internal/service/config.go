package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"quant-system/backend/internal/model"
)

// TushareConfigService 数据源配置（app_config.tushare.token）：
// token 页面可改、存数据库，值以库为准；与 Python 侧 sources/tushare.py
// load_token 同口径（不读环境变量，代码里不出现 token）。
type TushareConfigService struct {
	db *gorm.DB
}

func NewTushareConfigService(db *gorm.DB) *TushareConfigService {
	return &TushareConfigService{db: db}
}

// TushareConfigDTO 数据源配置（GET：只回 configured + 掩码，不回显完整 token）
type TushareConfigDTO struct {
	Configured  bool   `json:"configured"`   // 是否已配置 token
	TokenMasked string `json:"token_masked"` // "****abcd"；未配置为 ""
}

// Get 查询数据源配置
func (s *TushareConfigService) Get() (TushareConfigDTO, error) {
	var row model.AppConfig
	err := s.db.Where("key = ?", "tushare.token").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || row.Value == "" {
		return TushareConfigDTO{}, nil
	}
	if err != nil {
		return TushareConfigDTO{}, err
	}
	return TushareConfigDTO{
		Configured:  true,
		TokenMasked: maskToken(row.Value),
	}, nil
}

// Update 保存数据源配置（空 token = 清空，回到 AkShare 数据源）
func (s *TushareConfigService) Update(token string) error {
	token = strings.TrimSpace(token)
	row := model.AppConfig{
		Key:         "tushare.token",
		Value:       token,
		ValueType:   "secret",
		Description: "Tushare Pro token（数据主源，页面配置）",
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "value_type", "description", "updated_at"}),
	}).Create(&row).Error
}

// TestConnection 测试 Tushare 连接：请求 trade_cal 验证 token 有效性。
// 入参 token 为空时用库中已存 token（区分"测新值"与"测已存值"）。
func (s *TushareConfigService) TestConnection(token string) error {
	if token == "" {
		var row model.AppConfig
		if err := s.db.Where("key = ?", "tushare.token").First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("未配置 Tushare token")
			}
			return err
		}
		if row.Value == "" {
			return errors.New("未配置 Tushare token")
		}
		token = row.Value
	}
	today := time.Now().Format("20060102")
	payload, err := json.Marshal(map[string]interface{}{
		"api_name": "trade_cal",
		"token":    token,
		"params":   map[string]interface{}{"exchange": "SSE", "start_date": today, "end_date": today},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", "http://api.tushare.pro", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接 Tushare：%v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("Tushare 响应解析失败：%v", err)
	}
	if out.Code != 0 || len(out.Data) == 0 || string(out.Data) == "null" {
		msg := strings.TrimSpace(out.Msg)
		if msg == "" {
			msg = fmt.Sprintf("Tushare 返回错误 code=%d", out.Code)
		}
		return errors.New(msg)
	}
	return nil
}

func maskToken(t string) string {
	t = strings.TrimSpace(t)
	if len(t) <= 4 {
		return "****"
	}
	return "****" + t[len(t)-4:]
}
