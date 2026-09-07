// Package adminauth owns the management-console SSO boundary and Core session
// validation. External identity providers are adapters; roles and scopes are
// resolved from Core-owned admin identity records before any business API is
// reached.
package adminauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const (
	ProviderOIDC        = "oidc"
	ProviderDevelopment = "development"
	DefaultStateTTL     = 10 * time.Minute
)

var (
	ErrInvalidRequest   = errors.New("invalid admin SSO request")
	ErrSSOUnavailable   = errors.New("admin SSO provider is unavailable")
	ErrInvalidState     = errors.New("admin SSO state is invalid")
	ErrCodeInvalid      = errors.New("admin SSO authorization code is invalid")
	ErrIdentityUnmapped = errors.New("admin SSO identity is not provisioned")
	ErrAdminInactive    = errors.New("admin identity is inactive")
	ErrSessionNotFound  = errors.New("admin session not found")
	ErrSessionExpired   = errors.New("admin session is expired")
	ErrSessionRevoked   = errors.New("admin session is revoked")
)

type AdminIdentity struct {
	PublicID    string
	Provider    string
	ExternalSub string
	DisplayName string
	Role        string
	Enabled     bool
	AccessScope security.AccessScope
}

type ExternalIdentity struct {
	Provider    string
	ExternalSub string
	DisplayName string
}

type SSOState struct {
	PublicID    string
	StateHash   string
	Provider    string
	RedirectURI string
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
}

type Session struct {
	PublicID              string
	AdminIdentityPublicID string
	CreatedAt             time.Time
	LastSeenAt            time.Time
	ExpiresAt             time.Time
	AbsoluteExpiresAt     time.Time
	RevokedAt             *time.Time
	Identity              AdminIdentity
}

type Repository interface {
	CreateSSOState(context.Context, SSOState) error
	ConsumeSSOState(context.Context, string, time.Time) (SSOState, error)
	FindIdentity(context.Context, string, string) (AdminIdentity, error)
	CreateSession(context.Context, Session) error
	FindSession(context.Context, string) (Session, error)
	RevokeSession(context.Context, string, time.Time) error
}

type Provider interface {
	Begin(context.Context, string, string, string) (string, error)
	Exchange(context.Context, string, string) (ExternalIdentity, error)
}

type DisabledProvider struct{}

func (DisabledProvider) Begin(context.Context, string, string, string) (string, error) {
	return "", ErrSSOUnavailable
}

func (DisabledProvider) Exchange(context.Context, string, string) (ExternalIdentity, error) {
	return ExternalIdentity{}, ErrSSOUnavailable
}

type Config struct {
	AccessTokenTTL     time.Duration
	SessionTTL         time.Duration
	AbsoluteSessionTTL time.Duration
	StateTTL           time.Duration
	Provider           string
	StateCookieName    string
	SecureCookie       bool
	AllowedRedirectURI []string
}

type Service struct {
	repository Repository
	provider   Provider
	issuer     security.SessionTokenIssuer
	clock      clock.Clock
	cfg        Config
}

type StartResult struct {
	AuthorizationURL string
}

type PrincipalView struct {
	PublicID string `json:"public_id"`
	Type     string `json:"type"`
	Role     string `json:"role"`
}

type ExchangeResult struct {
	State           string        `json:"state"`
	AccessToken     string        `json:"access_token"`
	TokenType       string        `json:"token_type"`
	ExpiresIn       int64         `json:"expires_in"`
	SessionPublicID string        `json:"session_public_id"`
	Principal       PrincipalView `json:"principal"`
}

type SessionView struct {
	SessionPublicID string        `json:"session_public_id"`
	ExpiresAt       time.Time     `json:"expires_at"`
	Principal       PrincipalView `json:"principal"`
}

func NewService(repository Repository, provider Provider, issuer security.SessionTokenIssuer, now clock.Clock, cfg Config) *Service {
	if provider == nil {
		provider = DisabledProvider{}
	}
	if now == nil {
		now = clock.Real{}
	}
	if cfg.AccessTokenTTL <= 0 {
		cfg.AccessTokenTTL = 15 * time.Minute
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 30 * 24 * time.Hour
	}
	if cfg.AbsoluteSessionTTL <= 0 {
		cfg.AbsoluteSessionTTL = 90 * 24 * time.Hour
	}
	if cfg.StateTTL <= 0 {
		cfg.StateTTL = DefaultStateTTL
	}
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	if cfg.Provider == "" {
		cfg.Provider = ProviderWeCom
	}
	if strings.TrimSpace(cfg.StateCookieName) == "" {
		cfg.StateCookieName = "flight.admin.sso.state"
	}
	return &Service{repository: repository, provider: provider, issuer: issuer, clock: now, cfg: cfg}
}

