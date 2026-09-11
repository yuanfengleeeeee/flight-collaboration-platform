// Package taskchange contains the durable request/approval boundary for
// operational task changes. A request never mutates a task by itself; the
// repository applies it atomically only after an authorized manager review.
package taskchange

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const (
	ReviewApprove = "approve"
	ReviewReject  = "reject"
)

var (
	ErrRepositoryNotConfigured = errors.New("task change repository is not configured")
	ErrForbidden               = errors.New("principal is not allowed to manage task changes")
	ErrInvalidInput            = errors.New("invalid task change request")
	ErrNotFound                = errors.New("task change request not found")
	ErrConflict                = errors.New("task change request state conflict")
	ErrApplyFailed             = errors.New("approved task change could not be applied")
)

type CreateInput struct {
	TaskPublicID            string
	ExceptionPublicID       string
	Action                  taskmodule.ChangeAction
	Reason                  string
	TargetCandidatePublicID string
	TargetPlannedAt         *time.Time
	RequestID               string
	TraceID                 string
	SourceIP                string
	RequestedByPublicID     string
	RequestedAt             time.Time
	Scope                   security.AccessScope
}

type ReviewInput struct {
	PublicID   string
	Decision   string
	ReviewNote string
	ReviewedBy string
	ReviewedAt time.Time
	RequestID  string
	TraceID    string
	SourceIP   string
	Scope      security.AccessScope
}

type ListFilter struct {
	TaskPublicID string
	Status       string
	Page         int
	PageSize     int
	Scope        security.AccessScope
}

