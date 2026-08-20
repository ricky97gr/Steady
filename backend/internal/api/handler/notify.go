package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/service"
	"quant-system/backend/pkg/response"
)

// notifyConfigDTO 通知配置总览（设置页 GET 一次拉全）
type notifyConfigDTO struct {
	Events []service.NotifyEventDTO `json:"events"`
	Feishu service.FeishuConfigDTO  `json:"feishu"`
}

// taskRunDTO 任务执行记录单条
type taskRunDTO struct {
	ID        uint64      `json:"id"`
	TaskName  string      `json:"task_name"`
	RunDate   string      `json:"run_date"`
	Status    string      `json:"status"`
	Message   string      `json:"message"`
	Detail    interface{} `json:"detail"`
	CreatedAt string      `json:"created_at"`
}

// GetNotifyConfig 通知配置（GET /notify/config）：事件配置 + 飞书配置
func GetNotifyConfig(notifySvc *service.NotifyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		events, err := notifySvc.ListEvents()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询通知配置失败")
			return
		}
		feishu, err := notifySvc.GetFeishuConfig()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询飞书配置失败")
			return
		}
		response.OK(c, notifyConfigDTO{Events: events, Feishu: feishu})
	}
}

// UpdateNotifyEvent 更新单个通知事件（PUT /notify/config/:event）
func UpdateNotifyEvent(notifySvc *service.NotifyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventKey := c.Param("event")
		var d service.NotifyEventDTO
		if err := c.ShouldBindJSON(&d); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		d.EventKey = eventKey
		if err := notifySvc.UpdateEvent(eventKey, d); err != nil {
			if errors.Is(err, service.ErrNotifyEventMissing) {
				response.Fail(c, http.StatusNotFound, response.CodeResourceMissing, "通知事件不存在")
				return
			}
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, err.Error())
			return
		}
		response.OK(c, gin.H{"event_key": eventKey, "updated": true})
	}
}

// UpdateFeishuConfig 更新飞书配置（PUT /notify/config/feishu）
func UpdateFeishuConfig(notifySvc *service.NotifyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var d service.FeishuConfigDTO
		if err := c.ShouldBindJSON(&d); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		if d.Timeout < 1 || d.Timeout > 60 {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "timeout 应在 1~60 秒")
			return
		}
		if d.MaxRetries < 0 || d.MaxRetries > 5 {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "max_retries 应在 0~5")
			return
		}
		if err := notifySvc.UpdateFeishuConfig(d); err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "保存飞书配置失败")
			return
		}
		response.OK(c, gin.H{"updated": true})
	}
}

// SendTestCard 发送测试卡片（POST /notify/test）
func SendTestCard(notifySvc *service.NotifyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := notifySvc.SendTest(); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, err.Error())
			return
		}
		response.OK(c, gin.H{"sent": true})
	}
}

// GetTaskRuns 最近任务执行记录（GET /tasks/runs?limit=）
func GetTaskRuns(taskRunSvc *service.TaskRunService) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 20
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}
		items, err := taskRunSvc.ListRecent(limit)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]taskRunDTO, 0, len(items))
		for _, it := range items {
			var detail interface{}
			if len(it.Detail) > 0 {
				_ = json.Unmarshal(it.Detail, &detail) // JSONB → map/array
			}
			out = append(out, taskRunDTO{
				ID:        it.ID,
				TaskName:  it.TaskName,
				RunDate:   formatDate(it.RunDate),
				Status:    it.Status,
				Message:   it.Message,
				Detail:    detail,
				CreatedAt: formatDateTime(&it.CreatedAt),
			})
		}
		response.OK(c, gin.H{"items": out})
	}
}

// ManualExecuteDay 手动触发 ExecuteDay + SnapshotDay（POST /trading/execute-day）
func ManualExecuteDay(executeSvc *service.ExecuteService) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := executeSvc.ExecuteDayManual()
		if err != nil {
			if errors.Is(err, service.ErrNoMarketData) {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, err.Error())
				return
			}
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
			return
		}
		response.OK(c, gin.H{
			"trade_date": formatDate(res.TradeDate),
			"skipped":    res.Skipped,
			"buy_count":  res.BuyCount,
			"sell_count": res.SellCount,
			"manual":     res.Manual,
			"rejected":   res.Rejected,
			"nav":        res.Nav,
		})
	}
}
