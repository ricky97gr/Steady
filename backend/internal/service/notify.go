package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"quant-system/backend/internal/model"
)

// NotifyService 飞书通知：
// - 定时推送统一由 quant-engine 通知调度器负责（本服务不参与定时）
// - 本服务仅处理用户主动触发的推送（测试卡片 / 手动执行交易卡片）
// - 配置读写 app_config 表（页面可改；与 Python 侧 load_config 同口径）
type NotifyService struct {
	db *gorm.DB
}

func NewNotifyService(db *gorm.DB) *NotifyService { return &NotifyService{db: db} }

// NotifyEventDTO 通知事件配置（GET 返回 / PUT 接收）
type NotifyEventDTO struct {
	EventKey     string  `json:"event_key"`
	Name         string  `json:"name"`
	Enabled      bool    `json:"enabled"`
	ScheduleType string  `json:"schedule_type"` // weekday / trading_day / event
	Weekdays     string  `json:"weekdays"`      // '1,2,3,4,5'（1=周一..7=周日）
	SendAt       *string `json:"send_at"`       // HH:MM；event 型为 null
	Template     string  `json:"template"`
}

// FeishuConfigDTO 飞书配置（app_config.feishu.*）
type FeishuConfigDTO struct {
	Enabled      bool   `json:"enabled"`
	WebhookURL   string `json:"webhook_url"`
	DashboardURL string `json:"dashboard_url"`
	Timeout      int    `json:"timeout"`
	MaxRetries   int    `json:"max_retries"`
	Secret       string `json:"secret"`  // 签名校验密钥；留空=不签名
	AtAll        bool   `json:"at_all"`  // 通知卡片 @所有人
}

// ---------- 通知事件配置 ----------

func (s *NotifyService) ListEvents() ([]NotifyEventDTO, error) {
	var rows []model.NotifyConfig
	if err := s.db.Order("event_key").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]NotifyEventDTO, 0, len(rows))
	for _, r := range rows {
		d := NotifyEventDTO{
			EventKey: r.EventKey, Name: r.Name, Enabled: r.Enabled,
			ScheduleType: r.ScheduleType, Weekdays: r.Weekdays, Template: r.Template,
		}
		if r.SendAt != nil {
			v := *r.SendAt
			if len(v) >= 5 {
				v = v[:5] // "19:30:00" → "19:30"
			}
			d.SendAt = &v
		}
		out = append(out, d)
	}
	return out, nil
}

// UpdateEvent 更新单个通知事件配置（event 型忽略 send_at/weekdays）
func (s *NotifyService) UpdateEvent(eventKey string, d NotifyEventDTO) error {
	if d.ScheduleType != "weekday" && d.ScheduleType != "trading_day" &&
		d.ScheduleType != "event" {
		return errors.New("schedule_type 仅支持 weekday / trading_day / event")
	}
	var row model.NotifyConfig
	if err := s.db.First(&row, "event_key = ?", eventKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotifyEventMissing
		}
		return err
	}
	row.Enabled = d.Enabled
	row.ScheduleType = d.ScheduleType
	row.Template = d.Template
	row.Weekdays = ""
	row.SendAt = nil
	if d.ScheduleType == "weekday" {
		row.Weekdays = d.Weekdays
	}
	if d.ScheduleType == "weekday" || d.ScheduleType == "trading_day" {
		if d.SendAt == nil || *d.SendAt == "" {
			return errors.New("定时事件必须配置 send_at (HH:MM)")
		}
		t, err := time.Parse("15:04", *d.SendAt)
		if err != nil {
			return errors.New("send_at 格式应为 HH:MM")
		}
		s := t.Format("15:04:05")
		row.SendAt = &s
	}
	return s.db.Save(&row).Error
}

// ---------- 飞书配置 ----------

func (s *NotifyService) GetFeishuConfig() (FeishuConfigDTO, error) {
	var rows []model.AppConfig
	if err := s.db.Where("key LIKE 'feishu.%'").Find(&rows).Error; err != nil {
		return FeishuConfigDTO{}, err
	}
	vals := map[string]string{}
	for _, r := range rows {
		vals[r.Key] = r.Value
	}
	return FeishuConfigDTO{
		Enabled:      vals["feishu.enabled"] == "1" || vals["feishu.enabled"] == "true",
		WebhookURL:   vals["feishu.webhook_url"],
		DashboardURL: vals["feishu.dashboard_url"],
		Timeout:      atoiDef(vals["feishu.timeout"], 10),
		MaxRetries:   atoiDef(vals["feishu.max_retries"], 2),
		Secret:       vals["feishu.secret"],
		AtAll:        vals["feishu.at_all"] == "1" || vals["feishu.at_all"] == "true",
	}, nil
}

