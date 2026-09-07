package application

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/adminauth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/adminquery"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flightsync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	coreidentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/identity"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/management"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/managementrealtime"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/operations"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/taskchange"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformhealth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/health"
	platformhttpauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpauth"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	platformobservability "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	platformredis "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/redis"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Server struct {
	cfg         config.ServiceConfig
	db          *gorm.DB
	redis       *redis.Client
	log         *zap.Logger
	httpServer  *http.Server
	started     time.Time
	shutdown    sync.Once
	shutdownErr error
}

func NewServer(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger) *Server {
	return NewServerWithFlightTask(cfg, db, redisClient, log, nil)
}

func NewServerWithFlightTask(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service) *Server {
	return NewServerWithFlightTaskAndConfirmation(cfg, db, redisClient, log, arrivalService, nil)
}

func NewServerWithFlightTaskAndConfirmation(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellation(cfg, db, redisClient, log, arrivalService, confirmationService, nil)
}

func NewServerWithFlightTaskAndConfirmationAndCancellation(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuth(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, nil)
}

func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuth(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQuery(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, authenticator, nil)
}

func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQuery(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator, taskQueryService *flighttask.TaskQueryService) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentity(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService, nil, "")
}

func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentity(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator, taskQueryService *flighttask.TaskQueryService, identityService *coreidentity.Service, identitySharedKey string) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuth(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService, identityService, identitySharedKey, nil, nil)
}

// NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuth
// keeps the management SSO resolver at the Core boundary. Business handlers
// receive only the already-resolved Core principal; they never call an external
// identity provider or inspect SSO state directly.
func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuth(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator, taskQueryService *flighttask.TaskQueryService, identityService *coreidentity.Service, identitySharedKey string, adminAuthService *adminauth.Service, adminQueryService *adminquery.Service) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagement(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService, identityService, identitySharedKey, adminAuthService, adminQueryService, nil)
}

// NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagement
// adds the Core management workbench without changing the existing constructor
// used by foundation and slice tests.
func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagement(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator, taskQueryService *flighttask.TaskQueryService, identityService *coreidentity.Service, identitySharedKey string, adminAuthService *adminauth.Service, adminQueryService *adminquery.Service, managementService *management.Service) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSync(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService, identityService, identitySharedKey, adminAuthService, adminQueryService, managementService, nil)
}

func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSync(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator, taskQueryService *flighttask.TaskQueryService, identityService *coreidentity.Service, identitySharedKey string, adminAuthService *adminauth.Service, adminQueryService *adminquery.Service, managementService *management.Service, flightSyncService *flightsync.Service) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSyncAndOperations(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService, identityService, identitySharedKey, adminAuthService, adminQueryService, managementService, flightSyncService, nil)
}

func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSyncAndOperations(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator, taskQueryService *flighttask.TaskQueryService, identityService *coreidentity.Service, identitySharedKey string, adminAuthService *adminauth.Service, adminQueryService *adminquery.Service, managementService *management.Service, flightSyncService *flightsync.Service, operationsService *operations.Service) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSyncAndOperationsAndTaskChange(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService, identityService, identitySharedKey, adminAuthService, adminQueryService, managementService, flightSyncService, operationsService, nil)
}

func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSyncAndOperationsAndTaskChange(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator, taskQueryService *flighttask.TaskQueryService, identityService *coreidentity.Service, identitySharedKey string, adminAuthService *adminauth.Service, adminQueryService *adminquery.Service, managementService *management.Service, flightSyncService *flightsync.Service, operationsService *operations.Service, taskChangeService *taskchange.Service) *Server {
	return NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSyncAndOperationsAndTaskChangeAndRealtime(cfg, db, redisClient, log, arrivalService, confirmationService, cancellationService, authenticator, taskQueryService, identityService, identitySharedKey, adminAuthService, adminQueryService, managementService, flightSyncService, operationsService, taskChangeService, nil)
}

