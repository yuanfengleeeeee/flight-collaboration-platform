package management

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"golang.org/x/crypto/bcrypt"
)

type managementRepositoryStub struct {
	areaInput      AreaInput
	areaQuery      string
	teamQuery      string
	personnelInput PersonnelWrite
}

func (r *managementRepositoryStub) ListAreas(_ context.Context, query string, _ PageQuery, _ bool) (AreaListResult, error) {
	r.areaQuery = query
	return AreaListResult{}, nil
}
func (r *managementRepositoryStub) CreateArea(_ context.Context, input AreaInput, _ AuditMeta) (Area, error) {
	r.areaInput = input
	return Area{PublicID: "area-1", Code: input.Code, Name: input.Name, Enabled: input.Enabled}, nil
}
func (r *managementRepositoryStub) UpdateArea(context.Context, string, AreaPatch, AuditMeta) (Area, error) {
	return Area{}, nil
}
func (r *managementRepositoryStub) ListTeams(_ context.Context, _, query string, _ PageQuery, _ bool) (TeamListResult, error) {
	r.teamQuery = query
	return TeamListResult{}, nil
}
func (r *managementRepositoryStub) CreateTeam(context.Context, TeamInput, AuditMeta) (Team, error) {
	return Team{}, nil
}
func (r *managementRepositoryStub) UpdateTeam(context.Context, string, TeamPatch, AuditMeta) (Team, error) {
	return Team{}, nil
}
func (r *managementRepositoryStub) CreatePersonnel(_ context.Context, input PersonnelWrite, _ AuditMeta) (Personnel, error) {
	r.personnelInput = input
	return Personnel{PublicID: "personnel-1", HasCredential: true}, nil
}
func (r *managementRepositoryStub) UpdatePersonnel(context.Context, string, PersonnelPatch, AuditMeta) (Personnel, error) {
	return Personnel{}, nil
}
func (r *managementRepositoryStub) ResetPersonnelPassword(context.Context, string, string, AuditMeta) error {
	return nil
}
func (r *managementRepositoryStub) ListTemplates(context.Context, PageQuery, bool) (TemplateListResult, error) {
	return TemplateListResult{}, nil
}
func (r *managementRepositoryStub) CreateTemplate(context.Context, TaskTemplateWrite, AuditMeta) (TaskTemplate, error) {
	return TaskTemplate{}, nil
}
func (r *managementRepositoryStub) UpdateTemplate(context.Context, string, TaskTemplatePatch, AuditMeta) (TaskTemplate, error) {
	return TaskTemplate{}, nil
}
func (r *managementRepositoryStub) ListFlights(context.Context, string, PageQuery, bool) (FlightListResult, error) {
	return FlightListResult{}, nil
}
func (r *managementRepositoryStub) ListAdminIdentities(context.Context, PageQuery, bool) (AdminIdentityListResult, error) {
	return AdminIdentityListResult{}, nil
}
func (r *managementRepositoryStub) CreateAdminIdentity(context.Context, AdminIdentityWrite, AuditMeta) (AdminIdentity, error) {
	return AdminIdentity{}, nil
}
func (r *managementRepositoryStub) UpdateAdminIdentity(context.Context, string, AdminIdentityPatch, AuditMeta) (AdminIdentity, error) {
	return AdminIdentity{}, nil
}

type managementFixedClock struct{ now time.Time }

func (c managementFixedClock) Now() time.Time { return c.now }

func managementAdminPrincipal() security.Principal {
	return security.Principal{Type: security.HumanPrincipal, PublicID: "admin-1", Roles: []string{security.RoleAdmin}, Scopes: security.AccessScope{Global: true}}
}

