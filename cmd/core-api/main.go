package main

import (
	"context"
	"flag"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	coremysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/adapter/mysql"
	coreapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application"
	coreadminauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/adminauth"
	coreadminquery "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/adminquery"
	coreflightsync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flightsync"
	coreflighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	coreidentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/identity"
	coremanagement "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/management"
	coremanagementrealtime "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/managementrealtime"
	coreoperations "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/operations"
	coretaskchange "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/taskchange"
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
	var flightSyncService *coreflightsync.Service
	var taskQueryService *coreflighttask.TaskQueryService
	var identityService *coreidentity.Service
	if db != nil {
		repository := coremysql.NewFlightTaskRepository(db)
		arrivalService = coreflighttask.NewService(repository, clock.Real{})
		confirmationService = coreflighttask.NewConfirmationService(repository, iam.NewAuthorizer(), clock.Real{})
		arrivalService.SetAutomaticDispatcher(coreflighttask.NewAutomaticDispatcher(confirmationService, nil))
		flightSyncService = coreflightsync.NewService(coremysql.NewFlightSourceRepository(db), arrivalService, clock.Real{})
		cancellationService = coreflighttask.NewCancellationService(repository, iam.NewAuthorizer(), clock.Real{})
		taskQueryService = coreflighttask.NewTaskQueryService(repository, iam.NewAuthorizer())
		providerVerifier := coreidentity.ProviderVerifier(coreidentity.DisabledProviderVerifier{})
		if cfg.Identity.RealProvidersEnabled {
			remoteVerifier, verifierErr := coreidentity.NewRemoteProviderVerifier(coreidentity.RemoteProviderConfig{
				PersonalWeChatAppID:         cfg.Identity.PersonalWeChatAppID,
				PersonalWeChatSecret:        cfg.Identity.PersonalWeChatSecret,
				WeComCorpID:                 cfg.Identity.WeComCorpID,
				WeComAgentID:                cfg.Identity.WeComAgentID,
				WeComSecret:                 cfg.Identity.WeComSecret,
				WeComMiniappCode2SessionURL: cfg.Identity.WeComMiniappCode2SessionURL,
			})
			if verifierErr != nil {
				log.Error("configure real identity providers failed", zap.Error(verifierErr))
			} else {
				providerVerifier = remoteVerifier
			}
		} else if cfg.Core.Mode != "release" {
			providerVerifier = coreidentity.DevelopmentProviderVerifier{}
		}
		identityService = coreidentity.NewService(coremysql.NewIdentityRepository(db), providerVerifier, clock.Real{}, coreidentity.ServiceConfig{MaxLoginAttempts: cfg.Identity.MaxLoginAttempts, LockoutDuration: cfg.Identity.LockoutDuration(), BindingTicketTTL: cfg.Identity.BindingTicketTTL()})
	}
	coreJWTConfig := cfg.JWT
	coreJWTConfig.Audience += "-core"
	authenticator, err := platformsecurity.NewJWTAuthenticator(coreJWTConfig)
	if err != nil {
		log.Error("configure core jwt failed", zap.Error(err))
		return
	}
	var adminAuthService *coreadminauth.Service
	var adminQueryService *coreadminquery.Service
	var managementService *coremanagement.Service
	var operationsService *coreoperations.Service
	var taskChangeService *coretaskchange.Service
	var managementRealtimeService *coremanagementrealtime.Service
	if db != nil {
		adminQueryService = coreadminquery.NewService(coremysql.NewAdminQueryRepository(db), iam.NewAuthorizer())
		managementService = coremanagement.NewService(coremysql.NewManagementRepository(db), iam.NewAuthorizer(), clock.Real{})
		operationsService = coreoperations.NewService(coremysql.NewOperationsRepository(db), iam.NewAuthorizer(), coreoperations.RuntimeInfo{Environment: cfg.Core.Mode, RedisEnabled: rdb != nil, FlightSourceConfigured: strings.TrimSpace(cfg.Core.FlightSourceAPIKey) != "", RealIdentityProvider: cfg.Identity.RealProvidersEnabled, DevelopmentActorHeaders: cfg.Core.AllowDevActorHeaders})
		taskChangeService = coretaskchange.NewService(coremysql.NewTaskChangeRepository(db), iam.NewAuthorizer(), clock.Real{})
		managementRealtimeService = coremanagementrealtime.NewService(coremysql.NewManagementRealtimeRepository(db), iam.NewAuthorizer())
	}
	if db != nil && cfg.Identity.AdminSSOEnabled {
		var adminProvider coreadminauth.Provider = coreadminauth.DisabledProvider{}
		switch strings.ToLower(strings.TrimSpace(cfg.Identity.AdminSSOProvider)) {
		case coreadminauth.ProviderOIDC:
			adminProvider = &coreadminauth.OIDCProvider{
				AuthorizeURL: cfg.Identity.AdminSSOAuthorizeURL,
				TokenURL:     cfg.Identity.AdminSSOTokenURL,
				UserInfoURL:  cfg.Identity.AdminSSOUserInfoURL,
				ClientID:     cfg.Identity.AdminSSOClientID,
				ClientSecret: cfg.Identity.AdminSSOClientSecret,
				HTTPClient:   &http.Client{Timeout: 5 * time.Second},
			}
		case coreadminauth.ProviderWeCom:
			adminProvider = &coreadminauth.WeComProvider{
				CorpID:     cfg.Identity.WeComCorpID,
				AgentID:    cfg.Identity.WeComAgentID,
				Secret:     cfg.Identity.WeComSecret,
				HTTPClient: &http.Client{Timeout: 5 * time.Second},
			}
		case coreadminauth.ProviderDevelopment:
			// Development SSO is restricted by config validation to non-release
			// environments and still requires a provisioned Core identity.
			adminProvider = coreadminauth.DevelopmentProvider{CallbackSubject: cfg.Identity.AdminSSODevSubject}
		}
		adminAuthService = coreadminauth.NewService(coremysql.NewAdminAuthRepository(db), adminProvider, authenticator, clock.Real{}, coreadminauth.Config{
			AccessTokenTTL:     cfg.Identity.AccessTokenTTL(),
			SessionTTL:         cfg.Identity.SessionTTL(),
			AbsoluteSessionTTL: cfg.Identity.SessionAbsoluteTTL(),
			StateTTL:           cfg.Identity.AdminSSOStateTTL(),
			Provider:           cfg.Identity.AdminSSOProvider,
			SecureCookie:       cfg.Core.Mode == "release",
			AllowedRedirectURI: splitConfigList(cfg.Identity.AdminSSOAllowedRedirect),
		})
	}
	server := coreapp.NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSyncAndOperationsAndTaskChangeAndRealtime(cfg.Core, db, rdb, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService, identityService, cfg.Identity.InternalAPIKey, adminAuthService, adminQueryService, managementService, flightSyncService, operationsService, taskChangeService, managementRealtimeService)
	if err := server.Run(ctx); err != nil {
		log.Error("core api stopped with error", zap.Error(err))
	}
}

func splitConfigList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
