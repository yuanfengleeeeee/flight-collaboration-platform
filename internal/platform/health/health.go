// Package health contains non-sensitive liveness and readiness handlers.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
)

type Checker func(context.Context) error

type Endpoint struct {
	started  time.Time
	required map[string]Checker
	optional map[string]Checker
}

func New(started time.Time, required map[string]Checker, optional map[string]Checker) *Endpoint {
	return &Endpoint{started: started, required: required, optional: optional}
}

func (e *Endpoint) Live() gin.HandlerFunc {
	return func(c *gin.Context) {
		observability.HealthResponse(c, "ok", map[string]string{"process": "ok"}, http.StatusOK)
	}
}

func (e *Endpoint) Ready() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		status := "ready"
		code := http.StatusOK
		details := make(map[string]string, len(e.required)+len(e.optional))
		for name, checker := range e.required {
			if checker == nil {
				details[name] = "unavailable"
				status, code = "not_ready", http.StatusServiceUnavailable
				continue
			}
			if err := checker(ctx); err != nil {
				details[name] = "unavailable"
				status, code = "not_ready", http.StatusServiceUnavailable
			} else {
				details[name] = "ok"
			}
		}
		for name, checker := range e.optional {
			if checker == nil {
				details[name] = "disabled"
				continue
			}
			if err := checker(ctx); err != nil {
				details[name] = "unavailable"
			} else {
				details[name] = "ok"
			}
		}
		observability.HealthResponse(c, status, details, code)
	}
}
