package adminquery

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	platformhttpauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpauth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func RegisterRoutes(router gin.IRouter, service *Service, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	handler := Handler{service: service}
	personnelHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.ListPersonnel)
	assignmentHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.ListAssignments)
	exceptionHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.ListExceptions)
	reportHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.GetReportOverview)
	exceptionUpdateHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.UpdateException)
	router.GET("/api/v1/personnel", personnelHandlers...)
	router.GET("/api/v1/assignments", assignmentHandlers...)
	router.GET("/api/v1/exceptions", exceptionHandlers...)
	router.GET("/api/v1/reports/overview", reportHandlers...)
	router.PATCH("/api/v1/exceptions/:publicID", exceptionUpdateHandlers...)
}

type Handler struct{ service *Service }

func (h Handler) ListPersonnel(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		principal, ok = principalFromRequest(c)
	}
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "admin authentication is required")
		return
	}
	page, pageSize, err := pagination(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_management_query", "invalid pagination")
		return
	}
	result, err := h.service.ListPersonnel(c.Request.Context(), principal, PersonnelFilter{WorkState: c.Query("work_state"), TeamPublicID: c.Query("team_public_id"), AreaPublicID: c.Query("area_public_id"), Page: page, PageSize: pageSize})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) ListAssignments(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		principal, ok = principalFromRequest(c)
	}
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "admin authentication is required")
		return
	}
	page, pageSize, err := pagination(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_management_query", "invalid pagination")
		return
	}
	result, err := h.service.ListAssignments(c.Request.Context(), principal, AssignmentFilter{Status: c.Query("status"), TaskPublicID: c.Query("task_public_id"), PersonnelPublicID: c.Query("personnel_public_id"), Page: page, PageSize: pageSize})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) ListExceptions(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		principal, ok = principalFromRequest(c)
	}
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "admin authentication is required")
		return
	}
	page, pageSize, err := pagination(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_management_query", "invalid pagination")
		return
	}
	result, err := h.service.ListExceptions(c.Request.Context(), principal, ExceptionFilter{Status: c.Query("status"), Severity: c.Query("severity"), TaskPublicID: c.Query("task_public_id"), PersonnelPublicID: c.Query("personnel_public_id"), Page: page, PageSize: pageSize})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) GetReportOverview(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		principal, ok = principalFromRequest(c)
	}
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "admin authentication is required")
		return
	}
	result, err := h.service.GetReportOverview(c.Request.Context(), principal, ReportFilter{From: c.Query("from"), To: c.Query("to")})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) UpdateException(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		principal, ok = principalFromRequest(c)
	}
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "admin authentication is required")
		return
	}
	var request struct {
		Status         string `json:"status"`
		ResolutionNote string `json:"resolution_note"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_exception_update", "exception update body is invalid")
		return
	}
	if err := h.service.UpdateException(c.Request.Context(), principal, c.Param("publicID"), request.Status, request.ResolutionNote, ExceptionUpdate{RequestID: observability.RequestID(c), TraceID: observability.TraceID(c), SourceIP: c.ClientIP()}); err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"public_id": c.Param("publicID"), "status": request.Status})
}

func principalFromRequest(c *gin.Context) (security.Principal, bool) {
	if !platformhttpauth.DevelopmentActorHeadersAllowed(c) {
		return security.Principal{}, false
	}
	publicID := c.GetHeader("X-Actor-Public-ID")
	if publicID == "" {
		return security.Principal{}, false
	}
	role := strings.TrimSpace(c.GetHeader("X-Actor-Role"))
	if role == "" {
		role = strings.TrimSpace(strings.Split(c.GetHeader("X-Actor-Roles"), ",")[0])
	}
	teamIDs, teamOK := parseScopeIDs(c.GetHeader("X-Actor-Team-IDs"))
	areaIDs, areaOK := parseScopeIDs(c.GetHeader("X-Actor-Area-IDs"))
	if !teamOK || !areaOK {
		return security.Principal{}, false
	}
	return security.Principal{Type: security.HumanPrincipal, PublicID: publicID, Roles: []string{role}, Scopes: security.AccessScope{Global: strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Actor-Global")), "true"), TeamIDs: teamIDs, AreaIDs: areaIDs}}, true
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

func pagination(c *gin.Context) (int, int, error) {
	page, pageSize := defaultPage, defaultPageSize
	var err error
	if value := c.Query("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, err
		}
	}
	if value := c.Query("page_size"); value != "" {
		pageSize, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, err
		}
	}
	return page, pageSize, nil
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
		writeError(c, http.StatusForbidden, "forbidden", "principal is not allowed to read this management resource")
	case errors.Is(err, ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_management_query", "invalid management query")
	case errors.Is(err, ErrRepositoryNotConfigured):
		writeError(c, http.StatusServiceUnavailable, "management_query_unavailable", "management query service is unavailable")
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "exception_not_found", "exception was not found in the current scope")
	case errors.Is(err, ErrConflict):
		writeError(c, http.StatusConflict, "exception_state_conflict", "exception state cannot be changed from its current status")
	default:
		writeError(c, http.StatusServiceUnavailable, "management_query_unavailable", "management query service is unavailable")
	}
}
