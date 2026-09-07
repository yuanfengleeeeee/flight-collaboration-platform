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

	exceptionmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/exception"
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
	ErrNotFound                = errors.New("management resource not found")
	ErrConflict                = errors.New("management resource state conflict")
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

type ExceptionFilter struct {
	Status            string
	Severity          string
	TaskPublicID      string
	PersonnelPublicID string
	Page              int
	PageSize          int
	Scope             security.AccessScope
}

type ReportFilter struct {
	From  string
	To    string
	Scope security.AccessScope
}

type ExceptionUpdate struct {
	PublicID       string
	Status         string
	ResolutionNote string
	ActorPublicID  string
	RequestID      string
	TraceID        string
	SourceIP       string
	Now            time.Time
	Scope          security.AccessScope
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
	ReceiptStatus       string  `json:"receipt_status"`
	ConfirmationID      string  `json:"confirmation_id"`
	ConfirmedByPublicID string  `json:"confirmed_by_public_id"`
	ConfirmedAt         string  `json:"confirmed_at"`
	ReceivedAt          *string `json:"received_at,omitempty"`
	AcceptedAt          *string `json:"accepted_at,omitempty"`
	CompletedAt         *string `json:"completed_at,omitempty"`
	CancelledAt         *string `json:"cancelled_at,omitempty"`
	CancelReason        string  `json:"cancel_reason,omitempty"`
	PlannedAt           string  `json:"planned_at"`
}

