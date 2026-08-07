package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Application 负责装配 HTTP 服务及其基础依赖的生命周期。
// 业务模块只通过路由和服务接口接入，不在这里编排业务流程。
type Application struct {
	cfg    *config.Config
	db     *gorm.DB
	redis  *redis.Client
	server *http.Server

	shutdownOnce sync.Once
	shutdownErr  error
}

// NewApplication 创建应用实例。
func NewApplication(cfg *config.Config, db *gorm.DB, redisClient *redis.Client) *Application {
	return &Application{
		cfg:   cfg,
		db:    db,
		redis: redisClient,
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
			Handler:      SetupRouter(cfg, db, redisClient),
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		},
	}
}

// Run 启动 HTTP 服务，并在 ctx 结束时执行优雅关闭。
// 数据库不可用时仍允许进程启动，以便 liveness 可用；readiness 会返回未就绪。
func (a *Application) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("应用运行上下文不能为空")
	}

	listener, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("监听 HTTP 地址 %s 失败: %w", a.server.Addr, err)
	}

	common.L().Info("HTTP 服务启动", zap.String("addr", listener.Addr().String()))
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- a.server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		shutdownErr := a.Shutdown(context.Background())
		return errors.Join(err, shutdownErr)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.Shutdown(shutdownCtx)
	}
}

// Shutdown 按 HTTP、MySQL、Redis 的顺序关闭应用资源；可安全重复调用。
func (a *Application) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	a.shutdownOnce.Do(func() {
		var errs []error
		if err := a.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("关闭 HTTP 服务失败: %w", err))
		}
		if a.db != nil {
			if sqlDB, err := a.db.DB(); err != nil {
				errs = append(errs, fmt.Errorf("获取 MySQL 连接失败: %w", err))
			} else if err := sqlDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("关闭 MySQL 连接失败: %w", err))
			}
		}
		if a.redis != nil {
			if err := a.redis.Close(); err != nil {
				errs = append(errs, fmt.Errorf("关闭 Redis 连接失败: %w", err))
			}
		}
		a.shutdownErr = errors.Join(errs...)
	})

	return a.shutdownErr
}
