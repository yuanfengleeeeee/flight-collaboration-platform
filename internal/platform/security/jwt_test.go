package security

import (
	"testing"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
)

func TestJWTContainsIdentityClaimsButNotRolePermissions(t *testing.T) {
	authenticator, err := NewJWTAuthenticator(config.JWTConfig{Secret: "local-development-secret", Issuer: "core", Audience: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authenticator.Issue(Principal{Type: HumanPrincipal, PublicID: "employee-public-id", Roles: []string{"admin"}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authenticator.AuthenticateToken("Bearer " + token)
	if err != nil {
		t.Fatal(err)
	}
	if principal.PublicID != "employee-public-id" || principal.SessionID == "" || len(principal.Roles) != 0 {
		t.Fatalf("unexpected principal: %#v", principal)
	}
}

func TestJWTRejectsWrongIssuer(t *testing.T) {
	authenticator, err := NewJWTAuthenticator(config.JWTConfig{Secret: "local-development-secret", Issuer: "core", Audience: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authenticator.Issue(Principal{Type: HumanPrincipal, PublicID: "employee-public-id"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewJWTAuthenticator(config.JWTConfig{Secret: "local-development-secret", Issuer: "other", Audience: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.AuthenticateToken(token); err == nil {
		t.Fatal("expected issuer validation failure")
	}
}
