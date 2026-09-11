package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	coreauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/auth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
)

func TestJWTAuthAndRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.JWTConfig{Secret: "test-secret-value", ExpireHours: 1, Issuer: "flight-test"}
	r := gin.New()
	r.Use(JWTAuth(cfg))
	r.GET("/leader", RequireRole(coreauth.RoleLeader), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	leaderToken, err := coreauth.GenerateToken(cfg, 1, "leader", coreauth.RoleLeader)
	if err != nil {
		t.Fatal(err)
	}
	staffToken, err := coreauth.GenerateToken(cfg, 2, "staff", coreauth.RoleStaff)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		token      string
		statusCode int
	}{
		{name: "leader allowed", token: leaderToken, statusCode: http.StatusOK},
		{name: "staff forbidden", token: staffToken, statusCode: http.StatusForbidden},
		{name: "missing token", token: "", statusCode: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/leader", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)
			if resp.Code != tt.statusCode {
				t.Fatalf("expected status %d, got %d", tt.statusCode, resp.Code)
			}
		})
	}
}
