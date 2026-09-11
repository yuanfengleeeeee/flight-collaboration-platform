package flighttask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const (
	CommandEmployeeReceiveTask  = "employee_receive_task.v1"
	CommandEmployeeStartTask    = "employee_start_task.v1"
	CommandEmployeeAcceptTask   = "employee_accept_task.v1"
	CommandEmployeeCompleteTask = "employee_complete_task.v1"
	EventTaskReceived           = "task.received.v1"
	EventTaskStarted            = "task.started.v1"
	EventTaskAccepted           = "task.accepted.v1"
	EventTaskCompleted          = "task.completed.v1"
	ResultCommandApplied        = "applied"
	ResultAlreadyReceived       = "already_received"
	ResultAlreadyAccepted       = "already_accepted"
	ResultAlreadyCompleted      = "already_completed"
	ResultTaskCancelled         = "task_cancelled"
	ResultNotAssignedToActor    = "not_assigned_to_actor"
	ResultInvalidCommandState   = "invalid_state"
	ResultStaleAssignment       = "stale_assignment"
	ResultTaskNotFound          = "task_not_found"
	ResultAssignmentNotFound    = "assignment_not_found"
	ResultCommandInvalid        = "invalid_command"
)

// CommandError is terminal: business rejection must be acknowledged as
// failed/rejected instead of being retried by the delivery worker.
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

func CommandCode(err error) string {
	var commandErr *CommandError
	if errors.As(err, &commandErr) && commandErr != nil && commandErr.Code != "" {
		return commandErr.Code
	}
	return "internal_error"
}

type employeeTaskCommandPayload struct {
	AssignmentPublicID  string    `json:"assignment_public_id"`
	ExpectedSyncVersion uint64    `json:"expected_sync_version"`
	ClientOccurredAt    time.Time `json:"client_occurred_at"`
	Note                string    `json:"note"`
}

// EmployeeCommandTransaction is implemented by the Core business adapter.
// It is intentionally separate from the synchronization package so the Core
// Inbox can pass its transaction through without making sync own business SQL.
type EmployeeCommandTransaction interface {
	FindTaskForUpdate(ctx context.Context, publicID string) (taskmodule.Instance, error)
	FindAssignmentForUpdate(ctx context.Context, taskID uint64, publicID string) (taskmodule.Assignment, error)
	FindPersonnelForUpdate(ctx context.Context, personnelID uint64) (personnelmodule.CandidateRecord, error)
	MarkAssignmentReceived(ctx context.Context, assignmentID uint64, changedAt time.Time) error
	UpdateTaskSyncVersion(ctx context.Context, taskID, expectedSyncVersion uint64, changedAt time.Time) error
	UpdateTaskInProgress(ctx context.Context, taskID, expectedStatusVersion uint64, changedAt time.Time) error
	UpdateTaskCompleted(ctx context.Context, taskID, expectedStatusVersion uint64, changedAt time.Time) error
	UpdateAssignmentAccepted(ctx context.Context, assignmentID, expectedStatusVersion uint64, changedAt time.Time) error
	UpdateAssignmentCompleted(ctx context.Context, assignmentID, expectedStatusVersion uint64, changedAt time.Time) error
	UpdatePersonnelBusy(ctx context.Context, personnelID, expectedStatusVersion uint64, changedAt time.Time) error
	UpdatePersonnelIdle(ctx context.Context, personnelID, expectedStatusVersion uint64, changedAt time.Time) error
	CreateTaskStatusHistory(ctx context.Context, value taskmodule.StatusHistory) error
	CreateAssignmentStatusHistory(ctx context.Context, value taskmodule.AssignmentStatusHistory) error
	CreatePersonnelStatusHistory(ctx context.Context, value personnelmodule.StatusHistory) error
	AppendAudit(ctx context.Context, value coresync.AuditRecord) error
	AppendOutbox(ctx context.Context, value event.EventEnvelope) error
}

type EmployeeCommandService struct {
	authorizer security.Authorizer
	clock      clock.Clock
}

func NewEmployeeCommandService(authorizer security.Authorizer, now clock.Clock) *EmployeeCommandService {
	if authorizer == nil {
		authorizer = iam.NewAuthorizer()
	}
	if now == nil {
		now = clock.Real{}
	}
	return &EmployeeCommandService{authorizer: authorizer, clock: now}
}

// HandleEmployeeCommand is the Worker-facing adapter. The transaction value
// remains untyped at the sync boundary; only the business adapter can execute
// employee state transitions.
func HandleEmployeeCommand(ctx context.Context, tx any, command sharedCommandEnvelope) error {
	return NewEmployeeCommandService(nil, nil).Handle(ctx, tx, command)
}