type ListResult struct {
	Items    []taskmodule.ChangeRequest `json:"items"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
	Total    int64                      `json:"total"`
}

type Repository interface {
	CreateRequest(context.Context, taskmodule.ChangeRequest, security.AccessScope) (taskmodule.ChangeRequest, error)
	ListRequests(context.Context, ListFilter) ([]taskmodule.ChangeRequest, int64, error)
	ReviewRequest(context.Context, ReviewInput) (taskmodule.ChangeRequest, error)
}

type Service struct {
	repository Repository
	authorizer security.Authorizer
	clock      clock.Clock
}

func NewService(repository Repository, authorizer security.Authorizer, now clock.Clock) *Service {
	if authorizer == nil {
		authorizer = iam.NewAuthorizer()
	}
	if now == nil {
		now = clock.Real{}
	}
	return &Service{repository: repository, authorizer: authorizer, clock: now}
}

func (s *Service) Create(ctx context.Context, principal security.Principal, input CreateInput) (taskmodule.ChangeRequest, error) {
	if s == nil || s.repository == nil {
		return taskmodule.ChangeRequest{}, ErrRepositoryNotConfigured
	}
	if err := s.authorizeRequest(principal); err != nil {
		return taskmodule.ChangeRequest{}, err
	}
	input, err := normalizeCreateInput(input, principal.PublicID, s.now())
	if err != nil {
		return taskmodule.ChangeRequest{}, err
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return taskmodule.ChangeRequest{}, fmt.Errorf("generate task change public id: %w", err)
	}
	value := taskmodule.ChangeRequest{
		PublicID:                publicID,
		TaskPublicID:            input.TaskPublicID,
		ExceptionPublicID:       input.ExceptionPublicID,
		Action:                  input.Action,
		Reason:                  input.Reason,
		TargetCandidatePublicID: input.TargetCandidatePublicID,
		TargetPlannedAt:         input.TargetPlannedAt,
		Status:                  taskmodule.ChangeRequestPending,
		RequestedByPublicID:     input.RequestedByPublicID,
		RequestedAt:             input.RequestedAt,
		RequestID:               input.RequestID,
		TraceID:                 input.TraceID,
	}
	created, err := s.repository.CreateRequest(ctx, value, input.Scope)
	if err != nil {
		return taskmodule.ChangeRequest{}, fmt.Errorf("create task change request: %w", err)
	}
	return created, nil
}

func (s *Service) List(ctx context.Context, principal security.Principal, filter ListFilter) (ListResult, error) {
	if s == nil || s.repository == nil {
		return ListResult{}, ErrRepositoryNotConfigured
	}
	if err := s.authorizeManage(principal); err != nil {
		return ListResult{}, err
	}
	filter.TaskPublicID = strings.TrimSpace(filter.TaskPublicID)
	filter.Status = strings.TrimSpace(filter.Status)
	if filter.Status != "" && !validStatus(filter.Status) {
		return ListResult{}, ErrInvalidInput
	}
	page, pageSize, err := normalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return ListResult{}, err
	}
	filter.Page, filter.PageSize = page, pageSize
	filter.Scope = principal.Scopes
	items, total, err := s.repository.ListRequests(ctx, filter)
	if err != nil {
		return ListResult{}, fmt.Errorf("list task change requests: %w", err)
	}
	return ListResult{Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
}

func (s *Service) Review(ctx context.Context, principal security.Principal, input ReviewInput) (taskmodule.ChangeRequest, error) {
	if s == nil || s.repository == nil {
		return taskmodule.ChangeRequest{}, ErrRepositoryNotConfigured
	}
	if err := s.authorizeManage(principal); err != nil {
		return taskmodule.ChangeRequest{}, err
	}
	input.PublicID = strings.TrimSpace(input.PublicID)
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.ReviewNote = strings.TrimSpace(input.ReviewNote)
	if input.PublicID == "" || len(input.PublicID) > 128 || (input.Decision != ReviewApprove && input.Decision != ReviewReject) || len(input.ReviewNote) > 1024 {
		return taskmodule.ChangeRequest{}, ErrInvalidInput
	}
	if input.ReviewedBy == "" {
		input.ReviewedBy = principal.PublicID
	}
	if input.ReviewedAt.IsZero() {
		input.ReviewedAt = s.now()
	}
	input.Scope = principal.Scopes
	value, err := s.repository.ReviewRequest(ctx, input)
	if err != nil {
		return taskmodule.ChangeRequest{}, fmt.Errorf("review task change request: %w", err)
	}
	if value.Status == taskmodule.ChangeRequestFailed {
		return value, fmt.Errorf("%w: %s", ErrApplyFailed, value.FailureReason)
	}
	return value, nil
}

func (s *Service) authorizeRequest(principal security.Principal) error {
	if principal.Type != security.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" || s.authorizer == nil {
		return ErrForbidden
	}
	for _, role := range principal.Roles {
		if role == security.RoleStaff || role == security.RoleLeader || role == security.RoleManager || role == security.RoleAdmin {
			return nil
		}
	}
	return ErrForbidden
}

func (s *Service) authorizeManage(principal security.Principal) error {
	if principal.Type != security.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" || s.authorizer == nil {
		return ErrForbidden
	}
	// A leader may report or observe a change, but applying a task change is a
	// duty-manager/admin decision. Keeping this gate here prevents a caller
	// from turning the generic exception:manage permission into a bypass.
	manager := false
	for _, role := range principal.Roles {
		if role == security.RoleAdmin || role == security.RoleManager {
			manager = true
			break
		}
	}
	if !manager {
		return ErrForbidden
	}
	if err := s.authorizer.Authorize(principal, "exception:manage", security.AccessScope{}); err != nil {
		return ErrForbidden
	}
	return nil
}

func normalizeCreateInput(input CreateInput, actor string, now time.Time) (CreateInput, error) {
	input.TaskPublicID = strings.TrimSpace(input.TaskPublicID)
	input.ExceptionPublicID = strings.TrimSpace(input.ExceptionPublicID)
	input.Reason = strings.TrimSpace(input.Reason)
	input.TargetCandidatePublicID = strings.TrimSpace(input.TargetCandidatePublicID)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.TraceID = strings.TrimSpace(input.TraceID)
	input.SourceIP = strings.TrimSpace(input.SourceIP)
	input.RequestedByPublicID = strings.TrimSpace(input.RequestedByPublicID)
	if input.RequestedByPublicID == "" {
		input.RequestedByPublicID = actor
	}
	if input.TaskPublicID == "" || input.Reason == "" || len(input.TaskPublicID) > 128 || len(input.ExceptionPublicID) > 128 || len(input.Reason) > 1024 || len(input.TargetCandidatePublicID) > 128 {
		return CreateInput{}, ErrInvalidInput
	}
	switch input.Action {
	case taskmodule.ChangeActionPause, taskmodule.ChangeActionReassign, taskmodule.ChangeActionReschedule, taskmodule.ChangeActionCancel, taskmodule.ChangeActionResume:
	default:
		return CreateInput{}, ErrInvalidInput
	}
	if input.Action == taskmodule.ChangeActionReschedule && (input.TargetPlannedAt == nil || input.TargetPlannedAt.IsZero()) {
		return CreateInput{}, ErrInvalidInput
	}
	if input.RequestID == "" {
		input.RequestID = fmt.Sprintf("task-change:%s:%d", input.TaskPublicID, now.UnixNano())
	}
	if input.TraceID == "" {
		input.TraceID = input.RequestID
	}
	if input.SourceIP == "" {
		input.SourceIP = "internal"
	}
	input.RequestedAt = input.RequestedAt.UTC()
	if input.RequestedAt.IsZero() {
		input.RequestedAt = now.UTC()
	}
	if input.TargetPlannedAt != nil {
		value := input.TargetPlannedAt.UTC()
		input.TargetPlannedAt = &value
	}
	return input, nil
}

func normalizePage(page, pageSize int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return 0, 0, ErrInvalidInput
	}
	return page, pageSize, nil
}

func validStatus(value string) bool {
	switch taskmodule.ChangeRequestStatus(value) {
	case taskmodule.ChangeRequestPending, taskmodule.ChangeRequestApproved, taskmodule.ChangeRequestRejected, taskmodule.ChangeRequestApplied, taskmodule.ChangeRequestFailed:
		return true
	default:
		return false
	}
}

func (s *Service) now() time.Time {
	if s != nil && s.clock != nil {
		return s.clock.Now().UTC()
	}
	return time.Now().UTC()
}
