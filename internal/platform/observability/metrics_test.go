package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRegistryRendersCounterGaugeAndHistogram(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterCounter("test_requests_total", "Test requests.")
	registry.RegisterGauge("test_queue_depth", "Test queue depth.")
	registry.RegisterHistogram("test_duration_seconds", "Test duration.", []float64{0.1, 1})
	labels := Labels{"component": "edge", "route": "/api/v1/tasks"}
	registry.Inc("test_requests_total", labels)
	registry.SetGauge("test_queue_depth", Labels{"component": "edge"}, 3)
	registry.Observe("test_duration_seconds", labels, 0.2)

	output := registry.Render()
	for _, expected := range []string{
		`test_requests_total{component="edge",route="/api/v1/tasks"} 1`,
		`test_queue_depth{component="edge"} 3`,
		`test_duration_seconds_bucket{component="edge",le="1",route="/api/v1/tasks"} 1`,
		`test_duration_seconds_count{component="edge",route="/api/v1/tasks"} 1`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestMiddlewareUsesStableRouteLabel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := NewHTTPRegistry()
	router := gin.New()
	router.Use(MiddlewareWithMetrics(zap.NewNop(), "edge-api", registry))
	router.GET("/api/v1/tasks/:taskPublicID", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
	output := registry.Render()
	if !strings.Contains(output, `route="/api/v1/tasks/:taskPublicID"`) {
		t.Fatalf("expected route template in metrics, got:\n%s", output)
	}
	if strings.Contains(output, `route="/api/v1/tasks/task-1"`) {
		t.Fatalf("raw route parameter leaked into metrics, got:\n%s", output)
	}
}
