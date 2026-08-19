package response

import (
	"time"

	"github.com/gin-gonic/gin"
)

// 业务错误码约定（见 docs/技术准备文档.md §6.2）
const (
	CodeOK              = 0      // 成功
	CodeInvalidParam    = 40001  // 参数错误
	CodeResourceMissing = 40004  // 资源不存在
	CodeInternalError   = 50001  // 服务器内部错误
	CodeServiceUnavail  = 50301  // 服务暂不可用
)

// Body 统一响应结构
type Body struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data"`
	Timestamp string      `json:"timestamp"`
}

// OK 成功响应
func OK(c *gin.Context, data interface{}) {
	c.JSON(200, Body{
		Code:      CodeOK,
		Message:   "success",
		Data:      data,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

// Fail 失败响应
func Fail(c *gin.Context, httpStatus, code int, msg string) {
	c.JSON(httpStatus, Body{
		Code:      code,
		Message:   msg,
		Data:      nil,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}
