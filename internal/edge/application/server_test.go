package application

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	"go.uber.org/zap"
)

func TestHealthEndpointsWithoutMySQL(t *testing.T) {
	server := NewServer(config.ServiceConfig{Port: 18082, Mode: "test"}, nil, nil, zap.NewNop())
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	request, err := http.NewRequest(http.MethodGet, ts.URL+"/health/live", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Request-ID", "edge-test-request")
	response, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Request-ID") != "edge-test-request" || response.Header.Get("X-Trace-ID") == "" {
		t.Fatalf("unexpected liveness response: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