// NewServerWithFlightTask...AndRealtime adds the management hint stream while
// preserving the older constructor used by slice and foundation tests.
func NewServerWithFlightTaskAndConfirmationAndCancellationAndAuthAndTaskQueryAndIdentityAndAdminAuthAndManagementAndFlightSyncAndOperationsAndTaskChangeAndRealtime(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, arrivalService *flighttask.Service, confirmationService *flighttask.ConfirmationService, cancellationService *flighttask.CancellationService, authenticator platformsecurity.Authenticator, taskQueryService *flighttask.TaskQueryService, identityService *coreidentity.Service, identitySharedKey string, adminAuthService *adminauth.Service, adminQueryService *adminquery.Service, managementService *management.Service, flightSyncService *flightsync.Service, operationsService *operations.Service, taskChangeService *taskchange.Service, managementRealtimeService *managementrealtime.Service) *Server {
	if log == nil {
		log = zap.NewNop()
	}
	started := time.Now().UTC()
	endpoint := platformhealth.New(started,
		map[string]platformhealth.Checker{"mysql": func(ctx context.Context) error { return platformmysql.Ping(ctx, db) }},
		map[string]platformhealth.Checker{"redis": func(ctx context.Context) error { return platformredis.Ping(ctx, redisClient) }},
	)
	r := gin.New()
	metrics := platformobservability.NewHTTPRegistry()
	r.Use(gin.Recovery(), platformobservability.MiddlewareWithMetrics(log, "core-api", metrics))
	r.GET("/health/live", endpoint.Live())
	r.GET("/health/ready", endpoint.Ready())
	r.GET("/metrics", func(c *gin.Context) {
		if sqlDB, err := platformmysql.SQLDB(db); err == nil {
			platformobservability.RecordDBStats(metrics, "core", sqlDB)
		}
		metrics.Handler().ServeHTTP(c.Writer, c.Request)
	})
	coreidentity.RegisterInternalRoutes(r, identityService, identitySharedKey)
	adminauth.RegisterRoutes(r, adminAuthService, authenticator, cfg.AllowDevActorHeaders)
	r.GET("/api/v1/foundation", func(c *gin.Context) {
		businessMode := "foundation-only"
		if arrivalService != nil {
			businessMode = "bvs2-03-flight-task-candidate"
		}
		if confirmationService != nil {
			businessMode = "bvs2-04-leader-confirm"
		}
		if cancellationService != nil {
			businessMode = "bvs2-06-task-cancel"
		}
		c.JSON(http.StatusOK, gin.H{"component": "core-api", "architecture": "v2", "business_mode": businessMode})
	})
	flighttask.RegisterRoutes(r, arrivalService, flightSourceMiddleware(cfg))
	flightsync.RegisterRoutes(r, flightSyncService, flightSourceMiddleware(cfg))
	var authMiddleware gin.HandlerFunc
	if authenticator != nil || cfg.AllowDevActorHeaders {
		var resolver platformhttpauth.PrincipalResolver
		if adminAuthService != nil {
			resolver = adminAuthService
		}
		authMiddleware = platformhttpauth.RequireJWT(authenticator, resolver, cfg.AllowDevActorHeaders)
	}
	if authMiddleware == nil {
		flighttask.RegisterConfirmationRoutes(r, confirmationService)
		flighttask.RegisterCancellationRoutes(r, cancellationService)
	} else {
		flighttask.RegisterConfirmationRoutes(r, confirmationService, authMiddleware)
		flighttask.RegisterCancellationRoutes(r, cancellationService, authMiddleware)
	}
	if taskQueryService != nil {
		if authMiddleware == nil {
			flighttask.RegisterTaskQueryRoutes(r, taskQueryService)
		} else {
			flighttask.RegisterTaskQueryRoutes(r, taskQueryService, authMiddleware)
		}
	}
	if adminQueryService != nil {
		if authMiddleware == nil {
			adminquery.RegisterRoutes(r, adminQueryService)
		} else {
			adminquery.RegisterRoutes(r, adminQueryService, authMiddleware)
		}
	}
	if managementService != nil {
		if authMiddleware == nil {
			management.RegisterRoutes(r, managementService)
		} else {
			management.RegisterRoutes(r, managementService, authMiddleware)
		}
	}
	if operationsService != nil {
		if authMiddleware == nil {
			operations.RegisterRoutes(r, operationsService)
		} else {
			operations.RegisterRoutes(r, operationsService, authMiddleware)
		}
	}
	if taskChangeService != nil {
		if authMiddleware == nil {
			taskchange.RegisterRoutes(r, taskChangeService)
		} else {
			taskchange.RegisterRoutes(r, taskChangeService, authMiddleware)
		}
	}
	if managementRealtimeService != nil {
		if authMiddleware == nil {
			managementrealtime.RegisterRoutes(r, managementRealtimeService)
		} else {
			managementrealtime.RegisterRoutes(r, managementRealtimeService, authMiddleware)
		}
	}
	return &Server{
		cfg: cfg, db: db, redis: redisClient, log: log, started: started,
		httpServer: &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: r, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second},
	}
}

func flightSourceMiddleware(cfg config.ServiceConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(cfg.FlightSourceAPIKey)
		if key == "" && cfg.AllowDevActorHeaders {
			c.Next()
			return
		}
		provided := strings.TrimSpace(c.GetHeader("X-Flight-Source-Key"))
		if key == "" || len(provided) != len(key) || subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "flight_source_unauthorized", "message": "flight source authentication is required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) Handler() http.Handler { return s.httpServer.Handler }

func (s *Server) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("core server context is nil")
	}
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen core api on %s: %w", s.httpServer.Addr, err)
	}
	if tlsConfig, err := platformsecurity.NewServerTLSConfig(s.cfg.TLS); err != nil {
		_ = listener.Close()
		return fmt.Errorf("configure core api TLS: %w", err)
	} else if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- s.httpServer.Serve(listener) }()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.Join(err, s.Shutdown(context.Background()))
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.shutdown.Do(func() {
		var errs []error
		if err := s.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("shutdown core http: %w", err))
		}
		if err := platformmysql.Close(s.db); err != nil {
			errs = append(errs, err)
		}
		if s.redis != nil {
			if err := s.redis.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close core redis: %w", err))
			}
		}
		s.shutdownErr = errors.Join(errs...)
	})
	return s.shutdownErr
}
