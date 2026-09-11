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

	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const (
	OperationTaskConfirm                = "task_confirm"
	CommandTaskConfirm                  = "task.confirm.v1"
	ResultTaskAssigned                  = "assigned"
	ResultTaskAlreadyAssigned           = "task_already_assigned"
	ResultCandidateNoLongerEligible     = "candidate_no_longer_eligible"
	ResultConfirmationForbidden         = "forbidden"
	ResultConfirmationIDConflict        = "confirmation_id_conflict"
	ResultTaskVersionConflict           = "stale_task_version"
	ResultConfirmationInvalidState      = "invalid_state"
	ResultConfirmationTaskNotFound      = "task_not_found"
	ResultConfirmationCandidateNotFound = "candidate_not_found"
)

var (
	ErrConfirmationIDConflict        = &BusinessError{Code: ResultConfirmationIDConflict, Message: "confirmation id was already used with different content"}
	ErrTaskAlreadyAssigned           = &BusinessError{Code: ResultTaskAlreadyAssigned, Message: "task has already been assigned"}
	ErrCandidateNoLongerEligible     = &BusinessError{Code: ResultCandidateNoLongerEligible, Message: "candidate is no longer eligible"}
	ErrConfirmationForbidden         = &BusinessError{Code: ResultConfirmationForbidden, Message: "principal is not allowed to confirm this task"}
	ErrTaskVersionConflict           = &BusinessError{Code: ResultTaskVersionConflict, Message: "task version is stale"}
	ErrConfirmationInvalidState      = &BusinessError{Code: ResultConfirmationInvalidState, Message: "task is not awaiting confirmation"}
	ErrConfirmationTaskNotFound      = &BusinessError{Code: ResultConfirmationTaskNotFound, Message: "task not found"}
	ErrConfirmationCandidateNotFound = &BusinessError{Code: ResultConfirmationCandidateNotFound, Message: "candidate not found"}
	ErrPersonnelReservationConflict  = errors.New("personnel reservation conflict")
)

// ConfirmationInput is the human command boundary. The Principal is supplied
// by authentication middleware and is deliberately not accepted in the JSON
// request body.
type ConfirmationInput struct {
	Principal           security.Principal
	TaskPublicID        string
	CandidatePublicID   string
	ConfirmationID      string
	ExpectedTaskVersion uint64
	RequestID           string
	TraceID             string
	SourceIP            string
}

type ConfirmationResult struct {
	ResultCode         string `json:"result_code"`
	Duplicate          bool   `json:"duplicate"`
	TaskPublicID       string `json:"task_public_id"`
	TaskStatus         string `json:"task_status,omitempty"`
	TaskVersion        uint64 `json:"task_version"`
	CandidatePublicID  string `json:"candidate_public_id"`
	AssignmentPublicID string `json:"assignment_public_id,omitempty"`
	AssignmentStatus   string `json:"assignment_status,omitempty"`
	PersonnelPublicID  string `json:"personnel_public_id,omitempty"`
	PersonnelWorkState string `json:"personnel_work_state,omitempty"`
	SyncVersion        uint64 `json:"sync_version"`
}

// TaskProjectionEventPayload is the minimum immutable snapshot Edge needs to
// render an assigned task and its later Core-confirmed state changes. Edge
// never becomes the source of these facts.
type TaskProjectionEventPayload struct {
	TaskPublicID       string     `json:"task_public_id"`
	AssignmentPublicID string     `json:"assignment_public_id"`
	ConfirmationID     string     `json:"confirmation_id"`
	EmployeePublicID   string     `json:"employee_public_id"`
	FlightDisplayNo    string     `json:"flight_display_no"`
	TaskName           string     `json:"task_name"`
	AreaName           string     `json:"area_name"`
	PlannedAt          time.Time  `json:"planned_at"`
	BusinessStatus     string     `json:"business_status"`
	Message            string     `json:"message"`
	SyncVersion        uint64     `json:"sync_version"`
	ReceiptStatus      string     `json:"receipt_status,omitempty"`
	ReceivedAt         *time.Time `json:"received_at,omitempty"`
}

