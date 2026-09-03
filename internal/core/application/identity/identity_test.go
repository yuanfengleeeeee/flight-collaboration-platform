package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

type fakeRepository struct {
	staff        Staff
	bound        bool
	ticket       string
	completed    bool
	lastClient   string
	lastExternal ExternalIdentity
}

func (r *fakeRepository) FindStaff(_ context.Context, publicID string) (Staff, error) {
	if publicID != r.staff.PublicID {
		return Staff{}, ErrNotFound
	}
	return r.staff, nil
}

func (r *fakeRepository) VerifyPassword(_ context.Context, employeeNo, password string, _ time.Time, _ int, _ time.Duration) (Staff, error) {
	if employeeNo != r.staff.EmployeeNo || password != "correct horse" {
		return Staff{}, ErrInvalidCredentials
	}
	return r.staff, nil
}

func (r *fakeRepository) FindBindingForStaff(_ context.Context, _ string, _ string, _ string) (Staff, error) {
	if !r.bound {
		return Staff{}, ErrNotFound
	}
	return r.staff, nil
}

func (r *fakeRepository) FindBindingByExternal(_ context.Context, _ ExternalIdentity) (Staff, error) {
	if !r.bound {
		return Staff{}, ErrNotFound
	}
	return r.staff, nil
}

func (r *fakeRepository) IssueBindingTicket(_ context.Context, _ string, client string, _ time.Time, _ time.Duration) (string, error) {
	r.lastClient = client
	return r.ticket, nil
}

func (r *fakeRepository) CompleteBinding(_ context.Context, _ string, client string, external ExternalIdentity, _ time.Time) (Staff, error) {
	r.bound = true
	r.completed = true
	r.lastClient = client
	r.lastExternal = external
	return r.staff, nil
}

func TestPasswordLoginRequiresOneTimeBindingForMiniapp(t *testing.T) {
	repository := &fakeRepository{staff: Staff{PublicID: "staff-1", EmployeeNo: "E001", DisplayName: "Alice"}, ticket: "ticket-1"}
	service := NewService(repository, DevelopmentProviderVerifier{}, fixedClock{value: time.Unix(100, 0)}, ServiceConfig{})

	result, err := service.PasswordLogin(context.Background(), PasswordLoginInput{
		EmployeeNo: "E001", Password: "correct horse", Client: ClientEmployeeMiniapp,
		Provider: ProviderPersonalWechat, ProviderApp: "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != StateBindingRequired || result.BindingTicket != "ticket-1" {
		t.Fatalf("unexpected binding result: %#v", result)
	}
	if repository.lastClient != ClientEmployeeMiniapp {
		t.Fatalf("ticket issued for client %q", repository.lastClient)
	}
}

func TestPasswordLoginForWebDoesNotRequireExternalProvider(t *testing.T) {
	repository := &fakeRepository{staff: Staff{PublicID: "staff-1", EmployeeNo: "E001", DisplayName: "Alice"}}
	service := NewService(repository, DevelopmentProviderVerifier{}, fixedClock{value: time.Unix(100, 0)}, ServiceConfig{})

	result, err := service.PasswordLogin(context.Background(), PasswordLoginInput{
		EmployeeNo: "E001", Password: "correct horse", Client: ClientEmployeeWeb,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != StateAuthenticated || result.Staff.Role != "staff" {
		t.Fatalf("unexpected web login result: %#v", result)
	}
}

func TestPasswordLoginForMiniappCanIssueSessionWithoutExternalBinding(t *testing.T) {
	repository := &fakeRepository{staff: Staff{PublicID: "staff-1", EmployeeNo: "E001", DisplayName: "Alice"}}
	service := NewService(repository, DevelopmentProviderVerifier{}, fixedClock{value: time.Unix(100, 0)}, ServiceConfig{})

	result, err := service.PasswordLogin(context.Background(), PasswordLoginInput{
		EmployeeNo: "E001", Password: "correct horse", Client: ClientEmployeeMiniapp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != StateAuthenticated || result.Staff.PublicID != "staff-1" || result.BindingTicket != "" {
		t.Fatalf("unexpected miniapp password login result: %#v", result)
	}
}

func TestBindingAndExchangeUseProviderSubjectWithoutRealProvider(t *testing.T) {
	repository := &fakeRepository{staff: Staff{PublicID: "staff-1", EmployeeNo: "E001"}, ticket: "ticket-1"}
	service := NewService(repository, DevelopmentProviderVerifier{}, fixedClock{value: time.Unix(100, 0)}, ServiceConfig{})

	staff, err := service.CompleteBinding(context.Background(), "ticket-1", ProviderPersonalWechat, "mock:wx-user-1", ClientEmployeeMiniapp, "")
	if err != nil {
		t.Fatal(err)
	}
	if staff.PublicID != "staff-1" || !repository.completed || repository.lastExternal.ExternalSubject != "wx-user-1" {
		t.Fatalf("binding did not preserve provider subject: %#v %#v", staff, repository.lastExternal)
	}

	staff, err = service.Exchange(context.Background(), ProviderPersonalWechat, "mock:wx-user-1", ClientEmployeeMiniapp, "")
	if err != nil {
		t.Fatal(err)
	}
	if staff.PublicID != "staff-1" {
		t.Fatalf("unexpected exchange staff: %#v", staff)
	}
}

func TestExchangeMapsUnknownProviderSubject(t *testing.T) {
	repository := &fakeRepository{staff: Staff{PublicID: "staff-1", EmployeeNo: "E001"}}
	service := NewService(repository, DevelopmentProviderVerifier{}, fixedClock{value: time.Unix(100, 0)}, ServiceConfig{})

	_, err := service.Exchange(context.Background(), ProviderPersonalWechat, "mock:unknown", ClientEmployeeMiniapp, "")
	if !errors.Is(err, ErrIdentityUnmapped) {
		t.Fatalf("got %v, want ErrIdentityUnmapped", err)
	}
}
