// Package observability contains request/trace middleware shared by Core and Edge.
package observability

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	RequestIDKey = "request_id"
	TraceIDKey   = "trace_id"
)

func Middleware(log *zap.Logger, component string) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := headerOrGenerated(c.GetHeader("X-Request-ID"))
		traceID := headerOrGenerated(c.GetHeader("X-Trace-ID"))
		c.Set(RequestIDKey, requestID)
		c.Set(TraceIDKey, traceID)
		c.Header("X-Request-ID", requestID)
		c.Header("X-Trace-ID", traceID)
		started := time.Now()
		c.Next()
		log.Info("http request",
			zap.String("component", component), zap.String("request_id", requestID), zap.String("trace_id", traceID),
			zap.String("method", c.Request.Method), zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()), zap.Duration("duration", time.Since(started)),
		)
	}
}

func headerOrGenerated(value string) string {
	value = strings.TrimSpace(value)
	if value != "" && len(value) <= 128 {
		return value
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
}

func RequestID(c *gin.Context) string { return c.GetString(RequestIDKey) }
func TraceID(c *gin.Context) string   { return c.GetString(TraceIDKey) }

func HealthResponse(c *gin.Context, status string, details map[string]string, code int) {
	c.JSON(code, gin.H{"status": status, "details": details, "request_id": RequestID(c), "trace_id": TraceID(c)})
}

func InternalErrorResponse(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"code": "internal_error", "message": "internal server error", "request_id": RequestID(c), "trace_id": TraceID(c)})
}
