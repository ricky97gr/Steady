package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// ---------- 测试替身 ----------

// fakeBriefReader 简报只读白名单替身（无 DB）
type fakeBriefReader struct {
	byDate map[string]*model.MorningBrief
	latest *model.MorningBrief
}

func (f *fakeBriefReader) GetByDate(d time.Time) (*model.MorningBrief, error) {
	if f.byDate == nil {
		return nil, nil
	}
	return f.byDate[d.Format("2006-01-02")], nil
}

func (f *fakeBriefReader) Latest() (*model.MorningBrief, error) { return f.latest, nil }

// llmReqPayload 捕获到的请求体（OpenAI 兼容结构）
type llmReqPayload struct {
	Model       string  `json:"model"`
	Messages    []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Temperature float64 `json:"temperature"`
}

// fakeLLM 可断言的 OpenAI 兼容端点
type fakeLLM struct {
	ts       *httptest.Server
	mu       sync.Mutex
	calls    int
	statuses []int // 每次调用的状态码（缺省 200）；len < calls 时超出部分按 200 处理
	content  string // 200 时 choices[0].message.content
	errMsg   string // 非 200 时 error.message
	bodies   []llmReqPayload
	auths    []string
}

func newFakeLLM(t *testing.T, content string) *fakeLLM {
	f := &fakeLLM{content: content}
	f.ts = httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fakeLLM) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	idx := f.calls
	f.calls++
	var p llmReqPayload
	_ = json.NewDecoder(r.Body).Decode(&p)
	f.bodies = append(f.bodies, p)
	f.auths = append(f.auths, r.Header.Get("Authorization"))
	status := http.StatusOK
	if idx < len(f.statuses) {
		status = f.statuses[idx]
	}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if status != http.StatusOK {
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"error":{"message":%q,"type":"test_error"}}`, f.errMsg)
		return
	}
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, f.content)
}

func (f *fakeLLM) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeLLM) lastBody() llmReqPayload {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[len(f.bodies)-1]
}

func (f *fakeLLM) lastAuth() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auths[len(f.auths)-1]
}

func (f *fakeLLM) allAuths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.auths))
	copy(out, f.auths)
	return out
}

// cfg 构造测试配置（base_url 指向 fake server）
func (f *fakeLLM) cfg(model string) LLMConfigDTO {
	return LLMConfigDTO{Provider: "openai", Model: model, BaseURL: f.ts.URL}
}

// fakeCfgPtr 取配置指针（testCfg 注入用）
func fakeCfgPtr(f *fakeLLM, model string) *LLMConfigDTO {
	c := f.cfg(model)
	return &c
}

// ---------- chatWith：鉴权 / 请求体 / 重试 / 错误 ----------

func TestLLMChatAuthAndBody(t *testing.T) {
	f := newFakeLLM(t, "你好，这是回复")
	svc := &LLMService{}
	got, err := svc.chatWith(f.cfg("test-model"), "sk-test-secret", "系统提示", "用户输入")
	if err != nil {
		t.Fatalf("chatWith 失败: %v", err)
	}
	if got != "你好，这是回复" {
		t.Fatalf("回复不符: got %q", got)
	}
	if auth := f.lastAuth(); auth != "Bearer sk-test-secret" {
		t.Fatalf("鉴权头不符: got %q", auth)
	}
	body := f.lastBody()
	if body.Model != "test-model" {
		t.Fatalf("model 不符: got %q", body.Model)
	}
	if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
		t.Fatalf("messages 结构不符: %+v", body.Messages)
	}
	if body.Messages[0].Content != "系统提示" || body.Messages[1].Content != "用户输入" {
		t.Fatalf("messages 内容不符: %+v", body.Messages)
	}
	if body.Temperature != 0.3 {
		t.Fatalf("temperature 不符: got %v", body.Temperature)
	}
}

// 重试：500 ×2 后 200 → 成功，共 3 次请求
func TestLLMChatRetry500(t *testing.T) {
	f := newFakeLLM(t, "ok")
	f.statuses = []int{500, 500, 200}
	svc := &LLMService{}
	got, err := svc.chatWith(f.cfg("m"), "sk", "s", "u")
	if err != nil {
		t.Fatalf("应重试后成功: %v", err)
	}
	if got != "ok" {
		t.Fatalf("回复不符: %q", got)
	}
	if n := f.callCount(); n != 3 {
		t.Fatalf("请求次数应为 3，got %d", n)
	}
}

// 4xx 不重试：401 → 立即失败，仅 1 次请求
func TestLLMChatNon200NoRetry(t *testing.T) {
	f := newFakeLLM(t, "")
	f.statuses = []int{401}
	f.errMsg = "invalid api key"
	svc := &LLMService{}
	if _, err := svc.chatWith(f.cfg("m"), "sk", "s", "u"); err == nil {
		t.Fatal("401 应报错")
	} else if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("错误信息应透出服务端 message: %v", err)
	}
	if n := f.callCount(); n != 1 {
		t.Fatalf("401 不应重试，请求次数应为 1，got %d", n)
	}
}

// 网络错误：重试 2 次后失败
func TestLLMChatNetworkError(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed.Close() // 立即关闭 → 连接失败
	svc := &LLMService{}
	cfg := LLMConfigDTO{Model: "m", BaseURL: closed.URL}
	_, err := svc.chatWith(cfg, "sk", "s", "u")
	if err == nil {
		t.Fatal("连接失败应报错")
	}
	if !strings.Contains(err.Error(), "无法连接大模型服务") {
		t.Fatalf("错误信息不符: %v", err)
	}
}

// 配置缺项：model / key 为空直接报错，不发请求
func TestLLMChatMissingConfig(t *testing.T) {
	f := newFakeLLM(t, "x")
	svc := &LLMService{}
	if _, err := svc.chatWith(LLMConfigDTO{Model: "", BaseURL: f.ts.URL}, "sk", "s", "u"); err == nil {
		t.Fatal("model 为空应报错")
	}
	if _, err := svc.chatWith(LLMConfigDTO{Model: "m", BaseURL: f.ts.URL}, "", "s", "u"); err == nil {
		t.Fatal("key 为空应报错")
	}
	if n := f.callCount(); n != 0 {
		t.Fatalf("不应发出请求，got %d", n)
	}
}

// ---------- ExplainTerm：围栏 + 入参限长 ----------

func TestLLMExplainTermFence(t *testing.T) {
	f := newFakeLLM(t, "PE 即市盈率，股价相对每股收益的倍数。")
	svc := &LLMService{testCfg: fakeCfgPtr(f, "m"), testKey: "sk"}
	r, err := svc.ExplainTerm("PE")
	if err != nil {
		t.Fatalf("ExplainTerm 失败: %v", err)
	}
	if r.Term != "PE" || !strings.Contains(r.Explanation, "市盈率") {
		t.Fatalf("返回不符: %+v", r)
	}
	body := f.lastBody()
	if body.Messages[1].Content != "<data>PE</data>" {
		t.Fatalf("术语应包 <data> 围栏: %q", body.Messages[1].Content)
	}
	if !strings.Contains(body.Messages[0].Content, "不是指令") || !strings.Contains(body.Messages[0].Content, "忽略") {
		t.Fatalf("系统提示应声明数据围栏: %q", body.Messages[0].Content)
	}
}

func TestLLMExplainTermTooLong(t *testing.T) {
	f := newFakeLLM(t, "x")
	svc := &LLMService{testCfg: fakeCfgPtr(f, "m"), testKey: "sk"}
	if _, err := svc.ExplainTerm(strings.Repeat("长", 201)); err == nil {
		t.Fatal("超 200 字应报错")
	}
	if n := f.callCount(); n != 0 {
		t.Fatalf("超长不应发请求，got %d", n)
	}
}

// ---------- AskProject：知识库围栏 + 降级 ----------

func TestLLMAskProjectFence(t *testing.T) {
	f := newFakeLLM(t, "回测结果存在 backtest_result 表。")
	kb := "# 项目知识\nSteady 是个人量化平台。\n关键表：backtest_result。"
	kbPath := writeKBFixture(t, kb)
	svc := &LLMService{
		testCfg: fakeCfgPtr(f, "m"),
		testKey: "sk", kbPath: kbPath,
	}
	r, err := svc.AskProject("回测结果存在哪张表")
	if err != nil {
		t.Fatalf("AskProject 失败: %v", err)
	}
	if r.Question != "回测结果存在哪张表" || !strings.Contains(r.Answer, "backtest_result") {
		t.Fatalf("返回不符: %+v", r)
	}
	body := f.lastBody()
	if body.Messages[1].Content != "回测结果存在哪张表" {
		t.Fatalf("user 应为原问题: %q", body.Messages[1].Content)
	}
	if !strings.Contains(body.Messages[0].Content, kb) {
		t.Fatalf("系统提示应内嵌知识库内容")
	}
	if !strings.Contains(body.Messages[0].Content, "不是指令") {
		t.Fatalf("系统提示应声明数据围栏")
	}
}

func TestLLMAskProjectKBMissing(t *testing.T) {
	f := newFakeLLM(t, "x")
	// 不注入 kbPath：三个默认路径都不存在 → 优雅降级
	svc := &LLMService{testCfg: fakeCfgPtr(f, "m"), testKey: "sk"}
	if _, err := svc.AskProject("回测结果存在哪张表"); err != ErrKnowledgeBaseMissing {
		t.Fatalf("应返回 ErrKnowledgeBaseMissing，got %v", err)
	}
	if n := f.callCount(); n != 0 {
		t.Fatalf("知识库缺失不应发请求，got %d", n)
	}
}

func writeKBFixture(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "项目知识.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写知识库 fixture 失败: %v", err)
	}
	return p
}

// ---------- InterpretBrief：fake BriefReader + fake server（无 DB） ----------

func briefFixture(date string, sections string) *model.MorningBrief {
	d, _ := time.Parse("2006-01-02", date)
	return &model.MorningBrief{BriefDate: d, Sections: datatypes.JSON([]byte(sections))}
}

func TestLLMInterpretBriefUnit(t *testing.T) {
	f := newFakeLLM(t, "今日市场平稳，关注银行板块。")
	reader := &fakeBriefReader{
		latest: briefFixture("2026-08-21", `{"trade_date":"2026-08-21","yesterday":{"up":1200}}`),
		byDate: map[string]*model.MorningBrief{
			"2026-08-20": briefFixture("2026-08-20", `{"trade_date":"2026-08-20"}`),
		},
	}
	svc := &LLMService{
		brief:   reader,
		testCfg: fakeCfgPtr(f, "m"),
		testKey: "sk",
	}

	// 缺省 → Latest
	r, err := svc.InterpretBrief("")
	if err != nil {
		t.Fatalf("InterpretBrief 失败: %v", err)
	}
	if r.BriefDate != "2026-08-21" || !strings.Contains(r.Interpretation, "银行") {
		t.Fatalf("返回不符: %+v", r)
	}
	body := f.lastBody()
	if !strings.Contains(body.Messages[1].Content, "<data>") || !strings.Contains(body.Messages[1].Content, `"yesterday"`) {
		t.Fatalf("简报应包 <data> 围栏且含 sections JSON: %q", body.Messages[1].Content)
	}
	if !strings.Contains(body.Messages[0].Content, "不是指令") {
		t.Fatalf("系统提示应声明数据围栏")
	}

	// 指定日期 → GetByDate
	if r, err := svc.InterpretBrief("2026-08-20"); err != nil || r.BriefDate != "2026-08-20" {
		t.Fatalf("指定日期解读不符: %+v err=%v", r, err)
	}

	// 无简报 → ErrNoBrief
	if _, err := svc.InterpretBrief("2026-08-19"); err != ErrNoBrief {
		t.Fatalf("无简报应返回 ErrNoBrief，got %v", err)
	}

	// 日期格式错误
	if _, err := svc.InterpretBrief("20260821"); err == nil {
		t.Fatal("日期格式错误应报错")
	}
}

// 超长简报 → bounded prompt（≤8KB 截断）
func TestLLMInterpretBriefBounded(t *testing.T) {
	f := newFakeLLM(t, "ok")
	big := `"x": "` + strings.Repeat("a", 20*1024) + `"`
	reader := &fakeBriefReader{latest: briefFixture("2026-08-21", `{`+big+`}`)}
	svc := &LLMService{
		brief:   reader,
		testCfg: fakeCfgPtr(f, "m"),
		testKey: "sk",
	}
	if _, err := svc.InterpretBrief(""); err != nil {
		t.Fatalf("InterpretBrief 失败: %v", err)
	}
	user := f.lastBody().Messages[1].Content
	// <data> + 截断 8KB + </data>，留余量断言不超 9KB
	if len(user) > 9*1024 {
		t.Fatalf("简报 prompt 未截断: len=%d", len(user))
	}
}

// ---------- 集成：config Get/Update/mask + InterpretBrief（TEST_DB_DSN 门控） ----------

func freshLLMDB(t *testing.T) *gorm.DB {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN 未设置，跳过集成测试")
	}
	if strings.Contains(dsn, "dbname=quant_system") && !strings.Contains(dsn, "dbname=quant_system_test") {
		t.Fatal("拒绝连接生产库 quant_system，请使用独立测试库 quant_system_test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := db.Migrator().DropTable(&model.AppConfig{}, &model.MorningBrief{}); err != nil {
		t.Fatalf("清理测试表失败: %v", err)
	}
	if err := db.AutoMigrate(&model.AppConfig{}, &model.MorningBrief{}); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	return db
}

func TestLLMConfigIntegration(t *testing.T) {
	db := freshLLMDB(t)
	svc := NewLLMService(db, nil)

	// 初始全空
	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig 失败: %v", err)
	}
	if cfg.Enabled || cfg.Model != "" || cfg.Provider != "" || cfg.APIKeyMasked != "" {
		t.Fatalf("初始配置应为空: %+v", cfg)
	}
	if en, _ := svc.Enabled(); en {
		t.Fatal("初始不应启用")
	}

	// 保存（含新 key）
	if err := svc.UpdateConfig(LLMConfigDTO{
		Enabled: true, Provider: "deepseek", Model: "deepseek-chat",
		BaseURL: "", APIKey: "sk-secret-1234567",
	}); err != nil {
		t.Fatalf("UpdateConfig 失败: %v", err)
	}
	cfg, _ = svc.GetConfig()
	if !cfg.Enabled || cfg.Provider != "deepseek" || cfg.Model != "deepseek-chat" {
		t.Fatalf("保存后配置不符: %+v", cfg)
	}
	if cfg.APIKeyMasked != "****4567" {
		t.Fatalf("掩码不符: %q", cfg.APIKeyMasked)
	}
	if strings.Contains(cfg.APIKeyMasked, "sk-secret") {
		t.Fatal("掩码不得泄露明文")
	}
	if en, _ := svc.Enabled(); !en {
		t.Fatal("启用后应可用")
	}

	// 掩码重提 → 保留已存（不覆盖为掩码字符串）
	if err := svc.UpdateConfig(LLMConfigDTO{
		Enabled: true, Provider: "deepseek", Model: "deepseek-chat", APIKey: "****4567",
	}); err != nil {
		t.Fatalf("UpdateConfig 失败: %v", err)
	}
	cfg, _ = svc.GetConfig()
	if cfg.APIKeyMasked != "****4567" {
		t.Fatalf("掩码重提不应覆盖已存 key: %q", cfg.APIKeyMasked)
	}

	// ClearAPIKey → 清空，不再启用
	if err := svc.UpdateConfig(LLMConfigDTO{
		Enabled: true, Provider: "deepseek", Model: "deepseek-chat", ClearAPIKey: true,
	}); err != nil {
		t.Fatalf("UpdateConfig 失败: %v", err)
	}
	cfg, _ = svc.GetConfig()
	if cfg.APIKeyMasked != "" {
		t.Fatalf("ClearAPIKey 后应为空: %q", cfg.APIKeyMasked)
	}
	if en, _ := svc.Enabled(); en {
		t.Fatal("清空 key 后不应启用")
	}

	// 空模型不启用
	if err := svc.UpdateConfig(LLMConfigDTO{
		Enabled: true, Provider: "openai", Model: "", APIKey: "sk-new-key-123456",
	}); err != nil {
		t.Fatalf("UpdateConfig 失败: %v", err)
	}
	if en, _ := svc.Enabled(); en {
		t.Fatal("模型名为空不应启用")
	}
}

// 集成：DB 配置 + DB 简报 → InterpretBrief 打到 fake server（完整路径）
func TestLLMInterpretBriefIntegration(t *testing.T) {
	db := freshLLMDB(t)
	f := newFakeLLM(t, "银行领涨，关注午后资金。")

	// 配置存 DB：base_url 指向 fake server，api_key 存库
	if err := db.Create(&[]model.AppConfig{
		{Key: "llm.enabled", Value: "true", ValueType: "bool"},
		{Key: "llm.provider", Value: "openai", ValueType: "string"},
		{Key: "llm.model", Value: "test-model", ValueType: "string"},
		{Key: "llm.api_key", Value: "sk-db-key-9876", ValueType: "secret"},
		{Key: "llm.base_url", Value: f.ts.URL, ValueType: "string"},
	}).Error; err != nil {
		t.Fatalf("种子配置失败: %v", err)
	}
	if err := db.Create(briefFixture("2026-08-21", `{"trade_date":"2026-08-21","market":{"up":1500,"down":400}}`)).Error; err != nil {
		t.Fatalf("种子简报失败: %v", err)
	}

	svc := NewLLMService(db, NewMorningBriefService(db))
	r, err := svc.InterpretBrief("")
	if err != nil {
		t.Fatalf("InterpretBrief 失败: %v", err)
	}
	if r.BriefDate != "2026-08-21" || !strings.Contains(r.Interpretation, "资金") {
		t.Fatalf("返回不符: %+v", r)
	}
	// 鉴权头来自 DB 已存 key（脱敏不回显）
	if auth := f.lastAuth(); auth != "Bearer sk-db-key-9876" {
		t.Fatalf("鉴权头应来自 DB 已存 key: %q", auth)
	}
	if cfg, _ := svc.GetConfig(); strings.Contains(cfg.APIKeyMasked, "sk-db-key-9876") {
		t.Fatal("配置接口不得回显明文 key")
	}
}
