package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	coreauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/auth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
)

func TestHealthEndpointsWithoutDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Server: config.ServerConfig{Mode: gin.TestMode}}
	r := SetupRouter(cfg, nil, nil)

	tests := []struct {
		name       string
		path       string
		statusCode int
	}{
		{name: "liveness", path: "/health/live", statusCode: http.StatusOK},
		{name: "readiness", path: "/health/ready", statusCode: http.StatusServiceUnavailable},
		{name: "health compatibility", path: "/health", statusCode: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)

			if resp.Code != tt.statusCode {
				t.Fatalf("expected status %d, got %d", tt.statusCode, resp.Code)
			}
			if resp.Header().Get("X-Request-ID") == "" {
				t.Fatal("expected X-Request-ID response header")
			}
		})
	}
}

func TestAuthMeRequiresValidJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{Mode: gin.TestMode},
		JWT:    config.JWTConfig{Secret: "test-secret-value", ExpireHours: 1, Issuer: "flight-test"},
	}
	r := SetupRouter(cfg, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated status 401, got %d", resp.Code)
	}

	token, err := coreauth.GenerateToken(cfg.JWT, 7, "leader-one", coreauth.RoleLeader)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected authenticated status 200, got %d", resp.Code)
	}
	if got := resp.Header().Get("X-Request-ID"); got == "" {
		t.Fatal("expected request id on authenticated response")
	}
}
