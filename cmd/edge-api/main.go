package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"

	edgeapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformlogger "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/logger"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	platformredis "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/redis"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"go.uber.org/zap"
)

func main() {
	// Edge API 只连接 Edge DB，保存移动端 Projection、Command 和同步 Inbox；
	// 它不能通过数据库直接访问 Core 事实。
	configPath := flag.String("config", "configs/config.v2.yaml", "Architecture v2 config path")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}
	log, err := platformlogger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	db, err := platformmysql.Open(cfg.Edge.DB)
	if err != nil {
		log.Warn("edge mysql unavailable; keeping liveness", zap.Error(err))
		db = nil
	}
	rdb, err := platformredis.Open(cfg.Edge.Redis)
	if err != nil {
		log.Warn("edge redis unavailable; continuing without redis", zap.Error(err))
		rdb = nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var projectionStore edgesync.Store
	if db != nil {
		projectionStore = edgesync.NewSQLStore(db)
	}
	authenticator, err := platformsecurity.NewJWTAuthenticator(cfg.JWT)
	if err != nil {
		log.Error("configure edge jwt failed", zap.Error(err))
		return
	}
	server := edgeapp.NewServerWithStoreAndAuth(cfg.Edge, db, rdb, log, projectionStore, authenticator)
	if err := server.Run(ctx); err != nil {
		log.Error("edge api stopped with error", zap.Error(err))
	}
}
