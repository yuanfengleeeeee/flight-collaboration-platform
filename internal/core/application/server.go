package application

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
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
	if log == nil {
		log = zap.NewNop()
	}
	started := time.Now().UTC()
	endpoint := platformhealth.New(started,
		map[string]platformhealth.Checker{"mysql": func(ctx context.Context) error { return platformmysql.Ping(ctx, db) }},
		map[string]platformhealth.Checker{"redis": func(ctx context.Context) error { return platformredis.Ping(ctx, redisClient) }},
	)
	r := gin.New()
	r.Use(gin.Recovery(), platformobservability.Middleware(log, "core-api"))
	r.GET("/health/live", endpoint.Live())
	r.GET("/health/ready", endpoint.Ready())
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
	flighttask.RegisterRoutes(r, arrivalService)
	var authMiddleware gin.HandlerFunc
	if authenticator != nil || cfg.AllowDevActorHeaders {
		authMiddleware = platformhttpauth.RequireJWT(authenticator, nil, cfg.AllowDevActorHeaders)
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
	return &Server{
		cfg: cfg, db: db, redis: redisClient, log: log, started: started,
		httpServer: &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: r, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second},
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