// sharedCommandEnvelope keeps this file's public boundary explicit while
// avoiding a second command envelope type in the application package.
type sharedCommandEnvelope = event.CommandEnvelope

func (s *EmployeeCommandService) Handle(ctx context.Context, txValue any, command event.CommandEnvelope) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := command.Validate(); err != nil {
		return &CommandError{Code: ResultCommandInvalid, Message: "command envelope is invalid", Cause: err}
	}
	if command.CommandType != CommandEmployeeReceiveTask && command.CommandType != CommandEmployeeStartTask && command.CommandType != CommandEmployeeAcceptTask && command.CommandType != CommandEmployeeCompleteTask {
		return &CommandError{Code: ResultCommandInvalid, Message: "unsupported employee command type"}
	}
	var payload employeeTaskCommandPayload
	if err := json.Unmarshal(command.Payload, &payload); err != nil || strings.TrimSpace(payload.AssignmentPublicID) == "" || payload.ExpectedSyncVersion == 0 || len(payload.Note) > 255 {
		return &CommandError{Code: ResultCommandInvalid, Message: "employee command payload is invalid", Cause: err}
	}
	tx, ok := txValue.(EmployeeCommandTransaction)
	if !ok || tx == nil {
		return fmt.Errorf("employee command transaction is not configured")
	}
	task, err := tx.FindTaskForUpdate(ctx, command.AggregateID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &CommandError{Code: ResultTaskNotFound, Message: "task was not found", Cause: err}
		}
		return fmt.Errorf("find employee task: %w", err)
	}
	assignment, err := tx.FindAssignmentForUpdate(ctx, task.ID, payload.AssignmentPublicID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &CommandError{Code: ResultAssignmentNotFound, Message: "assignment was not found", Cause: err}
		}
		return fmt.Errorf("find employee assignment: %w", err)
	}
	person, err := tx.FindPersonnelForUpdate(ctx, assignment.PersonnelID)
	if err != nil {
		return fmt.Errorf("find assigned personnel: %w", err)
	}
	if assignment.PersonnelPublicID != command.ActorPublicID || person.PublicID != command.ActorPublicID {
		return &CommandError{Code: ResultNotAssignedToActor, Message: "command actor is not the assigned employee"}
	}
	permission := security.Permission("task:receive")
	if command.CommandType == CommandEmployeeStartTask {
		permission = "task:start"
	} else if command.CommandType == CommandEmployeeAcceptTask {
		// Keep the legacy command available for old clients, but its old
		// "accept" meaning is now explicitly the start-execution transition.
		permission = "task:accept"
	} else if command.CommandType == CommandEmployeeCompleteTask {
		permission = "task:complete"
	}
	principal := security.Principal{Type: security.HumanPrincipal, PublicID: command.ActorPublicID, Roles: []string{security.RoleStaff}, Scopes: security.AccessScope{UserID: person.ID}}
	if err := s.authorizer.Authorize(principal, permission, security.AccessScope{UserID: person.ID}); err != nil {
		return &CommandError{Code: ResultNotAssignedToActor, Message: "employee command is not authorized", Cause: err}
	}

	if task.Status == taskmodule.StatusCancelled || assignment.Status == taskmodule.AssignmentCancelled {
		return &CommandError{Code: ResultTaskCancelled, Message: "task has been cancelled"}
	}
	if command.CommandType == CommandEmployeeReceiveTask {
		if task.Status == taskmodule.StatusCompleted || assignment.Status == taskmodule.AssignmentCompleted || assignment.ReceiptStatus == taskmodule.AssignmentReceiptReceived {
			return nil
		}
		if task.Status != taskmodule.StatusAssigned || assignment.Status != taskmodule.AssignmentConfirmed || person.WorkState != personnelmodule.WorkStateReserved {
			return &CommandError{Code: ResultInvalidCommandState, Message: "task is not ready to acknowledge receipt"}
		}
		if task.SyncVersion != payload.ExpectedSyncVersion {
			return &CommandError{Code: ResultStaleAssignment, Message: "task projection version is stale"}
		}
		return s.receive(ctx, tx, command, payload, task, assignment)
	}

	if command.CommandType == CommandEmployeeStartTask || command.CommandType == CommandEmployeeAcceptTask {
		if task.Status == taskmodule.StatusCompleted || assignment.Status == taskmodule.AssignmentCompleted {
			return nil
		}
		if task.Status == taskmodule.StatusInProgress || assignment.Status == taskmodule.AssignmentAccepted {
			return nil
		}
		if task.Status != taskmodule.StatusAssigned || assignment.Status != taskmodule.AssignmentConfirmed || assignment.ReceiptStatus != taskmodule.AssignmentReceiptReceived || person.WorkState != personnelmodule.WorkStateReserved {
			return &CommandError{Code: ResultInvalidCommandState, Message: "task must be acknowledged before execution starts"}
		}
		if task.SyncVersion != payload.ExpectedSyncVersion {
			return &CommandError{Code: ResultStaleAssignment, Message: "task projection version is stale"}
		}
		return s.start(ctx, tx, command, payload, task, assignment, person)
	}

	if task.Status == taskmodule.StatusCompleted || assignment.Status == taskmodule.AssignmentCompleted {
		return nil
	}
	if task.Status != taskmodule.StatusInProgress || assignment.Status != taskmodule.AssignmentAccepted || person.WorkState != personnelmodule.WorkStateBusy {
		return &CommandError{Code: ResultInvalidCommandState, Message: "task is not ready to complete"}
	}
	if task.SyncVersion != payload.ExpectedSyncVersion {
		return &CommandError{Code: ResultStaleAssignment, Message: "task projection version is stale"}
	}
	return s.complete(ctx, tx, command, payload, task, assignment, person)
}

