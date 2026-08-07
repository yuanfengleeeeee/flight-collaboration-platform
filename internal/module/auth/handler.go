package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	coreauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/auth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var errInvalidCredentials = errors.New("invalid credentials")

// Handler 提供基础登录和当前用户接口，不包含业务资源授权。
type Handler struct {
	db  *gorm.DB
	jwt config.JWTConfig
}

// NewHandler 创建认证处理器。
func NewHandler(db *gorm.DB, jwtConfig config.JWTConfig) *Handler {
	return &Handler{db: db, jwt: jwtConfig}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 使用数据库中的 active 用户签发访问令牌。
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Username) == "" || req.Password == "" {
		common.FailWithHTTP(c, http.StatusBadRequest, common.CodeInvalidParam, "用户名或密码格式错误")
		return
	}
	if h.db == nil {
		common.FailWithHTTP(c, http.StatusServiceUnavailable, common.CodeServiceUnavailable, "认证服务暂不可用")
		return
	}

	var user model.User
	result := h.db.WithContext(c.Request.Context()).Where("username = ? AND status = ?", req.Username, "active").First(&user)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) || result.Error != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		common.FailWithHTTP(c, http.StatusUnauthorized, common.CodeUnauthorized, "用户名或密码错误")
		return
	}

	token, err := coreauth.GenerateToken(h.jwt, user.ID, user.Username, user.Role)
	if err != nil {
		common.FailWithHTTP(c, http.StatusInternalServerError, common.CodeInternalError, "无法创建登录会话")
		return
	}
	common.OK(c, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   h.jwt.ExpireHours * 3600,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"name":     user.Name,
			"role":     user.Role,
		},
	})
}

// Me 返回当前已通过 JWT 校验的身份信息。
func (h *Handler) Me(c *gin.Context) {
	claims, ok := c.Get("claims")
	if !ok {
		common.FailWithHTTP(c, http.StatusUnauthorized, common.CodeUnauthorized, "未提供认证信息")
		return
	}
	identity, ok := claims.(*coreauth.Claims)
	if !ok || identity == nil {
		common.FailWithHTTP(c, http.StatusUnauthorized, common.CodeUnauthorized, "认证信息无效")
		return
	}
	common.OK(c, gin.H{
		"user_id":  identity.UserID,
		"username": identity.Username,
		"role":     identity.Role,
	})
}
