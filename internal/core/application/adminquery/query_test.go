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
	exceptionFilter  ExceptionFilter
	reportFilter     ReportFilter
	updatedException ExceptionUpdate
}

func (r *adminQueryRepository) ListPersonnel(_ context.Context, filter PersonnelFilter) ([]Personnel, int64, error) {
	r.personnelFilter = filter
	return []Personnel{{PublicID: "personnel-1", WorkState: "idle"}}, 1, nil
}

func (r *adminQueryRepository) ListAssignments(_ context.Context, filter AssignmentFilter) ([]Assignment, int64, error) {
	r.assignmentFilter = filter
	return []Assignment{{PublicID: "assignment-1", Status: "confirmed"}}, 1, nil
}

func (r *adminQueryRepository) ListExceptions(_ context.Context, filter ExceptionFilter) ([]Exception, int64, error) {
	r.exceptionFilter = filter
	return []Exception{{PublicID: "exception-1", Severity: "high", Status: "open"}}, 1, nil
}

func (r *adminQueryRepository) GetReportOverview(_ context.Context, filter ReportFilter) (ReportOverview, error) {
	r.reportFilter = filter
	return ReportOverview{From: filter.From, To: filter.To, TaskCounts: map[string]int64{"completed": 4}}, nil
}

func (r *adminQueryRepository) UpdateException(_ context.Context, input ExceptionUpdate) error {
	r.updatedException = input
	return nil
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
	exceptionResult, err := service.ListExceptions(context.Background(), principal, ExceptionFilter{Severity: "high", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if exceptionResult.Total != 1 || repository.exceptionFilter.Scope.TeamIDs[0] != 20 || repository.exceptionFilter.Severity != "high" {
		t.Fatalf("unexpected exception result/filter: %#v %#v", exceptionResult, repository.exceptionFilter)
	}
	reportResult, err := service.GetReportOverview(context.Background(), principal, ReportFilter{From: "2026-09-01", To: "2026-09-03"})
	if err != nil {
		t.Fatal(err)
	}
	if reportResult.TaskCounts["completed"] != 4 || repository.reportFilter.Scope.TeamIDs[0] != 20 || repository.reportFilter.From != "2026-09-01" {
		t.Fatalf("unexpected report result/filter: %#v %#v", reportResult, repository.reportFilter)
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

func TestServiceRejectsInvalidReportDateRange(t *testing.T) {
	service := NewService(&adminQueryRepository{}, iam.NewAuthorizer())
	_, err := service.GetReportOverview(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "manager-1", Roles: []string{security.RoleManager}, Scopes: security.AccessScope{Global: true}}, ReportFilter{From: "2026-09-04", To: "2026-09-03"})
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("report query returned %v, want invalid input", err)
	}
}

func TestServiceUpdatesExceptionWithManagementScope(t *testing.T) {
	repository := &adminQueryRepository{}
	service := NewService(repository, iam.NewAuthorizer())
	principal := security.Principal{Type: security.HumanPrincipal, PublicID: "manager-1", Roles: []string{security.RoleManager}, Scopes: security.AccessScope{Global: true}}
	if err := service.UpdateException(context.Background(), principal, "exception-1", "resolved", "equipment replaced", ExceptionUpdate{}); err != nil {
		t.Fatal(err)
	}
	if repository.updatedException.PublicID != "exception-1" || repository.updatedException.Status != "resolved" || repository.updatedException.ActorPublicID != principal.PublicID {
		t.Fatalf("unexpected exception update: %#v", repository.updatedException)
	}
	if err := service.UpdateException(context.Background(), principal, "exception-1", "open", "", ExceptionUpdate{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reopen update returned %v, want invalid input", err)
	}
}
