package application

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformhealth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/health"
	platformhttpauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpauth"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	platformobservability "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	platformredis "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/redis"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Server struct {
	cfg         config.ServiceConfig
	db          *gorm.DB
	redis       *redis.Client
	store       edgesync.Store
	log         *zap.Logger
	httpServer  *http.Server
	shutdown    sync.Once
	shutdownErr error
}

func NewServer(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger) *Server {
	return NewServerWithStore(cfg, db, redisClient, log, nil)
}

func NewServerWithStore(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, store edgesync.Store) *Server {
	return NewServerWithStoreAndAuth(cfg, db, redisClient, log, store, nil)
}

func NewServerWithStoreAndAuth(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, store edgesync.Store, authenticator platformsecurity.Authenticator) *Server {
	// Foundation 阶段的路由直接围绕 Store 装配：移动端只读 Projection、写
	// Command；内部同步接口由 Core Worker 使用。真实业务阶段应再拆出 Handler
	// 和 Application Service，避免把业务规则继续堆在路由闭包中。
	if log == nil {
		log = zap.NewNop()
	}
	endpoint := platformhealth.New(time.Now().UTC(),
		map[string]platformhealth.Checker{"mysql": func(ctx context.Context) error { return platformmysql.Ping(ctx, db) }},
		map[string]platformhealth.Checker{"redis": func(ctx context.Context) error { return platformredis.Ping(ctx, redisClient) }},
	)
	r := gin.New()
	r.Use(gin.Recovery(), platformobservability.Middleware(log, "edge-api"))
	r.GET("/health/live", endpoint.Live())
	r.GET("/health/ready", endpoint.Ready())
	r.GET("/api/v1/foundation", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"component": "edge-api", "architecture": "v2", "data_mode": "projection-only"})
	})
	var employeeAuth gin.HandlerFunc
	if authenticator != nil || cfg.AllowDevActorHeaders {
		employeeAuth = platformhttpauth.RequireJWT(authenticator, nil, cfg.AllowDevActorHeaders)
	}
	registerEmployeeGET := func(path string, handler gin.HandlerFunc) {
		if employeeAuth == nil {
			r.GET(path, handler)
			return
		}
		r.GET(path, employeeAuth, handler)
	}
	registerEmployeePOST := func(path string, handler gin.HandlerFunc) {
		if employeeAuth == nil {
			r.POST(path, handler)
			return
		}
		r.POST(path, employeeAuth, handler)
	}
	registerEmployeeGET("/api/v1/tasks", func(c *gin.Context) {
		if store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "projection_unavailable", "message": "projection store unavailable"})
			return
		}
		actorPublicID, err := employeeActorPublicID(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthenticated", "message": "employee authentication is required"})
			return
		}
		values, err := store.ListTaskProjections(c.Request.Context(), actorPublicID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "projection_read_failed", "message": "projection read failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": values, "source": "edge_projection", "request_id": platformobservability.RequestID(c), "trace_id": platformobservability.TraceID(c)})
	})
	registerEmployeePOST("/api/v1/commands", func(c *gin.Context) {
		if store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "command_store_unavailable", "message": "command store unavailable"})
			return
		}
		var command sharedEvent.CommandEnvelope
		if err := c.ShouldBindJSON(&command); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_command", "message": "invalid command envelope"})
			return
		}
		actorPublicID, actorErr := employeeActorPublicID(c)
		if actorErr != nil || command.ActorPublicID != actorPublicID {
			c.JSON(http.StatusForbidden, gin.H{"code": "forbidden", "message": "command actor does not match authenticated employee"})
			return
		}
		duplicate, err := store.PutCommand(c.Request.Context(), command)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_command", "message": "command was rejected"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"command_id": command.CommandID, "status": "pending", "duplicate": duplicate, "request_id": platformobservability.RequestID(c), "trace_id": platformobservability.TraceID(c)})
	})
	registerEmployeeCommand := func(commandType string) gin.HandlerFunc {
		return func(c *gin.Context) {
			if store == nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"code": "command_store_unavailable", "message": "command store unavailable"})
				return
			}
			actorPublicID, actorErr := employeeActorPublicID(c)
			if actorErr != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthenticated", "message": "employee authentication is required"})
				return
			}
			var request employeeTaskCommandRequest
			if err := c.ShouldBindJSON(&request); err != nil || request.ExpectedSyncVersion == nil || request.AssignmentPublicID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_command", "message": "assignment_public_id and expected_sync_version are required"})
				return
			}
			clientOccurredAt := time.Now().UTC()
			if request.ClientOccurredAt != nil && !request.ClientOccurredAt.IsZero() {
				clientOccurredAt = request.ClientOccurredAt.UTC()
			}
			payload := employeeTaskCommandPayload{AssignmentPublicID: request.AssignmentPublicID, ExpectedSyncVersion: *request.ExpectedSyncVersion, ClientOccurredAt: clientOccurredAt, Note: request.Note}
			command, err := sharedEvent.NewCommand(commandType, actorPublicID, c.Param("taskPublicID"), platformobservability.TraceID(c), payload)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "command_create_failed", "message": "command could not be created"})
				return
			}
			duplicate, err := store.PutCommand(c.Request.Context(), command)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_command", "message": "command was rejected"})
				return
			}
			c.JSON(http.StatusAccepted, gin.H{"command_id": command.CommandID, "status": "pending", "duplicate": duplicate, "request_id": platformobservability.RequestID(c), "trace_id": command.TraceID})
		}
	}
	registerEmployeePOST("/api/v1/tasks/:taskPublicID/accept", registerEmployeeCommand("employee_accept_task.v1"))
	registerEmployeePOST("/api/v1/tasks/:taskPublicID/complete", registerEmployeeCommand("employee_complete_task.v1"))
	r.POST("/internal/sync/v1/events", func(c *gin.Context) {
		if store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "sync_store_unavailable", "message": "sync store unavailable"})
			return
		}
		var envelope sharedEvent.EventEnvelope
		if err := c.ShouldBindJSON(&envelope); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_event", "message": "invalid event envelope"})
			return
		}
		duplicate, err := store.ApplyEvent(c.Request.Context(), envelope, func(ctx context.Context, value sharedEvent.EventEnvelope) error {
			return projectEvent(ctx, store, value)
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "event_projection_failed", "message": "event projection failed"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"event_id": envelope.EventID, "status": "applied", "duplicate": duplicate})
	})
	r.GET("/internal/sync/v1/commands/pending", func(c *gin.Context) {
		if store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "command_store_unavailable", "message": "command store unavailable"})
			return
		}
		limit := 20
		if raw := c.Query("limit"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}
		values, err := store.ClaimPendingCommands(c.Request.Context(), limit, time.Now().UTC())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "command_read_failed", "message": "command read failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": values})
	})
	r.POST("/internal/sync/v1/commands/:commandID/ack", func(c *gin.Context) {
		if store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "command_store_unavailable", "message": "command store unavailable"})
			return
		}
		var body struct {
			Status        string     `json:"status"`
			Reason        string     `json:"reason"`
			NextAttemptAt *time.Time `json:"next_attempt_at"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_ack", "message": "invalid command acknowledgement"})
			return
		}
		var err error
		switch body.Status {
		case sharedEvent.StatusApplied, sharedEvent.StatusSent:
			err = store.MarkCommandSent(c.Request.Context(), c.Param("commandID"))
		case sharedEvent.StatusRetry:
			nextAttempt := time.Now().UTC().Add(time.Second)
			if body.NextAttemptAt != nil && !body.NextAttemptAt.IsZero() {
				nextAttempt = body.NextAttemptAt.UTC()
			}
			err = store.MarkCommandRetry(c.Request.Context(), c.Param("commandID"), nextAttempt, body.Reason)
		case sharedEvent.StatusFailed, sharedEvent.StatusRejected:
			err = store.MarkCommandFailed(c.Request.Context(), c.Param("commandID"), body.Reason)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_ack", "message": "unsupported command status"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "ack_failed", "message": "command acknowledgement failed"})
			return
		}
		c.Status(http.StatusNoContent)
	})
	return &Server{
		cfg: cfg, db: db, redis: redisClient, store: store, log: log,
		httpServer: &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: r, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second},
	}
}

func projectEvent(ctx context.Context, store edgesync.Store, envelope sharedEvent.EventEnvelope) error {
	if envelope.EventType == "probe.task_projection.updated.v1" {
		var payload struct {
			Projection edgesync.TaskProjection `json:"projection"`
		}
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf("decode task projection event: %w", err)
		}
		return store.UpsertTaskProjection(ctx, payload.Projection)
	}
	status, ok := taskProjectionStatus(envelope.EventType)
	if !ok {
		return nil
	}
	var payload taskProjectionEventPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("decode %s payload: %w", envelope.EventType, err)
	}
	// Accept the old wrapper shape during the Foundation-to-business migration,
	// while all new Core events use the direct frozen snapshot payload.
	if payload.TaskPublicID == "" {
		var wrapped struct {
			Projection taskProjectionEventPayload `json:"projection"`
		}
		if err := json.Unmarshal(envelope.Payload, &wrapped); err == nil {
			payload = wrapped.Projection
		}
	}
	if payload.TaskPublicID == "" || payload.EmployeePublicID == "" || payload.SyncVersion == 0 {
		return fmt.Errorf("%s payload is missing task snapshot fields", envelope.EventType)
	}
	if payload.BusinessStatus == "" {
		payload.BusinessStatus = status
	}
	if payload.BusinessStatus != status {
		return fmt.Errorf("%s payload business_status %q does not match event", envelope.EventType, payload.BusinessStatus)
	}
	return store.UpsertTaskProjection(ctx, edgesync.TaskProjection{
		PublicID: payload.TaskPublicID, AssignmentPublicID: payload.AssignmentPublicID,
		EmployeePublicID: payload.EmployeePublicID, FlightDisplayNo: payload.FlightDisplayNo,
		TaskName: payload.TaskName, AreaName: payload.AreaName, PlannedAt: payload.PlannedAt,
		Status: status, BusinessStatus: payload.BusinessStatus, Message: payload.Message,
		SyncVersion: payload.SyncVersion,
	})
}

func employeeActorPublicID(c *gin.Context) (string, error) {
	if principal, ok := platformhttpauth.Principal(c); ok {
		if principal.Type != platformsecurity.HumanPrincipal || principal.PublicID == "" {
			return "", fmt.Errorf("employee route requires a human principal")
		}
		return principal.PublicID, nil
	}
	if _, configured := c.Get("allow_dev_actor_headers"); configured && !platformhttpauth.DevelopmentActorHeadersAllowed(c) {
		return "", fmt.Errorf("development actor headers are disabled")
	}
	actorPublicID := c.GetHeader("X-Employee-Public-ID")
	if actorPublicID == "" {
		return "", fmt.Errorf("employee principal is missing")
	}
	return actorPublicID, nil
}

type employeeTaskCommandRequest struct {
	AssignmentPublicID  string     `json:"assignment_public_id"`
	ExpectedSyncVersion *uint64    `json:"expected_sync_version"`
	ClientOccurredAt    *time.Time `json:"client_occurred_at"`
	Note                string     `json:"note"`
}

type employeeTaskCommandPayload struct {
	AssignmentPublicID  string    `json:"assignment_public_id"`
	ExpectedSyncVersion uint64    `json:"expected_sync_version"`
	ClientOccurredAt    time.Time `json:"client_occurred_at"`
	Note                string    `json:"note,omitempty"`
}

type taskProjectionEventPayload struct {
	TaskPublicID       string    `json:"task_public_id"`
	AssignmentPublicID string    `json:"assignment_public_id"`
	ConfirmationID     string    `json:"confirmation_id,omitempty"`
	EmployeePublicID   string    `json:"employee_public_id"`
	FlightDisplayNo    string    `json:"flight_display_no"`
	TaskName           string    `json:"task_name"`
	AreaName           string    `json:"area_name"`
	PlannedAt          time.Time `json:"planned_at"`
	BusinessStatus     string    `json:"business_status"`
	Message            string    `json:"message"`
	SyncVersion        uint64    `json:"sync_version"`
}

func taskProjectionStatus(eventType string) (string, bool) {
	switch eventType {
	case "task.assigned.v1":
		return "assigned", true
	case "task.accepted.v1":
		return "in_progress", true
	case "task.completed.v1":
		return "completed", true
	case "task.cancelled.v1":
		return "cancelled", true
	default:
		return "", false
	}
}

func (s *Server) Handler() http.Handler { return s.httpServer.Handler }

func (s *Server) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("edge server context is nil")
	}
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen edge api on %s: %w", s.httpServer.Addr, err)
	}
	if tlsConfig, err := platformsecurity.NewServerTLSConfig(s.cfg.TLS); err != nil {
		_ = listener.Close()
		return fmt.Errorf("configure edge api TLS: %w", err)
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
			errs = append(errs, fmt.Errorf("shutdown edge http: %w", err))
		}
		if err := platformmysql.Close(s.db); err != nil {
			errs = append(errs, err)
		}
		if s.redis != nil {
			if err := s.redis.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close edge redis: %w", err))
			}
		}
		s.shutdownErr = errors.Join(errs...)
	})
	return s.shutdownErr
}