// TaskAssignedEventPayload remains an alias for callers of the BVS2-04 API.
type TaskAssignedEventPayload = TaskProjectionEventPayload

// ConfirmationRepository is intentionally separate from the arrival port: its
// transaction contains the Personnel and Assignment writes needed by Leader
// Confirm, without widening the already-tested BVS2-03 fake interfaces.
type ConfirmationRepository interface {
	FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error)
	WithinConfirmationTransaction(ctx context.Context, fn func(ConfirmationTransaction) error) error
}

type ConfirmationTransaction interface {
	FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error)
	FindTaskForUpdate(ctx context.Context, publicID string) (taskmodule.Instance, error)
	FindCandidateForUpdate(ctx context.Context, taskID uint64, publicID string) (taskmodule.Candidate, error)
	FindPersonnelForUpdate(ctx context.Context, personnelID uint64) (personnelmodule.CandidateRecord, error)
	HasPersonnelScheduleConflict(ctx context.Context, personnelID uint64, plannedAt time.Time) (bool, error)

	ReservePersonnel(ctx context.Context, personnelID, expectedStatusVersion uint64, changedAt time.Time) error
	InvalidateCandidate(ctx context.Context, candidateID uint64, reason string, changedAt time.Time) error
	MarkCandidatesConfirmed(ctx context.Context, taskID, selectedCandidateID uint64, changedAt time.Time) error
	UpdateTaskAssigned(ctx context.Context, taskID, expectedStatusVersion uint64, changedAt time.Time) error

	CreateAssignment(ctx context.Context, value *taskmodule.Assignment) error
	CreateTaskStatusHistory(ctx context.Context, value taskmodule.StatusHistory) error
	CreateAssignmentStatusHistory(ctx context.Context, value taskmodule.AssignmentStatusHistory) error
	CreatePersonnelStatusHistory(ctx context.Context, value personnelmodule.StatusHistory) error
	CreateBusinessIdempotency(ctx context.Context, value IdempotencyRecord) error
	AppendAudit(ctx context.Context, value coresync.AuditRecord) error
	AppendOutbox(ctx context.Context, value event.EventEnvelope) error
}

type ConfirmationService struct {
	repository ConfirmationRepository
	authorizer security.Authorizer
	clock      clock.Clock
}

func NewConfirmationService(repository ConfirmationRepository, authorizer security.Authorizer, now clock.Clock) *ConfirmationService {
	if authorizer == nil {
		// The default policy is the frozen v2 policy. Callers may inject a
		// policy-backed implementation later without changing this use case.
		authorizer = newDefaultAuthorizer()
	}
	if now == nil {
		now = clock.Real{}
	}
	return &ConfirmationService{repository: repository, authorizer: authorizer, clock: now}
}

// newDefaultAuthorizer is kept in this package to avoid coupling the use case
// to the IAM module's concrete policy implementation.
func newDefaultAuthorizer() security.Authorizer {
	return defaultAuthorizer{}
}

type defaultAuthorizer struct{}

func (defaultAuthorizer) Authorize(principal security.Principal, permission security.Permission, scope security.AccessScope) error {
	// This branch is replaced by the concrete IAM authorizer in the Core API.
	// Keeping a denying fallback prevents an accidentally unconfigured service
	// from granting a business write.
	return fmt.Errorf("authorization policy is not configured: %w", ErrConfirmationForbidden)
}