func (s *EmployeeCommandService) receive(ctx context.Context, tx EmployeeCommandTransaction, command event.CommandEnvelope, payload employeeTaskCommandPayload, task taskmodule.Instance, assignment taskmodule.Assignment) error {
	now := s.clock.Now().UTC()
	if err := tx.MarkAssignmentReceived(ctx, assignment.ID, now); err != nil {
		return fmt.Errorf("mark assignment received: %w", err)
	}
	if err := tx.UpdateTaskSyncVersion(ctx, task.ID, task.SyncVersion, now); err != nil {
		return fmt.Errorf("advance task receipt version: %w", err)
	}
	receivedAt := now
	assignment.ReceiptStatus = taskmodule.AssignmentReceiptReceived
	assignment.ReceivedAt = &receivedAt
	if err := tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: string(security.HumanPrincipal), ActorID: command.ActorPublicID, Action: command.CommandType, ResourceType: "task_assignment", ResourceID: assignment.PublicID, Result: ResultCommandApplied, TraceID: command.TraceID, OccurredAt: now}); err != nil {
		return fmt.Errorf("append receipt audit: %w", err)
	}
	return s.writeTaskEvent(ctx, tx, command, task, assignment, EventTaskReceived, taskmodule.StatusAssigned, task.SyncVersion+1, now)
}

func (s *EmployeeCommandService) start(ctx context.Context, tx EmployeeCommandTransaction, command event.CommandEnvelope, payload employeeTaskCommandPayload, task taskmodule.Instance, assignment taskmodule.Assignment, person personnelmodule.CandidateRecord) error {
	now := s.clock.Now().UTC()
	if err := tx.UpdateAssignmentAccepted(ctx, assignment.ID, assignment.StatusVersion, now); err != nil {
		return fmt.Errorf("accept assignment: %w", err)
	}
	if err := tx.UpdateTaskInProgress(ctx, task.ID, task.StatusVersion, now); err != nil {
		return fmt.Errorf("mark task in progress: %w", err)
	}
	if err := tx.UpdatePersonnelBusy(ctx, person.ID, person.StatusVersion, now); err != nil {
		return fmt.Errorf("mark personnel busy: %w", err)
	}
	if err := s.writeTransitionHistory(ctx, tx, command, payload.Note, task, assignment, person, taskmodule.StatusInProgress, taskmodule.AssignmentAccepted, personnelmodule.WorkStateBusy, now); err != nil {
		return err
	}
	return s.writeTaskEvent(ctx, tx, command, task, assignment, EventTaskStarted, taskmodule.StatusInProgress, task.SyncVersion+1, now)
}

func (s *EmployeeCommandService) complete(ctx context.Context, tx EmployeeCommandTransaction, command event.CommandEnvelope, payload employeeTaskCommandPayload, task taskmodule.Instance, assignment taskmodule.Assignment, person personnelmodule.CandidateRecord) error {
	now := s.clock.Now().UTC()
	if err := tx.UpdateAssignmentCompleted(ctx, assignment.ID, assignment.StatusVersion, now); err != nil {
		return fmt.Errorf("complete assignment: %w", err)
	}
	if err := tx.UpdateTaskCompleted(ctx, task.ID, task.StatusVersion, now); err != nil {
		return fmt.Errorf("mark task completed: %w", err)
	}
	if err := tx.UpdatePersonnelIdle(ctx, person.ID, person.StatusVersion, now); err != nil {
		return fmt.Errorf("mark personnel idle: %w", err)
	}
	if err := s.writeTransitionHistory(ctx, tx, command, payload.Note, task, assignment, person, taskmodule.StatusCompleted, taskmodule.AssignmentCompleted, personnelmodule.WorkStateIdle, now); err != nil {
		return err
	}
	return s.writeTaskEvent(ctx, tx, command, task, assignment, EventTaskCompleted, taskmodule.StatusCompleted, task.SyncVersion+1, now)
}

