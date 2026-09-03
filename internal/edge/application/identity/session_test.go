package identity

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sharedIdentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/identity"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

type testCoreClient struct {
	passwordResult sharedIdentity.PasswordVerifyResult
	staff          sharedIdentity.Staff
	staffErr       error
}

func (c testCoreClient) FindStaff(_ context.Context, publicID string) (sharedIdentity.Staff, error) {
	if c.staffErr != nil {
		return sharedIdentity.Staff{}, c.staffErr
	}
	if c.staff.PublicID == "" {
		return c.passwordResult.Staff, nil
	}
	if c.staff.PublicID != publicID {
		return sharedIdentity.Staff{}, fmt.Errorf("staff mismatch")
	}
	return c.staff, nil
}

func (c testCoreClient) VerifyPassword(context.Context, sharedIdentity.PasswordVerifyRequest) (sharedIdentity.PasswordVerifyResult, error) {
	return c.passwordResult, nil
}

func (c testCoreClient) Exchange(context.Context, sharedIdentity.ExchangeRequest) (sharedIdentity.Staff, error) {
	return c.staff, nil
}

func (c testCoreClient) CompleteBinding(context.Context, sharedIdentity.BindingRequest) (sharedIdentity.Staff, error) {
	return c.staff, nil
}

type testIssuer struct{}

func (testIssuer) IssueForSession(_ security.Principal, sessionID string, _ time.Duration) (string, error) {
	return fmt.Sprintf("access:%s", sessionID), nil
}

func TestPasswordLoginRefreshRotationAndReplayRevocation(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	client := testCoreClient{passwordResult: sharedIdentity.PasswordVerifyResult{
		State: "authenticated", Staff: sharedIdentity.Staff{PublicID: "staff-1", EmployeeNo: "E001", Role: "staff"},
	}}
	store := NewMemorySessionStore()
	service := NewService(client, store, testIssuer{}, fixedClock{value: now}, Config{AccessTokenTTL: time.Minute, SessionTTL: time.Hour, AbsoluteSessionTTL: 24 * time.Hour})

	first, err := service.PasswordLogin(context.Background(), sharedIdentity.PasswordVerifyRequest{EmployeeNo: "E001", Password: "correct", Client: "employee-web"})
	if err != nil {
		t.Fatal(err)
	}
	if first.AccessToken == "" || first.RefreshToken == "" || first.SessionPublicID == "" {
		t.Fatalf("incomplete login response: %#v", first)
	}

	second, err := service.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionPublicID == first.SessionPublicID || second.RefreshToken == first.RefreshToken {
		t.Fatalf("refresh token rotation did not replace session: %#v", second)
	}

	if _, err := service.Refresh(context.Background(), first.RefreshToken); !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("got %v, want refresh replay rejection", err)
	}
	if err := service.ValidatePrincipal(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", SessionID: second.SessionPublicID}); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("got %v, want replacement session revocation after replay", err)
	}
}

func TestBindingRequiredDoesNotIssueSession(t *testing.T) {
	client := testCoreClient{passwordResult: sharedIdentity.PasswordVerifyResult{State: "binding_required", BindingTicket: "ticket-1", Staff: sharedIdentity.Staff{PublicID: "staff-1"}}}
	service := NewService(client, NewMemorySessionStore(), testIssuer{}, fixedClock{value: time.Unix(100, 0)}, Config{})

	result, err := service.PasswordLogin(context.Background(), sharedIdentity.PasswordVerifyRequest{EmployeeNo: "E001", Password: "correct", Client: "employee-miniapp", Provider: "personal_wechat"})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "binding_required" || result.BindingTicket != "ticket-1" || result.AccessToken != "" || result.RefreshToken != "" {
		t.Fatalf("unexpected binding response: %#v", result)
	}
}

func TestLogoutRevokesCurrentSession(t *testing.T) {
	service := NewService(testCoreClient{passwordResult: sharedIdentity.PasswordVerifyResult{State: "authenticated", Staff: sharedIdentity.Staff{PublicID: "staff-1"}}}, NewMemorySessionStore(), testIssuer{}, fixedClock{value: time.Unix(100, 0)}, Config{})
	login, err := service.PasswordLogin(context.Background(), sharedIdentity.PasswordVerifyRequest{EmployeeNo: "E001", Password: "correct", Client: "employee-web"})
	if err != nil {
		t.Fatal(err)
	}
	principal := security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", SessionID: login.SessionPublicID}
	if err := service.Logout(context.Background(), principal); err != nil {
		t.Fatal(err)
	}
	if err := service.ValidatePrincipal(context.Background(), principal); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("got %v, want revoked session", err)
	}
}

func TestResolvePrincipalRestoresEmployeeRole(t *testing.T) {
	client := testCoreClient{passwordResult: sharedIdentity.PasswordVerifyResult{State: "authenticated", Staff: sharedIdentity.Staff{PublicID: "staff-1"}}}
	service := NewService(client, NewMemorySessionStore(), testIssuer{}, fixedClock{value: time.Unix(100, 0)}, Config{})
	login, err := service.PasswordLogin(context.Background(), sharedIdentity.PasswordVerifyRequest{EmployeeNo: "E001", Password: "correct", Client: "employee-web"})
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := service.ResolvePrincipal(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", SessionID: login.SessionPublicID})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Roles) != 1 || resolved.Roles[0] != security.RoleStaff {
		t.Fatalf("got roles %#v, want staff role", resolved.Roles)
	}
}

func TestInactiveStaffCannotUseExistingSession(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	store := NewMemorySessionStore()
	active := testCoreClient{passwordResult: sharedIdentity.PasswordVerifyResult{State: "authenticated", Staff: sharedIdentity.Staff{PublicID: "staff-1"}}}
	service := NewService(active, store, testIssuer{}, fixedClock{value: now}, Config{})
	login, err := service.PasswordLogin(context.Background(), sharedIdentity.PasswordVerifyRequest{EmployeeNo: "E001", Password: "correct", Client: "employee-web"})
	if err != nil {
		t.Fatal(err)
	}

	inactive := NewService(testCoreClient{staffErr: sharedIdentity.RemoteError{Status: 403, Code: "staff_inactive"}}, store, testIssuer{}, fixedClock{value: now}, Config{})
	principal := security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1", SessionID: login.SessionPublicID}
	if err := inactive.ValidatePrincipal(context.Background(), principal); !errors.Is(err, ErrStaffInactive) {
		t.Fatalf("got %v, want inactive staff rejection", err)
	}
}
