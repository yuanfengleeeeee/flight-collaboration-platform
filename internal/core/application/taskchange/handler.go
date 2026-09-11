package taskchange

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	platformhttpauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpauth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func RegisterRoutes(router gin.IRouter, service *Service, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	h := Handler{service: service}
	withAuth := func(handler gin.HandlerFunc) []gin.HandlerFunc {
		return append(append([]gin.HandlerFunc{}, middleware...), handler)
	}
	router.GET("/api/v1/task-change-requests", withAuth(h.list)...)
	router.POST("/api/v1/task-change-requests", withAuth(h.create)...)
	router.PATCH("/api/v1/task-change-requests/:publicID/review", withAuth(h.review)...)
}

type Handler struct{ service *Service }

type createRequest struct {
	TaskPublicID            string     `json:"task_public_id"`
	ExceptionPublicID       string     `json:"exception_public_id"`
	Action                  string     `json:"action"`
	Reason                  string     `json:"reason"`
	TargetCandidatePublicID string     `json:"target_candidate_public_id"`
	TargetPlannedAt         *time.Time `json:"target_planned_at"`
	RequestID               string     `json:"request_id"`
}

type reviewRequest struct {
	Decision   string `json:"decision"`
	ReviewNote string `json:"review_note"`
}

func (h Handler) list(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_task_change_query", "invalid pagination")
		return
	}
	result, err := h.service.List(c.Request.Context(), principal, ListFilter{TaskPublicID: c.Query("task_public_id"), Status: c.Query("status"), Page: page, PageSize: pageSize})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) create(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request createRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_task_change_request", "task change request body is invalid")
		return
	}
	requestID := strings.TrimSpace(request.RequestID)
	if requestID == "" {
		// The request middleware gives retries a stable key when the caller
		// forwards X-Request-ID, while still keeping each normal HTTP request
		// distinct.
		requestID = observability.RequestID(c)
	}
	value, err := h.service.Create(c.Request.Context(), principal, CreateInput{TaskPublicID: request.TaskPublicID, ExceptionPublicID: request.ExceptionPublicID, Action: taskmodule.ChangeAction(strings.TrimSpace(request.Action)), Reason: request.Reason, TargetCandidatePublicID: request.TargetCandidatePublicID, TargetPlannedAt: request.TargetPlannedAt, RequestID: requestID, TraceID: observability.TraceID(c), SourceIP: c.ClientIP(), Scope: principal.Scopes})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusCreated, value)
}

func (h Handler) review(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request reviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_task_change_review", "task change review body is invalid")
		return
	}
	value, err := h.service.Review(c.Request.Context(), principal, ReviewInput{PublicID: c.Param("publicID"), Decision: request.Decision, ReviewNote: request.ReviewNote, RequestID: observability.RequestID(c), TraceID: observability.TraceID(c), SourceIP: c.ClientIP(), Scope: principal.Scopes})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, value)
}

func principalFromRequest(c *gin.Context) (security.Principal, bool) {
	if principal, ok := platformhttpauth.Principal(c); ok {
		return principal, true
	}
	if !platformhttpauth.DevelopmentActorHeadersAllowed(c) {
		return security.Principal{}, false
	}
	publicID := strings.TrimSpace(c.GetHeader("X-Actor-Public-ID"))
	role := strings.TrimSpace(c.GetHeader("X-Actor-Role"))
	if role == "" {
		role = strings.TrimSpace(strings.Split(c.GetHeader("X-Actor-Roles"), ",")[0])
	}
	if publicID == "" || role == "" {
		return security.Principal{}, false
	}
	areas, areaOK := parseIDs(c.GetHeader("X-Actor-Area-IDs"))
	teams, teamOK := parseIDs(c.GetHeader("X-Actor-Team-IDs"))
	if !areaOK || !teamOK {
		return security.Principal{}, false
	}
	return security.Principal{Type: security.HumanPrincipal, PublicID: publicID, Roles: []string{role}, Scopes: security.AccessScope{Global: strings.EqualFold(c.GetHeader("X-Actor-Global"), "true"), AreaIDs: areas, TeamIDs: teams}}, true
}

func pagination(c *gin.Context) (int, int, bool) {
	page, pageSize := 1, 20
	var err error
	if value := c.Query("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, false
		}
	}
	if value := c.Query("page_size"); value != "" {
		pageSize, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, false
		}
	}
	return page, pageSize, page >= 1 && pageSize >= 1 && pageSize <= 100
}

func parseIDs(value string) ([]uint64, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, true
	}
	parts := strings.Split(value, ",")
	result := make([]uint64, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err != nil || parsed == 0 {
			return nil, false
		}
		result = append(result, parsed)
	}
	return result, true
}

func writeData(c *gin.Context, status int, value any) {
	c.JSON(status, gin.H{"data": value, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		writeError(c, http.StatusForbidden, "forbidden", "principal is not allowed to manage task changes")
	case errors.Is(err, ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_task_change_request", "task change request is invalid")
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "task_change_not_found", "task change request was not found")
	case errors.Is(err, ErrConflict):
		writeError(c, http.StatusConflict, "task_change_state_conflict", "task change request cannot be reviewed in its current state")
	case errors.Is(err, ErrApplyFailed):
		writeError(c, http.StatusConflict, "task_change_apply_failed", "approved task change could not be applied")
	case errors.Is(err, ErrRepositoryNotConfigured):
		writeError(c, http.StatusServiceUnavailable, "task_change_unavailable", "task change service is unavailable")
	default:
		writeError(c, http.StatusServiceUnavailable, "task_change_unavailable", "task change operation failed")
	}
}
