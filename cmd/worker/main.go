package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"
	"time"

	coremysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/adapter/mysql"
	coreapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	integrationsync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformlogger "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/logger"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	"go.uber.org/zap"
)

func main() {
	// Worker 位于 Core 一侧：主动把 Core Outbox 推给 Edge，并主动从 Edge
	// 拉取移动端 Command。它不连接 Edge DB，Core/Edge 的边界由 Transport 保持。
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

	db, err := platformmysql.Open(cfg.Core.DB)
	if err != nil {
		log.Warn("worker core mysql unavailable; retrying in worker loop", zap.Error(err))
		db = nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var coreStore coresync.Store
	if db == nil {
		coreStore = coresync.NewMemoryStore()
	} else {
		coreStore = coresync.NewSQLStore(db)
	}
	var commandProcessor integrationsync.CommandProcessor = coreStore
	if db != nil {
		commandProcessor = coremysql.NewFlightTaskRepository(db)
	}
	transport, err := integrationsync.NewHTTPTransportWithTLS(cfg.Sync.EdgeBaseURL, time.Duration(cfg.Sync.RequestTimeoutMS)*time.Millisecond, cfg.Sync.TLS)
	if err != nil {
		log.Error("create edge sync transport failed", zap.Error(err))
		return
	}
	syncWorker := integrationsync.NewWorkerWithCommandProcessor(coreStore, commandProcessor, nil, transport, log, integrationsync.RetryPolicy{MaxAttempts: cfg.Sync.MaxAttempts, BaseDelay: time.Second}, cfg.Sync.BatchSize)
	ticker := time.NewTicker(time.Duration(cfg.Sync.PollIntervalMS) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := syncWorker.DeliverOutbox(ctx); err != nil {
				log.Warn("deliver core outbox failed", zap.Error(err))
			}
			if err := syncWorker.PullCommands(ctx, coreapp.HandleCommand); err != nil {
				log.Warn("pull edge commands failed", zap.Error(err))
			}
		}
	}
}
