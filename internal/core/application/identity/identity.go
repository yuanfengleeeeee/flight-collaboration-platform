// Package identity contains the Core-owned employee credential and external
// identity binding use cases. Edge never reads these tables directly.
package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
)

const (
	ClientEmployeeMiniapp = "employee-miniapp"
	ClientEmployeeWeb     = "employee-web"

	ProviderPersonalWechat = "personal_wechat"
	ProviderWeCom          = "wecom"

	StateAuthenticated   = "authenticated"
	StateBindingRequired = "binding_required"

	DefaultBindingTicketTTL = 10 * time.Minute
	DefaultMaxLoginAttempts = 5
	DefaultLockoutDuration  = 15 * time.Minute
)

var (
	ErrInvalidCredentials      = errors.New("invalid employee credentials")
	ErrStaffInactive           = errors.New("staff is inactive")
	ErrIdentityUnmapped        = errors.New("external identity is not mapped")
	ErrIdentityBindingConflict = errors.New("external identity binding conflicts")
	ErrBindingTicketInvalid    = errors.New("binding ticket is invalid")
	ErrProviderCodeInvalid     = errors.New("provider code is invalid")
	ErrUnsupportedProvider     = errors.New("identity provider is unsupported")
	ErrNotFound                = errors.New("identity record not found")
)

