package taskchange

import (
	"context"
	"errors"
	"testing"
	"time"

	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func TestCreateTaskChangeUsesStableRequestContext(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	repository := &taskChangeFakeRepository{}
	service := NewService(repository, nil, fixedClock{value: now})

	value, err := service.Create(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", Roles: []string{security.RoleStaff}}, CreateInput{
		TaskPublicID: "task-1",
		Action:       taskmodule.ChangeActionPause,
		Reason:       "同航班保障冲突",
		RequestID:    "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != taskmodule.ChangeRequestPending || value.RequestedByPublicID != "staff-1" || value.RequestedAt != now || repository.created.RequestID != "request-1" {
		t.Fatalf("unexpected change request: %#v", value)
	}
}

func TestTaskChangeReviewRequiresDutyManagerOrAdmin(t *testing.T) {
	service := NewService(&taskChangeFakeRepository{reviewed: taskmodule.ChangeRequest{PublicID: "change-1", Status: taskmodule.ChangeRequestApplied}}, nil, fixedClock{value: time.Now().UTC()})
	input := ReviewInput{PublicID: "change-1", Decision: ReviewApprove}

	for _, principal := range []security.Principal{
		{Type: security.HumanPrincipal, PublicID: "leader-1", Roles: []string{security.RoleLeader}},
		{Type: security.HumanPrincipal, PublicID: "supervisor-1", Roles: []string{security.RoleSupervisor}},
	} {
		if _, err := service.Review(context.Background(), principal, input); !errors.Is(err, ErrForbidden) {
			t.Fatalf("principal %v review error = %v, want forbidden", principal.Roles, err)
		}
	}

	value, err := service.Review(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "manager-1", Roles: []string{security.RoleManager}, Scopes: security.AccessScope{Global: true}}, input)
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != taskmodule.ChangeRequestApplied {
		t.Fatalf("manager review status = %s", value.Status)
	}
}

func TestTaskChangeRescheduleRequiresTargetTime(t *testing.T) {
	service := NewService(&taskChangeFakeRepository{}, nil, fixedClock{value: time.Now().UTC()})
	_, err := service.Create(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", Roles: []string{security.RoleStaff}}, CreateInput{
		TaskPublicID: "task-1",
		Action:       taskmodule.ChangeActionReschedule,
		Reason:       "航班延误",
		RequestID:    "request-2",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reschedule without target error = %v, want invalid input", err)
	}
}

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

type taskChangeFakeRepository struct {
	created  taskmodule.ChangeRequest
	reviewed taskmodule.ChangeRequest
}

func (r *taskChangeFakeRepository) CreateRequest(_ context.Context, value taskmodule.ChangeRequest, _ security.AccessScope) (taskmodule.ChangeRequest, error) {
	r.created = value
	return value, nil
}

func (r *taskChangeFakeRepository) ListRequests(_ context.Context, _ ListFilter) ([]taskmodule.ChangeRequest, int64, error) {
	return nil, 0, nil
}

func (r *taskChangeFakeRepository) ReviewRequest(_ context.Context, input ReviewInput) (taskmodule.ChangeRequest, error) {
	if r.reviewed.PublicID == "" {
		return taskmodule.ChangeRequest{}, ErrNotFound
	}
	return r.reviewed, nil
}
