package flighttask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	OperationTaskCancel               = "task_cancel"
	CommandTaskCancel                 = "task.cancel.v1"
	EventTaskCancelled                = "task.cancelled.v1"
	ResultCancellationApplied         = "cancelled"
	ResultTaskAlreadyCancelled        = "already_cancelled"
	ResultTaskAlreadyCompleted        = "already_completed"
	ResultCancellationForbidden       = "forbidden"
	ResultCancellationIDConflict      = "cancellation_id_conflict"
	ResultCancellationVersionConflict = "stale_task_version"
	ResultCancellationInvalidState    = "invalid_state"
	ResultCancellationTaskNotFound    = "task_not_found"
)

var (
	ErrCancellationIDConflict      = &BusinessError{Code: ResultCancellationIDConflict, Message: "cancellation id was already used with different content"}
	ErrCancellationForbidden       = &BusinessError{Code: ResultCancellationForbidden, Message: "principal is not allowed to cancel this task"}
	ErrCancellationVersionConflict = &BusinessError{Code: ResultCancellationVersionConflict, Message: "task version is stale"}
	ErrCancellationInvalidState    = &BusinessError{Code: ResultCancellationInvalidState, Message: "task is not cancellable in its current state"}
	ErrCancellationTaskNotFound    = &BusinessError{Code: ResultCancellationTaskNotFound, Message: "task not found"}
	ErrCancellationInvalidInput    = &BusinessError{Code: "invalid_input", Message: "invalid task cancellation input"}
)

type CancellationInput struct {
	Principal           security.Principal
	TaskPublicID        string
	CancellationID      string
	ExpectedTaskVersion uint64
	Reason              string
	RequestID           string
	TraceID             string
	SourceIP            string
}

type CancellationResult struct {
	ResultCode         string `json:"result_code"`
	Duplicate          bool   `json:"duplicate"`
	TaskPublicID       string `json:"task_public_id"`
	TaskStatus         string `json:"task_status,omitempty"`
	TaskVersion        uint64 `json:"task_version"`
	AssignmentPublicID string `json:"assignment_public_id,omitempty"`
	AssignmentStatus   string `json:"assignment_status,omitempty"`
	PersonnelPublicID  string `json:"personnel_public_id,omitempty"`
	PersonnelWorkState string `json:"personnel_work_state,omitempty"`
	Reason             string `json:"reason,omitempty"`
	SyncVersion        uint64 `json:"sync_version"`
}

// CancellationRepository owns the Core transaction for explicit management
// cancellation. It is separate from the confirmation port so each use case
// exposes only the state transitions it is allowed to perform.
type CancellationRepository interface {
	FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error)
	WithinCancellationTransaction(ctx context.Context, fn func(CancellationTransaction) error) error
}

type CancellationTransaction interface {
	FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error)
	FindTaskForUpdate(ctx context.Context, publicID string) (taskmodule.Instance, error)
	FindActiveAssignmentForUpdate(ctx context.Context, taskID uint64) (taskmodule.Assignment, error)
	FindPersonnelForUpdate(ctx context.Context, personnelID uint64) (personnelmodule.CandidateRecord, error)

	InvalidateProposedCandidates(ctx context.Context, taskID uint64, reason string, changedAt time.Time) error
	UpdateTaskCancelled(ctx context.Context, taskID, expectedStatusVersion uint64, fromStatus taskmodule.Status, reason string, changedAt time.Time) error
	UpdateAssignmentCancelled(ctx context.Context, assignmentID, expectedStatusVersion uint64, fromStatus taskmodule.AssignmentStatus, cancellationID, reason string, changedAt time.Time) error
	ReleasePersonnelIdle(ctx context.Context, personnelID, expectedStatusVersion uint64, fromState personnelmodule.WorkState, changedAt time.Time) error

	CreateTaskStatusHistory(ctx context.Context, value taskmodule.StatusHistory) error
	CreateAssignmentStatusHistory(ctx context.Context, value taskmodule.AssignmentStatusHistory) error
	CreatePersonnelStatusHistory(ctx context.Context, value personnelmodule.StatusHistory) error
	CreateBusinessIdempotency(ctx context.Context, value IdempotencyRecord) error
	AppendAudit(ctx context.Context, value coresync.AuditRecord) error
	AppendOutbox(ctx context.Context, value event.EventEnvelope) error
}