func TestCreateAreaNormalizesInputAndAddsAuditTime(t *testing.T) {
	repository := &managementRepositoryStub{}
	service := NewService(repository, iam.NewAuthorizer(), managementFixedClock{now: time.Unix(100, 0)})

	value, err := service.CreateArea(context.Background(), managementAdminPrincipal(), AreaInput{Code: "  GATE  ", Name: "  Gate operations ", Enabled: true}, AuditMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if value.Code != "GATE" || value.Name != "Gate operations" || !repository.areaInput.Enabled {
		t.Fatalf("unexpected area result/input: %#v %#v", value, repository.areaInput)
	}
}

func TestListOrganizationNormalizesSearchQuery(t *testing.T) {
	repository := &managementRepositoryStub{}
	service := NewService(repository, iam.NewAuthorizer(), managementFixedClock{now: time.Unix(100, 0)})
	if _, err := service.ListAreas(context.Background(), managementAdminPrincipal(), "  到达  ", PageQuery{Page: 1, PageSize: 20}, false); err != nil {
		t.Fatal(err)
	}
	if repository.areaQuery != "到达" {
		t.Fatalf("area query = %q, want normalized query", repository.areaQuery)
	}
	if _, err := service.ListTeams(context.Background(), managementAdminPrincipal(), "area-1", "  行李  ", PageQuery{Page: 1, PageSize: 20}, false); err != nil {
		t.Fatal(err)
	}
	if repository.teamQuery != "行李" {
		t.Fatalf("team query = %q, want normalized query", repository.teamQuery)
	}
}

func TestListOrganizationRejectsOverlongSearchQuery(t *testing.T) {
	service := NewService(&managementRepositoryStub{}, iam.NewAuthorizer(), managementFixedClock{now: time.Unix(100, 0)})
	tooLong := strings.Repeat("a", 129)
	if _, err := service.ListAreas(context.Background(), managementAdminPrincipal(), tooLong, PageQuery{}, false); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("area search error = %v, want invalid input", err)
	}
	if _, err := service.ListTeams(context.Background(), managementAdminPrincipal(), "area-1", tooLong, PageQuery{}, false); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("team search error = %v, want invalid input", err)
	}
}

func TestCreatePersonnelHashesPasswordBeforeRepository(t *testing.T) {
	repository := &managementRepositoryStub{}
	service := NewService(repository, iam.NewAuthorizer(), managementFixedClock{now: time.Unix(100, 0)})

	_, err := service.CreatePersonnel(context.Background(), managementAdminPrincipal(), PersonnelWrite{EmployeeNo: " 0001 ", DisplayName: " Alice ", TeamPublicID: "team-1", PositionCode: "loader", Capabilities: []string{"scan"}}, "correct horse", AuditMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.personnelInput.PasswordHash == "correct horse" || bcrypt.CompareHashAndPassword([]byte(repository.personnelInput.PasswordHash), []byte("correct horse")) != nil {
		t.Fatal("personnel password was not stored as a valid bcrypt hash")
	}
	if repository.personnelInput.EmployeeNo != "0001" || repository.personnelInput.DisplayName != "Alice" {
		t.Fatalf("personnel input was not normalized: %#v", repository.personnelInput)
	}
}

func TestCreatePersonnelRequiresNumericEmployeeNumberAndStableCodes(t *testing.T) {
	service := NewService(&managementRepositoryStub{}, iam.NewAuthorizer(), managementFixedClock{now: time.Unix(100, 0)})
	base := PersonnelWrite{EmployeeNo: "0001", DisplayName: "Alice", TeamPublicID: "team-1", PositionCode: "loader", Capabilities: []string{"scan"}}
	for name, input := range map[string]PersonnelWrite{
		"alpha employee number":   {EmployeeNo: "E001", DisplayName: base.DisplayName, TeamPublicID: base.TeamPublicID, PositionCode: base.PositionCode, Capabilities: base.Capabilities},
		"short employee number":   {EmployeeNo: "123", DisplayName: base.DisplayName, TeamPublicID: base.TeamPublicID, PositionCode: base.PositionCode, Capabilities: base.Capabilities},
		"invalid position code":   {EmployeeNo: base.EmployeeNo, DisplayName: base.DisplayName, TeamPublicID: base.TeamPublicID, PositionCode: "Loader", Capabilities: base.Capabilities},
		"invalid capability code": {EmployeeNo: base.EmployeeNo, DisplayName: base.DisplayName, TeamPublicID: base.TeamPublicID, PositionCode: base.PositionCode, Capabilities: []string{"scan status"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.CreatePersonnel(context.Background(), managementAdminPrincipal(), input, "correct horse", AuditMeta{}); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want invalid input", err)
			}
		})
	}
}

func TestStaffCannotUseManagementWrites(t *testing.T) {
	service := NewService(&managementRepositoryStub{}, iam.NewAuthorizer(), managementFixedClock{now: time.Unix(100, 0)})
	principal := security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", Roles: []string{security.RoleStaff}, Scopes: security.AccessScope{Global: true}}
	_, err := service.CreateArea(context.Background(), principal, AreaInput{Code: "GATE", Name: "Gate"}, AuditMeta{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("staff management write returned %v, want forbidden", err)
	}
}

func TestListFlightsRejectsInvalidOperatingDate(t *testing.T) {
	service := NewService(&managementRepositoryStub{}, iam.NewAuthorizer(), managementFixedClock{now: time.Unix(100, 0)})
	_, err := service.ListFlights(context.Background(), managementAdminPrincipal(), "2026-99-99", PageQuery{}, false)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid operating date returned %v, want invalid input", err)
	}
}
