// Package adminquery contains read-side contracts for the management console.
// It exposes Core facts without allowing the browser to provide SQL scope
// predicates or bypass the IAM authorizer.
package adminquery

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
	ErrRepositoryNotConfigured = errors.New("admin query repository is not configured")
	ErrForbidden               = errors.New("principal is not allowed to read this management resource")
	ErrInvalidInput            = errors.New("invalid management query input")
)

type PersonnelFilter struct {
	WorkState    string
	TeamPublicID string
	AreaPublicID string
	Page         int
	PageSize     int
	Scope        security.AccessScope
}

type AssignmentFilter struct {
	Status            string
	TaskPublicID      string
	PersonnelPublicID string
	Page              int
	PageSize          int
	Scope             security.AccessScope
}

type Personnel struct {
	PublicID           string   `json:"public_id"`
	UserPublicID       string   `json:"user_public_id"`
	EmployeeNo         string   `json:"employee_no"`
	DisplayName        string   `json:"display_name"`
	AreaPublicID       string   `json:"area_public_id"`
	AreaName           string   `json:"area_name"`
	TeamPublicID       string   `json:"team_public_id"`
	TeamName           string   `json:"team_name"`
	PositionCode       string   `json:"position_code"`
	Capabilities       []string `json:"capabilities"`
	WorkState          string   `json:"work_state"`
	StatusVersion      uint64   `json:"status_version"`
	LastStateChangedAt string   `json:"last_state_changed_at"`
	UnavailableReason  string   `json:"unavailable_reason,omitempty"`
	Enabled            bool     `json:"enabled"`
}

type Assignment struct {
	PublicID            string  `json:"public_id"`
	TaskPublicID        string  `json:"task_public_id"`
	FlightPublicID      string  `json:"flight_public_id"`
	FlightDisplayNo     string  `json:"flight_display_no"`
	PersonnelPublicID   string  `json:"personnel_public_id"`
	PersonnelEmployeeNo string  `json:"personnel_employee_no"`
	PersonnelName       string  `json:"personnel_name"`
	AreaPublicID        string  `json:"area_public_id"`
	TeamPublicID        string  `json:"team_public_id"`
	Status              string  `json:"status"`
	StatusVersion       uint64  `json:"status_version"`
	ConfirmationID      string  `json:"confirmation_id"`
	ConfirmedByPublicID string  `json:"confirmed_by_public_id"`
	ConfirmedAt         string  `json:"confirmed_at"`
	AcceptedAt          *string `json:"accepted_at,omitempty"`
	CompletedAt         *string `json:"completed_at,omitempty"`
	CancelledAt         *string `json:"cancelled_at,omitempty"`
	CancelReason        string  `json:"cancel_reason,omitempty"`
	PlannedAt           string  `json:"planned_at"`
}

type PersonnelListResult struct {
	Items    []Personnel `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int64       `json:"total"`
}

type AssignmentListResult struct {
	Items    []Assignment `json:"items"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Total    int64        `json:"total"`
}

type Repository interface {
	ListPersonnel(context.Context, PersonnelFilter) ([]Personnel, int64, error)
	ListAssignments(context.Context, AssignmentFilter) ([]Assignment, int64, error)
}

type Service struct {
	repository Repository
	authorizer security.Authorizer
}

func NewService(repository Repository, authorizer security.Authorizer) *Service {
	return &Service{repository: repository, authorizer: authorizer}
}

func (s *Service) ListPersonnel(ctx context.Context, principal security.Principal, filter PersonnelFilter) (PersonnelListResult, error) {
	if s == nil || s.repository == nil {
		return PersonnelListResult{}, ErrRepositoryNotConfigured
	}
	if err := s.authorize(principal, "personnel:read"); err != nil {
		return PersonnelListResult{}, err
	}
	filter.Scope = principal.Scopes
	normalized, err := normalizePersonnelFilter(filter)
	if err != nil {
		return PersonnelListResult{}, err
	}
	items, total, err := s.repository.ListPersonnel(ctx, normalized)
	if err != nil {
		return PersonnelListResult{}, fmt.Errorf("list personnel: %w", err)
	}
	return PersonnelListResult{Items: items, Page: normalized.Page, PageSize: normalized.PageSize, Total: total}, nil
}

