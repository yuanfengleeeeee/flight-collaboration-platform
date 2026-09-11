package flighttask

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
)

func RegisterCancellationRoutes(router gin.IRouter, service *CancellationService, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	handlers := append(append([]gin.HandlerFunc{}, middleware...), CancellationHandler{service: service}.Cancel)
	router.POST("/api/v1/tasks/:taskPublicID/cancel", handlers...)
}

type CancellationHandler struct {
	service *CancellationService
}

type cancellationRequest struct {
	CancellationID      string  `json:"cancellation_id"`
	ExpectedTaskVersion *uint64 `json:"expected_task_version"`
	Reason              string  `json:"reason"`
}

func (h CancellationHandler) Cancel(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, ErrRepositoryNotConfigured)
		return
	}
	var request cancellationRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.ExpectedTaskVersion == nil {
		writeError(c, http.StatusBadRequest, ErrCancellationInvalidInput)
		return
	}
	principal, err := principalFromRequest(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	result, err := h.service.CancelTask(c.Request.Context(), CancellationInput{
		Principal: principal, TaskPublicID: c.Param("taskPublicID"), CancellationID: request.CancellationID,
		ExpectedTaskVersion: *request.ExpectedTaskVersion, Reason: request.Reason,
		RequestID: observability.RequestID(c), TraceID: observability.TraceID(c), SourceIP: c.ClientIP(),
	})
	if err != nil {
		writeError(c, statusForError(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}
