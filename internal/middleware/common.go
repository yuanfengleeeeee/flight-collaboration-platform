package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"go.uber.org/zap"
)

// Logger 请求日志中间件
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		cost := time.Since(start)
		common.L().Info("请求",
			zap.String("request_id", c.GetString("request_id")),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("cost", cost),
			zap.String("ip", c.ClientIP()),
		)
	}
}

// Recovery panic 恢复中间件
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered interface{}) {
		common.L().Error("panic 恢复",
			zap.String("request_id", c.GetString("request_id")),
			zap.Any("error", recovered),
			zap.String("path", c.Request.URL.Path),
		)
		common.FailWithHTTP(c, 500, common.CodeInternalError, "服务器内部错误")
		c.Abort()
	})
}

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Requested-With")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
