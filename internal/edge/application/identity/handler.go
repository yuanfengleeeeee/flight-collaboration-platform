package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	sharedIdentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/identity"
	platformhttpauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpauth"
	platformobservability "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
)

func RegisterRoutes(router gin.IRouter, service *Service, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	handler := Handler{service: service}
	router.POST("/api/v1/auth/password/login", handler.PasswordLogin)
	router.POST("/api/v1/auth/exchange", handler.Exchange)
	router.POST("/api/v1/auth/bindings/complete", handler.CompleteBinding)
	router.POST("/api/v1/auth/refresh", handler.Refresh)
	meHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.Me)
	logoutHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.Logout)
	router.GET("/api/v1/auth/me", meHandlers...)
	router.POST("/api/v1/auth/logout", logoutHandlers...)
}

type Handler struct {
	service *Service
}

type passwordLoginRequest struct {
	EmployeeNo  string `json:"employee_no" binding:"required"`
	Password    string `json:"password" binding:"required"`
	Client      string `json:"client" binding:"required"`
	Provider    string `json:"provider,omitempty"`
	ProviderApp string `json:"provider_app,omitempty"`
}

type exchangeRequest struct {
	Provider     string `json:"provider" binding:"required"`
	ProviderCode string `json:"provider_code" binding:"required"`
	Client       string `json:"client" binding:"required"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
}

type bindingRequest struct {
	BindingTicket string `json:"binding_ticket" binding:"required"`
	Provider      string `json:"provider" binding:"required"`
	ProviderCode  string `json:"provider_code" binding:"required"`
	Client        string `json:"client" binding:"required"`
	RedirectURI   string `json:"redirect_uri,omitempty"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h Handler) PasswordLogin(c *gin.Context) {
	var request passwordLoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_identity_request", "invalid password login request")
		return
	}
	result, err := h.service.PasswordLogin(c.Request.Context(), sharedIdentity.PasswordVerifyRequest{EmployeeNo: request.EmployeeNo, Password: request.Password, Client: request.Client, Provider: request.Provider, ProviderApp: request.ProviderApp})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) Exchange(c *gin.Context) {
	var request exchangeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_identity_request", "invalid identity exchange request")
		return
	}
	result, err := h.service.Exchange(c.Request.Context(), sharedIdentity.ExchangeRequest{Provider: request.Provider, ProviderCode: request.ProviderCode, Client: request.Client, RedirectURI: request.RedirectURI})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) CompleteBinding(c *gin.Context) {
	var request bindingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_identity_request", "invalid identity binding request")
		return
	}
	result, err := h.service.CompleteBinding(c.Request.Context(), sharedIdentity.BindingRequest{BindingTicket: request.BindingTicket, Provider: request.Provider, ProviderCode: request.ProviderCode, Client: request.Client, RedirectURI: request.RedirectURI}, request.Client)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) Refresh(c *gin.Context) {
	var request refreshRequest
	if err := c.ShouldBindJSON(&request); err != nil && c.GetHeader("Authorization") == "" {
		writeError(c, http.StatusBadRequest, "invalid_refresh_request", "invalid refresh request")
		return
	}
	refreshToken := strings.TrimSpace(request.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	}
	if refreshToken == "" {
		writeError(c, http.StatusBadRequest, "invalid_refresh_request", "invalid refresh request")
		return
	}
	result, err := h.service.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) Me(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "session_expired", "employee session is required")
		return
	}
	result, err := h.service.Me(c.Request.Context(), principal)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) Logout(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "session_expired", "employee session is required")
		return
	}
	if err := h.service.Logout(c.Request.Context(), principal); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeData(c *gin.Context, status int, value any) {
	c.JSON(status, gin.H{"data": value, "request_id": platformobservability.RequestID(c), "trace_id": platformobservability.TraceID(c)})
}

func writeServiceError(c *gin.Context, err error) {
	status, code, message := http.StatusServiceUnavailable, "authentication_unavailable", "authentication service is unavailable"
	var remoteErr sharedIdentity.RemoteError
	switch {
	case errors.Is(err, ErrStaffInactive):
		status, code, message = http.StatusForbidden, "staff_inactive", "staff account is inactive"
	case errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionExpired), errors.Is(err, ErrSessionRevoked), errors.Is(err, ErrRefreshReuse):
		status, code, message = http.StatusUnauthorized, "session_expired", "employee session is expired or revoked"
	case errors.As(err, &remoteErr):
		status, code = remoteErr.Status, remoteErr.Code
		message = publicMessage(code)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code, message = http.StatusServiceUnavailable, "authentication_unavailable", "authentication service is unavailable"
	}
	if status < http.StatusBadRequest || status > 599 {
		status = http.StatusServiceUnavailable
	}
	writeError(c, status, code, message)
}

func publicMessage(code string) string {
	switch code {
	case "invalid_credentials":
		return "employee credentials are invalid"
	case "identity_unmapped":
		return "identity is not bound to an employee"
	case "identity_binding_conflict":
		return "identity is already bound to another employee"
	case "staff_inactive":
		return "staff account is inactive"
	case "binding_ticket_invalid":
		return "binding ticket is invalid or expired"
	case "identity_service_unauthorized":
		return "authentication service is unavailable"
	case "identity_provider_unavailable":
		return "identity provider is temporarily unavailable"
	default:
		return "identity request was rejected"
	}
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "request_id": platformobservability.RequestID(c), "trace_id": platformobservability.TraceID(c)})
}
