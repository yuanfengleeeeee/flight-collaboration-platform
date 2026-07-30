package server

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/middleware"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/module/device"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/module/prediction"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var startTime = time.Now()

// Health 健康检查
func Health(db *gorm.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := "ok"
		details := gin.H{}

		// 检查 MySQL
		if db == nil {
			status = "degraded"
			details["mysql"] = "disabled"
		} else {
			sqlDB, err := db.DB()
			if err != nil {
				status = "degraded"
				details["mysql"] = "error: " + err.Error()
			} else if err := sqlDB.Ping(); err != nil {
				status = "degraded"
				details["mysql"] = "ping failed: " + err.Error()
			} else {
				details["mysql"] = "ok"
			}
		}

		// 检查 Redis(可选,失败不致命)
		if rdb != nil {
			if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
				details["redis"] = "ping failed: " + err.Error()
			} else {
				details["redis"] = "ok"
			}
		} else {
			details["redis"] = "disabled"
		}

		details["uptime"] = time.Since(startTime).String()

		common.OK(c, gin.H{
			"status":  status,
			"details": details,
		})
	}
}

// SetupRouter 装配路由
func SetupRouter(cfg *config.Config, db *gorm.DB, rdb *redis.Client) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	// 健康检查(无需鉴权)
	r.GET("/health", Health(db, rdb))

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 设备状态上报(预留,无需业务鉴权,后期加设备鉴权)
		v1.POST("/device/status", device.ReportStatus)
	}

	// AI 预测接口(预留)
	r.POST("/api/prediction/analyze", prediction.Analyze)

	// TODO: 业务模块路由(flight/task/event/personnel/rule/analytics/push/auth)
	// 后续按模块实现,通过 JWTAuth 中间件保护

	common.L().Info("路由装配完成",
		zap.Int("port", cfg.Server.Port),
		zap.String("mode", cfg.Server.Mode),
	)

	return r
}