func (s *ConfirmationService) ConfirmTask(ctx context.Context, input ConfirmationInput) (ConfirmationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.repository == nil {
		return ConfirmationResult{}, ErrRepositoryNotConfigured
	}
	normalized, err := normalizeConfirmationInput(input)
	if err != nil {
		return ConfirmationResult{}, err
	}
	requestHash, err := hashConfirmationInput(normalized)
	if err != nil {
		return ConfirmationResult{}, fmt.Errorf("hash task confirmation input: %w", err)
	}

	if existing, findErr := s.repository.FindBusinessIdempotency(ctx, OperationTaskConfirm, normalized.ConfirmationID); findErr == nil {
		return replayConfirmationResult(existing, requestHash)
	} else if !errors.Is(findErr, ErrNotFound) {
		return ConfirmationResult{}, fmt.Errorf("find task confirmation idempotency: %w", findErr)
	}

	var result ConfirmationResult
	var businessErr error
	err = s.repository.WithinConfirmationTransaction(ctx, func(tx ConfirmationTransaction) error {
		// Recheck after entering the transaction so concurrent retries cannot
		// produce a second Assignment or Outbox event.
		if existing, findErr := tx.FindBusinessIdempotency(ctx, OperationTaskConfirm, normalized.ConfirmationID); findErr == nil {
			result, businessErr = replayConfirmationResult(existing, requestHash)
			return nil
		} else if !errors.Is(findErr, ErrNotFound) {
			return fmt.Errorf("recheck task confirmation idempotency: %w", findErr)
		}

		now := s.now()
		task, findErr := tx.FindTaskForUpdate(ctx, normalized.TaskPublicID)
		if findErr != nil {
			if !errors.Is(findErr, ErrNotFound) {
				return fmt.Errorf("find task %s for confirmation: %w", normalized.TaskPublicID, findErr)
			}
			result = ConfirmationResult{ResultCode: ResultConfirmationTaskNotFound, TaskPublicID: normalized.TaskPublicID}
			businessErr = wrapBusiness(ErrConfirmationTaskNotFound, findErr)
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		if authErr := s.authorize(normalized.Principal, task); authErr != nil {
			result = confirmationResultForTask(ResultConfirmationForbidden, task, normalized.CandidatePublicID)
			businessErr = wrapBusiness(ErrConfirmationForbidden, authErr)
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}
		if task.Status == taskmodule.StatusAssigned {
			result = confirmationResultForTask(ResultTaskAlreadyAssigned, task, normalized.CandidatePublicID)
			businessErr = wrapBusiness(ErrTaskAlreadyAssigned, fmt.Errorf("task status is %s", task.Status))
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}
		if task.Status != taskmodule.StatusPendingDispatch && task.Status != taskmodule.StatusAwaitingConfirmation {
			result = confirmationResultForTask(ResultConfirmationInvalidState, task, normalized.CandidatePublicID)
			businessErr = wrapBusiness(ErrConfirmationInvalidState, fmt.Errorf("task status is %s", task.Status))
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}
		if task.StatusVersion != normalized.ExpectedTaskVersion {
			result = confirmationResultForTask(ResultTaskVersionConflict, task, normalized.CandidatePublicID)
			businessErr = wrapBusiness(ErrTaskVersionConflict, fmt.Errorf("expected version %d, current version %d", normalized.ExpectedTaskVersion, task.StatusVersion))
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		candidate, candidateErr := tx.FindCandidateForUpdate(ctx, task.ID, normalized.CandidatePublicID)
		if candidateErr != nil {
			if !errors.Is(candidateErr, ErrNotFound) {
				return fmt.Errorf("find candidate %s for confirmation: %w", normalized.CandidatePublicID, candidateErr)
			}
			result = confirmationResultForTask(ResultConfirmationCandidateNotFound, task, normalized.CandidatePublicID)
			businessErr = wrapBusiness(ErrConfirmationCandidateNotFound, candidateErr)
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}
		person, personErr := tx.FindPersonnelForUpdate(ctx, candidate.PersonnelID)
		if personErr != nil {
			if !errors.Is(personErr, ErrNotFound) {
				return fmt.Errorf("find personnel for confirmation: %w", personErr)
			}
			if err := tx.InvalidateCandidate(ctx, candidate.ID, ResultCandidateNoLongerEligible, now); err != nil {
				return fmt.Errorf("invalidate missing-personnel candidate: %w", err)
			}
			result = confirmationResultForCandidate(ResultCandidateNoLongerEligible, task, candidate, personnelmodule.CandidateRecord{})
			businessErr = wrapBusiness(ErrCandidateNoLongerEligible, personErr)
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		eligible, conflictErr := s.candidateStillEligible(ctx, tx, task, candidate, person)
		if conflictErr != nil {
			return conflictErr
		}
		if !eligible {
			if err := tx.InvalidateCandidate(ctx, candidate.ID, ResultCandidateNoLongerEligible, now); err != nil {
				return fmt.Errorf("invalidate ineligible candidate: %w", err)
			}
			result = confirmationResultForCandidate(ResultCandidateNoLongerEligible, task, candidate, person)
			businessErr = wrapBusiness(ErrCandidateNoLongerEligible, fmt.Errorf("candidate no longer satisfies current personnel facts"))
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		if err := tx.ReservePersonnel(ctx, person.ID, person.StatusVersion, now); err != nil {
			if !errors.Is(err, ErrPersonnelReservationConflict) {
				return fmt.Errorf("reserve personnel %s: %w", person.PublicID, err)
			}
			if invalidateErr := tx.InvalidateCandidate(ctx, candidate.ID, ResultCandidateNoLongerEligible, now); invalidateErr != nil {
				return fmt.Errorf("invalidate reservation-conflict candidate: %w", invalidateErr)
			}
			result = confirmationResultForCandidate(ResultCandidateNoLongerEligible, task, candidate, person)
			businessErr = wrapBusiness(ErrCandidateNoLongerEligible, err)
			return s.persistConfirmationOutcomeAndAudit(ctx, tx, normalized, requestHash, result, idempotencyRejected, businessErr, now)
		}

		assignmentPublicID, err := id.NewPublicID()
		if err != nil {
			return fmt.Errorf("generate assignment public id: %w", err)
		}
		assignment := taskmodule.Assignment{
			PublicID: assignmentPublicID, TaskID: task.ID, CandidateID: candidate.ID,
			PersonnelID: person.ID, PersonnelPublicID: person.PublicID,
			Status: taskmodule.AssignmentConfirmed, StatusVersion: 0,
			ReceiptStatus:  taskmodule.AssignmentReceiptPending,
			ConfirmationID: normalized.ConfirmationID, ConfirmedByPublicID: normalized.Principal.PublicID,
			ConfirmedAt: now,
		}
		if err := tx.CreateAssignment(ctx, &assignment); err != nil {
			return fmt.Errorf("create task assignment: %w", err)
		}
		if err := tx.MarkCandidatesConfirmed(ctx, task.ID, candidate.ID, now); err != nil {
			return fmt.Errorf("finalize task candidates: %w", err)
		}
		if err := tx.UpdateTaskAssigned(ctx, task.ID, task.StatusVersion, now); err != nil {
			return fmt.Errorf("mark task assigned: %w", err)
		}

		fromTaskStatus := task.Status
		taskHistoryID, err := id.NewPublicID()
		if err != nil {
			return fmt.Errorf("generate task confirmation history public id: %w", err)
		}
		if err := tx.CreateTaskStatusHistory(ctx, taskmodule.StatusHistory{
			PublicID: taskHistoryID, TaskID: task.ID, StatusVersion: task.StatusVersion + 1,
			FromStatus: &fromTaskStatus, ToStatus: taskmodule.StatusAssigned,
			ActorType: string(normalized.Principal.Type), ActorPublicID: normalized.Principal.PublicID,
			CommandID: normalized.ConfirmationID, OccurredAt: now,
		}); err != nil {
			return fmt.Errorf("create task assigned history: %w", err)
		}

		assignmentHistoryID, err := id.NewPublicID()
		if err != nil {
			return fmt.Errorf("generate assignment history public id: %w", err)
		}
		if err := tx.CreateAssignmentStatusHistory(ctx, taskmodule.AssignmentStatusHistory{
			PublicID: assignmentHistoryID, AssignmentID: assignment.ID, StatusVersion: assignment.StatusVersion,
			ToStatus: taskmodule.AssignmentConfirmed, ActorType: string(normalized.Principal.Type),
			ActorPublicID: normalized.Principal.PublicID, ConfirmationID: normalized.ConfirmationID, OccurredAt: now,
		}); err != nil {
			return fmt.Errorf("create assignment confirmed history: %w", err)
		}

		personHistoryID, err := id.NewPublicID()
		if err != nil {
			return fmt.Errorf("generate personnel reservation history public id: %w", err)
		}
		fromPersonState := person.WorkState
		if err := tx.CreatePersonnelStatusHistory(ctx, personnelmodule.StatusHistory{
			PublicID: personHistoryID, PersonnelID: person.ID, StatusVersion: person.StatusVersion + 1,
			FromState: &fromPersonState, ToState: personnelmodule.WorkStateReserved,
			Reason: "task confirmed", ActorType: string(normalized.Principal.Type), ActorPublicID: normalized.Principal.PublicID,
			AssignmentID: assignment.ID, CommandID: normalized.ConfirmationID, OccurredAt: now,
		}); err != nil {
			return fmt.Errorf("create personnel reservation history: %w", err)
		}

		result = ConfirmationResult{
			ResultCode: ResultTaskAssigned, TaskPublicID: task.PublicID, TaskStatus: string(taskmodule.StatusAssigned),
			TaskVersion: task.StatusVersion + 1, CandidatePublicID: candidate.PublicID,
			AssignmentPublicID: assignment.PublicID, AssignmentStatus: string(assignment.Status),
			PersonnelPublicID: person.PublicID, PersonnelWorkState: string(personnelmodule.WorkStateReserved),
			SyncVersion: task.SyncVersion + 1,
		}
		assignedEvent, err := newTaskAssignedEvent(task, assignment, normalized.ConfirmationID, normalized.TraceID, now)
		if err != nil {
			return err
		}
		if err := tx.AppendOutbox(ctx, assignedEvent); err != nil {
			return fmt.Errorf("append task assigned outbox: %w", err)
		}
		if err := s.appendConfirmationAudit(ctx, tx, normalized, task.PublicID, result.ResultCode, now); err != nil {
			return err
		}
		return s.persistConfirmationOutcome(ctx, tx, normalized, requestHash, result, idempotencySucceeded, "", now)
	})
	if err != nil {
		if errors.Is(err, ErrDuplicate) {
			if existing, findErr := s.repository.FindBusinessIdempotency(ctx, OperationTaskConfirm, normalized.ConfirmationID); findErr == nil {
				return replayConfirmationResult(existing, requestHash)
			} else if !errors.Is(findErr, ErrNotFound) {
				return ConfirmationResult{}, fmt.Errorf("load task confirmation after duplicate: %w", findErr)
			}
		}
		return ConfirmationResult{}, fmt.Errorf("confirm task: %w", err)
	}
	if businessErr != nil {
		return result, businessErr
	}
	return result, nil
}

func normalizeConfirmationInput(input ConfirmationInput) (ConfirmationInput, error) {
	input.TaskPublicID = strings.TrimSpace(input.TaskPublicID)
	input.CandidatePublicID = strings.TrimSpace(input.CandidatePublicID)
	input.ConfirmationID = strings.TrimSpace(input.ConfirmationID)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.TraceID = strings.TrimSpace(input.TraceID)
	input.SourceIP = strings.TrimSpace(input.SourceIP)
	if input.TaskPublicID == "" || input.CandidatePublicID == "" || input.ConfirmationID == "" {
		return ConfirmationInput{}, ErrInvalidInput
	}
	if len(input.TaskPublicID) > 36 || len(input.CandidatePublicID) > 36 || len(input.ConfirmationID) > 128 || len(input.Principal.PublicID) > 36 || len(input.RequestID) > 128 || len(input.TraceID) > 128 || len(input.SourceIP) > 64 {
		return ConfirmationInput{}, ErrInvalidInput
	}
	var err error
	if input.RequestID == "" {
		input.RequestID, err = id.NewPublicID()
		if err != nil {
			return ConfirmationInput{}, fmt.Errorf("generate confirmation request id: %w", err)
		}
	}
	if input.TraceID == "" {
		input.TraceID, err = id.NewPublicID()
		if err != nil {
			return ConfirmationInput{}, fmt.Errorf("generate confirmation trace id: %w", err)
		}
	}
	if input.SourceIP == "" {
		input.SourceIP = "internal"
	}
	return input, nil
}

func hashConfirmationInput(input ConfirmationInput) (string, error) {
	canonical := struct {
		TaskPublicID        string   `json:"task_public_id"`
		CandidatePublicID   string   `json:"candidate_public_id"`
		ExpectedTaskVersion uint64   `json:"expected_task_version"`
		ActorType           string   `json:"actor_type"`
		ActorPublicID       string   `json:"actor_public_id"`
		Roles               []string `json:"roles"`
	}{TaskPublicID: input.TaskPublicID, CandidatePublicID: input.CandidatePublicID, ExpectedTaskVersion: input.ExpectedTaskVersion, ActorType: string(input.Principal.Type), ActorPublicID: input.Principal.PublicID, Roles: input.Principal.Roles}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *ConfirmationService) authorize(principal security.Principal, task taskmodule.Instance) error {
	if principal.Type != security.HumanPrincipal && !(principal.Type == security.MachinePrincipal && principal.MachineUse == AutomaticDispatchMachineUse) {
		return fmt.Errorf("principal is not allowed to confirm tasks")
	}
	if err := s.authorizer.Authorize(principal, "task:assign", security.AccessScope{TeamIDs: []uint64{task.TeamID}}); err != nil {
		return err
	}
	// The task is constrained by both Team and Area. Check both dimensions so
	// an actor scoped to a different team in the same area cannot confirm it.
	if err := s.authorizer.Authorize(principal, "task:assign", security.AccessScope{AreaIDs: []uint64{task.AreaID}}); err != nil {
		return err
	}
	return nil
}

func (s *ConfirmationService) candidateStillEligible(ctx context.Context, tx ConfirmationTransaction, task taskmodule.Instance, candidate taskmodule.Candidate, person personnelmodule.CandidateRecord) (bool, error) {
	if candidate.Status != taskmodule.CandidateProposed || candidate.TaskID != task.ID || person.ID != candidate.PersonnelID {
		return false, nil
	}
	if person.TeamID != task.TeamID || person.AreaID != task.AreaID || person.PositionCode != task.RequiredPositionCode || person.WorkState != personnelmodule.WorkStateIdle || !person.Enabled || !hasAllCapabilities(person.Capabilities, task.RequiredCapabilities) {
		return false, nil
	}
	conflict, err := tx.HasPersonnelScheduleConflict(ctx, person.ID, task.PlannedAt)
	if err != nil {
		return false, fmt.Errorf("check personnel schedule conflict: %w", err)
	}
	return !conflict, nil
}

func confirmationResultForTask(code string, task taskmodule.Instance, candidatePublicID string) ConfirmationResult {
	return ConfirmationResult{ResultCode: code, TaskPublicID: task.PublicID, TaskStatus: string(task.Status), TaskVersion: task.StatusVersion, CandidatePublicID: candidatePublicID, SyncVersion: task.SyncVersion}
}

func confirmationResultForCandidate(code string, task taskmodule.Instance, candidate taskmodule.Candidate, person personnelmodule.CandidateRecord) ConfirmationResult {
	result := confirmationResultForTask(code, task, candidate.PublicID)
	result.PersonnelPublicID = person.PublicID
	result.PersonnelWorkState = string(person.WorkState)
	return result
}

func (s *ConfirmationService) persistConfirmationOutcomeAndAudit(ctx context.Context, tx ConfirmationTransaction, input ConfirmationInput, requestHash string, result ConfirmationResult, status string, businessErr error, processedAt time.Time) error {
	if err := s.appendConfirmationAudit(ctx, tx, input, result.TaskPublicID, result.ResultCode, processedAt); err != nil {
		return err
	}
	return s.persistConfirmationOutcome(ctx, tx, input, requestHash, result, status, errorSummary(businessErr), processedAt)
}

func (s *ConfirmationService) persistConfirmationOutcome(ctx context.Context, tx ConfirmationTransaction, input ConfirmationInput, requestHash string, result ConfirmationResult, status, errorSummaryValue string, processedAt time.Time) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal task confirmation result: %w", err)
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return fmt.Errorf("generate task confirmation idempotency public id: %w", err)
	}
	return tx.CreateBusinessIdempotency(ctx, IdempotencyRecord{
		PublicID: publicID, OperationType: OperationTaskConfirm, IdempotencyKey: input.ConfirmationID,
		RequestHash: requestHash, Status: status, AggregateType: "task", AggregatePublicID: result.TaskPublicID,
		ActorType: string(input.Principal.Type), ActorPublicID: input.Principal.PublicID, CommandType: CommandTaskConfirm,
		ResultCode: result.ResultCode, ResultStatus: result.ResultCode, ResultPayload: payload, ErrorSummary: errorSummaryValue,
		RequestID: input.RequestID, TraceID: input.TraceID, FirstProcessedAt: processedAt, LastProcessedAt: processedAt,
	})
}

func (s *ConfirmationService) appendConfirmationAudit(ctx context.Context, tx ConfirmationTransaction, input ConfirmationInput, resourceID, result string, occurredAt time.Time) error {
	return tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: string(input.Principal.Type), ActorID: input.Principal.PublicID, Action: "task.confirm", ResourceType: "task", ResourceID: resourceID, Result: result, RequestID: input.RequestID, TraceID: input.TraceID, SourceIP: input.SourceIP, OccurredAt: occurredAt})
}

