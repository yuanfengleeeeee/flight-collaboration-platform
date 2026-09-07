package flighttask

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

// RegisterConfirmationRoutes exposes the Core human command boundary. A real
// deployment should populate security.Principal in auth middleware; the
// X-Actor-* fallback keeps this v2 internal API testable until JWT middleware
// is wired into the Core entrypoint.
func RegisterConfirmationRoutes(router gin.IRouter, service *ConfirmationService, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	handlers := append(append([]gin.HandlerFunc{}, middleware...), ConfirmationHandler{service: service}.Confirm)
	router.POST("/api/v1/tasks/:taskPublicID/confirm", handlers...)
}

type ConfirmationHandler struct {
	service *ConfirmationService
}

type confirmationRequest struct {
	CandidatePublicID   string  `json:"candidate_public_id"`
	ConfirmationID      string  `json:"confirmation_id"`
	ExpectedTaskVersion *uint64 `json:"expected_task_version"`
}

func (h ConfirmationHandler) Confirm(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, ErrRepositoryNotConfigured)
		return
	}
	var request confirmationRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.ExpectedTaskVersion == nil {
		writeError(c, http.StatusBadRequest, ErrInvalidInput)
		return
	}
	principal, err := principalFromRequest(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	result, err := h.service.ConfirmTask(c.Request.Context(), ConfirmationInput{
		Principal: principal, TaskPublicID: c.Param("taskPublicID"), CandidatePublicID: request.CandidatePublicID,
		ConfirmationID: request.ConfirmationID, ExpectedTaskVersion: *request.ExpectedTaskVersion,
		RequestID: observability.RequestID(c), TraceID: observability.TraceID(c), SourceIP: c.ClientIP(),
	})
	if err != nil {
		writeError(c, statusForError(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func principalFromRequest(c *gin.Context) (security.Principal, error) {
	if value, ok := c.Get("principal"); ok {
		switch principal := value.(type) {
		case security.Principal:
			return principal, nil
		case *security.Principal:
			if principal != nil {
				return *principal, nil
			}
		}
	}
	roles := splitHeader(c.GetHeader("X-Actor-Roles"))
	if len(roles) == 0 {
		roles = splitHeader(c.GetHeader("X-Actor-Role"))
	}
	teams, err := parseUint64Header(c.GetHeader("X-Actor-Team-IDs"))
	if err != nil {
		return security.Principal{}, err
	}
	areas, err := parseUint64Header(c.GetHeader("X-Actor-Area-IDs"))
	if err != nil {
		return security.Principal{}, err
	}
	userID, err := parseUint64Value(c.GetHeader("X-Actor-User-ID"))
	if err != nil {
		return security.Principal{}, err
	}
	global, err := parseBoolHeader(c.GetHeader("X-Actor-Global"))
	if err != nil {
		return security.Principal{}, err
	}
	return security.Principal{Type: security.PrincipalType(strings.TrimSpace(c.GetHeader("X-Actor-Type"))), PublicID: strings.TrimSpace(c.GetHeader("X-Actor-Public-ID")), Roles: roles, Scopes: security.AccessScope{Global: global, TeamIDs: teams, AreaIDs: areas, UserID: userID}}, nil
}

func splitHeader(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func parseUint64Header(value string) ([]uint64, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := splitHeader(value)
	result := make([]uint64, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil || parsed == 0 {
			return nil, ErrInvalidInput
		}
		result = append(result, parsed)
	}
	return result, nil
}

func parseUint64Value(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return 0, ErrInvalidInput
	}
	return parsed, nil
}

func parseBoolHeader(value string) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, ErrInvalidInput
	}
	return parsed, nil
}
