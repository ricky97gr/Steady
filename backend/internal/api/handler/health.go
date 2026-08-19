package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HealthCheck 健康检查：校验服务与数据库连通性
func HealthCheck(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		}

		sqlDB, err := db.DB()
		if err != nil || sqlDB.Ping() != nil {
			status["db"] = "down"
			c.JSON(http.StatusServiceUnavailable, status)
			return
		}
		status["db"] = "ok"

		c.JSON(http.StatusOK, status)
	}
}