func (s *Service) ListAssignments(ctx context.Context, principal security.Principal, filter AssignmentFilter) (AssignmentListResult, error) {
	if s == nil || s.repository == nil {
		return AssignmentListResult{}, ErrRepositoryNotConfigured
	}
	if err := s.authorize(principal, "assignment:read"); err != nil {
		return AssignmentListResult{}, err
	}
	filter.Scope = principal.Scopes
	normalized, err := normalizeAssignmentFilter(filter)
	if err != nil {
		return AssignmentListResult{}, err
	}
	items, total, err := s.repository.ListAssignments(ctx, normalized)
	if err != nil {
		return AssignmentListResult{}, fmt.Errorf("list assignments: %w", err)
	}
	return AssignmentListResult{Items: items, Page: normalized.Page, PageSize: normalized.PageSize, Total: total}, nil
}

func (s *Service) authorize(principal security.Principal, permission security.Permission) error {
	if principal.Type != security.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" || s.authorizer == nil {
		return ErrForbidden
	}
	if err := s.authorizer.Authorize(principal, permission, security.AccessScope{}); err != nil {
		return fmt.Errorf("%w: %v", ErrForbidden, err)
	}
	if !principal.Scopes.Global && len(principal.Scopes.TeamIDs) == 0 && len(principal.Scopes.AreaIDs) == 0 && principal.Scopes.UserID == 0 {
		return ErrForbidden
	}
	return nil
}

func normalizePersonnelFilter(filter PersonnelFilter) (PersonnelFilter, error) {
	filter.WorkState = strings.TrimSpace(filter.WorkState)
	filter.TeamPublicID = strings.TrimSpace(filter.TeamPublicID)
	filter.AreaPublicID = strings.TrimSpace(filter.AreaPublicID)
	if filter.WorkState != "" && filter.WorkState != "idle" && filter.WorkState != "reserved" && filter.WorkState != "busy" && filter.WorkState != "unavailable" {
		return PersonnelFilter{}, ErrInvalidInput
	}
	if len(filter.TeamPublicID) > 128 || len(filter.AreaPublicID) > 128 {
		return PersonnelFilter{}, ErrInvalidInput
	}
	return normalizePagination(filter)
}

func normalizeAssignmentFilter(filter AssignmentFilter) (AssignmentFilter, error) {
	filter.Status = strings.TrimSpace(filter.Status)
	filter.TaskPublicID = strings.TrimSpace(filter.TaskPublicID)
	filter.PersonnelPublicID = strings.TrimSpace(filter.PersonnelPublicID)
	if filter.Status != "" && filter.Status != "confirmed" && filter.Status != "accepted" && filter.Status != "completed" && filter.Status != "cancelled" {
		return AssignmentFilter{}, ErrInvalidInput
	}
	if len(filter.TaskPublicID) > 128 || len(filter.PersonnelPublicID) > 128 {
		return AssignmentFilter{}, ErrInvalidInput
	}
	if filter.Page == 0 {
		filter.Page = defaultPage
	}
	if filter.PageSize == 0 {
		filter.PageSize = defaultPageSize
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > maxPageSize {
		return AssignmentFilter{}, ErrInvalidInput
	}
	return filter, nil
}

func normalizePagination(filter PersonnelFilter) (PersonnelFilter, error) {
	if filter.Page == 0 {
		filter.Page = defaultPage
	}
	if filter.PageSize == 0 {
		filter.PageSize = defaultPageSize
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > maxPageSize {
		return PersonnelFilter{}, ErrInvalidInput
	}
	return filter, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05.999999Z07:00")
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}
