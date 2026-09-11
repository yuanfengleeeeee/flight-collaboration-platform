package flightsync

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	flightintegration "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/flight"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
)

func RegisterRoutes(router gin.IRouter, service *Service, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	handler := Handler{service: service}
	chain := append(append([]gin.HandlerFunc{}, middleware...), handler.Ingest)
	router.POST("/internal/integration/v1/flight-source/sync", chain...)
	router.GET("/internal/integration/v1/flight-source/health", append(append([]gin.HandlerFunc{}, middleware...), handler.Health)...)
	router.GET("/internal/integration/v1/flight-source/reconciliation", append(append([]gin.HandlerFunc{}, middleware...), handler.Reconciliation)...)
}

func (h Handler) Health(c *gin.Context) {
	value, err := h.service.SourceHealth(c.Request.Context(), c.Query("provider"))
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			writeError(c, http.StatusBadRequest, "invalid_flight_source_provider", "provider is invalid")
			return
		}
		writeError(c, http.StatusServiceUnavailable, "flight_source_health_unavailable", "flight source health is unavailable")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": value, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func (h Handler) Reconciliation(c *gin.Context) {
	value, err := h.service.LatestReconciliation(c.Request.Context(), c.Query("provider"))
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			writeError(c, http.StatusBadRequest, "invalid_flight_source_provider", "provider is invalid")
			return
		}
		if errors.Is(err, ErrRepositoryNotConfigured) {
			writeError(c, http.StatusServiceUnavailable, "flight_source_reconciliation_unavailable", "flight source reconciliation is unavailable")
			return
		}
		writeError(c, http.StatusInternalServerError, "flight_source_reconciliation_failed", "flight source reconciliation could not be read")
		return
	}
	if value.CheckedAt.IsZero() {
		writeError(c, http.StatusNotFound, "flight_source_reconciliation_not_found", "no flight source reconciliation has been recorded")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": value, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

type Handler struct{ service *Service }

type syncScheduleRequest struct {
	ExternalFlightID string    `json:"external_flight_id"`
	FlightDisplayNo  string    `json:"flight_display_no"`
	OperatingDate    time.Time `json:"operating_date"`
	ScheduledAt      time.Time `json:"scheduled_at"`
}

type syncEventRequest struct {
	ExternalEventID   string     `json:"external_event_id"`
	ExternalFlightID  string     `json:"external_flight_id"`
	Status            string     `json:"status"`
	OccurredAt        time.Time  `json:"occurred_at"`
	ActualArrivalAt   *time.Time `json:"actual_arrival_at"`
	ActualDepartureAt *time.Time `json:"actual_departure_at"`
	ScheduledAt       *time.Time `json:"scheduled_at"`
	Reason            string     `json:"reason"`
}

type syncRequest struct {
	Provider  string                `json:"provider"`
	Schedules []syncScheduleRequest `json:"schedules"`
	Events    []syncEventRequest    `json:"events"`
}

func (h Handler) Ingest(c *gin.Context) {
	var request syncRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_flight_source_input", "flight source payload is invalid")
		return
	}
	input := IngestInput{Provider: request.Provider, Schedules: make([]flightintegration.Schedule, 0, len(request.Schedules)), Events: make([]flightintegration.Event, 0, len(request.Events))}
	for _, schedule := range request.Schedules {
		input.Schedules = append(input.Schedules, flightintegration.Schedule{ExternalFlightID: schedule.ExternalFlightID, FlightDisplayNo: schedule.FlightDisplayNo, OperatingDate: schedule.OperatingDate, ScheduledAt: schedule.ScheduledAt})
	}
	for _, sourceEvent := range request.Events {
		input.Events = append(input.Events, flightintegration.Event{ExternalEventID: sourceEvent.ExternalEventID, ExternalFlightID: sourceEvent.ExternalFlightID, Status: sourceEvent.Status, OccurredAt: sourceEvent.OccurredAt, ActualArrivalAt: sourceEvent.ActualArrivalAt, ActualDepartureAt: sourceEvent.ActualDepartureAt, ScheduledAt: sourceEvent.ScheduledAt, Reason: sourceEvent.Reason})
	}
	result, err := h.service.Ingest(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrUnsupportedEvent) {
			writeError(c, http.StatusBadRequest, "invalid_flight_source_input", "flight source payload is invalid or unsupported")
			return
		}
		// Do not return database or queue internals to an upstream provider. The
		// durable inbox and diagnostics endpoint retain the detailed error.
		writeError(c, http.StatusServiceUnavailable, "flight_source_sync_unavailable", "flight source staging is temporarily unavailable")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"status": "staged", "accepted": result.Accepted, "updated": result.Updated, "duplicate": result.Duplicate}, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}
