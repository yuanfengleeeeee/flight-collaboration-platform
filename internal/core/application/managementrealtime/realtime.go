// Package managementrealtime exposes a short-lived SSE hint stream for the
// management workbench. Core remains authoritative: clients always recover by
// querying the paged task/assignment APIs after reconnecting.
package managementrealtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	platformhttpauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpauth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

var (
	ErrRepositoryNotConfigured = errors.New("management realtime repository is not configured")
	ErrForbidden               = errors.New("principal is not allowed to subscribe to management realtime")
)

type Event struct {
	ID                uint64          `json:"id"`
	EventID           string          `json:"event_id"`
	EventType         string          `json:"event_type"`
	AggregateType     string          `json:"aggregate_type"`
	AggregatePublicID string          `json:"aggregate_public_id"`
	TeamID            uint64          `json:"team_id,omitempty"`
	AreaID            uint64          `json:"area_id,omitempty"`
	OccurredAt        time.Time       `json:"occurred_at"`
	Payload           json.RawMessage `json:"payload,omitempty"`
}

type Repository interface {
	ListManagementEvents(context.Context, uint64, int) ([]Event, error)
}

type Service struct {
	repository Repository
	authorizer security.Authorizer
	interval   time.Duration
	maxWait    time.Duration
}

func NewService(repository Repository, authorizer security.Authorizer) *Service {
	return &Service{repository: repository, authorizer: authorizer, interval: time.Second, maxWait: 60 * time.Second}
}

func RegisterRoutes(router gin.IRouter, service *Service, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	chain := append(append([]gin.HandlerFunc{}, middleware...), func(c *gin.Context) {
		principal, ok := principalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthenticated", "message": "management authentication is required"})
			return
		}
		service.ServeHTTP(c.Writer, c.Request, principal)
	})
	router.GET("/api/v1/realtime/management", chain...)
}

func (s *Service) ServeHTTP(writer http.ResponseWriter, request *http.Request, principal security.Principal) {
	if s == nil || s.repository == nil {
		http.Error(writer, "management realtime is unavailable", http.StatusServiceUnavailable)
		return
	}
	if principal.Type != security.HumanPrincipal || principal.PublicID == "" || s.authorizer == nil || s.authorizer.Authorize(principal, "event:read", security.AccessScope{}) != nil {
		http.Error(writer, ErrForbidden.Error(), http.StatusForbidden)
		return
	}
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "management realtime requires streaming support", http.StatusInternalServerError)
		return
	}
	afterID := parseLastEventID(request)
	wait := s.maxWait
	if raw := request.URL.Query().Get("wait_seconds"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 && time.Duration(seconds)*time.Second < wait {
			wait = time.Duration(seconds) * time.Second
		}
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.Header().Set("Vary", "Authorization")
	_, _ = fmt.Fprint(writer, "event: ready\ndata: {\"scope\":\"role_scoped\"}\n\n")
	flusher.Flush()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-timer.C:
			_, _ = fmt.Fprint(writer, ": keep-alive\n\n")
			flusher.Flush()
			return
		case <-heartbeat.C:
			// Core's HTTP write deadline is finite. Periodic comments keep a
			// quiet stream alive without manufacturing business events.
			_, _ = fmt.Fprint(writer, ": heartbeat\n\n")
			flusher.Flush()
		case <-ticker.C:
			events, err := s.repository.ListManagementEvents(request.Context(), afterID, 100)
			if err != nil {
				_, _ = fmt.Fprint(writer, "event: error\ndata: {\"code\":\"management_realtime_read_failed\"}\n\n")
				flusher.Flush()
				return
			}
			for _, value := range events {
				if value.ID > afterID {
					afterID = value.ID
				}
				if !scopeAllows(principal.Scopes, value) {
					continue
				}
				encoded, err := json.Marshal(value)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", value.ID, safeEventName(value.EventType), encoded)
			}
			if len(events) > 0 {
				flusher.Flush()
			}
		}
	}
}

func principalFromContext(c *gin.Context) (security.Principal, bool) {
	return platformhttpauth.Principal(c)
}

func parseLastEventID(request *http.Request) uint64 {
	raw := strings.TrimSpace(request.Header.Get("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(request.URL.Query().Get("after_id"))
	}
	value, _ := strconv.ParseUint(raw, 10, 64)
	return value
}

func scopeAllows(scope security.AccessScope, value Event) bool {
	if scope.Global {
		return true
	}
	if value.TeamID != 0 {
		for _, id := range scope.TeamIDs {
			if id == value.TeamID {
				return true
			}
		}
	}
	if value.AreaID != 0 {
		for _, id := range scope.AreaIDs {
			if id == value.AreaID {
				return true
			}
		}
	}
	return false
}

func safeEventName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "task.updated"
	}
	return strings.ReplaceAll(value, " ", "_")
}