type Staff struct {
	PublicID    string `json:"public_id"`
	EmployeeNo  string `json:"employee_no"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type ExternalIdentity struct {
	Provider        string
	ProviderApp     string
	ExternalSubject string
}

type PasswordLoginInput struct {
	EmployeeNo  string
	Password    string
	Client      string
	Provider    string
	ProviderApp string
}

type PasswordLoginResult struct {
	State         string `json:"state"`
	Staff         Staff  `json:"staff"`
	BindingTicket string `json:"binding_ticket,omitempty"`
}

type Repository interface {
	FindStaff(context.Context, string) (Staff, error)
	VerifyPassword(context.Context, string, string, time.Time, int, time.Duration) (Staff, error)
	FindBindingForStaff(context.Context, string, string, string) (Staff, error)
	FindBindingByExternal(context.Context, ExternalIdentity) (Staff, error)
	IssueBindingTicket(context.Context, string, string, time.Time, time.Duration) (string, error)
	CompleteBinding(context.Context, string, string, ExternalIdentity, time.Time) (Staff, error)
}

type ProviderVerifier interface {
	Verify(context.Context, string, string, string) (ExternalIdentity, error)
}

// ProviderAppResolver lets a verifier keep the provider application scope on
// the server. The client may select a provider, but it must not select which
// AppID/CorpID owns the external identity binding.
type ProviderAppResolver interface {
	ProviderApp(string) string
}

type Service struct {
	repository       Repository
	providerVerifier ProviderVerifier
	clock            clock.Clock
	maxLoginAttempts int
	lockoutDuration  time.Duration
	ticketTTL        time.Duration
}

type ServiceConfig struct {
	MaxLoginAttempts int
	LockoutDuration  time.Duration
	BindingTicketTTL time.Duration
}

func NewService(repository Repository, providerVerifier ProviderVerifier, now clock.Clock, cfg ServiceConfig) *Service {
	if now == nil {
		now = clock.Real{}
	}
	if cfg.MaxLoginAttempts <= 0 {
		cfg.MaxLoginAttempts = DefaultMaxLoginAttempts
	}
	if cfg.LockoutDuration <= 0 {
		cfg.LockoutDuration = DefaultLockoutDuration
	}
	if cfg.BindingTicketTTL <= 0 {
		cfg.BindingTicketTTL = DefaultBindingTicketTTL
	}
	return &Service{repository: repository, providerVerifier: providerVerifier, clock: now, maxLoginAttempts: cfg.MaxLoginAttempts, lockoutDuration: cfg.LockoutDuration, ticketTTL: cfg.BindingTicketTTL}
}

func (s *Service) PasswordLogin(ctx context.Context, input PasswordLoginInput) (PasswordLoginResult, error) {
	if s == nil || s.repository == nil {
		return PasswordLoginResult{}, fmt.Errorf("identity repository is not configured")
	}
	input.EmployeeNo = strings.TrimSpace(input.EmployeeNo)
	input.Client = strings.TrimSpace(input.Client)
	input.Provider = strings.TrimSpace(input.Provider)
	input.ProviderApp = strings.TrimSpace(input.ProviderApp)
	if input.EmployeeNo == "" || input.Password == "" || !validClient(input.Client) {
		return PasswordLoginResult{}, ErrInvalidCredentials
	}
	if input.Provider != "" && !validProvider(input.Provider) {
		return PasswordLoginResult{}, ErrUnsupportedProvider
	}
	now := s.clock.Now().UTC()
	staff, err := s.repository.VerifyPassword(ctx, input.EmployeeNo, input.Password, now, s.maxLoginAttempts, s.lockoutDuration)
	if err != nil {
		return PasswordLoginResult{}, err
	}
	if staff.Role == "" {
		staff.Role = "staff"
	}
	if input.Provider == "" {
		return PasswordLoginResult{State: StateAuthenticated, Staff: staff}, nil
	}
	if input.ProviderApp == "" {
		input.ProviderApp = "local"
	}
	if resolver, ok := s.providerVerifier.(ProviderAppResolver); ok {
		if providerApp := strings.TrimSpace(resolver.ProviderApp(input.Provider)); providerApp != "" {
			input.ProviderApp = providerApp
		}
	}
	if _, err := s.repository.FindBindingForStaff(ctx, staff.PublicID, input.Provider, input.ProviderApp); err == nil {
		return PasswordLoginResult{State: StateAuthenticated, Staff: staff}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return PasswordLoginResult{}, err
	}
	ticket, err := s.repository.IssueBindingTicket(ctx, staff.PublicID, input.Client, now, s.ticketTTL)
	if err != nil {
		return PasswordLoginResult{}, err
	}
	return PasswordLoginResult{State: StateBindingRequired, Staff: staff, BindingTicket: ticket}, nil
}

func (s *Service) FindStaff(ctx context.Context, staffPublicID string) (Staff, error) {
	if s == nil || s.repository == nil {
		return Staff{}, fmt.Errorf("identity repository is not configured")
	}
	staff, err := s.repository.FindStaff(ctx, strings.TrimSpace(staffPublicID))
	if errors.Is(err, ErrNotFound) {
		return Staff{}, ErrStaffInactive
	}
	return staff, err
}

func (s *Service) Exchange(ctx context.Context, provider, providerCode, client, redirectURI string) (Staff, error) {
	if s == nil || s.repository == nil || s.providerVerifier == nil {
		return Staff{}, fmt.Errorf("identity provider is not configured")
	}
	provider = strings.TrimSpace(provider)
	client = strings.TrimSpace(client)
	if !validProvider(provider) || !validClient(client) {
		return Staff{}, ErrUnsupportedProvider
	}
	identity, err := s.providerVerifier.Verify(ctx, provider, strings.TrimSpace(providerCode), strings.TrimSpace(redirectURI))
	if err != nil {
		return Staff{}, err
	}
	staff, err := s.repository.FindBindingByExternal(ctx, identity)
	if errors.Is(err, ErrNotFound) {
		return Staff{}, ErrIdentityUnmapped
	}
	return staff, err
}

func (s *Service) CompleteBinding(ctx context.Context, ticket, provider, providerCode, client, redirectURI string) (Staff, error) {
	if s == nil || s.repository == nil || s.providerVerifier == nil {
		return Staff{}, fmt.Errorf("identity provider is not configured")
	}
	provider = strings.TrimSpace(provider)
	client = strings.TrimSpace(client)
	if !validProvider(provider) || strings.TrimSpace(ticket) == "" || !validClient(client) {
		return Staff{}, ErrBindingTicketInvalid
	}
	identity, err := s.providerVerifier.Verify(ctx, provider, strings.TrimSpace(providerCode), strings.TrimSpace(redirectURI))
	if err != nil {
		return Staff{}, err
	}
	return s.repository.CompleteBinding(ctx, ticket, client, identity, s.clock.Now().UTC())
}

func validClient(value string) bool {
	return value == ClientEmployeeMiniapp || value == ClientEmployeeWeb
}

func validProvider(value string) bool {
	return value == ProviderPersonalWechat || value == ProviderWeCom
}

// DevelopmentProviderVerifier accepts only an explicit mock code. It prevents
// local tests from pretending that a real WeChat or WeCom integration exists.
type DevelopmentProviderVerifier struct{}

func (DevelopmentProviderVerifier) ProviderApp(string) string { return "local" }

func (DevelopmentProviderVerifier) Verify(_ context.Context, provider, code, _ string) (ExternalIdentity, error) {
	if !validProvider(provider) || !strings.HasPrefix(code, "mock:") {
		return ExternalIdentity{}, ErrProviderCodeInvalid
	}
	subject := strings.TrimPrefix(code, "mock:")
	if subject == "" || strings.Contains(subject, ":") {
		return ExternalIdentity{}, ErrProviderCodeInvalid
	}
	return ExternalIdentity{Provider: provider, ProviderApp: "local", ExternalSubject: subject}, nil
}

type DisabledProviderVerifier struct{}

func (DisabledProviderVerifier) ProviderApp(string) string { return "" }

func (DisabledProviderVerifier) Verify(context.Context, string, string, string) (ExternalIdentity, error) {
	return ExternalIdentity{}, ErrProviderCodeInvalid
}

// HashBindingTicket is used by the Core persistence adapter so the raw
// one-time ticket never has to be stored in the database.
func HashBindingTicket(ticket string) string {
	digest := sha256.Sum256([]byte(ticket))
	return hex.EncodeToString(digest[:])
}
