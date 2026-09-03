package event

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/middleware"
)

type fakeFlightArrivalRecorder struct {
	result RecordResult
	err    error
	input  RecordInput
	calls  int
}

func (f *fakeFlightArrivalRecorder) RecordFlightArrived(_ context.Context, input RecordInput) (RecordResult, error) {
	f.calls++
	f.input = input
	return f.result, f.err
}

type arrivalHandlerResponse struct {
	Code      int    `json:"code"`
	RequestID string `json:"request_id"`
	Data      struct {
		EventID   int64 `json:"event_id"`
		TaskID    int64 `json:"task_id"`
		Duplicate bool  `json:"duplicate"`
	} `json:"data"`
}

func TestFlightArrivalHandlerRejectsInvalidRequest(t *testing.T) {
	recorder := &fakeFlightArrivalRecorder{}
	router := newArrivalHandlerTestRouter(recorder)

	for _, test := range []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: "{"},
		{name: "missing occurrence time", body: `{"flight_id":101,"team_id":201,"template_id":301}`},
		{name: "invalid identifier", body: `{"flight_id":0,"team_id":201,"template_id":301,"occurrence_time":"2026-08-07T10:00:00Z"}`},
		{name: "invalid timestamp", body: `{"flight_id":101,"team_id":201,"template_id":301,"occurrence_time":"not-a-time"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performArrivalRequest(router, test.body, "")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			body := decodeArrivalHandlerResponse(t, response)
			if body.Code != common.CodeInvalidParam {
				t.Fatalf("code = %d, want %d", body.Code, common.CodeInvalidParam)
			}
		})
	}
	if recorder.calls != 0 {
		t.Fatalf("recorder calls = %d, want 0", recorder.calls)
	}
}

func TestFlightArrivalHandlerSuccessAndDuplicate(t *testing.T) {
	occurrence := "2026-08-07T10:00:00+08:00"
	body := `{"flight_id":101,"team_id":201,"template_id":301,"source_event_id":" arrival-42 ","occurrence_time":"` + occurrence + `"}`

	for _, test := range []struct {
		name       string
		result     RecordResult
		wantStatus int
	}{
		{name: "created", result: RecordResult{EventID: 401, TaskID: 501}, wantStatus: http.StatusCreated},
		{name: "duplicate", result: RecordResult{EventID: 401, TaskID: 501, Duplicate: true}, wantStatus: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &fakeFlightArrivalRecorder{result: test.result}
			response := performArrivalRequest(newArrivalHandlerTestRouter(recorder), body, "arrival-request-42")
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			responseBody := decodeArrivalHandlerResponse(t, response)
			if responseBody.Code != common.CodeSuccess || responseBody.Data.EventID != 401 || responseBody.Data.TaskID != 501 || responseBody.Data.Duplicate != test.result.Duplicate {
				t.Fatalf("response = %+v", responseBody)
			}
			if responseBody.RequestID != "arrival-request-42" || response.Header().Get("X-Request-ID") != "arrival-request-42" {
				t.Fatalf("request ID was not propagated: body=%q header=%q", responseBody.RequestID, response.Header().Get("X-Request-ID"))
			}
			if recorder.calls != 1 || recorder.input.SourceEventID != " arrival-42 " {
				t.Fatalf("recorder calls/input = %d/%+v", recorder.calls, recorder.input)
			}
			wantTime, _ := time.Parse(time.RFC3339, occurrence)
			if !recorder.input.OccurrenceTime.Equal(wantTime) {
				t.Fatalf("occurrence time = %s, want %s", recorder.input.OccurrenceTime, wantTime)
			}
		})
	}
}

func TestFlightArrivalHandlerMapsServiceErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   int
	}{
		{name: "invalid input", err: ErrInvalidInput, wantStatus: http.StatusBadRequest, wantCode: common.CodeInvalidParam},
		{name: "not found", err: ErrTeamNotFound, wantStatus: http.StatusNotFound, wantCode: common.CodeNotFound},
		{name: "disabled", err: ErrTemplateDisabled, wantStatus: http.StatusConflict, wantCode: common.CodeConflict},
		{name: "trigger mismatch", err: ErrTemplateTriggerMismatch, wantStatus: http.StatusConflict, wantCode: common.CodeConflict},
		{name: "repository unavailable", err: ErrRepositoryUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: common.CodeServiceUnavailable},
		{name: "internal", err: errors.New("database driver detail"), wantStatus: http.StatusInternalServerError, wantCode: common.CodeInternalError},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &fakeFlightArrivalRecorder{err: test.err}
			response := performArrivalRequest(newArrivalHandlerTestRouter(recorder), validArrivalRequestBody, "")
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			responseBody := decodeArrivalHandlerResponse(t, response)
			if responseBody.Code != test.wantCode {
				t.Fatalf("code = %d, want %d", responseBody.Code, test.wantCode)
			}
			if strings.Contains(response.Body.String(), "database driver detail") {
				t.Fatal("response must not expose service error detail")
			}
		})
	}
}

const validArrivalRequestBody = `{"flight_id":101,"team_id":201,"template_id":301,"occurrence_time":"2026-08-07T10:00:00Z"}`

func newArrivalHandlerTestRouter(recorder FlightArrivalRecorder) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.POST("/api/v1/events/flight-arrived", NewFlightArrivalHandler(recorder).RecordFlightArrived)
	return router
}

func performArrivalRequest(router *gin.Engine, body, requestID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/events/flight-arrived", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	if requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeArrivalHandlerResponse(t *testing.T, response *httptest.ResponseRecorder) arrivalHandlerResponse {
	t.Helper()
	var body arrivalHandlerResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return body
}
