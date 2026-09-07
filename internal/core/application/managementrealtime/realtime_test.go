package managementrealtime

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func TestManagementRealtimeFiltersByAreaOrTeamScope(t *testing.T) {
	event := Event{ID: 7, EventType: "task.assigned.v1", AggregateType: "task", AggregatePublicID: "task-7", TeamID: 20, AreaID: 10, OccurredAt: time.Now().UTC()}
	service := NewService(&realtimeFakeRepository{events: []Event{event}}, iam.NewAuthorizer())
	service.interval = time.Millisecond
	service.maxWait = 20 * time.Millisecond
	request := httptest.NewRequest("GET", "/api/v1/realtime/management?wait_seconds=1", nil)
	writer := httptest.NewRecorder()
	service.ServeHTTP(writer, request, security.Principal{Type: security.HumanPrincipal, PublicID: "leader-1", Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{TeamIDs: []uint64{20}}})
	if !strings.Contains(writer.Body.String(), "task.assigned.v1") {
		t.Fatalf("scoped leader did not receive team event: %s", writer.Body.String())
	}

	writer = httptest.NewRecorder()
	request = httptest.NewRequest("GET", "/api/v1/realtime/management?wait_seconds=1", nil)
	service.ServeHTTP(writer, request, security.Principal{Type: security.HumanPrincipal, PublicID: "leader-2", Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{TeamIDs: []uint64{99}}})
	if strings.Contains(writer.Body.String(), "task.assigned.v1") {
		t.Fatalf("out-of-scope leader received event: %s", writer.Body.String())
	}
}

func TestManagementRealtimeRequiresEventRead(t *testing.T) {
	service := NewService(&realtimeFakeRepository{}, iam.NewAuthorizer())
	writer := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/realtime/management", nil)
	service.ServeHTTP(writer, request, security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", Roles: []string{security.RoleStaff}})
	if writer.Code != 403 {
		t.Fatalf("staff status = %d, want 403", writer.Code)
	}
}

func TestScopeAllowsGlobalAndMatchingArea(t *testing.T) {
	value := Event{TeamID: 20, AreaID: 10}
	if !scopeAllows(security.AccessScope{Global: true}, value) || !scopeAllows(security.AccessScope{AreaIDs: []uint64{10}}, value) || scopeAllows(security.AccessScope{AreaIDs: []uint64{99}}, value) {
		t.Fatal("unexpected management event scope result")
	}
}

type realtimeFakeRepository struct{ events []Event }

func (r *realtimeFakeRepository) ListManagementEvents(_ context.Context, _ uint64, _ int) ([]Event, error) {
	return append([]Event(nil), r.events...), nil
}
