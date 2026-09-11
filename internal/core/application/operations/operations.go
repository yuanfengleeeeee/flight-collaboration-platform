// Package operations contains the read-only Core models used by the
// operations, governance, scope and diagnostics workbenches.
package operations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

var (
	ErrRepositoryNotConfigured = errors.New("operations repository is not configured")
	ErrForbidden               = errors.New("principal is not allowed to read operations data")
	ErrInvalidInput            = errors.New("invalid operations query")
)

type PageQuery struct {
	Page     int
	PageSize int
}

type StatusFilter struct {
	WorkState    string
	TeamPublicID string
	AreaPublicID string
	PageQuery
	Scope security.AccessScope
}

type StatusHistoryFilter struct {
	PersonnelPublicID string
	PageQuery
	Scope security.AccessScope
}

type EventFilter struct {
	EventType      string
	Status         string
	FlightPublicID string
	From           time.Time
	To             time.Time
	PageQuery
	Scope security.AccessScope
}

type AuditFilter struct {
	ActorID      string
	Action       string
	ResourceType string
	From         time.Time
	To           time.Time
	PageQuery
}

type PersonnelStatus struct {
	PublicID           string `json:"public_id"`
	EmployeeNo         string `json:"employee_no"`
	DisplayName        string `json:"display_name"`
	AreaPublicID       string `json:"area_public_id"`
	AreaName           string `json:"area_name"`
	TeamPublicID       string `json:"team_public_id"`
	TeamName           string `json:"team_name"`
	PositionCode       string `json:"position_code"`
	CapabilityCode     string `json:"capability_code"`
	WorkState          string `json:"work_state"`
	StatusVersion      uint64 `json:"status_version"`
	LastStateChangedAt string `json:"last_state_changed_at"`
	UnavailableReason  string `json:"unavailable_reason,omitempty"`
	Enabled            bool   `json:"enabled"`
}

type StatusHistory struct {
	PublicID          string  `json:"public_id"`
	PersonnelPublicID string  `json:"personnel_public_id"`
	StatusVersion     uint64  `json:"status_version"`
	FromState         *string `json:"from_state,omitempty"`
	ToState           string  `json:"to_state"`
	Reason            string  `json:"reason,omitempty"`
	ActorType         string  `json:"actor_type"`
	ActorPublicID     string  `json:"actor_public_id,omitempty"`
	AssignmentID      *uint64 `json:"assignment_id,omitempty"`
	CommandID         string  `json:"command_id,omitempty"`
	OccurredAt        string  `json:"occurred_at"`
}

type Event struct {
	PublicID          string `json:"public_id"`
	EventType         string `json:"event_type"`
	Status            string `json:"status"`
	AggregateType     string `json:"aggregate_type"`
	AggregatePublicID string `json:"aggregate_public_id"`
	FlightPublicID    string `json:"flight_public_id,omitempty"`
	FlightDisplayNo   string `json:"flight_display_no,omitempty"`
	Source            string `json:"source"`
	OccurredAt        string `json:"occurred_at"`
	LastError         string `json:"last_error,omitempty"`
}

type AuditEntry struct {
	ID           uint64 `json:"id"`
	ActorType    string `json:"actor_type"`
	ActorID      string `json:"actor_id"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Result       string `json:"result"`
	RequestID    string `json:"request_id"`
	TraceID      string `json:"trace_id"`
	SourceIP     string `json:"source_ip"`
	OccurredAt   string `json:"occurred_at"`
}

type StatusListResult struct {
	Items    []PersonnelStatus `json:"items"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int64             `json:"total"`
}

type StatusHistoryListResult struct {
	Items    []StatusHistory `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int64           `json:"total"`
}

type EventListResult struct {
	Items    []Event `json:"items"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
	Total    int64   `json:"total"`
}

