package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/model"
	"quant-system/backend/internal/service"
	"quant-system/backend/pkg/response"
)

// GetMorningBrief 早盘简报（GET /morning-brief?date=YYYY-MM-DD，缺省最近一份）。
// 无数据返回 40004（前端 Empty 兜底）。
func GetMorningBrief(briefSvc *service.MorningBriefService) gin.HandlerFunc {
	return func(c *gin.Context) {
		dateStr := c.Query("date")
		var (
			row *model.MorningBrief
			err error
		)
		if dateStr == "" {
			row, err = briefSvc.Latest()
		} else {
			d, perr := time.ParseInLocation("2006-01-02", dateStr, time.Local)
			if perr != nil {
				response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "date 格式应为 YYYY-MM-DD")
				return
			}
			row, err = briefSvc.GetByDate(d)
		}
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询早盘简报失败")
			return
		}
		if row == nil {
			response.Fail(c, http.StatusNotFound, response.CodeResourceMissing, "该日暂无早盘简报")
			return
		}
		var sections interface{}
		_ = json.Unmarshal(row.Sections, &sections) // JSONB → map
		response.OK(c, gin.H{
			"brief_date": formatDate(row.BriefDate),
			"sections":   sections,
		})
	}
}