type Exception struct {
	PublicID           string  `json:"public_id"`
	TaskPublicID       string  `json:"task_public_id"`
	FlightPublicID     string  `json:"flight_public_id"`
	FlightDisplayNo    string  `json:"flight_display_no"`
	AssignmentPublicID string  `json:"assignment_public_id"`
	PersonnelPublicID  string  `json:"personnel_public_id"`
	PersonnelName      string  `json:"personnel_name"`
	AreaPublicID       string  `json:"area_public_id"`
	TeamPublicID       string  `json:"team_public_id"`
	Category           string  `json:"category"`
	Severity           string  `json:"severity"`
	Description        string  `json:"description"`
	Status             string  `json:"status"`
	ReportedByPublicID string  `json:"reported_by_public_id"`
	ReportedAt         string  `json:"reported_at"`
	ResolvedByPublicID string  `json:"resolved_by_public_id,omitempty"`
	ResolvedAt         *string `json:"resolved_at,omitempty"`
	ResolutionNote     string  `json:"resolution_note,omitempty"`
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

type ExceptionListResult struct {
	Items    []Exception `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int64       `json:"total"`
}

type ReportOverview struct {
	From             string           `json:"from,omitempty"`
	To               string           `json:"to,omitempty"`
	TaskCounts       map[string]int64 `json:"task_counts"`
	FlightCounts     map[string]int64 `json:"flight_counts"`
	AssignmentCounts map[string]int64 `json:"assignment_counts"`
	PersonnelCounts  map[string]int64 `json:"personnel_counts"`
	ExceptionCounts  map[string]int64 `json:"exception_counts"`
}

type Repository interface {
	ListPersonnel(context.Context, PersonnelFilter) ([]Personnel, int64, error)
	ListAssignments(context.Context, AssignmentFilter) ([]Assignment, int64, error)
	ListExceptions(context.Context, ExceptionFilter) ([]Exception, int64, error)
	GetReportOverview(context.Context, ReportFilter) (ReportOverview, error)
	UpdateException(context.Context, ExceptionUpdate) error
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

func (s *Service) ListExceptions(ctx context.Context, principal security.Principal, filter ExceptionFilter) (ExceptionListResult, error) {
	if s == nil || s.repository == nil {
		return ExceptionListResult{}, ErrRepositoryNotConfigured
	}
	if err := s.authorize(principal, "exception:read"); err != nil {
		return ExceptionListResult{}, err
	}
	filter.Scope = principal.Scopes
	normalized, err := normalizeExceptionFilter(filter)
	if err != nil {
		return ExceptionListResult{}, err
	}
	items, total, err := s.repository.ListExceptions(ctx, normalized)
	if err != nil {
		return ExceptionListResult{}, fmt.Errorf("list exceptions: %w", err)
	}
	return ExceptionListResult{Items: items, Page: normalized.Page, PageSize: normalized.PageSize, Total: total}, nil
}

func (s *Service) GetReportOverview(ctx context.Context, principal security.Principal, filter ReportFilter) (ReportOverview, error) {
	if s == nil || s.repository == nil {
		return ReportOverview{}, ErrRepositoryNotConfigured
	}
	if err := s.authorize(principal, "analytics:read"); err != nil {
		return ReportOverview{}, err
	}
	filter.Scope = principal.Scopes
	normalized, err := normalizeReportFilter(filter)
	if err != nil {
		return ReportOverview{}, err
	}
	result, err := s.repository.GetReportOverview(ctx, normalized)
	if err != nil {
		return ReportOverview{}, fmt.Errorf("get report overview: %w", err)
	}
	return result, nil
}

func (s *Service) UpdateException(ctx context.Context, principal security.Principal, publicID, status, resolutionNote string, meta ExceptionUpdate) error {
	if s == nil || s.repository == nil {
		return ErrRepositoryNotConfigured
	}
	if err := s.authorize(principal, "exception:manage"); err != nil {
		return err
	}
	publicID = strings.TrimSpace(publicID)
	status = strings.TrimSpace(status)
	resolutionNote = strings.TrimSpace(resolutionNote)
	if publicID == "" || len(publicID) > 128 || (status != string(exceptionmodule.StatusAcknowledged) && status != string(exceptionmodule.StatusResolved) && status != string(exceptionmodule.StatusRejected)) || len(resolutionNote) > 1024 {
		return ErrInvalidInput
	}
	meta.PublicID = publicID
	meta.Status = status
	meta.ResolutionNote = resolutionNote
	meta.ActorPublicID = principal.PublicID
	meta.Scope = principal.Scopes
	if meta.Now.IsZero() {
		meta.Now = time.Now().UTC()
	}
	if err := s.repository.UpdateException(ctx, meta); err != nil {
		return fmt.Errorf("update exception: %w", err)
	}
	return nil
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

func normalizeExceptionFilter(filter ExceptionFilter) (ExceptionFilter, error) {
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Severity = strings.TrimSpace(filter.Severity)
	filter.TaskPublicID = strings.TrimSpace(filter.TaskPublicID)
	filter.PersonnelPublicID = strings.TrimSpace(filter.PersonnelPublicID)
	if filter.Status != "" {
		switch exceptionmodule.Status(filter.Status) {
		case exceptionmodule.StatusOpen, exceptionmodule.StatusAcknowledged, exceptionmodule.StatusResolved, exceptionmodule.StatusRejected:
		default:
			return ExceptionFilter{}, ErrInvalidInput
		}
	}
	if filter.Severity != "" {
		switch exceptionmodule.Severity(filter.Severity) {
		case exceptionmodule.SeverityLow, exceptionmodule.SeverityMedium, exceptionmodule.SeverityHigh, exceptionmodule.SeverityCritical:
		default:
			return ExceptionFilter{}, ErrInvalidInput
		}
	}
	if len(filter.TaskPublicID) > 128 || len(filter.PersonnelPublicID) > 128 {
		return ExceptionFilter{}, ErrInvalidInput
	}
	if filter.Page == 0 {
		filter.Page = defaultPage
	}
	if filter.PageSize == 0 {
		filter.PageSize = defaultPageSize
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > maxPageSize {
		return ExceptionFilter{}, ErrInvalidInput
	}
	return filter, nil
}

func normalizeReportFilter(filter ReportFilter) (ReportFilter, error) {
	filter.From = strings.TrimSpace(filter.From)
	filter.To = strings.TrimSpace(filter.To)
	var from, to time.Time
	var err error
	if filter.From != "" {
		from, err = time.Parse("2006-01-02", filter.From)
		if err != nil {
			return ReportFilter{}, ErrInvalidInput
		}
	}
	if filter.To != "" {
		to, err = time.Parse("2006-01-02", filter.To)
		if err != nil {
			return ReportFilter{}, ErrInvalidInput
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return ReportFilter{}, ErrInvalidInput
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
