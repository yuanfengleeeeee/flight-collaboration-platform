package exception

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	exceptionmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/exception"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const CommandReportTaskException = "employee_report_task_exception.v1"

var (
	ErrInvalidCommand = errors.New("invalid task exception command")
	ErrNotAssigned    = errors.New("task exception actor is not assigned to the task")
	ErrStaleTask      = errors.New("task exception task projection is stale")
)

type CommandError struct {
	Code    string
	Message string
	Cause   error
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *CommandError) Unwrap() error  { return e.Cause }
func (e *CommandError) Terminal() bool { return true }

type commandPayload struct {
	AssignmentPublicID  string     `json:"assignment_public_id"`
	ExpectedSyncVersion uint64     `json:"expected_sync_version"`
	Category            string     `json:"category"`
	Severity            string     `json:"severity"`
	Description         string     `json:"description"`
	ClientOccurredAt    time.Time  `json:"client_occurred_at"`
	ChangeAction        string     `json:"change_action"`
	TargetCandidateID   string     `json:"target_candidate_public_id"`
	TargetPlannedAt     *time.Time `json:"target_planned_at"`
}

type Transaction interface {
	FindTaskForUpdate(context.Context, string) (taskmodule.Instance, error)
	FindAssignmentForUpdate(context.Context, uint64, string) (taskmodule.Assignment, error)
	FindPersonnelForUpdate(context.Context, uint64) (personnelmodule.CandidateRecord, error)
	CreateTaskException(context.Context, exceptionmodule.Record) error
	AppendAudit(context.Context, coresync.AuditRecord) error
}

// ChangeRequestWriter is optional for foundation test stores. Production Core
// transactions implement it so an employee exception and its requested
// operational change are committed together, before any manager review.
type ChangeRequestWriter interface {
	CreateTaskChangeRequest(context.Context, taskmodule.ChangeRequest) error
}

type Service struct {
	authorizer security.Authorizer
	clock      clock.Clock
}

func NewService(authorizer security.Authorizer, now clock.Clock) *Service {
	if authorizer == nil {
		authorizer = iam.NewAuthorizer()
	}
	if now == nil {
		now = clock.Real{}
	}
	return &Service{authorizer: authorizer, clock: now}
}

func HandleCommand(ctx context.Context, tx any, command sharedEvent.CommandEnvelope) error {
	return NewService(nil, nil).Handle(ctx, tx, command)
}