type CancellationService struct {
	repository CancellationRepository
	authorizer security.Authorizer
	clock      clock.Clock
}

func NewCancellationService(repository CancellationRepository, authorizer security.Authorizer, now clock.Clock) *CancellationService {
	if authorizer == nil {
		authorizer = iam.NewAuthorizer()
	}
	if now == nil {
		now = clock.Real{}
	}
	return &CancellationService{repository: repository, authorizer: authorizer, clock: now}
}

func (s *CancellationService) CancelTask(ctx context.Context, input CancellationInput) (CancellationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.repository == nil {
		return CancellationResult{}, ErrRepositoryNotConfigured
	}
	normalized, err := normalizeCancellationInput(input)
	if err != nil {
		return CancellationResult{}, err
	}
	requestHash, err := hashCancellationInput(normalized)
	if err != nil {
		return CancellationResult{}, fmt.Errorf("hash task cancellation input: %w", err)
	}

	if existing, findErr := s.repository.FindBusinessIdempotency(ctx, OperationTaskCancel, normalized.CancellationID); findErr == nil {
		return replayCancellationResult(existing, requestHash)
	} else if !errors.Is(findErr, ErrNotFound) {
		return CancellationResult{}, fmt.Errorf("find task cancellation idempotency: %w", findErr)
	}

	var result CancellationResult
	var businessErr error
	err = s.repository.WithinCancellationTransaction(ctx, func(tx CancellationTransaction) error {
		if existing, findErr := tx.FindBusinessIdempotency(ctx, OperationTaskCancel, normalized.CancellationID); findErr == nil {
			result, businessErr = replayCancellationResult(existing, requestHash)
			return nil
		} else if !errors.Is(findErr, ErrNotFound) {
			return fmt.Errorf("recheck task cancellation idempotency: %w", findErr)
		}

		now := s.now()
		task, findErr := tx.FindTaskForUpdate(ctx, normalized.TaskPublicID)
		if findErr != nil {
			if !errors.Is(findErr, ErrNotFound) {
				return fmt.Errorf("find task %s for cancellation: %w", normalized.TaskPublicID, findErr)
			}
			result = CancellationResult{ResultCode: ResultCancellationTaskNotFound, TaskPublicID: normalized.TaskPublicID, Reason: normalized.Reason}
			businessErr = wrapBusiness(ErrCancellationTaskNotFound, findErr)
			return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		if authErr := s.authorize(normalized.Principal, task); authErr != nil {
			result = cancellationResultForTask(ResultCancellationForbidden, task, normalized.Reason)
			businessErr = wrapBusiness(ErrCancellationForbidden, authErr)
			return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		switch task.Status {
		case taskmodule.StatusCancelled:
			result = cancellationResultForTask(ResultTaskAlreadyCancelled, task, normalized.Reason)
			return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencySucceeded, nil, now)
		case taskmodule.StatusCompleted:
			result = cancellationResultForTask(ResultTaskAlreadyCompleted, task, normalized.Reason)
			return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencySucceeded, nil, now)
		case taskmodule.StatusAwaitingConfirmation, taskmodule.StatusAssigned, taskmodule.StatusInProgress:
			// These are the only non-terminal states in the frozen cancellation
			// state machine. The leader restriction is stricter than the shared
			// task:cancel permission and is therefore checked here.
		default:
			result = cancellationResultForTask(ResultCancellationInvalidState, task, normalized.Reason)
			businessErr = wrapBusiness(ErrCancellationInvalidState, fmt.Errorf("task status is %s", task.Status))
			return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		if cancellationLeaderOnly(normalized.Principal) && task.Status == taskmodule.StatusInProgress {
			result = cancellationResultForTask(ResultCancellationForbidden, task, normalized.Reason)
			businessErr = wrapBusiness(ErrCancellationForbidden, fmt.Errorf("leader cannot cancel an in-progress task"))
			return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}
		if task.StatusVersion != normalized.ExpectedTaskVersion {
			result = cancellationResultForTask(ResultCancellationVersionConflict, task, normalized.Reason)
			businessErr = wrapBusiness(ErrCancellationVersionConflict, fmt.Errorf("expected version %d, current version %d", normalized.ExpectedTaskVersion, task.StatusVersion))
			return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		var assignment taskmodule.Assignment
		var person personnelmodule.CandidateRecord
		if task.Status == taskmodule.StatusAwaitingConfirmation {
			if err := tx.InvalidateProposedCandidates(ctx, task.ID, normalized.Reason, now); err != nil {
				return fmt.Errorf("invalidate proposed candidates for task %s: %w", task.PublicID, err)
			}
		} else {
			assignment, findErr = tx.FindActiveAssignmentForUpdate(ctx, task.ID)
			if findErr != nil {
				if errors.Is(findErr, ErrNotFound) {
					result = cancellationResultForTask(ResultCancellationInvalidState, task, normalized.Reason)
					businessErr = wrapBusiness(ErrCancellationInvalidState, fmt.Errorf("task %s has no active assignment", task.PublicID))
					return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
				}
				return fmt.Errorf("find active assignment for task %s: %w", task.PublicID, findErr)
			}
			person, findErr = tx.FindPersonnelForUpdate(ctx, assignment.PersonnelID)
			if findErr != nil {
				return fmt.Errorf("find assigned personnel for task %s: %w", task.PublicID, findErr)
			}
			expectedAssignmentStatus := taskmodule.AssignmentConfirmed
			expectedPersonnelState := personnelmodule.WorkStateReserved
			if task.Status == taskmodule.StatusInProgress {
				expectedAssignmentStatus = taskmodule.AssignmentAccepted
				expectedPersonnelState = personnelmodule.WorkStateBusy
			}
			if assignment.Status != expectedAssignmentStatus || person.WorkState != expectedPersonnelState {
				result = cancellationResultForAssignment(ResultCancellationInvalidState, task, assignment, person, normalized.Reason)
				businessErr = wrapBusiness(ErrCancellationInvalidState, fmt.Errorf("assignment is %s and personnel is %s", assignment.Status, person.WorkState))
				return s.persistCancellationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
			}
			if err := tx.UpdateAssignmentCancelled(ctx, assignment.ID, assignment.StatusVersion, assignment.Status, normalized.CancellationID, normalized.Reason, now); err != nil {
				return fmt.Errorf("cancel assignment %s: %w", assignment.PublicID, err)
			}
			if err := tx.ReleasePersonnelIdle(ctx, person.ID, person.StatusVersion, person.WorkState, now); err != nil {
				return fmt.Errorf("release personnel %s: %w", person.PublicID, err)
			}
		}

		if err := tx.UpdateTaskCancelled(ctx, task.ID, task.StatusVersion, task.Status, normalized.Reason, now); err != nil {
			return fmt.Errorf("cancel task %s: %w", task.PublicID, err)
		}
		fromTaskStatus := task.Status
		taskHistoryID, err := id.NewPublicID()
		if err != nil {
			return fmt.Errorf("generate task cancellation history public id: %w", err)
		}
		if err := tx.CreateTaskStatusHistory(ctx, taskmodule.StatusHistory{PublicID: taskHistoryID, TaskID: task.ID, StatusVersion: task.StatusVersion + 1, FromStatus: &fromTaskStatus, ToStatus: taskmodule.StatusCancelled, Reason: normalized.Reason, ActorType: string(normalized.Principal.Type), ActorPublicID: normalized.Principal.PublicID, CommandID: normalized.CancellationID, OccurredAt: now}); err != nil {
			return fmt.Errorf("create task cancellation history: %w", err)
		}

		if assignment.ID != 0 {
			fromAssignmentStatus := assignment.Status
			assignmentHistoryID, err := id.NewPublicID()
			if err != nil {
				return fmt.Errorf("generate assignment cancellation history public id: %w", err)
			}
			if err := tx.CreateAssignmentStatusHistory(ctx, taskmodule.AssignmentStatusHistory{PublicID: assignmentHistoryID, AssignmentID: assignment.ID, StatusVersion: assignment.StatusVersion + 1, FromStatus: &fromAssignmentStatus, ToStatus: taskmodule.AssignmentCancelled, Reason: normalized.Reason, ActorType: string(normalized.Principal.Type), ActorPublicID: normalized.Principal.PublicID, CommandID: normalized.CancellationID, ConfirmationID: assignment.ConfirmationID, CancellationID: normalized.CancellationID, OccurredAt: now}); err != nil {
				return fmt.Errorf("create assignment cancellation history: %w", err)
			}
			fromPersonnelState := person.WorkState
			personnelHistoryID, err := id.NewPublicID()
			if err != nil {
				return fmt.Errorf("generate personnel cancellation history public id: %w", err)
			}
			if err := tx.CreatePersonnelStatusHistory(ctx, personnelmodule.StatusHistory{PublicID: personnelHistoryID, PersonnelID: person.ID, StatusVersion: person.StatusVersion + 1, FromState: &fromPersonnelState, ToState: personnelmodule.WorkStateIdle, Reason: normalized.Reason, ActorType: string(normalized.Principal.Type), ActorPublicID: normalized.Principal.PublicID, AssignmentID: assignment.ID, CommandID: normalized.CancellationID, OccurredAt: now}); err != nil {
				return fmt.Errorf("create personnel cancellation history: %w", err)
			}
		}

		result = cancellationResultForTask(ResultCancellationApplied, task, normalized.Reason)
		result.TaskStatus = string(taskmodule.StatusCancelled)
		result.TaskVersion = task.StatusVersion + 1
		result.SyncVersion = task.SyncVersion + 1
		if assignment.ID != 0 {
			result.AssignmentPublicID = assignment.PublicID
			result.AssignmentStatus = string(taskmodule.AssignmentCancelled)
			result.PersonnelPublicID = person.PublicID
			result.PersonnelWorkState = string(personnelmodule.WorkStateIdle)
		}
		cancelledEvent, err := newTaskCancelledEvent(task, assignment, normalized.CancellationID, normalized.TraceID, now)
		if err != nil {
			return err
		}
		if err := tx.AppendOutbox(ctx, cancelledEvent); err != nil {
			return fmt.Errorf("append task cancelled outbox: %w", err)
		}
		if err := s.appendCancellationAudit(ctx, tx, normalized, task.PublicID, result.ResultCode, now); err != nil {
			return err
		}
		return s.persistCancellationOutcome(ctx, tx, normalized, requestHash, result, idempotencySucceeded, "", now)
	})
	if err != nil {
		if errors.Is(err, ErrDuplicate) {
			if existing, findErr := s.repository.FindBusinessIdempotency(ctx, OperationTaskCancel, normalized.CancellationID); findErr == nil {
				return replayCancellationResult(existing, requestHash)
			} else if !errors.Is(findErr, ErrNotFound) {
				return CancellationResult{}, fmt.Errorf("load task cancellation after duplicate: %w", findErr)
			}
		}
		return CancellationResult{}, fmt.Errorf("cancel task: %w", err)
	}
	if businessErr != nil {
		return result, businessErr
	}
	return result, nil
}

func normalizeCancellationInput(input CancellationInput) (CancellationInput, error) {
	input.TaskPublicID = strings.TrimSpace(input.TaskPublicID)
	input.CancellationID = strings.TrimSpace(input.CancellationID)
	input.Reason = strings.TrimSpace(input.Reason)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.TraceID = strings.TrimSpace(input.TraceID)
	input.SourceIP = strings.TrimSpace(input.SourceIP)
	if input.TaskPublicID == "" || input.CancellationID == "" || input.Reason == "" {
		return CancellationInput{}, ErrCancellationInvalidInput
	}
	if len(input.TaskPublicID) > 36 || len(input.CancellationID) > 128 || len(input.Reason) > 255 || len(input.Principal.PublicID) > 36 || len(input.RequestID) > 128 || len(input.TraceID) > 128 || len(input.SourceIP) > 64 {
		return CancellationInput{}, ErrCancellationInvalidInput
	}
	var err error
	if input.RequestID == "" {
		input.RequestID, err = id.NewPublicID()
		if err != nil {
			return CancellationInput{}, fmt.Errorf("generate cancellation request id: %w", err)
		}
	}
	if input.TraceID == "" {
		input.TraceID, err = id.NewPublicID()
		if err != nil {
			return CancellationInput{}, fmt.Errorf("generate cancellation trace id: %w", err)
		}
	}
	if input.SourceIP == "" {
		input.SourceIP = "internal"
	}
	return input, nil
}

func hashCancellationInput(input CancellationInput) (string, error) {
	canonical := struct {
		TaskPublicID        string   `json:"task_public_id"`
		CancellationID      string   `json:"cancellation_id"`
		ExpectedTaskVersion uint64   `json:"expected_task_version"`
		Reason              string   `json:"reason"`
		ActorType           string   `json:"actor_type"`
		ActorPublicID       string   `json:"actor_public_id"`
		Roles               []string `json:"roles"`
	}{TaskPublicID: input.TaskPublicID, CancellationID: input.CancellationID, ExpectedTaskVersion: input.ExpectedTaskVersion, Reason: input.Reason, ActorType: string(input.Principal.Type), ActorPublicID: input.Principal.PublicID, Roles: input.Principal.Roles}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *CancellationService) authorize(principal security.Principal, task taskmodule.Instance) error {
	if principal.Type != security.HumanPrincipal {
		return fmt.Errorf("only human principals may cancel tasks")
	}
	if err := s.authorizer.Authorize(principal, "task:cancel", security.AccessScope{TeamIDs: []uint64{task.TeamID}}); err != nil {
		return err
	}
	if err := s.authorizer.Authorize(principal, "task:cancel", security.AccessScope{AreaIDs: []uint64{task.AreaID}}); err != nil {
		return err
	}
	return nil
}

func cancellationLeaderOnly(principal security.Principal) bool {
	leader := false
	managerOrAdmin := false
	for _, role := range principal.Roles {
		switch role {
		case security.RoleLeader:
			leader = true
		case security.RoleManager, security.RoleAdmin:
			managerOrAdmin = true
		}
	}
	return leader && !managerOrAdmin
}

func cancellationResultForTask(code string, task taskmodule.Instance, reason string) CancellationResult {
	return CancellationResult{ResultCode: code, TaskPublicID: task.PublicID, TaskStatus: string(task.Status), TaskVersion: task.StatusVersion, Reason: reason, SyncVersion: task.SyncVersion}
}

func cancellationResultForAssignment(code string, task taskmodule.Instance, assignment taskmodule.Assignment, person personnelmodule.CandidateRecord, reason string) CancellationResult {
	result := cancellationResultForTask(code, task, reason)
	result.AssignmentPublicID = assignment.PublicID
	result.AssignmentStatus = string(assignment.Status)
	result.PersonnelPublicID = person.PublicID
	result.PersonnelWorkState = string(person.WorkState)
	return result
}

func (s *CancellationService) persistCancellationOutcomeAndAudit(ctx context.Context, tx CancellationTransaction, input CancellationInput, requestHash string, result CancellationResult, status string, businessErr error, processedAt time.Time) error {
	if err := s.appendCancellationAudit(ctx, tx, input, result.TaskPublicID, result.ResultCode, processedAt); err != nil {
		return err
	}
	return s.persistCancellationOutcome(ctx, tx, input, requestHash, result, status, errorSummary(businessErr), processedAt)
}

func (s *CancellationService) persistCancellationOutcome(ctx context.Context, tx CancellationTransaction, input CancellationInput, requestHash string, result CancellationResult, status, errorSummaryValue string, processedAt time.Time) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal task cancellation result: %w", err)
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return fmt.Errorf("generate task cancellation idempotency public id: %w", err)
	}
	return tx.CreateBusinessIdempotency(ctx, IdempotencyRecord{PublicID: publicID, OperationType: OperationTaskCancel, IdempotencyKey: input.CancellationID, RequestHash: requestHash, Status: status, AggregateType: "task", AggregatePublicID: result.TaskPublicID, ActorType: string(input.Principal.Type), ActorPublicID: input.Principal.PublicID, CommandType: CommandTaskCancel, ResultCode: result.ResultCode, ResultStatus: result.TaskStatus, ResultPayload: payload, ErrorSummary: errorSummaryValue, RequestID: input.RequestID, TraceID: input.TraceID, FirstProcessedAt: processedAt, LastProcessedAt: processedAt})
}

