package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	coremysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/adapter/mysql"
	coreapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application"
	coreflightsync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flightsync"
	coreflighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	integrationsync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformlogger "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/logger"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	platformobservability "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
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
	var flightSyncService *coreflightsync.Service
	var receiptTimeoutReassigner *coreflighttask.ReceiptTimeoutReassigner
	if db != nil {
		commandProcessor = coremysql.NewFlightTaskRepository(db)
		repository := coremysql.NewFlightTaskRepository(db)
		arrivalService := coreflighttask.NewService(repository, clock.Real{})
		confirmationService := coreflighttask.NewConfirmationService(repository, iam.NewAuthorizer(), clock.Real{})
		arrivalService.SetAutomaticDispatcher(coreflighttask.NewAutomaticDispatcher(confirmationService, nil))
		flightSyncService = coreflightsync.NewService(coremysql.NewFlightSourceRepository(db), arrivalService, clock.Real{})
		receiptTimeoutReassigner = coreflighttask.NewReceiptTimeoutReassigner(repository)
	}
	transport, err := integrationsync.NewHTTPTransportWithTLS(cfg.Sync.EdgeBaseURL, time.Duration(cfg.Sync.RequestTimeoutMS)*time.Millisecond, cfg.Sync.TLS)
	if err != nil {
		log.Error("create edge sync transport failed", zap.Error(err))
		return
	}
	metrics := platformobservability.NewWorkerRegistry()
	workerID := strings.TrimSpace(os.Getenv("FLIGHT_WORKER_ID"))
	if workerID == "" {
		hostname, hostnameErr := os.Hostname()
		if hostnameErr != nil || strings.TrimSpace(hostname) == "" {
			hostname = "worker"
		}
		workerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}
	syncWorker := integrationsync.NewWorkerWithCommandProcessor(coreStore, commandProcessor, nil, transport, log, integrationsync.RetryPolicy{MaxAttempts: cfg.Sync.MaxAttempts, BaseDelay: time.Second}, cfg.Sync.BatchSize).
		SetWorkerID(workerID).
		SetLeaseDuration(time.Duration(cfg.Sync.ClaimLeaseSeconds) * time.Second).
		SetMetrics(metrics)
	metricsAddr := strings.TrimSpace(os.Getenv("FLIGHT_WORKER_METRICS_ADDR"))
	if metricsAddr == "" {
		metricsAddr = ":9090"
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		syncWorker.RefreshMetrics(request.Context())
		if sqlDB, statsErr := platformmysql.SQLDB(db); statsErr == nil {
			platformobservability.RecordDBStats(metrics, "worker", sqlDB)
		}
		metrics.Handler().ServeHTTP(w, request)
	}))
	metricsServer := &http.Server{
		Addr:              metricsAddr,
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           metricsMux,
	}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("worker metrics server stopped", zap.Error(err), zap.String("addr", metricsAddr))
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Warn("shutdown worker metrics server failed", zap.Error(err))
		}
	}()
	ticker := time.NewTicker(time.Duration(cfg.Sync.PollIntervalMS) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if receiptTimeoutReassigner != nil {
				timeout := coreflighttask.DefaultAssignmentReceiptTimeout
				if cfg.Sync.AssignmentReceiptTimeoutSeconds > 0 {
					timeout = time.Duration(cfg.Sync.AssignmentReceiptTimeoutSeconds) * time.Second
				}
				if _, err := receiptTimeoutReassigner.Run(ctx, time.Now().UTC(), timeout, cfg.Sync.BatchSize, workerID); err != nil {
					log.Warn("reassign unreceived task assignments failed", zap.Error(err))
				}
			}
			if flightSyncService != nil {
				if _, err := flightSyncService.ApplyPending(ctx, cfg.Sync.BatchSize, workerID); err != nil {
					log.Warn("apply flight source inbox failed", zap.Error(err))
				}
			}
			if err := syncWorker.DeliverOutbox(ctx); err != nil {
				log.Warn("deliver core outbox failed", zap.Error(err))
			}
			if err := syncWorker.PullCommands(ctx, coreapp.HandleCommand); err != nil {
				log.Warn("pull edge commands failed", zap.Error(err))
			}
		}
	}
}