func (s *Service) Handle(ctx context.Context, txValue any, command sharedEvent.CommandEnvelope) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := command.Validate(); err != nil {
		return &CommandError{Code: "invalid_command", Message: "command envelope is invalid", Cause: err}
	}
	if command.CommandType != CommandReportTaskException {
		return &CommandError{Code: "invalid_command", Message: "unsupported task exception command"}
	}
	var payload commandPayload
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return &CommandError{Code: "invalid_command", Message: "task exception payload is invalid", Cause: err}
	}
	if err := validatePayload(payload); err != nil {
		return &CommandError{Code: "invalid_command", Message: err.Error(), Cause: ErrInvalidCommand}
	}
	tx, ok := txValue.(Transaction)
	if !ok || tx == nil {
		return fmt.Errorf("task exception transaction is not configured")
	}
	task, err := tx.FindTaskForUpdate(ctx, command.AggregateID)
	if err != nil {
		return fmt.Errorf("find task for exception: %w", err)
	}
	assignment, err := tx.FindAssignmentForUpdate(ctx, task.ID, payload.AssignmentPublicID)
	if err != nil {
		return fmt.Errorf("find assignment for exception: %w", err)
	}
	person, err := tx.FindPersonnelForUpdate(ctx, assignment.PersonnelID)
	if err != nil {
		return fmt.Errorf("find personnel for exception: %w", err)
	}
	if assignment.PersonnelPublicID != command.ActorPublicID || person.PublicID != command.ActorPublicID {
		return &CommandError{Code: "not_assigned_to_actor", Message: ErrNotAssigned.Error(), Cause: ErrNotAssigned}
	}
	principal := security.Principal{Type: security.HumanPrincipal, PublicID: command.ActorPublicID, Roles: []string{security.RoleStaff}, Scopes: security.AccessScope{UserID: person.ID}}
	if s.authorizer != nil {
		if err := s.authorizer.Authorize(principal, "exception:report", security.AccessScope{UserID: person.ID}); err != nil {
			return &CommandError{Code: "not_assigned_to_actor", Message: "employee exception report is not authorized", Cause: err}
		}
	}
	if task.SyncVersion != payload.ExpectedSyncVersion {
		return &CommandError{Code: "stale_assignment", Message: ErrStaleTask.Error(), Cause: ErrStaleTask}
	}
	reportedAt := s.clock.Now().UTC()
	publicID, err := id.NewPublicID()
	if err != nil {
		return fmt.Errorf("generate task exception public id: %w", err)
	}
	record := exceptionmodule.Record{PublicID: publicID, TaskID: task.ID, TaskPublicID: task.PublicID, AssignmentID: assignment.ID, AssignmentPublicID: assignment.PublicID, PersonnelID: person.ID, PersonnelPublicID: person.PublicID, Category: payload.Category, Severity: exceptionmodule.Severity(payload.Severity), Description: payload.Description, Status: exceptionmodule.StatusOpen, ReportedByPublicID: command.ActorPublicID, ReportedAt: reportedAt}
	if err := tx.CreateTaskException(ctx, record); err != nil {
		return fmt.Errorf("create task exception: %w", err)
	}
	if strings.TrimSpace(payload.ChangeAction) != "" {
		writer, ok := txValue.(ChangeRequestWriter)
		if !ok {
			return fmt.Errorf("task change request transaction is not configured")
		}
		changePublicID, err := id.NewPublicID()
		if err != nil {
			return fmt.Errorf("generate task change request public id: %w", err)
		}
		changeAction := taskmodule.ChangeAction(strings.TrimSpace(payload.ChangeAction))
		switch changeAction {
		case taskmodule.ChangeActionPause, taskmodule.ChangeActionReassign, taskmodule.ChangeActionReschedule, taskmodule.ChangeActionCancel, taskmodule.ChangeActionResume:
		default:
			return &CommandError{Code: "invalid_change_action", Message: "task change action is invalid", Cause: ErrInvalidCommand}
		}
		changeRequestID := "change:" + command.CommandID
		if err := writer.CreateTaskChangeRequest(ctx, taskmodule.ChangeRequest{PublicID: changePublicID, TaskID: task.ID, TaskPublicID: task.PublicID, ExceptionPublicID: publicID, Action: changeAction, Reason: payload.Description, TargetCandidatePublicID: strings.TrimSpace(payload.TargetCandidateID), TargetPlannedAt: payload.TargetPlannedAt, Status: taskmodule.ChangeRequestPending, RequestedByPublicID: command.ActorPublicID, RequestedAt: reportedAt, RequestID: changeRequestID, TraceID: command.TraceID}); err != nil {
			return fmt.Errorf("create task change request: %w", err)
		}
	}
	if err := tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: string(security.HumanPrincipal), ActorID: command.ActorPublicID, Action: CommandReportTaskException, ResourceType: "task_exception", ResourceID: publicID, Result: string(exceptionmodule.StatusOpen), TraceID: command.TraceID, OccurredAt: reportedAt}); err != nil {
		return fmt.Errorf("append task exception audit: %w", err)
	}
	return nil
}

func validatePayload(payload commandPayload) error {
	if strings.TrimSpace(payload.AssignmentPublicID) == "" || payload.ExpectedSyncVersion == 0 {
		return errors.New("assignment_public_id and expected_sync_version are required")
	}
	if category := strings.TrimSpace(payload.Category); category == "" || len(category) > 64 {
		return errors.New("category is required and must be at most 64 characters")
	}
	switch exceptionmodule.Severity(strings.TrimSpace(payload.Severity)) {
	case exceptionmodule.SeverityLow, exceptionmodule.SeverityMedium, exceptionmodule.SeverityHigh, exceptionmodule.SeverityCritical:
	default:
		return errors.New("severity must be low, medium, high or critical")
	}
	if description := strings.TrimSpace(payload.Description); description == "" || len(description) > 1024 {
		return errors.New("description is required and must be at most 1024 characters")
	}
	if action := strings.TrimSpace(payload.ChangeAction); action != "" {
		switch taskmodule.ChangeAction(action) {
		case taskmodule.ChangeActionPause, taskmodule.ChangeActionReassign, taskmodule.ChangeActionReschedule, taskmodule.ChangeActionCancel, taskmodule.ChangeActionResume:
		default:
			return errors.New("change_action is invalid")
		}
		if taskmodule.ChangeAction(action) == taskmodule.ChangeActionReschedule && (payload.TargetPlannedAt == nil || payload.TargetPlannedAt.IsZero()) {
			return errors.New("target_planned_at is required for reschedule")
		}
	}
	return nil
}