func (s *NotifyService) UpdateFeishuConfig(d FeishuConfigDTO) error {
	rows := []model.AppConfig{
		{Key: "feishu.enabled", Value: strconv.FormatBool(d.Enabled), ValueType: "bool"},
		{Key: "feishu.webhook_url", Value: d.WebhookURL, ValueType: "secret"},
		{Key: "feishu.dashboard_url", Value: d.DashboardURL, ValueType: "string"},
		{Key: "feishu.timeout", Value: strconv.Itoa(d.Timeout), ValueType: "int"},
		{Key: "feishu.max_retries", Value: strconv.Itoa(d.MaxRetries), ValueType: "int"},
		{Key: "feishu.secret", Value: d.Secret, ValueType: "secret"},
		{Key: "feishu.at_all", Value: strconv.FormatBool(d.AtAll), ValueType: "bool"},
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "value_type", "updated_at"}),
	}).Create(&rows).Error
}

// ---------- 发送 ----------

// Ready 是否可推送（启用且已配置 webhook）
func (s *NotifyService) Ready() (bool, error) {
	cfg, err := s.GetFeishuConfig()
	if err != nil {
		return false, err
	}
	return cfg.Enabled && cfg.WebhookURL != "", nil
}

// SendCard 发送飞书富文本卡片（同步；用户主动触发的推送）
func (s *NotifyService) SendCard(title, content, template, footer string) error {
	cfg, err := s.GetFeishuConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled || cfg.WebhookURL == "" {
		return errors.New("飞书通知未启用或未配置 webhook，请先在设置页配置")
	}
	if cfg.AtAll { // @所有人：lark_md 内容前置 @ 标签，触发全员提及通知
		content = `<at id="all"></at>` + "\n\n" + content
	}
	payload := buildCardPayload(title, content, template, footer)
	if cfg.Secret != "" { // 开启签名校验：请求体携带 timestamp + sign
		ts := time.Now().Unix()
		payload["timestamp"] = strconv.FormatInt(ts, 10)
		payload["sign"] = genSign(cfg.Secret, ts)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second}
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var fb struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&fb)
		resp.Body.Close()
		if fb.Code == 0 {
			return nil
		}
		lastErr = fmt.Errorf("飞书返回错误 code=%d msg=%s", fb.Code, fb.Msg)
	}
	return fmt.Errorf("飞书推送失败（重试 %d 次）：%v", cfg.MaxRetries, lastErr)
}

// SendTest 发送测试卡片
func (s *NotifyService) SendTest() error {
	cfg, err := s.GetFeishuConfig()
	if err != nil {
		return err
	}
	content := "**测试卡片**\n\n如果能看到这条消息，说明飞书机器人配置正常。\n\n" +
		"[打开 Dashboard](" + dashboardURL(cfg.DashboardURL) + ")"
	return s.SendCard("Steady · 测试通知", content, "green", "测试推送")
}

// ---------- 辅助 ----------

var ErrNotifyEventMissing = errors.New("通知事件不存在")

func atoiDef(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func dashboardURL(d string) string {
	if d == "" {
		return "http://localhost"
	}
	return d
}

// genSign 飞书机器人签名：sign = base64(hmac_sha256(key=f"{ts}\n{secret}", data=空))
// 与 quant-engine/app/notify.py gen_sign 同算法（官方：string_to_sign 作 HMAC 密钥）。
func genSign(secret string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(fmt.Sprintf("%d\n%s", ts, secret)))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func buildCardPayload(title, content, template, footer string) map[string]interface{} {
	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": content},
		},
		map[string]interface{}{"tag": "hr"},
	}
	if footer != "" {
		elements = append(elements, map[string]interface{}{
			"tag":      "note",
			"elements": []interface{}{map[string]interface{}{"tag": "plain_text", "content": footer}},
		})
	}
	return map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"config":   map[string]interface{}{"wide_screen_mode": true},
			"header":   map[string]interface{}{"template": template, "title": map[string]interface{}{"tag": "plain_text", "content": title}},
			"elements": elements,
		},
	}
}
