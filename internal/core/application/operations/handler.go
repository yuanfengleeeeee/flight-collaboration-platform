package operations

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
	router.GET("/api/v1/personnel/status", withAuth(h.listPersonnelStatus)...)
	router.GET("/api/v1/personnel/:publicID/status-history", withAuth(h.listPersonnelStatusHistory)...)
	router.GET("/api/v1/events", withAuth(h.listEvents)...)
	router.GET("/api/v1/audit", withAuth(h.listAudit)...)
	router.GET("/api/v1/scopes", withAuth(h.getScopes)...)
	router.GET("/api/v1/diagnostics", withAuth(h.getDiagnostics)...)
}

type Handler struct{ service *Service }

func (h Handler) listPersonnelStatus(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "operations authentication is required")
		return
	}
	page, err := pagination(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_operations_query", "invalid pagination")
		return
	}
	result, err := h.service.ListPersonnelStatus(c.Request.Context(), principal, StatusFilter{WorkState: c.Query("work_state"), TeamPublicID: c.Query("team_public_id"), AreaPublicID: c.Query("area_public_id"), PageQuery: page})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) listPersonnelStatusHistory(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "operations authentication is required")
		return
	}
	page, err := pagination(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_operations_query", "invalid pagination")
		return
	}
	result, err := h.service.ListPersonnelStatusHistory(c.Request.Context(), principal, StatusHistoryFilter{PersonnelPublicID: c.Param("publicID"), PageQuery: page})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) listEvents(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "operations authentication is required")
		return
	}
	page, err := pagination(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_operations_query", "invalid pagination")
		return
	}
	from, to, err := timeRange(c.Query("from"), c.Query("to"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_operations_query", "invalid time range")
		return
	}
	result, err := h.service.ListEvents(c.Request.Context(), principal, EventFilter{EventType: c.Query("event_type"), Status: c.Query("status"), FlightPublicID: c.Query("flight_public_id"), From: from, To: to, PageQuery: page})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) listAudit(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "operations authentication is required")
		return
	}
	page, err := pagination(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_operations_query", "invalid pagination")
		return
	}
	from, to, err := timeRange(c.Query("from"), c.Query("to"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_operations_query", "invalid time range")
		return
	}
	result, err := h.service.ListAudit(c.Request.Context(), principal, AuditFilter{ActorID: c.Query("actor_id"), Action: c.Query("action"), ResourceType: c.Query("resource_type"), From: from, To: to, PageQuery: page})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) getScopes(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "operations authentication is required")
		return
	}
	result, err := h.service.GetScopes(c.Request.Context(), principal)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) getDiagnostics(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "operations authentication is required")
		return
	}
	result, err := h.service.GetDiagnostics(c.Request.Context(), principal)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
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
	teamIDs, teamOK := parseScopeIDs(c.GetHeader("X-Actor-Team-IDs"))
	areaIDs, areaOK := parseScopeIDs(c.GetHeader("X-Actor-Area-IDs"))
	if !teamOK || !areaOK {
		return security.Principal{}, false
	}
	return security.Principal{Type: security.HumanPrincipal, PublicID: publicID, Roles: []string{role}, Scopes: security.AccessScope{Global: strings.EqualFold(c.GetHeader("X-Actor-Global"), "true"), TeamIDs: teamIDs, AreaIDs: areaIDs}}, true
}

func parseScopeIDs(value string) ([]uint64, bool) {
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

func pagination(c *gin.Context) (PageQuery, error) {
	page := PageQuery{Page: defaultPage, PageSize: defaultPageSize}
	var err error
	if value := c.Query("page"); value != "" {
		page.Page, err = strconv.Atoi(value)
		if err != nil {
			return PageQuery{}, err
		}
	}
	if value := c.Query("page_size"); value != "" {
		page.PageSize, err = strconv.Atoi(value)
		if err != nil {
			return PageQuery{}, err
		}
	}
	if page.Page < 1 || page.PageSize < 1 || page.PageSize > maxPageSize {
		return PageQuery{}, ErrInvalidInput
	}
	return page, nil
}

func timeRange(fromValue, toValue string) (time.Time, time.Time, error) {
	var from, to time.Time
	var err error
	if strings.TrimSpace(fromValue) != "" {
		from, err = time.Parse(time.RFC3339, fromValue)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if strings.TrimSpace(toValue) != "" {
		to, err = time.Parse(time.RFC3339, toValue)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		return time.Time{}, time.Time{}, ErrInvalidInput
	}
	return from, to, nil
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
		writeError(c, http.StatusForbidden, "forbidden", "principal is not allowed to read operations data")
	case errors.Is(err, ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_operations_query", "invalid operations query")
	case errors.Is(err, ErrRepositoryNotConfigured):
		writeError(c, http.StatusServiceUnavailable, "operations_unavailable", "operations service is unavailable")
	default:
		writeError(c, http.StatusServiceUnavailable, "operations_unavailable", "operations query failed")
	}
}
