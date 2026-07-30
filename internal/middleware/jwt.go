package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
)

// Claims JWT 载荷
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT
func GenerateToken(cfg config.JWTConfig, userID int64, username, role string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(cfg.ExpireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.Secret))
}

// ParseToken 解析 JWT
func ParseToken(cfg config.JWTConfig, tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(cfg.Secret), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// JWTAuth JWT 鉴权中间件
func JWTAuth(cfg config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			common.FailWithHTTP(c, 401, common.CodeUnauthorized, "未提供认证信息")
			c.Abort()
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			common.FailWithHTTP(c, 401, common.CodeUnauthorized, "认证格式错误")
			c.Abort()
			return
		}

		claims, err := ParseToken(cfg, parts[1])
		if err != nil {
			common.FailWithHTTP(c, 401, common.CodeUnauthorized, "认证失效或过期")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// RequireRole 角色校验中间件
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			common.FailWithHTTP(c, 403, common.CodeForbidden, "无权限")
			c.Abort()
			return
		}
		userRole := role.(string)
		for _, r := range roles {
			if userRole == r {
				c.Next()
				return
			}
		}
		common.FailWithHTTP(c, 403, common.CodeForbidden, "无权限")
		c.Abort()
	}
}
