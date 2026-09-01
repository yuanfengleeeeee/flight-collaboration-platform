package flighttask

import (
	"context"
	"errors"
	"fmt"
	"strings"

	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

const (
	defaultTaskPage     = 1
	defaultTaskPageSize = 20
	maxTaskPageSize     = 100
)

var (
	ErrTaskQueryForbidden    = &BusinessError{Code: "forbidden", Message: "principal is not allowed to read these tasks"}
	ErrTaskQueryNotFound     = &BusinessError{Code: "task_not_found", Message: "task not found"}
	ErrTaskQueryInvalidInput = &BusinessError{Code: "invalid_input", Message: "invalid task query input"}
)

// TaskQueryFilter is the read-side boundary for Core task management. Scope
// values are populated by the authenticated Principal; callers cannot supply
// arbitrary SQL predicates.
type TaskQueryFilter struct {
	Status         string
	FlightPublicID string
	Page           int
	PageSize       int
	Scope          security.AccessScope
}

// TaskReadModel is assembled by the Core adapter from task facts and their
// current candidate/assignment facts. It is deliberately separate from the
// write-side transaction ports.
type TaskReadModel struct {
	Task             taskmodule.Instance
	TemplatePublicID string
	AreaPublicID     string
	TeamPublicID     string
	Candidates       []taskmodule.Candidate
	Assignment       *taskmodule.Assignment
}

type TaskQueryRepository interface {
	ListTaskReadModels(ctx context.Context, filter TaskQueryFilter) ([]TaskReadModel, int64, error)
	FindTaskReadModel(ctx context.Context, publicID string, scope security.AccessScope) (TaskReadModel, error)
}

type TaskQueryService struct {
	repository TaskQueryRepository
	authorizer security.Authorizer
}

func NewTaskQueryService(repository TaskQueryRepository, authorizer security.Authorizer) *TaskQueryService {
	return &TaskQueryService{repository: repository, authorizer: authorizer}
}

type TaskListResult struct {
	Items    []TaskView `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Total    int64      `json:"total"`
}

type TaskView struct {
	PublicID             string                    `json:"public_id"`
	FlightPublicID       string                    `json:"flight_public_id"`
	FlightDisplayNo      string                    `json:"flight_display_no"`
	TemplatePublicID     string                    `json:"template_public_id"`
	AreaPublicID         string                    `json:"area_public_id"`
	TeamPublicID         string                    `json:"team_public_id"`
	TriggerType          string                    `json:"trigger_type"`
	GenerationKey        string                    `json:"generation_key"`
	SourceEventID        string                    `json:"source_event_id"`
	TemplateVersion      uint                      `json:"template_version"`
	RequiredPositionCode string                    `json:"required_position_code"`
	RequiredCapabilities []string                  `json:"required_capabilities"`
	Name                 string                    `json:"name"`
	Message              string                    `json:"message"`
	PlannedAt            string                    `json:"planned_at"`
	Status               string                    `json:"status"`
	StatusVersion        uint64                    `json:"status_version"`
	SyncVersion          uint64                    `json:"sync_version"`
	Candidates           []TaskCandidateDetailView `json:"candidates"`
	Assignment           *TaskAssignmentDetailView `json:"assignment,omitempty"`
}

type TaskCandidateDetailView struct {
	PublicID                   string   `json:"public_id"`
	PersonnelPublicID          string   `json:"personnel_public_id"`
	Rank                       int      `json:"rank"`
	Status                     string   `json:"status"`
	MatchedPositionCode        string   `json:"matched_position_code"`
	MatchedCapabilities        []string `json:"matched_capabilities"`
	PersonnelWorkStateSnapshot string   `json:"personnel_work_state_snapshot"`
	PersonnelStateChangedAt    string   `json:"personnel_state_changed_at"`
	RejectionReason            string   `json:"rejection_reason,omitempty"`
	SelectedAt                 *string  `json:"selected_at,omitempty"`
	InvalidatedAt              *string  `json:"invalidated_at,omitempty"`
}

type TaskAssignmentDetailView struct {
	PublicID            string `json:"public_id"`
	CandidatePublicID   string `json:"candidate_public_id"`
	PersonnelPublicID   string `json:"personnel_public_id"`
	Status              string `json:"status"`
	StatusVersion       uint64 `json:"status_version"`
	ConfirmationID      string `json:"confirmation_id"`
	ConfirmedByPublicID string `json:"confirmed_by_public_id"`
	ConfirmedAt         string `json:"confirmed_at"`
}

func (s *TaskQueryService) ListTasks(ctx context.Context, principal security.Principal, filter TaskQueryFilter) (TaskListResult, error) {
	if s == nil || s.repository == nil {
		return TaskListResult{}, ErrRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.authorizeRead(principal); err != nil {
		return TaskListResult{}, err
	}
	filter.Scope = principal.Scopes
	normalized, err := normalizeTaskQueryFilter(filter)
	if err != nil {
		return TaskListResult{}, err
	}
	rows, total, err := s.repository.ListTaskReadModels(ctx, normalized)
	if err != nil {
		return TaskListResult{}, fmt.Errorf("list tasks: %w", err)
	}
	items := make([]TaskView, 0, len(rows))
	for _, row := range rows {
		items = append(items, taskViewFromReadModel(row))
	}
	return TaskListResult{Items: items, Page: normalized.Page, PageSize: normalized.PageSize, Total: total}, nil
}

func (s *TaskQueryService) GetTask(ctx context.Context, principal security.Principal, publicID string) (TaskView, error) {
	if s == nil || s.repository == nil {
		return TaskView{}, ErrRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.authorizeRead(principal); err != nil {
		return TaskView{}, err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" || len(publicID) > 128 {
		return TaskView{}, ErrTaskQueryInvalidInput
	}
	row, err := s.repository.FindTaskReadModel(ctx, publicID, principal.Scopes)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return TaskView{}, ErrTaskQueryNotFound
		}
		return TaskView{}, fmt.Errorf("get task %s: %w", publicID, err)
	}
	return taskViewFromReadModel(row), nil
}

func (s *TaskQueryService) authorizeRead(principal security.Principal) error {
	if principal.Type != security.HumanPrincipal || principal.PublicID == "" {
		return ErrTaskQueryForbidden
	}
	if s.authorizer == nil {
		return ErrTaskQueryForbidden
	}
	if err := s.authorizer.Authorize(principal, "task:read", security.AccessScope{}); err != nil {
		return wrapBusiness(ErrTaskQueryForbidden, err)
	}
	if !principal.Scopes.Global && len(principal.Scopes.TeamIDs) == 0 && len(principal.Scopes.AreaIDs) == 0 && principal.Scopes.UserID == 0 {
		return ErrTaskQueryForbidden
	}
	return nil
}

func normalizeTaskQueryFilter(filter TaskQueryFilter) (TaskQueryFilter, error) {
	filter.Status = strings.TrimSpace(filter.Status)
	filter.FlightPublicID = strings.TrimSpace(filter.FlightPublicID)
	if filter.Status != "" {
		switch taskmodule.Status(filter.Status) {
		case taskmodule.StatusAwaitingConfirmation, taskmodule.StatusAssigned, taskmodule.StatusInProgress, taskmodule.StatusCompleted, taskmodule.StatusCancelled:
		default:
			return TaskQueryFilter{}, ErrTaskQueryInvalidInput
		}
	}
	if len(filter.FlightPublicID) > 128 {
		return TaskQueryFilter{}, ErrTaskQueryInvalidInput
	}
	if filter.Page == 0 {
		filter.Page = defaultTaskPage
	}
	if filter.PageSize == 0 {
		filter.PageSize = defaultTaskPageSize
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > maxTaskPageSize {
		return TaskQueryFilter{}, ErrTaskQueryInvalidInput
	}
	return filter, nil
}

func taskViewFromReadModel(row TaskReadModel) TaskView {
	task := row.Task
	view := TaskView{
		PublicID: task.PublicID, FlightPublicID: task.FlightPublicID, FlightDisplayNo: task.FlightDisplayNo,
		TemplatePublicID: row.TemplatePublicID, AreaPublicID: row.AreaPublicID, TeamPublicID: row.TeamPublicID,
		TriggerType: string(task.TriggerType), GenerationKey: task.GenerationKey, SourceEventID: task.SourceEventID,
		TemplateVersion: task.TemplateVersion, RequiredPositionCode: task.RequiredPositionCode,
		RequiredCapabilities: append([]string(nil), task.RequiredCapabilities...), Name: task.Name, Message: task.Message,
		PlannedAt: task.PlannedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00"), Status: string(task.Status),
		StatusVersion: task.StatusVersion, SyncVersion: task.SyncVersion,
		Candidates: make([]TaskCandidateDetailView, 0, len(row.Candidates)),
	}
	for _, candidate := range row.Candidates {
		view.Candidates = append(view.Candidates, taskCandidateDetailViewFromDomain(candidate))
	}
	if row.Assignment != nil {
		assignment := row.Assignment
		view.Assignment = &TaskAssignmentDetailView{PublicID: assignment.PublicID, CandidatePublicID: candidatePublicID(row.Candidates, assignment.CandidateID), PersonnelPublicID: assignment.PersonnelPublicID, Status: string(assignment.Status), StatusVersion: assignment.StatusVersion, ConfirmationID: assignment.ConfirmationID, ConfirmedByPublicID: assignment.ConfirmedByPublicID, ConfirmedAt: assignment.ConfirmedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00")}
	}
	return view
}

func taskCandidateDetailViewFromDomain(candidate taskmodule.Candidate) TaskCandidateDetailView {
	view := TaskCandidateDetailView{PublicID: candidate.PublicID, PersonnelPublicID: candidate.PersonnelPublicID, Rank: candidate.Rank, Status: string(candidate.Status), MatchedPositionCode: candidate.MatchedPositionCode, MatchedCapabilities: append([]string(nil), candidate.MatchedCapabilities...), PersonnelWorkStateSnapshot: candidate.PersonnelWorkStateSnapshot, PersonnelStateChangedAt: candidate.PersonnelStateChangedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00"), RejectionReason: candidate.RejectionReason}
	if candidate.SelectedAt != nil {
		value := candidate.SelectedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00")
		view.SelectedAt = &value
	}
	if candidate.InvalidatedAt != nil {
		value := candidate.InvalidatedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00")
		view.InvalidatedAt = &value
	}
	return view
}

func candidatePublicID(candidates []taskmodule.Candidate, candidateID uint64) string {
	for _, candidate := range candidates {
		if candidate.ID == candidateID {
			return candidate.PublicID
		}
	}
	return ""
}