func (s *EmployeeCommandService) writeTransitionHistory(ctx context.Context, tx EmployeeCommandTransaction, command event.CommandEnvelope, note string, task taskmodule.Instance, assignment taskmodule.Assignment, person personnelmodule.CandidateRecord, taskStatus taskmodule.Status, assignmentStatus taskmodule.AssignmentStatus, personnelState personnelmodule.WorkState, occurredAt time.Time) error {
	reason := strings.TrimSpace(note)
	if reason == "" {
		reason = strings.TrimSuffix(strings.TrimPrefix(command.CommandType, "employee_"), "_task")
	}
	fromTask := task.Status
	taskHistoryID, err := id.NewPublicID()
	if err != nil {
		return fmt.Errorf("generate task history id: %w", err)
	}
	if err := tx.CreateTaskStatusHistory(ctx, taskmodule.StatusHistory{PublicID: taskHistoryID, TaskID: task.ID, StatusVersion: task.StatusVersion + 1, FromStatus: &fromTask, ToStatus: taskStatus, Reason: reason, ActorType: string(security.HumanPrincipal), ActorPublicID: command.ActorPublicID, CommandID: command.CommandID, OccurredAt: occurredAt}); err != nil {
		return fmt.Errorf("create task status history: %w", err)
	}
	fromAssignment := assignment.Status
	assignmentHistoryID, err := id.NewPublicID()
	if err != nil {
		return fmt.Errorf("generate assignment history id: %w", err)
	}
	if err := tx.CreateAssignmentStatusHistory(ctx, taskmodule.AssignmentStatusHistory{PublicID: assignmentHistoryID, AssignmentID: assignment.ID, StatusVersion: assignment.StatusVersion + 1, FromStatus: &fromAssignment, ToStatus: assignmentStatus, Reason: reason, ActorType: string(security.HumanPrincipal), ActorPublicID: command.ActorPublicID, CommandID: command.CommandID, OccurredAt: occurredAt}); err != nil {
		return fmt.Errorf("create assignment status history: %w", err)
	}
	fromPersonnel := person.WorkState
	personnelHistoryID, err := id.NewPublicID()
	if err != nil {
		return fmt.Errorf("generate personnel history id: %w", err)
	}
	if err := tx.CreatePersonnelStatusHistory(ctx, personnelmodule.StatusHistory{PublicID: personnelHistoryID, PersonnelID: person.ID, StatusVersion: person.StatusVersion + 1, FromState: &fromPersonnel, ToState: personnelState, Reason: reason, ActorType: string(security.HumanPrincipal), ActorPublicID: command.ActorPublicID, AssignmentID: assignment.ID, CommandID: command.CommandID, OccurredAt: occurredAt}); err != nil {
		return fmt.Errorf("create personnel status history: %w", err)
	}
	if err := tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: string(security.HumanPrincipal), ActorID: command.ActorPublicID, Action: command.CommandType, ResourceType: "task", ResourceID: task.PublicID, Result: ResultCommandApplied, TraceID: command.TraceID, OccurredAt: occurredAt}); err != nil {
		return fmt.Errorf("append employee command audit: %w", err)
	}
	return nil
}

func (s *EmployeeCommandService) writeTaskEvent(ctx context.Context, tx EmployeeCommandTransaction, command event.CommandEnvelope, task taskmodule.Instance, assignment taskmodule.Assignment, eventType string, status taskmodule.Status, syncVersion uint64, occurredAt time.Time) error {
	payload := TaskProjectionEventPayload{TaskPublicID: task.PublicID, AssignmentPublicID: assignment.PublicID, EmployeePublicID: assignment.PersonnelPublicID, FlightDisplayNo: task.FlightDisplayNo, TaskName: task.Name, AreaName: task.AreaName, PlannedAt: task.PlannedAt.UTC(), BusinessStatus: string(status), Message: task.Message, SyncVersion: syncVersion, ReceiptStatus: string(assignment.ReceiptStatus), ReceivedAt: assignment.ReceivedAt}
	envelope, err := event.NewEvent(eventType, "task", task.PublicID, "core-flight-task", payload)
	if err != nil {
		return fmt.Errorf("create %s event: %w", eventType, err)
	}
	envelope.TraceID = command.TraceID
	envelope.CorrelationID = command.CommandID
	envelope.OccurredAt = occurredAt
	if err := tx.AppendOutbox(ctx, envelope); err != nil {
		return fmt.Errorf("append %s outbox: %w", eventType, err)
	}
	return nil
}
