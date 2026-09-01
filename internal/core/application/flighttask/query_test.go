package flighttask

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func TestTaskQueryServiceListsScopedTaskDetails(t *testing.T) {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	repository := &taskQueryFakeRepository{total: 1}
	repository.rows = []TaskReadModel{{
		Task:             taskmodule.Instance{ID: 10, PublicID: "task-1", FlightPublicID: "flight-1", FlightDisplayNo: "MU100", TriggerType: taskmodule.TriggerFlightArrived, GenerationKey: "flight-1:v1", SourceEventID: "arrival-1", TemplateVersion: 1, RequiredPositionCode: "ramp", RequiredCapabilities: []string{"ramp"}, Name: "Unload", Message: "Unload baggage", PlannedAt: now, Status: taskmodule.StatusAwaitingConfirmation, StatusVersion: 0, SyncVersion: 1},
		TemplatePublicID: "template-1", AreaPublicID: "area-1", TeamPublicID: "team-1",
		Candidates: []taskmodule.Candidate{{ID: 20, PublicID: "candidate-1", TaskID: 10, PersonnelID: 30, PersonnelPublicID: "person-1", Rank: 1, Status: taskmodule.CandidateProposed, MatchedPositionCode: "ramp", MatchedCapabilities: []string{"ramp"}, PersonnelWorkStateSnapshot: "idle", PersonnelStateChangedAt: now}},
	}}
	service := NewTaskQueryService(repository, iam.NewAuthorizer())
	result, err := service.ListTasks(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "leader-1", Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{Global: true}}, TaskQueryFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Candidates[0].PersonnelPublicID != "person-1" {
		t.Fatalf("unexpected task result: %#v", result)
	}
	if repository.lastFilter.Scope.Global != true || repository.lastFilter.PageSize != 20 {
		t.Fatalf("query scope/filter not forwarded: %#v", repository.lastFilter)
	}
}

func TestTaskQueryServiceRequiresReadPermissionAndScope(t *testing.T) {
	service := NewTaskQueryService(&taskQueryFakeRepository{}, iam.NewAuthorizer())
	_, err := service.ListTasks(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", Roles: []string{security.RoleStaff}}, TaskQueryFilter{})
	if CodeOf(err) != ErrTaskQueryForbidden.Code {
		t.Fatalf("error = %v, want forbidden", err)
	}
	_, err = service.ListTasks(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", Roles: []string{security.RoleStaff}, Scopes: security.AccessScope{Global: true}}, TaskQueryFilter{PageSize: 101})
	if !errors.Is(err, ErrTaskQueryInvalidInput) {
		t.Fatalf("error = %v, want invalid input", err)
	}
}

func TestTaskQueryHandlerReturnsListEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &taskQueryFakeRepository{rows: []TaskReadModel{{Task: taskmodule.Instance{PublicID: "task-1", PlannedAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), Status: taskmodule.StatusCompleted}}}, total: 1}
	router := gin.New()
	RegisterTaskQueryRoutes(router, NewTaskQueryService(repository, iam.NewAuthorizer()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?page=1&page_size=10", nil)
	request.Header.Set("X-Actor-Type", string(security.HumanPrincipal))
	request.Header.Set("X-Actor-Public-ID", "admin-1")
	request.Header.Set("X-Actor-Roles", security.RoleAdmin)
	request.Header.Set("X-Actor-Global", "true")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !containsQueryResponse(recorder.Body.String(), `"total":1`) || !containsQueryResponse(recorder.Body.String(), `"page_size":10`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

type taskQueryFakeRepository struct {
	rows       []TaskReadModel
	total      int64
	lastFilter TaskQueryFilter
}

func (f *taskQueryFakeRepository) ListTaskReadModels(_ context.Context, filter TaskQueryFilter) ([]TaskReadModel, int64, error) {
	f.lastFilter = filter
	return f.rows, f.total, nil
}

func (f *taskQueryFakeRepository) FindTaskReadModel(_ context.Context, publicID string, scope security.AccessScope) (TaskReadModel, error) {
	f.lastFilter.Scope = scope
	for _, row := range f.rows {
		if row.Task.PublicID == publicID {
			return row, nil
		}
	}
	return TaskReadModel{}, ErrNotFound
}

func containsQueryResponse(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