func (s *Service) Start(ctx context.Context, state, redirectURI string) (StartResult, error) {
	if s == nil || s.repository == nil || s.provider == nil {
		return StartResult{}, ErrSSOUnavailable
	}
	state = strings.TrimSpace(state)
	redirectURI = strings.TrimSpace(redirectURI)
	if !validProvider(s.cfg.Provider) || len(state) < 16 || len(state) > 256 || !validRedirectURI(redirectURI, s.cfg.AllowedRedirectURI) {
		return StartResult{}, ErrInvalidRequest
	}
	now := s.clock.Now().UTC()
	stateID, err := id.NewPublicID()
	if err != nil {
		return StartResult{}, fmt.Errorf("generate admin SSO state ID: %w", err)
	}
	stateRecord := SSOState{PublicID: stateID, StateHash: HashState(state), Provider: s.cfg.Provider, RedirectURI: redirectURI, ExpiresAt: now.Add(s.cfg.StateTTL)}
	if err := s.repository.CreateSSOState(ctx, stateRecord); err != nil {
		return StartResult{}, fmt.Errorf("create admin SSO state: %w", err)
	}
	authorizationURL, err := s.provider.Begin(ctx, state, redirectURI, s.cfg.Provider)
	if err != nil {
		return StartResult{}, err
	}
	if !validHTTPURL(authorizationURL) {
		return StartResult{}, fmt.Errorf("%w: provider returned an invalid authorization URL", ErrSSOUnavailable)
	}
	return StartResult{AuthorizationURL: authorizationURL}, nil
}

func (s *Service) Exchange(ctx context.Context, state, code, redirectURI string) (ExchangeResult, error) {
	if s == nil || s.repository == nil || s.provider == nil || s.issuer == nil {
		return ExchangeResult{}, ErrSSOUnavailable
	}
	state = strings.TrimSpace(state)
	code = strings.TrimSpace(code)
	redirectURI = strings.TrimSpace(redirectURI)
	if state == "" || code == "" || !validRedirectURI(redirectURI, s.cfg.AllowedRedirectURI) {
		return ExchangeResult{}, ErrInvalidRequest
	}
	now := s.clock.Now().UTC()
	record, err := s.repository.ConsumeSSOState(ctx, HashState(state), now)
	if err != nil {
		return ExchangeResult{}, err
	}
	if record.Provider != s.cfg.Provider || record.RedirectURI != redirectURI {
		return ExchangeResult{}, ErrInvalidState
	}
	external, err := s.provider.Exchange(ctx, code, redirectURI)
	if err != nil {
		return ExchangeResult{}, err
	}
	if external.Provider == "" {
		external.Provider = record.Provider
	}
	if external.Provider != record.Provider || strings.TrimSpace(external.ExternalSub) == "" {
		return ExchangeResult{}, ErrCodeInvalid
	}
	identity, err := s.repository.FindIdentity(ctx, external.Provider, external.ExternalSub)
	if err != nil {
		if errors.Is(err, ErrIdentityUnmapped) {
			return ExchangeResult{}, err
		}
		return ExchangeResult{}, fmt.Errorf("resolve admin identity: %w", err)
	}
	if !identity.Enabled {
		return ExchangeResult{}, ErrAdminInactive
	}
	if !IsValidAdminRole(identity.Role) {
		return ExchangeResult{}, ErrAdminInactive
	}
	session, err := s.newSession(identity, now)
	if err != nil {
		return ExchangeResult{}, err
	}
	if err := s.repository.CreateSession(ctx, session); err != nil {
		return ExchangeResult{}, fmt.Errorf("create admin session: %w", err)
	}
	principal := principalForSession(session)
	accessToken, err := s.issuer.IssueForSession(principal, session.PublicID, s.cfg.AccessTokenTTL)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("issue admin access token: %w", err)
	}
	return ExchangeResult{State: "authenticated", AccessToken: accessToken, TokenType: "Bearer", ExpiresIn: int64(s.cfg.AccessTokenTTL / time.Second), SessionPublicID: session.PublicID, Principal: principalView(principal)}, nil
}