func (s *CancellationService) appendCancellationAudit(ctx context.Context, tx CancellationTransaction, input CancellationInput, resourceID, result string, occurredAt time.Time) error {
	return tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: string(input.Principal.Type), ActorID: input.Principal.PublicID, Action: "task.cancel", ResourceType: "task", ResourceID: resourceID, Result: result, RequestID: input.RequestID, TraceID: input.TraceID, SourceIP: input.SourceIP, OccurredAt: occurredAt})
}

func newTaskCancelledEvent(task taskmodule.Instance, assignment taskmodule.Assignment, cancellationID, traceID string, occurredAt time.Time) (event.EventEnvelope, error) {
	payload := TaskProjectionEventPayload{TaskPublicID: task.PublicID, AssignmentPublicID: assignment.PublicID, ConfirmationID: assignment.ConfirmationID, EmployeePublicID: assignment.PersonnelPublicID, FlightDisplayNo: task.FlightDisplayNo, TaskName: task.Name, AreaName: task.AreaName, PlannedAt: task.PlannedAt.UTC(), BusinessStatus: string(taskmodule.StatusCancelled), Message: task.Message, SyncVersion: task.SyncVersion + 1}
	envelope, err := event.NewEvent(EventTaskCancelled, "task", task.PublicID, "core-flight-task", payload)
	if err != nil {
		return event.EventEnvelope{}, fmt.Errorf("create task cancelled event: %w", err)
	}
	envelope.TraceID = traceID
	envelope.CorrelationID = cancellationCorrelationID(cancellationID)
	envelope.OccurredAt = occurredAt.UTC()
	return envelope, nil
}

