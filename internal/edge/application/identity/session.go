package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	sharedIdentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/identity"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrSessionRevoked  = errors.New("session revoked")
	ErrRefreshReuse    = errors.New("refresh token was reused")
	ErrStaffInactive   = errors.New("staff is inactive")
)

type Session struct {
	SessionPublicID           string
	ActorPublicID             string
	Client                    string
	RefreshTokenHash          string
	CreatedAt                 time.Time
	LastSeenAt                time.Time
	ExpiresAt                 time.Time
	AbsoluteExpiresAt         time.Time
	RevokedAt                 *time.Time
	ReplacedBySessionPublicID string
}

type SessionStore interface {
	CreateSession(context.Context, Session) error
	FindSession(context.Context, string) (Session, error)
	FindSessionByRefreshHash(context.Context, string) (Session, error)
	RotateSession(context.Context, string, Session, time.Time) error
	RevokeSession(context.Context, string, time.Time) error
}

type Config struct {
	AccessTokenTTL     time.Duration
	SessionTTL         time.Duration
	AbsoluteSessionTTL time.Duration
}

type Service struct {
	core           sharedIdentity.Client
	sessions       SessionStore
	issuer         security.SessionTokenIssuer
	clock          clock.Clock
	accessTokenTTL time.Duration
	sessionTTL     time.Duration
	absoluteTTL    time.Duration
}

type LoginResponse struct {
	State           string        `json:"state"`
	BindingTicket   string        `json:"binding_ticket,omitempty"`
	AccessToken     string        `json:"access_token,omitempty"`
	RefreshToken    string        `json:"refresh_token,omitempty"`
	TokenType       string        `json:"token_type,omitempty"`
	ExpiresIn       int64         `json:"expires_in,omitempty"`
	SessionPublicID string        `json:"session_public_id,omitempty"`
	Principal       PrincipalView `json:"principal,omitempty"`
}

type SessionView struct {
	SessionPublicID string        `json:"session_public_id"`
	ExpiresAt       time.Time     `json:"expires_at"`
	Principal       PrincipalView `json:"principal"`
}

type PrincipalView struct {
	PublicID string `json:"public_id"`
	Type     string `json:"type"`
	Role     string `json:"role"`
}

func NewService(core sharedIdentity.Client, sessions SessionStore, issuer security.SessionTokenIssuer, now clock.Clock, cfg Config) *Service {
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
	return &Service{core: core, sessions: sessions, issuer: issuer, clock: now, accessTokenTTL: cfg.AccessTokenTTL, sessionTTL: cfg.SessionTTL, absoluteTTL: cfg.AbsoluteSessionTTL}
}

func (s *Service) PasswordLogin(ctx context.Context, request sharedIdentity.PasswordVerifyRequest) (LoginResponse, error) {
	if s == nil || s.core == nil || s.sessions == nil || s.issuer == nil {
		return LoginResponse{}, fmt.Errorf("employee identity service is not configured")
	}
	result, err := s.core.VerifyPassword(ctx, request)
	if err != nil {
		return LoginResponse{}, err
	}
	if result.State == "binding_required" {
		return LoginResponse{State: result.State, BindingTicket: result.BindingTicket, Principal: principalView(toPrincipal(result.Staff))}, nil
	}
	return s.issueSession(ctx, result.Staff, request.Client, s.clock.Now().UTC())
}

func (s *Service) Exchange(ctx context.Context, request sharedIdentity.ExchangeRequest) (LoginResponse, error) {
	if s == nil || s.core == nil || s.sessions == nil || s.issuer == nil {
		return LoginResponse{}, fmt.Errorf("employee identity service is not configured")
	}
	staff, err := s.core.Exchange(ctx, request)
	if err != nil {
		return LoginResponse{}, err
	}
	return s.issueSession(ctx, staff, request.Client, s.clock.Now().UTC())
}

