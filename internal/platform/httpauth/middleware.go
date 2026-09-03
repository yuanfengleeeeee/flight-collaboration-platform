// Package httpauth adapts the platform security ports to Gin HTTP handlers.
// It is intentionally kept outside the security package so JWT/RBAC domain
// code does not depend on a web framework.
package httpauth

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

const principalContextKey = "principal"

type PrincipalResolver interface {
	ResolvePrincipal(context.Context, security.Principal) (security.Principal, error)
}

type PrincipalResolverFunc func(context.Context, security.Principal) (security.Principal, error)

func (f PrincipalResolverFunc) ResolvePrincipal(ctx context.Context, principal security.Principal) (security.Principal, error) {
	return f(ctx, principal)
}

// RequireJWT authenticates a browser/mobile request and injects the resolved
// principal into Gin context. Development actor headers are an explicit,
// opt-in compatibility adapter and are never accepted in release mode.
func RequireJWT(authenticator security.Authenticator, resolver PrincipalResolver, allowDevActorHeaders bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if allowDevActorHeaders && hasDevelopmentActorHeaders(c) && c.GetHeader("Authorization") == "" {
			c.Set("allow_dev_actor_headers", true)
			c.Next()
			return
		}
		if authenticator == nil {
			writeError(c, http.StatusServiceUnavailable, "authentication_unavailable", "authentication is not configured")
			c.Abort()
			return
		}
		token := c.GetHeader("Authorization")
		if token == "" {
			writeError(c, http.StatusUnauthorized, "unauthenticated", "authentication is required")
			c.Abort()
			return
		}
		principal, err := authenticator.AuthenticateToken(token)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthenticated", "authentication is required")
			c.Abort()
			return
		}
		if resolver != nil {
			principal, err = resolver.ResolvePrincipal(c.Request.Context(), principal)
			if err != nil {
				writeError(c, http.StatusUnauthorized, "unauthenticated", "authentication is required")
				c.Abort()
				return
			}
		}
		c.Set(principalContextKey, principal)
		c.Set("allow_dev_actor_headers", false)
		c.Next()
	}
}

func Principal(c *gin.Context) (security.Principal, bool) {
	if c == nil {
		return security.Principal{}, false
	}
	value, ok := c.Get(principalContextKey)
	if !ok {
		return security.Principal{}, false
	}
	switch principal := value.(type) {
	case security.Principal:
		return principal, true
	case *security.Principal:
		if principal != nil {
			return *principal, true
		}
	}
	return security.Principal{}, false
}

// SetPrincipal is used by protocol adapters that authenticate before handing
// control to an upgraded connection, such as the WebSocket ticket path.
func SetPrincipal(c *gin.Context, principal security.Principal) {
	if c == nil {
		return
	}
	c.Set(principalContextKey, principal)
}

func DevelopmentActorHeadersAllowed(c *gin.Context) bool {
	if c == nil {
		return false
	}
	allowed, ok := c.Get("allow_dev_actor_headers")
	return ok && allowed == true
}

func hasDevelopmentActorHeaders(c *gin.Context) bool {
	return c.GetHeader("X-Actor-Public-ID") != "" || c.GetHeader("X-Employee-Public-ID") != ""
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}