func cancellationCorrelationID(cancellationID string) string {
	if len(cancellationID) <= 36 {
		return cancellationID
	}
	digest := sha256.Sum256([]byte("task.cancel:" + cancellationID))
	value := digest[:16]
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func replayCancellationResult(record IdempotencyRecord, requestHash string) (CancellationResult, error) {
	if record.RequestHash != requestHash {
		return CancellationResult{}, wrapBusiness(ErrCancellationIDConflict, fmt.Errorf("request hash differs for key %q", record.IdempotencyKey))
	}
	if len(record.ResultPayload) == 0 {
		return CancellationResult{}, fmt.Errorf("idempotency record %s has no result payload", record.PublicID)
	}
	var result CancellationResult
	if err := json.Unmarshal(record.ResultPayload, &result); err != nil {
		return CancellationResult{}, fmt.Errorf("decode task cancellation result %s: %w", record.PublicID, err)
	}
	result.Duplicate = true
	if record.Status == idempotencyRejected {
		return result, cancellationBusinessError(result.ResultCode, errors.New(record.ErrorSummary))
	}
	return result, nil
}

func cancellationBusinessError(code string, cause error) error {
	if cause == nil || strings.TrimSpace(cause.Error()) == "" {
		cause = errors.New("previous task cancellation operation was rejected")
	}
	switch code {
	case ResultCancellationForbidden:
		return wrapBusiness(ErrCancellationForbidden, cause)
	case ResultCancellationVersionConflict:
		return wrapBusiness(ErrCancellationVersionConflict, cause)
	case ResultCancellationInvalidState:
		return wrapBusiness(ErrCancellationInvalidState, cause)
	case ResultCancellationTaskNotFound:
		return wrapBusiness(ErrCancellationTaskNotFound, cause)
	default:
		return &BusinessError{Code: "operation_rejected", Message: "task cancellation operation was rejected", Cause: cause}
	}
}

func (s *CancellationService) now() time.Time {
	if s != nil && s.clock != nil {
		value := s.clock.Now()
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Now().UTC()
}
