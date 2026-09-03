package adminauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

type adminAuthClock struct{ now time.Time }

func (c adminAuthClock) Now() time.Time { return c.now }

type adminAuthProvider struct{}

func (adminAuthProvider) Begin(_ context.Context, state, redirectURI, _ string) (string, error) {
	return redirectURI + "?state=" + state, nil
}

func (adminAuthProvider) Exchange(_ context.Context, code, _ string) (ExternalIdentity, error) {
	if code != "code-1" {
		return ExternalIdentity{}, ErrCodeInvalid
	}
	return ExternalIdentity{Provider: ProviderOIDC, ExternalSub: "subject-1", DisplayName: "Manager"}, nil
}

type adminAuthIssuer struct {
	principal security.Principal
	sessionID string
}

func (i *adminAuthIssuer) IssueForSession(principal security.Principal, sessionID string, _ time.Duration) (string, error) {
	i.principal = principal
	i.sessionID = sessionID
	return "core-admin-token", nil
}

type adminAuthRepository struct {
	state     SSOState
	stateUsed bool
	identity  AdminIdentity
	sessions  map[string]Session
	revoked   map[string]bool
}

func (r *adminAuthRepository) CreateSSOState(_ context.Context, value SSOState) error {
	r.state = value
	return nil
}

func (r *adminAuthRepository) ConsumeSSOState(_ context.Context, stateHash string, now time.Time) (SSOState, error) {
	if r.stateUsed || r.state.StateHash != stateHash || !r.state.ExpiresAt.After(now) {
		return SSOState{}, ErrInvalidState
	}
	r.stateUsed = true
	consumed := now
	r.state.ConsumedAt = &consumed
	return r.state, nil
}

func (r *adminAuthRepository) FindIdentity(_ context.Context, provider, subject string) (AdminIdentity, error) {
	if r.identity.Provider != provider || r.identity.ExternalSub != subject {
		return AdminIdentity{}, ErrIdentityUnmapped
	}
	return r.identity, nil
}

func (r *adminAuthRepository) CreateSession(_ context.Context, value Session) error {
	if r.sessions == nil {
		r.sessions = make(map[string]Session)
	}
	r.sessions[value.PublicID] = value
	return nil
}

func (r *adminAuthRepository) FindSession(_ context.Context, sessionID string) (Session, error) {
	value, ok := r.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if r.revoked != nil && r.revoked[sessionID] {
		now := time.Now().UTC()
		value.RevokedAt = &now
	}
	return value, nil
}

func (r *adminAuthRepository) RevokeSession(_ context.Context, sessionID string, now time.Time) error {
	if _, ok := r.sessions[sessionID]; !ok {
		return ErrSessionNotFound
	}
	if r.revoked == nil {
		r.revoked = make(map[string]bool)
	}
	r.revoked[sessionID] = true
	return nil
}

func TestServiceExchangesOneTimeStateIntoCoreSession(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	repository := &adminAuthRepository{identity: AdminIdentity{PublicID: "admin-1", Provider: ProviderOIDC, ExternalSub: "subject-1", DisplayName: "Manager", Role: security.RoleManager, Enabled: true, AccessScope: security.AccessScope{Global: true}}}
	issuer := &adminAuthIssuer{}
	service := NewService(repository, adminAuthProvider{}, issuer, adminAuthClock{now: now}, Config{Provider: ProviderOIDC, AllowedRedirectURI: []string{"http://admin.local/sso/callback"}})
	if _, err := service.Start(context.Background(), "state-1234567890123456", "http://admin.local/sso/callback"); err != nil {
		t.Fatal(err)
	}
	result, err := service.Exchange(context.Background(), "state-1234567890123456", "code-1", "http://admin.local/sso/callback")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "core-admin-token" || result.Principal.Role != security.RoleManager || result.SessionPublicID == "" {
		t.Fatalf("unexpected exchange result: %#v", result)
	}
	if issuer.principal.Scopes.Global != true || issuer.sessionID != result.SessionPublicID {
		t.Fatalf("issuer did not receive the resolved admin principal: %#v", issuer.principal)
	}
	resolved, err := service.ResolvePrincipal(context.Background(), security.Principal{Type: security.HumanPrincipal, PublicID: "admin-1", SessionID: result.SessionPublicID})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Roles) != 1 || resolved.Roles[0] != security.RoleManager || !resolved.Scopes.Global {
		t.Fatalf("unexpected resolved principal: %#v", resolved)
	}
	if _, err := service.Exchange(context.Background(), "state-1234567890123456", "code-1", "http://admin.local/sso/callback"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("replayed state returned %v", err)
	}
}

func TestServiceAcceptsWeComProvider(t *testing.T) {
	repository := &adminAuthRepository{}
	service := NewService(repository, adminAuthProvider{}, &adminAuthIssuer{}, adminAuthClock{now: time.Now().UTC()}, Config{Provider: ProviderWeCom, AllowedRedirectURI: []string{"http://admin.local/sso/callback"}})
	if _, err := service.Start(context.Background(), "state-wecom-123456", "http://admin.local/sso/callback"); err != nil {
		t.Fatalf("wecom provider was rejected: %v", err)
	}
}

func TestHandlerRequiresStateCookieAndClearsItAfterExchange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &adminAuthRepository{identity: AdminIdentity{PublicID: "admin-1", Provider: ProviderOIDC, ExternalSub: "subject-1", Role: security.RoleAdmin, Enabled: true}}
	service := NewService(repository, adminAuthProvider{}, &adminAuthIssuer{}, adminAuthClock{now: time.Now().UTC()}, Config{Provider: ProviderOIDC, AllowedRedirectURI: []string{"http://admin.local/sso/callback"}})
	router := gin.New()
	RegisterRoutes(router, service, nil, false)
	start := httptest.NewRecorder()
	startRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth/sso/start?state=state-1234567890123456&redirect_uri=http%3A%2F%2Fadmin.local%2Fsso%2Fcallback", nil)
	router.ServeHTTP(start, startRequest)
	if start.Code != http.StatusFound {
		t.Fatalf("start status = %d, body=%s", start.Code, start.Body.String())
	}
	cookie := start.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "HttpOnly") {
		t.Fatalf("state cookie is not HttpOnly: %q", cookie)
	}
	exchange := httptest.NewRecorder()
	exchangeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/sso/exchange", strings.NewReader(`{"state":"state-1234567890123456","code":"code-1","redirect_uri":"http://admin.local/sso/callback"}`))
	exchangeRequest.Header.Set("Content-Type", "application/json")
	exchangeRequest.Header.Set("Cookie", cookie)
	router.ServeHTTP(exchange, exchangeRequest)
	if exchange.Code != http.StatusOK || !strings.Contains(exchange.Body.String(), "core-admin-token") {
		t.Fatalf("exchange status = %d, body=%s", exchange.Code, exchange.Body.String())
	}
	if !strings.Contains(exchange.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("exchange did not clear state cookie: %q", exchange.Header().Get("Set-Cookie"))
	}
}
