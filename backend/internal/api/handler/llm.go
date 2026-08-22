package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/service"
	"quant-system/backend/pkg/response"
)

// 大模型能力统一入口。三能力在未启用（开关关 / 未配模型 / 未配 key）时返回 50301；
// 上游调用失败（断网 / 限流）同样返回 50301，业务码约定见 docs/技术准备文档.md §6.2。

type llmTermDTO struct {
	Term string `json:"term"`
}

type llmQuestionDTO struct {
	Question string `json:"question"`
}

type llmBriefDTO struct {
	BriefDate string `json:"brief_date"` // 空 = 最近一份
}

// GetLLMConfig 查询大模型配置（GET /config/llm）
func GetLLMConfig(svc *service.LLMService) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, err := svc.GetConfig()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询大模型配置失败")
			return
		}
		response.OK(c, cfg)
	}
}

// UpdateLLMConfig 保存大模型配置（PUT /config/llm）
func UpdateLLMConfig(svc *service.LLMService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var d service.LLMConfigDTO
		if err := c.ShouldBindJSON(&d); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		if err := svc.UpdateConfig(d); err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "保存大模型配置失败")
			return
		}
		response.OK(c, gin.H{"updated": true})
	}
}

// TestLLM 测试大模型连接（POST /config/llm/test）：用已存配置发最小请求
func TestLLM(svc *service.LLMService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.TestConnection(); err != nil {
			response.Fail(c, http.StatusServiceUnavailable, response.CodeServiceUnavail, err.Error())
			return
		}
		response.OK(c, gin.H{"ok": true})
	}
}

// ExplainTerm 术语解释（POST /llm/glossary）
func ExplainTerm(svc *service.LLMService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var d llmTermDTO
		if err := c.ShouldBindJSON(&d); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		if !llmEnabled(c, svc) {
			return
		}
		res, err := svc.ExplainTerm(d.Term)
		if err != nil {
			llmFail(c, err, "术语解释")
			return
		}
		response.OK(c, res)
	}
}

// AskProject 项目问答（POST /llm/ask）
func AskProject(svc *service.LLMService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var d llmQuestionDTO
		if err := c.ShouldBindJSON(&d); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		if !llmEnabled(c, svc) {
			return
		}
		res, err := svc.AskProject(d.Question)
		if err != nil {
			llmFail(c, err, "项目问答")
			return
		}
		response.OK(c, res)
	}
}

// InterpretBrief 简报解读（POST /llm/interpret-brief）
func InterpretBrief(svc *service.LLMService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var d llmBriefDTO
		if err := c.ShouldBindJSON(&d); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		if !llmEnabled(c, svc) {
			return
		}
		res, err := svc.InterpretBrief(d.BriefDate)
		if err != nil {
			llmFail(c, err, "简报解读")
			return
		}
		response.OK(c, res)
	}
}

// ---------- 辅助 ----------

// llmEnabled 未启用时统一 50301 并返回 false（已写响应，调用方直接 return）
func llmEnabled(c *gin.Context, svc *service.LLMService) bool {
	en, err := svc.Enabled()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询大模型配置失败")
		return false
	}
	if !en {
		response.Fail(c, http.StatusServiceUnavailable, response.CodeServiceUnavail,
			"大模型能力未启用，请先在设置页配置")
		return false
	}
	return true
}

// llmFail 按错误类型分派业务码：入参校验 → 40001；资源缺失 → 40004；上游 → 50301。
// 失败消息统一不泄露 api_key / 请求 body（服务层已保证）。
func llmFail(c *gin.Context, err error, op string) {
	switch {
	case errors.Is(err, service.ErrTermEmpty), errors.Is(err, service.ErrTermTooLong),
		errors.Is(err, service.ErrQuestionEmpty), errors.Is(err, service.ErrQuestionTooLong):
		response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, err.Error())
	case errors.Is(err, service.ErrNoBrief):
		response.Fail(c, http.StatusNotFound, response.CodeResourceMissing, "该日暂无早盘简报")
	case errors.Is(err, service.ErrKnowledgeBaseMissing):
		response.Fail(c, http.StatusServiceUnavailable, response.CodeServiceUnavail, "知识库未加载")
	default:
		response.Fail(c, http.StatusServiceUnavailable, response.CodeServiceUnavail, op+"失败："+err.Error())
	}
}
