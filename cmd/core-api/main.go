package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"

	coremysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/adapter/mysql"
	coreapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application"
	coreflighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformlogger "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/logger"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	platformredis "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/redis"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"go.uber.org/zap"
)

func main() {
	// Core API 只组装 Core 自己的依赖；业务规则由 internal/core/application
	// 和后续业务模块承载，入口不直接处理业务数据。
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
		log.Warn("core mysql unavailable; keeping liveness", zap.Error(err))
		db = nil
	}
	rdb, err := platformredis.Open(cfg.Core.Redis)
	if err != nil {
		log.Warn("core redis unavailable; continuing without redis", zap.Error(err))
		rdb = nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var arrivalService *coreflighttask.Service
	var confirmationService *coreflighttask.ConfirmationService
	var cancellationService *coreflighttask.CancellationService
	var taskQueryService *coreflighttask.TaskQueryService
	if db != nil {
		repository := coremysql.NewFlightTaskRepository(db)
		arrivalService = coreflighttask.NewService(repository, clock.Real{})
		confirmationService = coreflighttask.NewConfirmationService(repository, iam.NewAuthorizer(), clock.Real{})
		cancellationService = coreflighttask.NewCancellationService(repository, iam.NewAuthorizer(), clock.Real{})
		taskQueryService = coreflighttask.NewTaskQueryService(repository, iam.NewAuthorizer())
	}
	authenticator, err := platformsecurity.NewJWTAuthenticator(cfg.JWT)
	if err != nil {
		log.Error("configure core jwt failed", zap.Error(err))
		return
	}
	server := coreapp.NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQuery(cfg.Core, db, rdb, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService)
	if err := server.Run(ctx); err != nil {
		log.Error("core api stopped with error", zap.Error(err))
	}
}
