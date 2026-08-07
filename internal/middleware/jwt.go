package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	coreauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/auth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
)

// Claims 保留 middleware 包的兼容类型别名。
type Claims = coreauth.Claims

// GenerateToken 保留旧调用入口，实际实现位于 internal/auth。
func GenerateToken(cfg config.JWTConfig, userID int64, username, role string) (string, error) {
	return coreauth.GenerateToken(cfg, userID, username, role)
}

// ParseToken 保留旧调用入口，实际实现位于 internal/auth。
func ParseToken(cfg config.JWTConfig, tokenString string) (*Claims, error) {
	return coreauth.ParseToken(cfg, tokenString)
}

// JWTAuth JWT 鉴权中间件。
func JWTAuth(cfg config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			common.FailWithHTTP(c, 401, common.CodeUnauthorized, "未提供认证信息")
			c.Abort()
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			common.FailWithHTTP(c, 401, common.CodeUnauthorized, "认证格式错误")
			c.Abort()
			return
		}

		claims, err := coreauth.ParseToken(cfg, strings.TrimSpace(parts[1]))
		if err != nil {
			common.FailWithHTTP(c, 401, common.CodeUnauthorized, "认证失效或过期")
			c.Abort()
			return
		}

		c.Set("claims", claims)
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// CurrentClaims 从请求上下文读取已验证的身份。
func CurrentClaims(c *gin.Context) (*Claims, bool) {
	value, exists := c.Get("claims")
	if !exists {
		return nil, false
	}
	claims, ok := value.(*Claims)
	return claims, ok && claims != nil
}

// RequireRole 角色校验中间件。
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		userRole, ok := role.(string)
		if !exists || !ok {
			common.FailWithHTTP(c, 403, common.CodeForbidden, "无权限")
			c.Abort()
			return
		}
		for _, allowed := range roles {
			if userRole == allowed {
				c.Next()
				return
			}
		}
		common.FailWithHTTP(c, 403, common.CodeForbidden, "无权限")
		c.Abort()
	}
}

// RequirePermission 按角色权限表校验请求；资源数据范围由后续业务模块实现。
func RequirePermission(permission coreauth.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		userRole, ok := role.(string)
		if !exists || !ok || !coreauth.RoleAllows(userRole, permission) {
			common.FailWithHTTP(c, 403, common.CodeForbidden, "无权限")
			c.Abort()
			return
		}
		c.Next()
	}
}
