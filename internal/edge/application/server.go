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
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	edgeidentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/identity"
	edgenotification "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/notification"
	edgerealtime "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/realtime"
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
	notifier    edgenotification.Publisher
	realtime    *edgerealtime.Endpoint
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
	return NewServerWithStoreAndAuthAndIdentity(cfg, db, redisClient, log, store, authenticator, nil)
}

func NewServerWithStoreAndAuthAndIdentity(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, store edgesync.Store, authenticator platformsecurity.Authenticator, identityService *edgeidentity.Service) *Server {
	return NewServerWithStoreAndAuthAndIdentityAndNotifications(cfg, db, redisClient, log, store, authenticator, identityService, nil)
}

func NewServerWithStoreAndAuthAndIdentityAndNotifications(cfg config.ServiceConfig, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, store edgesync.Store, authenticator platformsecurity.Authenticator, identityService *edgeidentity.Service, notifier edgenotification.Publisher) *Server {
	// Foundation 阶段的路由直接围绕 Store 装配：移动端只读 Projection、写
	// Command；内部同步接口由 Core Worker 使用。真实业务阶段应再拆出 Handler
	// 和 Application Service，避免把业务规则继续堆在路由闭包中。
	if log == nil {
		log = zap.NewNop()
	}
	if notifier == nil {
		notifier = edgenotification.NewInMemoryFanout()
	}
	endpoint := platformhealth.New(time.Now().UTC(),
		map[string]platformhealth.Checker{"mysql": func(ctx context.Context) error { return platformmysql.Ping(ctx, db) }},
		map[string]platformhealth.Checker{"redis": func(ctx context.Context) error { return platformredis.Ping(ctx, redisClient) }},
	)
	r := gin.New()
	metrics := platformobservability.NewHTTPRegistry()
	var realtimeSubscribers edgenotification.Subscriber
	if subscriber, ok := notifier.(edgenotification.Subscriber); ok {
		realtimeSubscribers = subscriber
	}
	var ticketStore edgerealtime.TicketStorePort
	if db != nil {
		ticketStore = edgesync.NewSQLRealtimeTicketStore(db, edgerealtime.DefaultTicketTTL)
	}
	realtimeEndpoint := edgerealtime.NewEndpointWithTicketStore(realtimeSubscribers, metrics, log, edgerealtime.Config{}, ticketStore)
	r.Use(gin.Recovery(), platformobservability.MiddlewareWithMetrics(log, "edge-api", metrics))
	r.GET("/health/live", endpoint.Live())
	r.GET("/health/ready", endpoint.Ready())
	r.GET("/metrics", func(c *gin.Context) {
		if sqlDB, err := platformmysql.SQLDB(db); err == nil {
			platformobservability.RecordDBStats(metrics, "edge", sqlDB)
		}
		refreshEdgeMetrics(c.Request.Context(), store, metrics)
		metrics.Handler().ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/api/v1/foundation", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"component": "edge-api", "architecture": "v2", "data_mode": "projection-only"})
	})
	var employeeAuth gin.HandlerFunc
	if authenticator != nil || cfg.AllowDevActorHeaders {
		var resolver platformhttpauth.PrincipalResolver
		if identityService != nil {
			resolver = identityService
		}
		employeeAuth = platformhttpauth.RequireJWT(authenticator, resolver, cfg.AllowDevActorHeaders)
	}
	registerRealtimeUnavailable := func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "realtime_unavailable", "message": "realtime notification service is unavailable"})
	}
	if employeeAuth == nil {
		r.POST(edgerealtime.TicketPath, registerRealtimeUnavailable)
		r.GET(edgerealtime.WebSocketPath, registerRealtimeUnavailable)
	} else {
		r.POST(edgerealtime.TicketPath, employeeAuth, func(c *gin.Context) {
			principal, err := realtimePrincipal(c)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthenticated", "message": "employee authentication is required"})
				return
			}
			ticket, err := realtimeEndpoint.IssueTicket(principal)
			if err != nil {
				registerRealtimeUnavailable(c)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": ticket, "request_id": platformobservability.RequestID(c), "trace_id": platformobservability.TraceID(c)})
		})
		r.GET(edgerealtime.WebSocketPath, realtimeAuthMiddleware(employeeAuth, realtimeEndpoint, identityService), func(c *gin.Context) {
			principal, err := realtimePrincipal(c)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthenticated", "message": "employee authentication is required"})
				return
			}
			if !realtimeEndpoint.Ready() {
				registerRealtimeUnavailable(c)
				return
			}
			realtimeEndpoint.ServeHTTP(c.Writer, c.Request, principal.PublicID)
		})
	}
	if identityService != nil {
		edgeidentity.RegisterRoutes(r, identityService, employeeAuth)
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
		snapshotAt := time.Now().UTC()
		var snapshot edgesync.TaskSnapshot
		if provider, ok := store.(edgesync.TaskSnapshotProvider); ok {
			snapshot, err = provider.ListTaskSnapshot(c.Request.Context(), actorPublicID, snapshotAt)
		} else {
			var values []edgesync.TaskProjection
			values, err = store.ListTaskProjections(c.Request.Context(), actorPublicID)
			snapshot = edgesync.TaskSnapshot{Items: values, SnapshotAt: snapshotAt}
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "projection_read_failed", "message": "projection read failed"})
			return
		}
		lagState := "unknown"
		lagSeconds := 0.0
		if snapshot.ProjectionLagKnown {
			lagState = "known"
			lagSeconds = snapshot.ProjectionLag.Seconds()
		}
		c.JSON(http.StatusOK, gin.H{
			"items":                  snapshot.Items,
			"source":                 "edge_projection",
			"sync_mode":              "full_snapshot",
			"snapshot_at":            snapshot.SnapshotAt.UTC(),
			"projection_revision":    snapshot.ProjectionRevision,
			"projection_lag_seconds": lagSeconds,
			"projection_lag_state":   lagState,
			"next_cursor":            nil,
			"reset_required":         false,
			"request_id":             platformobservability.RequestID(c),
			"trace_id":               platformobservability.TraceID(c),
		})
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
			writeCommandPutError(c, err)
			return
		}
		record, err := store.FindCommand(c.Request.Context(), command.CommandID)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "command_status_unavailable", "message": "command status unavailable"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"command_id": command.CommandID, "status": publicCommandStatus(record.Status), "duplicate": duplicate, "request_id": platformobservability.RequestID(c), "trace_id": command.TraceID})
	})
	registerEmployeeGET("/api/v1/commands/:commandID", func(c *gin.Context) {
		if store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "command_store_unavailable", "message": "command store unavailable"})
			return
		}
		actorPublicID, err := employeeActorPublicID(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthenticated", "message": "employee authentication is required"})
			return
		}
		record, err := store.FindCommand(c.Request.Context(), c.Param("commandID"))
		if errors.Is(err, edgesync.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "command_not_found", "message": "command not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "command_status_unavailable", "message": "command status unavailable"})
			return
		}
		if record.Envelope.ActorPublicID != actorPublicID {
			c.JSON(http.StatusForbidden, gin.H{"code": "forbidden", "message": "command does not belong to authenticated employee"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": commandStatusView(record), "request_id": platformobservability.RequestID(c), "trace_id": platformobservability.TraceID(c)})
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
			if err := c.ShouldBindJSON(&request); err != nil || request.CommandID == "" || request.ExpectedSyncVersion == nil || request.AssignmentPublicID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_command", "message": "command_id, assignment_public_id and expected_sync_version are required"})
				return
			}
			var clientOccurredAt *time.Time
			if request.ClientOccurredAt != nil && !request.ClientOccurredAt.IsZero() {
				occurredAt := request.ClientOccurredAt.UTC()
				clientOccurredAt = &occurredAt
			}
			payload := employeeTaskCommandPayload{AssignmentPublicID: request.AssignmentPublicID, ExpectedSyncVersion: *request.ExpectedSyncVersion, ClientOccurredAt: clientOccurredAt, Note: request.Note}
			commandOccurredAt := time.Time{}
			if clientOccurredAt != nil {
				commandOccurredAt = *clientOccurredAt
			}
			command, err := sharedEvent.NewCommandWithID(request.CommandID, commandType, actorPublicID, c.Param("taskPublicID"), platformobservability.TraceID(c), commandOccurredAt, payload)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "command_create_failed", "message": "command could not be created"})
				return
			}
			duplicate, err := store.PutCommand(c.Request.Context(), command)
			if err != nil {
				writeCommandPutError(c, err)
				return
			}
			record, err := store.FindCommand(c.Request.Context(), command.CommandID)
			if err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"code": "command_status_unavailable", "message": "command status unavailable"})
				return
			}
			c.JSON(http.StatusAccepted, gin.H{"command_id": command.CommandID, "status": publicCommandStatus(record.Status), "duplicate": duplicate, "request_id": platformobservability.RequestID(c), "trace_id": command.TraceID})
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
		if !duplicate {
			if taskChanged, ok, notificationErr := taskChangedNotification(envelope); notificationErr != nil {
				log.Warn("build task notification failed", zap.Error(notificationErr))
			} else if ok {
				result, publishErr := notifier.Publish(c.Request.Context(), taskChanged)
				resultLabel := "delivered"
				if result.SubscriberCount == 0 {
					resultLabel = "no_subscribers"
				} else if publishErr != nil {
					resultLabel = "failed"
				}
				metrics.Inc("flight_notification_delivery_total", platformobservability.Labels{"component": "edge", "result": resultLabel})
				if publishErr != nil {
					log.Warn("task notification delivery failed", zap.Error(publishErr), zap.String("notification_id", taskChanged.NotificationID), zap.String("task_public_id", taskChanged.TaskPublicID))
				}
			}
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
		workerID := strings.TrimSpace(c.Query("worker_id"))
		leaseDuration := 30 * time.Second
		if raw := c.Query("lease_seconds"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 3600 {
				leaseDuration = time.Duration(parsed) * time.Second
			}
		}
		var values []edgesync.CommandRecord
		var err error
		if workerID != "" {
			if leased, ok := store.(edgesync.LeasedStore); ok {
				values, err = leased.ClaimPendingCommandsWithLease(c.Request.Context(), limit, time.Now().UTC(), workerID, leaseDuration)
			} else {
				values, err = store.ClaimPendingCommands(c.Request.Context(), limit, time.Now().UTC())
			}
		} else {
			values, err = store.ClaimPendingCommands(c.Request.Context(), limit, time.Now().UTC())
		}
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
			LeaseOwner    string     `json:"lease_owner"`
			NextAttemptAt *time.Time `json:"next_attempt_at"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_ack", "message": "invalid command acknowledgement"})
			return
		}
		var err error
		leased, hasLeaseStore := store.(edgesync.LeasedStore)
		switch body.Status {
		case sharedEvent.StatusApplied, sharedEvent.StatusSent:
			if hasLeaseStore && strings.TrimSpace(body.LeaseOwner) != "" {
				err = leased.MarkCommandSentWithLease(c.Request.Context(), c.Param("commandID"), strings.TrimSpace(body.LeaseOwner))
			} else {
				err = store.MarkCommandSent(c.Request.Context(), c.Param("commandID"))
			}
		case sharedEvent.StatusRetry:
			nextAttempt := time.Now().UTC().Add(time.Second)
			if body.NextAttemptAt != nil && !body.NextAttemptAt.IsZero() {
				nextAttempt = body.NextAttemptAt.UTC()
			}
			if hasLeaseStore && strings.TrimSpace(body.LeaseOwner) != "" {
				err = leased.MarkCommandRetryWithLease(c.Request.Context(), c.Param("commandID"), strings.TrimSpace(body.LeaseOwner), nextAttempt, body.Reason)
			} else {
				err = store.MarkCommandRetry(c.Request.Context(), c.Param("commandID"), nextAttempt, body.Reason)
			}
		case sharedEvent.StatusFailed, sharedEvent.StatusRejected:
			if hasLeaseStore && strings.TrimSpace(body.LeaseOwner) != "" {
				err = leased.MarkCommandFailedWithLease(c.Request.Context(), c.Param("commandID"), strings.TrimSpace(body.LeaseOwner), body.Reason)
			} else {
				err = store.MarkCommandFailed(c.Request.Context(), c.Param("commandID"), body.Reason)
			}
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
		cfg: cfg, db: db, redis: redisClient, store: store, notifier: notifier, realtime: realtimeEndpoint, log: log,
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

func taskChangedNotification(envelope sharedEvent.EventEnvelope) (edgenotification.TaskChanged, bool, error) {
	if envelope.EventType == "probe.task_projection.updated.v1" {
		var payload struct {
			Projection edgesync.TaskProjection `json:"projection"`
		}
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return edgenotification.TaskChanged{}, false, fmt.Errorf("decode task notification projection: %w", err)
		}
		reason := payload.Projection.BusinessStatus
		if reason == "" {
			reason = payload.Projection.Status
		}
		if reason == "" {
			reason = "updated"
		}
		notification, err := edgenotification.NewTaskChanged(payload.Projection.EmployeePublicID, payload.Projection.PublicID, payload.Projection.SyncVersion, reason, time.Now().UTC())
		return notification, true, err
	}
	status, ok := taskProjectionStatus(envelope.EventType)
	if !ok {
		return edgenotification.TaskChanged{}, false, nil
	}
	var payload taskProjectionEventPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return edgenotification.TaskChanged{}, false, fmt.Errorf("decode %s task notification: %w", envelope.EventType, err)
	}
	if payload.TaskPublicID == "" {
		var wrapped struct {
			Projection taskProjectionEventPayload `json:"projection"`
		}
		if err := json.Unmarshal(envelope.Payload, &wrapped); err == nil {
			payload = wrapped.Projection
		}
	}
	if payload.TaskPublicID == "" || payload.EmployeePublicID == "" || payload.SyncVersion == 0 {
		return edgenotification.TaskChanged{}, false, fmt.Errorf("%s payload is missing notification fields", envelope.EventType)
	}
	notification, err := edgenotification.NewTaskChanged(payload.EmployeePublicID, payload.TaskPublicID, payload.SyncVersion, status, time.Now().UTC())
	return notification, true, err
}

func refreshEdgeMetrics(ctx context.Context, store edgesync.Store, metrics *platformobservability.Registry) {
	provider, ok := store.(edgesync.SyncQueueStatsProvider)
	if !ok || metrics == nil {
		return
	}
	stats, err := provider.SyncQueueStats(ctx, time.Now().UTC())
	if err != nil {
		return
	}
	labels := platformobservability.Labels{"component": "edge", "queue": "edge_commands"}
	metrics.SetGauge("flight_sync_queue_pending_items", labels, float64(stats.PendingCommandCount))
	metrics.SetGauge("flight_sync_queue_oldest_age_seconds", labels, stats.OldestPendingCommandAge.Seconds())
	inboxLabels := platformobservability.Labels{"component": "edge", "queue": "edge_inbox"}
	metrics.SetGauge("flight_sync_queue_pending_items", inboxLabels, float64(stats.PendingInboxCount))
	metrics.SetGauge("flight_sync_queue_failed_items", inboxLabels, float64(stats.FailedInboxCount))
	metrics.SetGauge("flight_sync_queue_oldest_age_seconds", inboxLabels, stats.OldestPendingInboxAge.Seconds())
	metrics.SetGauge("flight_sync_projection_lag_seconds", platformobservability.Labels{"component": "edge"}, stats.ProjectionLag.Seconds())
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

func realtimeAuthMiddleware(employeeAuth gin.HandlerFunc, endpoint *edgerealtime.Endpoint, identityService *edgeidentity.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if employeeAuth == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "realtime_unavailable", "message": "realtime notification service is unavailable"})
			c.Abort()
			return
		}
		if c.GetHeader("Authorization") == "" {
			if _, offered := edgerealtime.TicketFromProtocolHeader(c.GetHeader("Sec-WebSocket-Protocol")); offered {
				principal, err := endpoint.ConsumeTicketFromProtocolHeader(c.GetHeader("Sec-WebSocket-Protocol"))
				if err != nil {
					c.JSON(http.StatusUnauthorized, gin.H{"code": "realtime_ticket_invalid", "message": "realtime connection ticket is invalid or expired"})
					c.Abort()
					return
				}
				if identityService != nil {
					principal, err = identityService.ResolvePrincipal(c.Request.Context(), principal)
					if err != nil {
						c.JSON(http.StatusUnauthorized, gin.H{"code": "session_expired", "message": "employee session is expired or revoked"})
						c.Abort()
						return
					}
				}
				platformhttpauth.SetPrincipal(c, principal)
				c.Set("allow_dev_actor_headers", false)
				c.Next()
				return
			}
		}
		employeeAuth(c)
	}
}

func realtimePrincipal(c *gin.Context) (platformsecurity.Principal, error) {
	if principal, ok := platformhttpauth.Principal(c); ok {
		if principal.Type != platformsecurity.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" {
			return platformsecurity.Principal{}, fmt.Errorf("realtime route requires a human principal")
		}
		return principal, nil
	}
	if _, allowed := c.Get("allow_dev_actor_headers"); !allowed || !platformhttpauth.DevelopmentActorHeadersAllowed(c) {
		return platformsecurity.Principal{}, fmt.Errorf("employee principal is missing")
	}
	actorPublicID := c.GetHeader("X-Employee-Public-ID")
	if strings.TrimSpace(actorPublicID) == "" {
		return platformsecurity.Principal{}, fmt.Errorf("employee principal is missing")
	}
	return platformsecurity.Principal{Type: platformsecurity.HumanPrincipal, PublicID: strings.TrimSpace(actorPublicID), Subject: strings.TrimSpace(actorPublicID)}, nil
}

type employeeTaskCommandRequest struct {
	CommandID           string     `json:"command_id"`
	AssignmentPublicID  string     `json:"assignment_public_id"`
	ExpectedSyncVersion *uint64    `json:"expected_sync_version"`
	ClientOccurredAt    *time.Time `json:"client_occurred_at"`
	Note                string     `json:"note"`
}

type commandStatusResult struct {
	CommandID     string     `json:"command_id"`
	CommandType   string     `json:"command_type"`
	AggregateID   string     `json:"aggregate_id"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	ErrorCode     string     `json:"error_code,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func commandStatusView(record edgesync.CommandRecord) commandStatusResult {
	status := publicCommandStatus(record.Status)
	result := commandStatusResult{
		CommandID: record.Envelope.CommandID, CommandType: record.Envelope.CommandType,
		AggregateID: record.Envelope.AggregateID, Status: status, Attempts: record.Attempts,
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}
	if (status == "pending" || status == "syncing") && !record.NextAttempt.IsZero() {
		nextAttempt := record.NextAttempt.UTC()
		result.NextAttemptAt = &nextAttempt
	}
	if status == "failed" && record.LastError != "" {
		// The durable error text can contain internal implementation details;
		// expose only a stable client-facing category from this endpoint.
		result.ErrorCode = "command_failed"
	}
	return result
}

func publicCommandStatus(status string) string {
	switch status {
	case sharedEvent.StatusPending:
		return "pending"
	case sharedEvent.StatusProcessing, sharedEvent.StatusRetry:
		return "syncing"
	case sharedEvent.StatusSent, sharedEvent.StatusApplied:
		return "confirmed"
	case sharedEvent.StatusFailed, sharedEvent.StatusRejected:
		return "failed"
	default:
		return "syncing"
	}
}

func writeCommandPutError(c *gin.Context, err error) {
	if errors.Is(err, edgesync.ErrCommandIDConflict) {
		c.JSON(http.StatusConflict, gin.H{"code": "command_id_conflict", "message": "command id was already used with different content"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_command", "message": "command was rejected"})
}

type employeeTaskCommandPayload struct {
	AssignmentPublicID  string     `json:"assignment_public_id"`
	ExpectedSyncVersion uint64     `json:"expected_sync_version"`
	ClientOccurredAt    *time.Time `json:"client_occurred_at,omitempty"`
	Note                string     `json:"note,omitempty"`
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
		if s.realtime != nil {
			s.realtime.Close()
		}
		if err := s.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("shutdown edge http: %w", err))
		}
		if err := platformmysql.Close(s.db); err != nil {
			errs = append(errs, err)
		}
		if closer, ok := s.notifier.(interface{ Close() }); ok {
			closer.Close()
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