func (s *Service) ResolvePrincipal(ctx context.Context, principal security.Principal) (security.Principal, error) {
	resolved, _, err := s.resolveSession(ctx, principal)
	return resolved, err
}

func (s *Service) resolveSession(ctx context.Context, principal security.Principal) (security.Principal, Session, error) {
	if s == nil || s.repository == nil || principal.Type != security.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" || strings.TrimSpace(principal.SessionID) == "" {
		return security.Principal{}, Session{}, ErrSessionExpired
	}
	session, err := s.repository.FindSession(ctx, principal.SessionID)
	if err != nil {
		return security.Principal{}, Session{}, err
	}
	now := s.clock.Now().UTC()
	if session.AdminIdentityPublicID != principal.PublicID || session.RevokedAt != nil || !session.ExpiresAt.After(now) || !session.AbsoluteExpiresAt.After(now) {
		return security.Principal{}, Session{}, ErrSessionExpired
	}
	if !session.Identity.Enabled {
		return security.Principal{}, Session{}, ErrAdminInactive
	}
	principal.Roles = []string{session.Identity.Role}
	principal.Scopes = session.Identity.AccessScope
	principal.Subject = principal.PublicID
	return principal, session, nil
}

func (s *Service) Me(ctx context.Context, principal security.Principal) (SessionView, error) {
	resolved, session, err := s.resolveSession(ctx, principal)
	if err != nil {
		return SessionView{}, err
	}
	return SessionView{SessionPublicID: resolved.SessionID, ExpiresAt: session.ExpiresAt.UTC(), Principal: principalView(resolved)}, nil
}

func (s *Service) Logout(ctx context.Context, principal security.Principal) error {
	if s == nil || s.repository == nil || principal.SessionID == "" {
		return ErrSessionExpired
	}
	if _, err := s.ResolvePrincipal(ctx, principal); err != nil {
		return err
	}
	return s.repository.RevokeSession(ctx, principal.SessionID, s.clock.Now().UTC())
}

func (s *Service) StateCookieName() string { return s.cfg.StateCookieName }

func (s *Service) SecureCookie() bool { return s.cfg.SecureCookie }

func (s *Service) StateMaxAge() int { return int(s.cfg.StateTTL / time.Second) }

func (s *Service) newSession(identity AdminIdentity, now time.Time) (Session, error) {
	sessionID, err := id.NewPublicID()
	if err != nil {
		return Session{}, fmt.Errorf("generate admin session ID: %w", err)
	}
	absolute := now.Add(s.cfg.AbsoluteSessionTTL)
	return Session{PublicID: sessionID, AdminIdentityPublicID: identity.PublicID, CreatedAt: now, LastSeenAt: now, ExpiresAt: minTime(now.Add(s.cfg.SessionTTL), absolute), AbsoluteExpiresAt: absolute, Identity: identity}, nil
}

func principalForSession(session Session) security.Principal {
	return security.Principal{Type: security.HumanPrincipal, PublicID: session.AdminIdentityPublicID, Subject: session.AdminIdentityPublicID, SessionID: session.PublicID, Roles: []string{session.Identity.Role}, Scopes: session.Identity.AccessScope}
}

func principalView(principal security.Principal) PrincipalView {
	role := ""
	if len(principal.Roles) > 0 {
		role = principal.Roles[0]
	}
	return PrincipalView{PublicID: principal.PublicID, Type: string(principal.Type), Role: role}
}

func IsValidAdminRole(role string) bool {
	switch strings.TrimSpace(role) {
	case security.RoleAdmin, security.RoleManager, security.RoleLeader, security.RoleSupervisor:
		return true
	default:
		return false
	}
}

func validRedirectURI(value string, allowed []string) bool {
	if !validHTTPURL(value) {
		return false
	}
	for _, candidate := range allowed {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func HashState(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func validProvider(value string) bool {
	return value == ProviderOIDC || value == ProviderWeCom || value == ProviderDevelopment
}
