package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/middleware"
	authmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/module/auth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/module/device"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/module/prediction"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var startTime = time.Now()

// Liveness 表示进程仍然可以接受 HTTP 请求，不检查外部依赖。
func Liveness() gin.HandlerFunc {
	return func(c *gin.Context) {
		common.OK(c, gin.H{
			"status": "ok",
			"uptime": time.Since(startTime).Seconds(),
		})
	}
}

// Readiness 检查服务是否具备处理依赖数据库的请求条件。
// MySQL 是必需依赖；Redis 是可选依赖，异常只反映在 details 中。
func Readiness(db *gorm.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := "ready"
		httpCode := http.StatusOK
		details := gin.H{"mysql": "ok", "redis": "disabled"}

		if db == nil {
			status = "not_ready"
			httpCode = http.StatusServiceUnavailable
			details["mysql"] = "unavailable"
		} else {
			sqlDB, err := db.DB()
			if err != nil {
				status = "not_ready"
				httpCode = http.StatusServiceUnavailable
				details["mysql"] = "unavailable"
			} else {
				pingCtx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
				err = sqlDB.PingContext(pingCtx)
				cancel()
				if err != nil {
					status = "not_ready"
					httpCode = http.StatusServiceUnavailable
					details["mysql"] = "unavailable"
				}
			}
		}

		if rdb != nil {
			pingCtx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
			if err := rdb.Ping(pingCtx).Err(); err != nil {
				details["redis"] = "unavailable"
			} else {
				details["redis"] = "ok"
			}
			cancel()
		}

		data := gin.H{
			"status":  status,
			"details": details,
			"uptime":  time.Since(startTime).Seconds(),
		}
		common.Respond(c, httpCode, common.CodeSuccess, status, data)
	}
}

// Health 保留旧路由语义，作为 readiness 的兼容别名。
func Health(db *gorm.DB, rdb *redis.Client) gin.HandlerFunc {
	return Readiness(db, rdb)
}

// SetupRouter 装配路由
func SetupRouter(cfg *config.Config, db *gorm.DB, rdb *redis.Client) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	// 健康检查(无需鉴权)
	r.GET("/health/live", Liveness())
	r.GET("/health/ready", Readiness(db, rdb))
	r.GET("/health", Health(db, rdb))

	// API v1
	v1 := r.Group("/api/v1")
	{
		authHandler := authmodule.NewHandler(db, cfg.JWT)
		v1.POST("/auth/login", authHandler.Login)
		authenticated := v1.Group("/auth")
		authenticated.Use(middleware.JWTAuth(cfg.JWT))
		authenticated.GET("/me", authHandler.Me)

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
