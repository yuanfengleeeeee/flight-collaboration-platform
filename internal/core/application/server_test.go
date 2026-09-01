package application

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	"go.uber.org/zap"
)

func TestHealthEndpointsWithoutMySQL(t *testing.T) {
	server := NewServer(config.ServiceConfig{Port: 18081, Mode: "test"}, nil, nil, zap.NewNop())
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	live, err := ts.Client().Get(ts.URL + "/health/live")
	if err != nil {
		t.Fatal(err)
	}
	if live.StatusCode != http.StatusOK || live.Header.Get("X-Request-ID") == "" || live.Header.Get("X-Trace-ID") == "" {
		t.Fatalf("unexpected liveness response: %d", live.StatusCode)
	}
	_ = live.Body.Close()

	ready, err := ts.Client().Get(ts.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	if ready.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", ready.StatusCode, http.StatusServiceUnavailable)
	}
	_ = ready.Body.Close()
}
