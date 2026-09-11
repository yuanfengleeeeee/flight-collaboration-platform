package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	edgemysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/adapter/mysql"
	edgeapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application"
	edgeidentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/identity"
	edgenotification "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/notification"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	sharedIdentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/identity"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
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
	var sessionStore *edgemysql.SessionRepository
	if db != nil {
		projectionStore = edgesync.NewSQLStore(db)
		sessionStore = edgemysql.NewSessionRepository(db)
	}
	edgeJWTConfig := cfg.JWT
	edgeJWTConfig.Audience += "-edge"
	authenticator, err := platformsecurity.NewJWTAuthenticator(edgeJWTConfig)
	if err != nil {
		log.Error("configure edge jwt failed", zap.Error(err))
		return
	}
	var identityService *edgeidentity.Service
	if sessionStore != nil && cfg.Identity.CoreBaseURL != "" {
		coreIdentityClient, clientErr := sharedIdentity.NewHTTPClient(cfg.Identity.CoreBaseURL, cfg.Identity.InternalAPIKey, &http.Client{Timeout: 5 * time.Second})
		if clientErr != nil {
			log.Error("configure core identity client failed", zap.Error(clientErr))
			return
		}
		identityService = edgeidentity.NewService(coreIdentityClient, sessionStore, authenticator, clock.Real{}, edgeidentity.Config{AccessTokenTTL: cfg.Identity.AccessTokenTTL(), SessionTTL: cfg.Identity.SessionTTL(), AbsoluteSessionTTL: cfg.Identity.SessionAbsoluteTTL()})
	}
	var notifier edgenotification.Publisher = edgenotification.NewInMemoryFanout()
	if rdb != nil {
		instanceID := strings.TrimSpace(os.Getenv("FLIGHT_EDGE_INSTANCE_ID"))
		if instanceID == "" {
			hostname, hostnameErr := os.Hostname()
			if hostnameErr != nil || strings.TrimSpace(hostname) == "" {
				hostname = "edge"
			}
			instanceID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
		}
		redisFanout, fanoutErr := edgenotification.NewRedisFanout(nil, rdb, "", instanceID)
		if fanoutErr != nil {
			log.Warn("configure edge redis fan-out failed; continuing locally", zap.Error(fanoutErr))
		} else {
			notifier = redisFanout
		}
	}
	server := edgeapp.NewServerWithStoreAndAuthAndIdentityAndNotifications(cfg.Edge, db, rdb, log, projectionStore, authenticator, identityService, notifier)
	if err := server.Run(ctx); err != nil {
		log.Error("edge api stopped with error", zap.Error(err))
	}
}