func (s *Service) CompleteBinding(ctx context.Context, request sharedIdentity.BindingRequest, client string) (LoginResponse, error) {
	if s == nil || s.core == nil || s.sessions == nil || s.issuer == nil {
		return LoginResponse{}, fmt.Errorf("employee identity service is not configured")
	}
	request.Client = client
	staff, err := s.core.CompleteBinding(ctx, request)
	if err != nil {
		return LoginResponse{}, err
	}
	return s.issueSession(ctx, staff, client, s.clock.Now().UTC())
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (LoginResponse, error) {
	if s == nil || s.sessions == nil || s.issuer == nil {
		return LoginResponse{}, fmt.Errorf("employee session service is not configured")
	}
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return LoginResponse{}, ErrRefreshReuse
	}
	now := s.clock.Now().UTC()
	old, err := s.sessions.FindSessionByRefreshHash(ctx, HashRefreshToken(refreshToken))
	if err != nil {
		return LoginResponse{}, err
	}
	if old.RevokedAt != nil {
		_ = s.sessions.RotateSession(ctx, old.RefreshTokenHash, Session{}, now)
		return LoginResponse{}, ErrRefreshReuse
	}
	if !old.ExpiresAt.After(now) || !old.AbsoluteExpiresAt.After(now) {
		return LoginResponse{}, ErrSessionExpired
	}
	newSession, newRefreshToken, err := newSession(old.ActorPublicID, old.Client, now, minTime(now.Add(s.sessionTTL), old.AbsoluteExpiresAt))
	if err != nil {
		return LoginResponse{}, err
	}
	newSession.AbsoluteExpiresAt = old.AbsoluteExpiresAt
	if err := s.sessions.RotateSession(ctx, old.RefreshTokenHash, newSession, now); err != nil {
		return LoginResponse{}, err
	}
	return s.issueSessionResponse(newSession, newRefreshToken)
}

func (s *Service) Logout(ctx context.Context, principal security.Principal) error {
	if s == nil || s.sessions == nil || principal.SessionID == "" {
		return ErrSessionExpired
	}
	return s.sessions.RevokeSession(ctx, principal.SessionID, s.clock.Now().UTC())
}

func (s *Service) Me(ctx context.Context, principal security.Principal) (SessionView, error) {
	if err := s.ValidatePrincipal(ctx, principal); err != nil {
		return SessionView{}, err
	}
	session, err := s.sessions.FindSession(ctx, principal.SessionID)
	if err != nil {
		return SessionView{}, err
	}
	return SessionView{SessionPublicID: session.SessionPublicID, ExpiresAt: session.ExpiresAt.UTC(), Principal: principalView(principal)}, nil
}

func (s *Service) ResolvePrincipal(ctx context.Context, principal security.Principal) (security.Principal, error) {
	if err := s.ValidatePrincipal(ctx, principal); err != nil {
		return security.Principal{}, err
	}
	// Employee sessions are issued only for the staff client boundary. The
	// access JWT deliberately does not carry a permission catalog, so restore
	// the stable display role after the session and current Staff are checked.
	if len(principal.Roles) == 0 {
		principal.Roles = []string{security.RoleStaff}
	}
	return principal, nil
}

func (s *Service) ValidatePrincipal(ctx context.Context, principal security.Principal) error {
	if s == nil || s.core == nil || s.sessions == nil || principal.Type != security.HumanPrincipal || principal.PublicID == "" || principal.SessionID == "" {
		return ErrSessionExpired
	}
	session, err := s.sessions.FindSession(ctx, principal.SessionID)
	if err != nil {
		return err
	}
	now := s.clock.Now().UTC()
	if session.ActorPublicID != principal.PublicID {
		return ErrSessionExpired
	}
	staff, err := s.core.FindStaff(ctx, principal.PublicID)
	if err != nil {
		var remoteErr sharedIdentity.RemoteError
		if errors.As(err, &remoteErr) && remoteErr.Code == "staff_inactive" {
			return ErrStaffInactive
		}
		return err
	}
	if staff.PublicID != principal.PublicID {
		return ErrSessionExpired
	}
	if session.RevokedAt != nil {
		return ErrSessionRevoked
	}
	if !session.ExpiresAt.After(now) || !session.AbsoluteExpiresAt.After(now) {
		return ErrSessionExpired
	}
	return nil
}

func (s *Service) issueSession(ctx context.Context, value sharedIdentity.Staff, client string, now time.Time) (LoginResponse, error) {
	if s == nil || s.sessions == nil || s.issuer == nil || strings.TrimSpace(value.PublicID) == "" || (client != "employee-miniapp" && client != "employee-web") {
		return LoginResponse{}, fmt.Errorf("invalid employee session subject")
	}
	absoluteExpiresAt := now.Add(s.absoluteTTL)
	session, refreshToken, err := newSession(value.PublicID, client, now, minTime(now.Add(s.sessionTTL), absoluteExpiresAt))
	if err != nil {
		return LoginResponse{}, err
	}
	session.AbsoluteExpiresAt = absoluteExpiresAt
	if err := s.sessions.CreateSession(ctx, session); err != nil {
		return LoginResponse{}, err
	}
	return s.issueSessionResponse(session, refreshToken)
}