func newTaskAssignedEvent(task taskmodule.Instance, assignment taskmodule.Assignment, confirmationID, traceID string, occurredAt time.Time) (event.EventEnvelope, error) {
	payload := TaskAssignedEventPayload{TaskPublicID: task.PublicID, AssignmentPublicID: assignment.PublicID, ConfirmationID: confirmationID, EmployeePublicID: assignment.PersonnelPublicID, FlightDisplayNo: task.FlightDisplayNo, TaskName: task.Name, AreaName: task.AreaName, PlannedAt: task.PlannedAt.UTC(), BusinessStatus: string(taskmodule.StatusAssigned), Message: task.Message, SyncVersion: task.SyncVersion + 1, ReceiptStatus: string(assignment.ReceiptStatus)}
	envelope, err := event.NewEvent("task.assigned.v1", "task", task.PublicID, "core-flight-task", payload)
	if err != nil {
		return event.EventEnvelope{}, fmt.Errorf("create task assigned event: %w", err)
	}
	envelope.TraceID = traceID
	envelope.OccurredAt = occurredAt.UTC()
	return envelope, nil
}

func replayConfirmationResult(record IdempotencyRecord, requestHash string) (ConfirmationResult, error) {
	if record.RequestHash != requestHash {
		return ConfirmationResult{}, wrapBusiness(ErrConfirmationIDConflict, fmt.Errorf("request hash differs for key %q", record.IdempotencyKey))
	}
	if len(record.ResultPayload) == 0 {
		return ConfirmationResult{}, fmt.Errorf("idempotency record %s has no result payload", record.PublicID)
	}
	var result ConfirmationResult
	if err := json.Unmarshal(record.ResultPayload, &result); err != nil {
		return ConfirmationResult{}, fmt.Errorf("decode task confirmation result %s: %w", record.PublicID, err)
	}
	result.Duplicate = true
	if record.Status == idempotencyRejected {
		return result, confirmationBusinessError(result.ResultCode, errors.New(record.ErrorSummary))
	}
	return result, nil
}

