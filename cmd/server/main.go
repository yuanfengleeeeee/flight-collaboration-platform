package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/model"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/server"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func main() {
	// 解析参数
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic("加载配置失败: " + err.Error())
	}

	// 初始化日志
	common.InitLogger(cfg.Log.Level, cfg.Log.Encoding, cfg.Log.Output)
	common.L().Info("航空保障智能协同平台启动中",
		zap.String("config", *configPath),
	)

	// 连接 MySQL(原型阶段:连接失败不退出,降级运行便于无库调试)
	var db *gorm.DB
	db, err = store.NewMySQL(cfg.MySQL)
	if err != nil {
		common.L().Warn("连接 MySQL 失败,降级运行(业务接口将不可用)", zap.Error(err))
		db = nil
	} else {
		common.L().Info("MySQL 连接成功",
			zap.String("host", cfg.MySQL.Host),
			zap.String("database", cfg.MySQL.Database),
		)
		// 自动迁移(原型阶段)
		if err := db.AutoMigrate(
			&model.Flight{},
			&model.User{},
			&model.UserPosition{},
			&model.Position{},
			&model.TaskTemplate{},
			&model.TaskInstance{},
			&model.TaskAssignment{},
			&model.Event{},
			&model.PersonnelStatus{},
			&model.Rule{},
			&model.FlightOperationStatistics{},
			&model.EventStatistics{},
			&model.ResourceStatistics{},
			&model.PredictionInterface{},
		); err != nil {
			common.L().Warn("自动迁移失败", zap.Error(err))
		} else {
			common.L().Info("数据库自动迁移完成")
		}
	}

	// 连接 Redis(可选)
	var rdb *redis.Client
	rdb, err = store.NewRedis(cfg.Redis)
	if err != nil {
		common.L().Warn("连接 Redis 失败,降级运行", zap.Error(err))
		rdb = nil
	} else {
		common.L().Info("Redis 连接成功", zap.String("addr", cfg.Redis.Addr()))
	}

	// 装配路由
	r := server.SetupRouter(cfg, db, rdb)

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		common.L().Info("HTTP 服务启动", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			common.L().Fatal("HTTP 服务启动失败", zap.Error(err))
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	common.L().Info("正在关闭服务...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		common.L().Error("服务关闭异常", zap.Error(err))
	}

	// 关闭数据库连接
	if db != nil {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	if rdb != nil {
		_ = rdb.Close()
	}

	common.L().Info("服务已退出")
}
