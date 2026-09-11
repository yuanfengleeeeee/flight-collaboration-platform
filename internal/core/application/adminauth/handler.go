package adminauth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	platformhttpauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpauth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func RegisterRoutes(router gin.IRouter, service *Service, authenticator security.Authenticator, allowDevActorHeaders bool) {
	if router == nil || service == nil {
		return
	}
	handler := Handler{service: service}
	router.GET("/api/v1/admin/auth/sso/start", handler.Start)
	router.POST("/api/v1/admin/auth/sso/exchange", handler.Exchange)
	authentication := platformhttpauth.RequireJWT(authenticator, service, allowDevActorHeaders)
	router.GET("/api/v1/admin/auth/me", authentication, handler.Me)
	router.POST("/api/v1/admin/auth/logout", authentication, handler.Logout)
}

type Handler struct{ service *Service }

type exchangeRequest struct {
	State       string `json:"state" binding:"required"`
	Code        string `json:"code" binding:"required"`
	RedirectURI string `json:"redirect_uri" binding:"required"`
}

func (h Handler) Start(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	redirectURI := strings.TrimSpace(c.Query("redirect_uri"))
	result, err := h.service.Start(c.Request.Context(), state, redirectURI)
	if err != nil {
		writeError(c, statusForError(err), codeForError(err), publicMessage(err))
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: h.service.StateCookieName(), Value: state, Path: "/", HttpOnly: true, Secure: h.service.SecureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: h.service.StateMaxAge()})
	c.Redirect(http.StatusFound, result.AuthorizationURL)
}

func (h Handler) Exchange(c *gin.Context) {
	var request exchangeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_admin_sso_request", "invalid admin SSO exchange request")
		return
	}
	defer http.SetCookie(c.Writer, &http.Cookie{Name: h.service.StateCookieName(), Value: "", Path: "/", HttpOnly: true, Secure: h.service.SecureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	stateCookie, err := c.Request.Cookie(h.service.StateCookieName())
	if err != nil || stateCookie.Value == "" || stateCookie.Value != request.State {
		writeError(c, http.StatusUnauthorized, "invalid_admin_sso_state", "admin SSO state is invalid or expired")
		return
	}
	result, err := h.service.Exchange(c.Request.Context(), request.State, request.Code, request.RedirectURI)
	if err != nil {
		writeError(c, statusForError(err), codeForError(err), publicMessage(err))
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) Me(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "admin session is required")
		return
	}
	result, err := h.service.Me(c.Request.Context(), principal)
	if err != nil {
		writeError(c, statusForError(err), codeForError(err), publicMessage(err))
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) Logout(c *gin.Context) {
	principal, ok := platformhttpauth.Principal(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "admin session is required")
		return
	}
	if err := h.service.Logout(c.Request.Context(), principal); err != nil {
		writeError(c, statusForError(err), codeForError(err), publicMessage(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func writeData(c *gin.Context, status int, value any) {
	c.JSON(status, gin.H{"data": value, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, ErrSSOUnavailable), errors.Is(err, ErrProviderUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrInvalidState), errors.Is(err, ErrCodeInvalid), errors.Is(err, ErrSessionExpired), errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionRevoked):
		return http.StatusUnauthorized
	case errors.Is(err, ErrIdentityUnmapped), errors.Is(err, ErrAdminInactive):
		return http.StatusForbidden
	case errors.Is(err, ErrInvalidRequest):
		return http.StatusBadRequest
	default:
		return http.StatusServiceUnavailable
	}
}

func codeForError(err error) string {
	switch {
	case errors.Is(err, ErrSSOUnavailable), errors.Is(err, ErrProviderUnavailable):
		return "admin_sso_unavailable"
	case errors.Is(err, ErrInvalidState):
		return "invalid_admin_sso_state"
	case errors.Is(err, ErrCodeInvalid):
		return "invalid_admin_sso_code"
	case errors.Is(err, ErrIdentityUnmapped):
		return "admin_identity_unmapped"
	case errors.Is(err, ErrAdminInactive):
		return "admin_identity_inactive"
	case errors.Is(err, ErrSessionExpired), errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionRevoked):
		return "admin_session_expired"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_admin_sso_request"
	default:
		return "admin_authentication_failed"
	}
}

func publicMessage(err error) string {
	switch {
	case errors.Is(err, ErrSSOUnavailable), errors.Is(err, ErrProviderUnavailable):
		return "admin SSO provider is temporarily unavailable"
	case errors.Is(err, ErrInvalidState):
		return "admin SSO state is invalid or expired"
	case errors.Is(err, ErrCodeInvalid):
		return "admin SSO authorization code is invalid"
	case errors.Is(err, ErrIdentityUnmapped):
		return "admin SSO identity is not provisioned"
	case errors.Is(err, ErrAdminInactive):
		return "admin identity is inactive"
	case errors.Is(err, ErrSessionExpired), errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionRevoked):
		return "admin session is expired or revoked"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid admin SSO request"
	default:
		return "admin authentication failed"
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
