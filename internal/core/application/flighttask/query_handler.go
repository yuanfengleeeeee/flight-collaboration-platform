package flighttask

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
)

func RegisterTaskQueryRoutes(router gin.IRouter, service *TaskQueryService, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	handler := TaskQueryHandler{service: service}
	listHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.List)
	detailHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.Get)
	historyHandlers := append(append([]gin.HandlerFunc{}, middleware...), handler.History)
	router.GET("/api/v1/tasks", listHandlers...)
	router.GET("/api/v1/tasks/:taskPublicID", detailHandlers...)
	router.GET("/api/v1/tasks/:taskPublicID/history", historyHandlers...)
}

type TaskQueryHandler struct {
	service *TaskQueryService
}

func (h TaskQueryHandler) List(c *gin.Context) {
	principal, err := principalFromRequest(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	page, pageSize, err := parseTaskPagination(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	result, err := h.service.ListTasks(c.Request.Context(), principal, TaskQueryFilter{Status: c.Query("status"), FlightPublicID: c.Query("flight_public_id"), Page: page, PageSize: pageSize})
	if err != nil {
		writeError(c, statusForError(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func (h TaskQueryHandler) Get(c *gin.Context) {
	principal, err := principalFromRequest(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	result, err := h.service.GetTask(c.Request.Context(), principal, c.Param("taskPublicID"))
	if err != nil {
		writeError(c, statusForError(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func (h TaskQueryHandler) History(c *gin.Context) {
	principal, err := principalFromRequest(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	result, err := h.service.GetTaskHistory(c.Request.Context(), principal, c.Param("taskPublicID"))
	if err != nil {
		writeError(c, statusForError(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func parseTaskPagination(c *gin.Context) (int, int, error) {
	page, pageSize := defaultTaskPage, defaultTaskPageSize
	var err error
	if value := c.Query("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, ErrTaskQueryInvalidInput
		}
	}
	if value := c.Query("page_size"); value != "" {
		pageSize, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, ErrTaskQueryInvalidInput
		}
	}
	return page, pageSize, nil
}
