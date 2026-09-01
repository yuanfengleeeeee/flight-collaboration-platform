package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDisabledRedisDoesNotBlockReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoint := New(time.Now().UTC(), map[string]Checker{"mysql": func(context.Context) error { return nil }}, map[string]Checker{"redis": nil})
	router := gin.New()
	router.GET("/health/ready", endpoint.Ready())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("readiness status = %d", recorder.Code)
	}
	var body struct {
		Details map[string]string `json:"details"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Details["redis"] != "disabled" {
		t.Fatalf("redis detail = %q", body.Details["redis"])
	}
}
