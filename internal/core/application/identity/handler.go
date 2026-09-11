package identity

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	sharedIdentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/identity"
)

// RegisterInternalRoutes exposes only the Core-owned service contract used by
// Edge. The shared key is a local development boundary; production should
// deploy this route behind the existing private network and mTLS boundary.
func RegisterInternalRoutes(router gin.IRouter, service *Service, sharedKey string) {
	if router == nil || service == nil {
		return
	}
	authenticate := func(c *gin.Context) {
		if sharedKey != "" && !secureStringEqual(c.GetHeader("X-Internal-Identity-Key"), sharedKey) {
			writeError(c, http.StatusUnauthorized, "identity_service_unauthorized", "identity service authentication is required")
			c.Abort()
			return
		}
		c.Next()
	}
	router.POST("/internal/identity/v1/password/verify", authenticate, func(c *gin.Context) {
		var request sharedIdentity.PasswordVerifyRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_identity_request", "invalid password verification request")
			return
		}
		result, err := service.PasswordLogin(c.Request.Context(), PasswordLoginInput{EmployeeNo: request.EmployeeNo, Password: request.Password, Client: request.Client, Provider: request.Provider, ProviderApp: request.ProviderApp})
		if err != nil {
			writeIdentityError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	})
	router.POST("/internal/identity/v1/staff/status", authenticate, func(c *gin.Context) {
		var request sharedIdentity.StaffStatusRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_identity_request", "invalid staff status request")
			return
		}
		staff, err := service.FindStaff(c.Request.Context(), request.PublicID)
		if err != nil {
			writeIdentityError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": staff})
	})
	router.POST("/internal/identity/v1/providers/exchange", authenticate, func(c *gin.Context) {
		var request sharedIdentity.ExchangeRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_identity_request", "invalid identity exchange request")
			return
		}
		staff, err := service.Exchange(c.Request.Context(), request.Provider, request.ProviderCode, request.Client, request.RedirectURI)
		if err != nil {
			writeIdentityError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": staff})
	})
	router.POST("/internal/identity/v1/bindings/complete", authenticate, func(c *gin.Context) {
		var request sharedIdentity.BindingRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_identity_request", "invalid identity binding request")
			return
		}
		staff, err := service.CompleteBinding(c.Request.Context(), request.BindingTicket, request.Provider, request.ProviderCode, request.Client, request.RedirectURI)
		if err != nil {
			writeIdentityError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": staff})
	})
}

func writeIdentityError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	code := "identity_request_rejected"
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		status, code = http.StatusUnauthorized, "invalid_credentials"
	case errors.Is(err, ErrProviderCodeInvalid):
		status, code = http.StatusUnauthorized, "provider_code_invalid"
	case errors.Is(err, ErrProviderUnavailable):
		status, code = http.StatusServiceUnavailable, "identity_provider_unavailable"
	case errors.Is(err, ErrStaffInactive):
		status, code = http.StatusForbidden, "staff_inactive"
	case errors.Is(err, ErrIdentityUnmapped):
		status, code = http.StatusForbidden, "identity_unmapped"
	case errors.Is(err, ErrIdentityBindingConflict):
		status, code = http.StatusConflict, "identity_binding_conflict"
	case errors.Is(err, ErrBindingTicketInvalid):
		status, code = http.StatusUnauthorized, "binding_ticket_invalid"
	case errors.Is(err, ErrUnsupportedProvider):
		status, code = http.StatusBadRequest, "unsupported_identity_provider"
	}
	writeError(c, status, code, safeIdentityMessage(code))
}

func safeIdentityMessage(code string) string {
	switch code {
	case "invalid_credentials":
		return "employee credentials are invalid"
	case "provider_code_invalid":
		return "identity provider code is invalid"
	case "staff_inactive":
		return "staff account is inactive"
	case "identity_unmapped":
		return "identity is not bound to an employee"
	case "identity_binding_conflict":
		return "identity is already bound to another employee"
	case "binding_ticket_invalid":
		return "binding ticket is invalid or expired"
	case "identity_provider_unavailable":
		return "identity provider is temporarily unavailable"
	default:
		return "identity request was rejected"
	}
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}

func secureStringEqual(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