func (s *Service) issueSessionResponse(session Session, refreshToken string) (LoginResponse, error) {
	principal := security.Principal{Type: security.HumanPrincipal, PublicID: session.ActorPublicID, Subject: session.ActorPublicID, SessionID: session.SessionPublicID, Roles: []string{"staff"}}
	accessToken, err := s.issuer.IssueForSession(principal, session.SessionPublicID, s.accessTokenTTL)
	if err != nil {
		return LoginResponse{}, err
	}
	return LoginResponse{State: "authenticated", AccessToken: accessToken, RefreshToken: refreshToken, TokenType: "Bearer", ExpiresIn: int64(s.accessTokenTTL / time.Second), SessionPublicID: session.SessionPublicID, Principal: principalView(principal)}, nil
}

func newSession(actorPublicID, client string, now, expiresAt time.Time) (Session, string, error) {
	sessionID, err := id.NewPublicID()
	if err != nil {
		return Session{}, "", err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return Session{}, "", fmt.Errorf("generate refresh token entropy: %w", err)
	}
	refreshToken := base64.RawURLEncoding.EncodeToString(secret)
	return Session{SessionPublicID: sessionID, ActorPublicID: actorPublicID, Client: client, RefreshTokenHash: HashRefreshToken(refreshToken), CreatedAt: now, LastSeenAt: now, ExpiresAt: expiresAt, AbsoluteExpiresAt: expiresAt}, refreshToken, nil
}

func toPrincipal(value sharedIdentity.Staff) security.Principal {
	role := value.Role
	if role == "" {
		role = "staff"
	}
	return security.Principal{Type: security.HumanPrincipal, PublicID: value.PublicID, Subject: value.PublicID, Roles: []string{role}}
}

func principalView(value security.Principal) PrincipalView {
	role := ""
	if len(value.Roles) > 0 {
		role = value.Roles[0]
	}
	return PrincipalView{PublicID: value.PublicID, Type: string(value.Type), Role: role}
}

func HashRefreshToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

type MemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]Session
	byHash   map[string]string
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{sessions: make(map[string]Session), byHash: make(map[string]string)}
}

func (s *MemorySessionStore) CreateSession(ctx context.Context, value Session) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if value.SessionPublicID == "" || value.ActorPublicID == "" || value.RefreshTokenHash == "" {
		return fmt.Errorf("session identity fields are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[value.SessionPublicID]; exists {
		return fmt.Errorf("session already exists")
	}
	if _, exists := s.byHash[value.RefreshTokenHash]; exists {
		return fmt.Errorf("refresh token already exists")
	}
	s.sessions[value.SessionPublicID] = value
	s.byHash[value.RefreshTokenHash] = value.SessionPublicID
	return nil
}

func (s *MemorySessionStore) FindSession(ctx context.Context, sessionID string) (Session, error) {
	if err := contextError(ctx); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return value, nil
}

func (s *MemorySessionStore) FindSessionByRefreshHash(ctx context.Context, hash string) (Session, error) {
	if err := contextError(ctx); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sessionID, ok := s.byHash[hash]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	value, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return value, nil
}

func (s *MemorySessionStore) RotateSession(ctx context.Context, refreshHash string, replacement Session, now time.Time) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sessionID, ok := s.byHash[refreshHash]
	if !ok {
		return ErrSessionNotFound
	}
	old := s.sessions[sessionID]
	if old.RevokedAt != nil {
		if old.ReplacedBySessionPublicID != "" {
			revoked := now.UTC()
			if next, exists := s.sessions[old.ReplacedBySessionPublicID]; exists {
				next.RevokedAt = &revoked
				s.sessions[next.SessionPublicID] = next
			}
		}
		return ErrRefreshReuse
	}
	if replacement.SessionPublicID == "" {
		return ErrSessionExpired
	}
	revoked := now.UTC()
	old.RevokedAt = &revoked
	old.ReplacedBySessionPublicID = replacement.SessionPublicID
	s.sessions[old.SessionPublicID] = old
	s.sessions[replacement.SessionPublicID] = replacement
	s.byHash[replacement.RefreshTokenHash] = replacement.SessionPublicID
	return nil
}

func (s *MemorySessionStore) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	if value.RevokedAt == nil {
		revoked := now.UTC()
		value.RevokedAt = &revoked
		s.sessions[sessionID] = value
	}
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
