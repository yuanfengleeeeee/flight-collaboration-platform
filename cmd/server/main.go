package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
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

	// 连接 MySQL；连接失败时保留 liveness，readiness 会明确报告未就绪。
	var db = (*gorm.DB)(nil)
	db, err = store.NewMySQL(cfg.MySQL)
	if err != nil {
		common.L().Warn("连接 MySQL 失败,降级运行(业务接口将不可用)", zap.Error(err))
		db = nil
	} else {
		common.L().Info("MySQL 连接成功",
			zap.String("host", cfg.MySQL.Host),
			zap.String("database", cfg.MySQL.Database),
		)
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

	// 装配应用并在收到退出信号后统一关闭资源。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := server.NewApplication(cfg, db, rdb)
	if err := app.Run(ctx); err != nil {
		common.L().Error("服务运行失败", zap.Error(err))
	}
	common.L().Info("服务已退出")
}