func confirmationBusinessError(code string, cause error) error {
	if strings.TrimSpace(cause.Error()) == "" {
		cause = errors.New("previous task confirmation operation was rejected")
	}
	switch code {
	case ResultTaskAlreadyAssigned:
		return wrapBusiness(ErrTaskAlreadyAssigned, cause)
	case ResultCandidateNoLongerEligible:
		return wrapBusiness(ErrCandidateNoLongerEligible, cause)
	case ResultConfirmationForbidden:
		return wrapBusiness(ErrConfirmationForbidden, cause)
	case ResultTaskVersionConflict:
		return wrapBusiness(ErrTaskVersionConflict, cause)
	case ResultConfirmationInvalidState:
		return wrapBusiness(ErrConfirmationInvalidState, cause)
	case ResultConfirmationTaskNotFound:
		return wrapBusiness(ErrConfirmationTaskNotFound, cause)
	case ResultConfirmationCandidateNotFound:
		return wrapBusiness(ErrConfirmationCandidateNotFound, cause)
	default:
		return &BusinessError{Code: "operation_rejected", Message: "task confirmation operation was rejected", Cause: cause}
	}
}

func (s *ConfirmationService) now() time.Time {
	if s != nil && s.clock != nil {
		value := s.clock.Now()
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Now().UTC()
}
