package flighttask

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
)

// RegisterRoutes exposes the internal Core arrival input. Authentication for
// external flight systems remains an integration concern; this handler only
// translates HTTP metadata into the application input and never touches GORM.
func RegisterRoutes(router gin.IRouter, service *Service) {
	if router == nil || service == nil {
		return
	}
	handler := Handler{service: service}
	router.POST("/api/v1/flights/:flightPublicID/arrival", handler.RecordArrival)
}

type Handler struct {
	service *Service
}

type arrivalRequest struct {
	SourceEventID   string    `json:"source_event_id"`
	OccurredAt      time.Time `json:"occurred_at"`
	ActualArrivalAt time.Time `json:"actual_arrival_at"`
	Source          string    `json:"source"`
}

func (h Handler) RecordArrival(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, ErrRepositoryNotConfigured)
		return
	}
	var request arrivalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, ErrInvalidInput)
		return
	}
	result, err := h.service.RecordFlightArrived(c.Request.Context(), ArrivalInput{
		FlightPublicID:  c.Param("flightPublicID"),
		SourceEventID:   request.SourceEventID,
		OccurredAt:      request.OccurredAt,
		ActualArrivalAt: request.ActualArrivalAt,
		Source:          request.Source,
		ActorType:       "machine",
		ActorPublicID:   c.GetHeader("X-Actor-Public-ID"),
		RequestID:       observability.RequestID(c),
		TraceID:         observability.TraceID(c),
		SourceIP:        c.ClientIP(),
	})
	if err != nil {
		writeError(c, statusForError(err), err)
		return
	}
	status := http.StatusCreated
	if result.Duplicate {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"data": result, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func statusForError(err error) int {
	switch CodeOf(err) {
	case ErrFlightNotFound.Code:
		return http.StatusNotFound
	case ErrSourceEventConflict.Code, ErrFlightStatusConflict.Code:
		return http.StatusConflict
	case ErrConfirmationForbidden.Code:
		return http.StatusForbidden
	case ErrCancellationForbidden.Code:
		return http.StatusForbidden
	case ErrConfirmationTaskNotFound.Code, ErrConfirmationCandidateNotFound.Code:
		return http.StatusNotFound
	case ErrCancellationTaskNotFound.Code:
		return http.StatusNotFound
	case ErrTaskQueryNotFound.Code:
		return http.StatusNotFound
	case ErrTaskQueryForbidden.Code:
		return http.StatusForbidden
	case ErrTaskAlreadyAssigned.Code, ErrCandidateNoLongerEligible.Code, ErrTaskVersionConflict.Code, ErrConfirmationInvalidState.Code, ErrConfirmationIDConflict.Code, ErrCancellationVersionConflict.Code, ErrCancellationInvalidState.Code, ErrCancellationIDConflict.Code:
		return http.StatusConflict
	case ErrInvalidInput.Code, ErrTemplateInvalid.Code, ErrCancellationInvalidInput.Code:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"code": CodeOf(err), "message": MessageOf(err), "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}
