package event

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
)

// FlightArrivalRecorder is the small service boundary used by the HTTP handler.
// It keeps handler tests independent of GORM and MySQL.
type FlightArrivalRecorder interface {
	RecordFlightArrived(context.Context, RecordInput) (RecordResult, error)
}

// FlightArrivalHandler handles manual flight-arrival event recording requests.
type FlightArrivalHandler struct {
	recorder FlightArrivalRecorder
}

// NewFlightArrivalHandler constructs a handler over the flight-arrival service.
func NewFlightArrivalHandler(recorder FlightArrivalRecorder) *FlightArrivalHandler {
	return &FlightArrivalHandler{recorder: recorder}
}

type recordFlightArrivedRequest struct {
	FlightID       int64     `json:"flight_id"`
	TeamID         int64     `json:"team_id"`
	TemplateID     int64     `json:"template_id"`
	SourceEventID  string    `json:"source_event_id"`
	OccurrenceTime time.Time `json:"occurrence_time"`
}

// RecordFlightArrived records one flight-arrival event and its initial task.
func (h *FlightArrivalHandler) RecordFlightArrived(c *gin.Context) {
	var request recordFlightArrivedRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validRecordFlightArrivedRequest(request) {
		common.FailWithHTTP(c, http.StatusBadRequest, common.CodeInvalidParam, "请求参数无效")
		return
	}
	if h == nil || h.recorder == nil {
		common.FailWithHTTP(c, http.StatusServiceUnavailable, common.CodeServiceUnavailable, "事件服务暂不可用")
		return
	}

	result, err := h.recorder.RecordFlightArrived(c.Request.Context(), RecordInput{
		FlightID:       request.FlightID,
		TeamID:         request.TeamID,
		TemplateID:     request.TemplateID,
		SourceEventID:  request.SourceEventID,
		OccurrenceTime: request.OccurrenceTime,
	})
	if err != nil {
		respondRecordFlightArrivedError(c, err)
		return
	}

	status := http.StatusCreated
	if result.Duplicate {
		status = http.StatusOK
	}
	common.Respond(c, status, common.CodeSuccess, "success", gin.H{
		"event_id":  result.EventID,
		"task_id":   result.TaskID,
		"duplicate": result.Duplicate,
	})
}

func validRecordFlightArrivedRequest(request recordFlightArrivedRequest) bool {
	return request.FlightID > 0 && request.TeamID > 0 && request.TemplateID > 0 &&
		!request.OccurrenceTime.IsZero() && len(strings.TrimSpace(request.SourceEventID)) <= 128
}

func respondRecordFlightArrivedError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		common.FailWithHTTP(c, http.StatusBadRequest, common.CodeInvalidParam, "请求参数无效")
	case errors.Is(err, ErrFlightNotFound), errors.Is(err, ErrTeamNotFound), errors.Is(err, ErrTemplateNotFound):
		common.FailWithHTTP(c, http.StatusNotFound, common.CodeNotFound, "关联资源不存在")
	case errors.Is(err, ErrFlightDisabled), errors.Is(err, ErrTeamDisabled), errors.Is(err, ErrTemplateDisabled), errors.Is(err, ErrTemplateTriggerMismatch):
		common.FailWithHTTP(c, http.StatusConflict, common.CodeConflict, "资源状态或触发条件冲突")
	case errors.Is(err, ErrRepositoryUnavailable):
		common.FailWithHTTP(c, http.StatusServiceUnavailable, common.CodeServiceUnavailable, "事件服务暂不可用")
	default:
		common.FailWithHTTP(c, http.StatusInternalServerError, common.CodeInternalError, "记录航班到达事件失败")
	}
}
