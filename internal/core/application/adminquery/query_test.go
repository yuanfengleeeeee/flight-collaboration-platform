package adminquery

import (
	"context"
	"errors"
	"testing"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

type adminQueryRepository struct {
	personnelFilter  PersonnelFilter
	assignmentFilter AssignmentFilter
}

func (r *adminQueryRepository) ListPersonnel(_ context.Context, filter PersonnelFilter) ([]Personnel, int64, error) {
	r.personnelFilter = filter
	return []Personnel{{PublicID: "personnel-1", WorkState: "idle"}}, 1, nil
}

func (r *adminQueryRepository) ListAssignments(_ context.Context, filter AssignmentFilter) ([]Assignment, int64, error) {
	r.assignmentFilter = filter
	return []Assignment{{PublicID: "assignment-1", Status: "confirmed"}}, 1, nil
}

func TestServiceAppliesPrincipalScopeToManagementQueries(t *testing.T) {
	repository := &adminQueryRepository{}
	service := NewService(repository, iam.NewAuthorizer())
	principal := security.Principal{Type: security.HumanPrincipal, PublicID: "leader-1", Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{TeamIDs: []uint64{20}, AreaIDs: []uint64{10}}}
	personnelResult, err := service.ListPersonnel(context.Background(), principal, PersonnelFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if personnelResult.Total != 1 || len(repository.personnelFilter.Scope.TeamIDs) != 1 || repository.personnelFilter.Scope.TeamIDs[0] != 20 {
		t.Fatalf("unexpected personnel result/filter: %#v %#v", personnelResult, repository.personnelFilter)
	}
	assignmentResult, err := service.ListAssignments(context.Background(), principal, AssignmentFilter{Status: "confirmed", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if assignmentResult.Total != 1 || repository.assignmentFilter.Status != "confirmed" {
		t.Fatalf("unexpected assignment result/filter: %#v %#v", assignmentResult, repository.assignmentFilter)
	}
}

func TestServiceRejectsStaffManagementQueries(t *testing.T) {
	service := NewService(&adminQueryRepository{}, iam.NewAuthorizer())
	_, err := service.ListPersonnel(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", Roles: []string{security.RoleStaff}, Scopes: security.AccessScope{Global: true}}, PersonnelFilter{})
	if err == nil || !errors.Is(err, ErrForbidden) {
		t.Fatalf("personnel query returned %v, want forbidden", err)
	}
	_, err = service.ListAssignments(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", Roles: []string{security.RoleStaff}, Scopes: security.AccessScope{Global: true}}, AssignmentFilter{})
	if err == nil || !errors.Is(err, ErrForbidden) {
		t.Fatalf("assignment query returned %v, want forbidden", err)
	}
}