type AuditListResult struct {
	Items    []AuditEntry `json:"items"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Total    int64        `json:"total"`
}

type ScopeView struct {
	PrincipalPublicID string   `json:"principal_public_id"`
	Roles             []string `json:"roles"`
	Global            bool     `json:"global"`
	AreaIDs           []uint64 `json:"area_ids"`
	TeamIDs           []uint64 `json:"team_ids"`
}

type RuntimeInfo struct {
	Environment             string `json:"environment"`
	RedisEnabled            bool   `json:"redis_enabled"`
	FlightSourceConfigured  bool   `json:"flight_source_configured"`
	RealIdentityProvider    bool   `json:"real_identity_provider"`
	DevelopmentActorHeaders bool   `json:"development_actor_headers"`
}

type SyncDiagnostics struct {
	FlightSourcePending int64 `json:"flight_source_pending"`
	FlightSourceRetry   int64 `json:"flight_source_retry"`
	FlightSourceFailed  int64 `json:"flight_source_failed"`
	OutboxPending       int64 `json:"outbox_pending"`
	OutboxFailed        int64 `json:"outbox_failed"`
	CoreInboxFailed     int64 `json:"core_inbox_failed"`
}

type Diagnostics struct {
	Component         string          `json:"component"`
	GeneratedAt       string          `json:"generated_at"`
	DatabaseReachable bool            `json:"database_reachable"`
	Runtime           RuntimeInfo     `json:"runtime"`
	Sync              SyncDiagnostics `json:"sync"`
}

type Repository interface {
	ListPersonnelStatus(context.Context, StatusFilter) ([]PersonnelStatus, int64, error)
	ListPersonnelStatusHistory(context.Context, StatusHistoryFilter) ([]StatusHistory, int64, error)
	ListEvents(context.Context, EventFilter) ([]Event, int64, error)
	ListAudit(context.Context, AuditFilter) ([]AuditEntry, int64, error)
	GetDiagnostics(context.Context) (SyncDiagnostics, error)
}

type Service struct {
	repository Repository
	authorizer security.Authorizer
	runtime    RuntimeInfo
}

func NewService(repository Repository, authorizer security.Authorizer, runtime RuntimeInfo) *Service {
	return &Service{repository: repository, authorizer: authorizer, runtime: runtime}
}

func (s *Service) ListPersonnelStatus(ctx context.Context, principal security.Principal, filter StatusFilter) (StatusListResult, error) {
	if err := s.authorize(principal, "status:read"); err != nil {
		return StatusListResult{}, err
	}
	filter.Scope = principal.Scopes
	page, err := normalizePage(filter.PageQuery)
	if err != nil {
		return StatusListResult{}, err
	}
	filter.PageQuery = page
	items, total, err := s.repository.ListPersonnelStatus(ctx, filter)
	if err != nil {
		return StatusListResult{}, fmt.Errorf("list personnel status: %w", err)
	}
	return StatusListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (s *Service) ListPersonnelStatusHistory(ctx context.Context, principal security.Principal, filter StatusHistoryFilter) (StatusHistoryListResult, error) {
	if err := s.authorize(principal, "status:read"); err != nil {
		return StatusHistoryListResult{}, err
	}
	filter.PersonnelPublicID = strings.TrimSpace(filter.PersonnelPublicID)
	if filter.PersonnelPublicID == "" {
		return StatusHistoryListResult{}, ErrInvalidInput
	}
	filter.Scope = principal.Scopes
	page, err := normalizePage(filter.PageQuery)
	if err != nil {
		return StatusHistoryListResult{}, err
	}
	filter.PageQuery = page
	items, total, err := s.repository.ListPersonnelStatusHistory(ctx, filter)
	if err != nil {
		return StatusHistoryListResult{}, fmt.Errorf("list personnel status history: %w", err)
	}
	return StatusHistoryListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (s *Service) ListEvents(ctx context.Context, principal security.Principal, filter EventFilter) (EventListResult, error) {
	if err := s.authorize(principal, "event:read"); err != nil {
		return EventListResult{}, err
	}
	filter.Scope = principal.Scopes
	page, err := normalizePage(filter.PageQuery)
	if err != nil {
		return EventListResult{}, err
	}
	filter.PageQuery = page
	items, total, err := s.repository.ListEvents(ctx, filter)
	if err != nil {
		return EventListResult{}, fmt.Errorf("list operations events: %w", err)
	}
	return EventListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (s *Service) ListAudit(ctx context.Context, principal security.Principal, filter AuditFilter) (AuditListResult, error) {
	if err := s.authorize(principal, "audit:read"); err != nil {
		return AuditListResult{}, err
	}
	if !principal.Scopes.Global {
		return AuditListResult{}, ErrForbidden
	}
	page, err := normalizePage(filter.PageQuery)
	if err != nil {
		return AuditListResult{}, err
	}
	filter.PageQuery = page
	items, total, err := s.repository.ListAudit(ctx, filter)
	if err != nil {
		return AuditListResult{}, fmt.Errorf("list audit: %w", err)
	}
	return AuditListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (s *Service) GetScopes(_ context.Context, principal security.Principal) (ScopeView, error) {
	if err := s.authorize(principal, "scope:read"); err != nil {
		return ScopeView{}, err
	}
	return ScopeView{PrincipalPublicID: principal.PublicID, Roles: append([]string{}, principal.Roles...), Global: principal.Scopes.Global, AreaIDs: append([]uint64{}, principal.Scopes.AreaIDs...), TeamIDs: append([]uint64{}, principal.Scopes.TeamIDs...)}, nil
}

func (s *Service) GetDiagnostics(ctx context.Context, principal security.Principal) (Diagnostics, error) {
	if err := s.authorize(principal, "diagnostics:read"); err != nil {
		return Diagnostics{}, err
	}
	if s == nil || s.repository == nil {
		return Diagnostics{}, ErrRepositoryNotConfigured
	}
	sync, err := s.repository.GetDiagnostics(ctx)
	if err != nil {
		return Diagnostics{}, fmt.Errorf("get diagnostics: %w", err)
	}
	return Diagnostics{Component: "core-api", GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), DatabaseReachable: true, Runtime: s.runtime, Sync: sync}, nil
}

func (s *Service) authorize(principal security.Principal, permission string) error {
	if s == nil || s.repository == nil || s.authorizer == nil {
		return ErrRepositoryNotConfigured
	}
	if principal.Type != security.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" {
		return ErrForbidden
	}
	if err := s.authorizer.Authorize(principal, security.Permission(permission), security.AccessScope{}); err != nil {
		return ErrForbidden
	}
	return nil
}

func normalizePage(page PageQuery) (PageQuery, error) {
	if page.Page == 0 {
		page.Page = defaultPage
	}
	if page.PageSize == 0 {
		page.PageSize = defaultPageSize
	}
	if page.Page < 1 || page.PageSize < 1 || page.PageSize > maxPageSize {
		return PageQuery{}, ErrInvalidInput
	}
	return page, nil
}
