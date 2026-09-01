package flighttask

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

func TestCancelAwaitingTaskInvalidatesProposedCandidates(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	repository := newCancellationFakeRepository(now)
	service := NewCancellationService(repository, iam.NewAuthorizer(), fixedClock{value: now})

	result, err := service.CancelTask(context.Background(), cancellationInput(security.RoleLeader, "cancel-awaiting", 0))
	if err != nil {
		t.Fatal(err)
	}
	if result.ResultCode != ResultCancellationApplied || result.TaskStatus != string(taskmodule.StatusCancelled) || result.TaskVersion != 1 || result.SyncVersion != 1 {
		t.Fatalf("unexpected cancellation result: %#v", result)
	}
	if repository.task.Status != taskmodule.StatusCancelled || repository.task.StatusVersion != 1 || repository.task.SyncVersion != 1 {
		t.Fatalf("task was not cancelled: %#v", repository.task)
	}
	for _, candidate := range repository.candidates {
		if candidate.Status != taskmodule.CandidateInvalidated || candidate.RejectionReason != "operator requested cancellation" {
			t.Fatalf("proposed candidate was not invalidated: %#v", candidate)
		}
	}
	if repository.assignment != nil || len(repository.assignmentHistories) != 0 || len(repository.personnelHistories) != 0 {
		t.Fatalf("awaiting cancellation released an unexpected assignment/personnel: assignment=%#v assignment_history=%d personnel_history=%d", repository.assignment, len(repository.assignmentHistories), len(repository.personnelHistories))
	}
	if len(repository.taskHistories) != 1 || len(repository.audits) != 1 || len(repository.outbox) != 1 || len(repository.idempotency) != 1 {
		t.Fatalf("unexpected cancellation side effects: task_history=%d audits=%d outbox=%d idempotency=%d", len(repository.taskHistories), len(repository.audits), len(repository.outbox), len(repository.idempotency))
	}
	if repository.outbox[0].EventType != EventTaskCancelled || repository.outbox[0].CorrelationID != "cancel-awaiting" {
		t.Fatalf("unexpected cancelled event envelope: %#v", repository.outbox[0])
	}
	var payload TaskProjectionEventPayload
	if err := json.Unmarshal(repository.outbox[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AssignmentPublicID != "" || payload.EmployeePublicID != "" || payload.BusinessStatus != string(taskmodule.StatusCancelled) || payload.SyncVersion != 1 {
		t.Fatalf("unexpected awaiting cancellation payload: %#v", payload)
	}
}

func TestCancelAssignedTaskReleasesPersonnelAndReplaysIdempotently(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	repository := newCancellationFakeRepository(now)
	repository.task.Status = taskmodule.StatusAssigned
	repository.task.StatusVersion = 1
	repository.task.SyncVersion = 1
	repository.candidates[0].Status = taskmodule.CandidateSelected
	repository.assignment = &taskmodule.Assignment{ID: 500, PublicID: "assignment-1", TaskID: 100, CandidateID: 201, PersonnelID: 11, PersonnelPublicID: "person-1", Status: taskmodule.AssignmentConfirmed, ConfirmationID: "confirmation-1"}
	person := repository.people[11]
	person.WorkState = personnelmodule.WorkStateReserved
	person.StatusVersion = 1
	repository.people[11] = person
	service := NewCancellationService(repository, iam.NewAuthorizer(), fixedClock{value: now})
	input := cancellationInput(security.RoleManager, "cancel-assigned", 1)

	first, err := service.CancelTask(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ResultCode != ResultCancellationApplied || first.AssignmentStatus != string(taskmodule.AssignmentCancelled) || first.PersonnelWorkState != string(personnelmodule.WorkStateIdle) {
		t.Fatalf("unexpected assigned cancellation result: %#v", first)
	}
	if repository.assignment.Status != taskmodule.AssignmentCancelled || repository.assignment.StatusVersion != 1 {
		t.Fatalf("assignment was not cancelled: %#v", repository.assignment)
	}
	if len(repository.assignmentHistories) != 1 || repository.assignmentHistories[0].CancellationID != "cancel-assigned" {
		t.Fatalf("assignment cancellation metadata was not recorded: %#v", repository.assignmentHistories)
	}
	if person := repository.people[11]; person.WorkState != personnelmodule.WorkStateIdle || person.StatusVersion != 2 {
		t.Fatalf("personnel was not released: %#v", person)
	}
	if repository.candidates[0].Status != taskmodule.CandidateSelected || len(repository.taskHistories) != 1 || len(repository.assignmentHistories) != 1 || len(repository.personnelHistories) != 1 || len(repository.outbox) != 1 {
		t.Fatalf("assigned cancellation changed incorrect facts: candidate=%#v task_history=%d assignment_history=%d personnel_history=%d outbox=%d", repository.candidates[0], len(repository.taskHistories), len(repository.assignmentHistories), len(repository.personnelHistories), len(repository.outbox))
	}

	replay, err := service.CancelTask(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Duplicate || replay.AssignmentPublicID != first.AssignmentPublicID || len(repository.taskHistories) != 1 || len(repository.assignmentHistories) != 1 || len(repository.personnelHistories) != 1 || len(repository.outbox) != 1 {
		t.Fatalf("cancellation replay created new effects: first=%#v replay=%#v task_history=%d assignment_history=%d personnel_history=%d outbox=%d", first, replay, len(repository.taskHistories), len(repository.assignmentHistories), len(repository.personnelHistories), len(repository.outbox))
	}

	differentID := input
	differentID.CancellationID = "cancel-assigned-again"
	differentID.Reason = "late duplicate request"
	terminal, err := service.CancelTask(context.Background(), differentID)
	if err != nil || terminal.ResultCode != ResultTaskAlreadyCancelled || len(repository.outbox) != 1 || len(repository.taskHistories) != 1 {
		t.Fatalf("different cancellation id had unexpected terminal result: result=%#v err=%v outbox=%d task_history=%d", terminal, err, len(repository.outbox), len(repository.taskHistories))
	}
}

func TestCancelInProgressTaskAllowsManagerAndRejectsLeader(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	leaderRepository := newCancellationFakeRepository(now)
	prepareInProgressCancellation(leaderRepository)
	leaderService := NewCancellationService(leaderRepository, iam.NewAuthorizer(), fixedClock{value: now})

	forbidden, err := leaderService.CancelTask(context.Background(), cancellationInput(security.RoleLeader, "cancel-leader-progress", 2))
	if CodeOf(err) != ResultCancellationForbidden || forbidden.ResultCode != ResultCancellationForbidden {
		t.Fatalf("leader cancellation should be forbidden: result=%#v err=%v", forbidden, err)
	}
	if leaderRepository.task.Status != taskmodule.StatusInProgress || leaderRepository.people[11].WorkState != personnelmodule.WorkStateBusy || len(leaderRepository.outbox) != 0 {
		t.Fatalf("forbidden leader cancellation changed facts: task=%#v person=%#v outbox=%d", leaderRepository.task, leaderRepository.people[11], len(leaderRepository.outbox))
	}

	managerRepository := newCancellationFakeRepository(now)
	prepareInProgressCancellation(managerRepository)
	managerService := NewCancellationService(managerRepository, iam.NewAuthorizer(), fixedClock{value: now})
	result, err := managerService.CancelTask(context.Background(), cancellationInput(security.RoleManager, "cancel-manager-progress", 2))
	if err != nil || result.ResultCode != ResultCancellationApplied || managerRepository.task.Status != taskmodule.StatusCancelled || managerRepository.assignment.Status != taskmodule.AssignmentCancelled || managerRepository.people[11].WorkState != personnelmodule.WorkStateIdle {
		t.Fatalf("manager in-progress cancellation failed: result=%#v err=%v task=%#v assignment=%#v person=%#v", result, err, managerRepository.task, managerRepository.assignment, managerRepository.people[11])
	}
}

func TestCancelTaskRejectsDifferentPayloadForSameCancellationID(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	repository := newCancellationFakeRepository(now)
	service := NewCancellationService(repository, iam.NewAuthorizer(), fixedClock{value: now})
	input := cancellationInput(security.RoleManager, "cancel-conflict", 0)
	if _, err := service.CancelTask(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	conflicting := input
	conflicting.Reason = "different reason"
	if _, err := service.CancelTask(context.Background(), conflicting); CodeOf(err) != ResultCancellationIDConflict {
		t.Fatalf("cancellation id conflict error = %v", err)
	}
}

func cancellationInput(role, cancellationID string, expectedVersion uint64) CancellationInput {
	return CancellationInput{
		Principal:    security.Principal{Type: security.HumanPrincipal, PublicID: role + "-1", Roles: []string{role}, Scopes: security.AccessScope{Global: true}},
		TaskPublicID: "task-1", CancellationID: cancellationID, ExpectedTaskVersion: expectedVersion,
		Reason: "operator requested cancellation", RequestID: "request-1", TraceID: "trace-1", SourceIP: "127.0.0.1",
	}
}

func prepareInProgressCancellation(repository *cancellationFakeRepository) {
	repository.task.Status = taskmodule.StatusInProgress
	repository.task.StatusVersion = 2
	repository.task.SyncVersion = 2
	repository.candidates[0].Status = taskmodule.CandidateSelected
	repository.assignment = &taskmodule.Assignment{ID: 500, PublicID: "assignment-1", TaskID: 100, CandidateID: 201, PersonnelID: 11, PersonnelPublicID: "person-1", Status: taskmodule.AssignmentAccepted, StatusVersion: 1, ConfirmationID: "confirmation-1"}
	person := repository.people[11]
	person.WorkState = personnelmodule.WorkStateBusy
	person.StatusVersion = 2
	repository.people[11] = person
}

type cancellationFakeRepository struct {
	task                taskmodule.Instance
	candidates          []taskmodule.Candidate
	people              map[uint64]personnelmodule.CandidateRecord
	assignment          *taskmodule.Assignment
	taskHistories       []taskmodule.StatusHistory
	assignmentHistories []taskmodule.AssignmentStatusHistory
	personnelHistories  []personnelmodule.StatusHistory
	audits              []coresync.AuditRecord
	outbox              []sharedEvent.EventEnvelope
	idempotency         map[string]IdempotencyRecord
}

func newCancellationFakeRepository(now time.Time) *cancellationFakeRepository {
	return &cancellationFakeRepository{
		task: taskmodule.Instance{ID: 100, PublicID: "task-1", FlightPublicID: "flight-1", FlightDisplayNo: "CA1234", AreaID: 10, AreaName: "arrival", TeamID: 20, Name: "arrival support", Message: "meet aircraft", PlannedAt: now.Add(15 * time.Minute), Status: taskmodule.StatusAwaitingConfirmation},
		candidates: []taskmodule.Candidate{
			{ID: 201, PublicID: "candidate-1", TaskID: 100, PersonnelID: 11, PersonnelPublicID: "person-1", Status: taskmodule.CandidateProposed},
			{ID: 202, PublicID: "candidate-2", TaskID: 100, PersonnelID: 12, PersonnelPublicID: "person-2", Status: taskmodule.CandidateProposed},
		},
		people: map[uint64]personnelmodule.CandidateRecord{
			11: {ID: 11, PublicID: "person-1", TeamID: 20, AreaID: 10, WorkState: personnelmodule.WorkStateIdle},
			12: {ID: 12, PublicID: "person-2", TeamID: 20, AreaID: 10, WorkState: personnelmodule.WorkStateIdle},
		},
		idempotency: make(map[string]IdempotencyRecord),
	}
}

func (r *cancellationFakeRepository) FindBusinessIdempotency(_ context.Context, operationType, key string) (IdempotencyRecord, error) {
	value, ok := r.idempotency[operationType+"\x00"+key]
	if !ok {
		return IdempotencyRecord{}, ErrNotFound
	}
	return value, nil
}

func (r *cancellationFakeRepository) WithinCancellationTransaction(_ context.Context, fn func(CancellationTransaction) error) error {
	snapshot := r.clone()
	err := fn(&cancellationFakeTransaction{repository: r})
	if err != nil {
		*r = *snapshot
	}
	return err
}

func (r *cancellationFakeRepository) clone() *cancellationFakeRepository {
	clone := *r
	clone.candidates = append([]taskmodule.Candidate(nil), r.candidates...)
	clone.people = make(map[uint64]personnelmodule.CandidateRecord, len(r.people))
	for key, value := range r.people {
		clone.people[key] = value
	}
	if r.assignment != nil {
		assignment := *r.assignment
		clone.assignment = &assignment
	}
	clone.taskHistories = append([]taskmodule.StatusHistory(nil), r.taskHistories...)
	clone.assignmentHistories = append([]taskmodule.AssignmentStatusHistory(nil), r.assignmentHistories...)
	clone.personnelHistories = append([]personnelmodule.StatusHistory(nil), r.personnelHistories...)
	clone.audits = append([]coresync.AuditRecord(nil), r.audits...)
	clone.outbox = append([]sharedEvent.EventEnvelope(nil), r.outbox...)
	clone.idempotency = make(map[string]IdempotencyRecord, len(r.idempotency))
	for key, value := range r.idempotency {
		value.ResultPayload = append([]byte(nil), value.ResultPayload...)
		clone.idempotency[key] = value
	}
	return &clone
}

type cancellationFakeTransaction struct{ repository *cancellationFakeRepository }

func (tx *cancellationFakeTransaction) FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error) {
	return tx.repository.FindBusinessIdempotency(ctx, operationType, key)
}

func (tx *cancellationFakeTransaction) FindTaskForUpdate(_ context.Context, publicID string) (taskmodule.Instance, error) {
	if tx.repository.task.PublicID != publicID {
		return taskmodule.Instance{}, ErrNotFound
	}
	return tx.repository.task, nil
}

func (tx *cancellationFakeTransaction) FindActiveAssignmentForUpdate(_ context.Context, taskID uint64) (taskmodule.Assignment, error) {
	if tx.repository.assignment == nil || tx.repository.assignment.TaskID != taskID || (tx.repository.assignment.Status != taskmodule.AssignmentConfirmed && tx.repository.assignment.Status != taskmodule.AssignmentAccepted) {
		return taskmodule.Assignment{}, ErrNotFound
	}
	return *tx.repository.assignment, nil
}

func (tx *cancellationFakeTransaction) FindPersonnelForUpdate(_ context.Context, personnelID uint64) (personnelmodule.CandidateRecord, error) {
	value, ok := tx.repository.people[personnelID]
	if !ok {
		return personnelmodule.CandidateRecord{}, ErrNotFound
	}
	return value, nil
}

func (tx *cancellationFakeTransaction) InvalidateProposedCandidates(_ context.Context, taskID uint64, reason string, changedAt time.Time) error {
	for index := range tx.repository.candidates {
		candidate := &tx.repository.candidates[index]
		if candidate.TaskID == taskID && candidate.Status == taskmodule.CandidateProposed {
			candidate.Status = taskmodule.CandidateInvalidated
			candidate.RejectionReason = reason
			candidate.InvalidatedAt = &changedAt
		}
	}
	return nil
}

func (tx *cancellationFakeTransaction) UpdateTaskCancelled(_ context.Context, taskID, expectedStatusVersion uint64, fromStatus taskmodule.Status, _ string, _ time.Time) error {
	if tx.repository.task.ID != taskID || tx.repository.task.Status != fromStatus || tx.repository.task.StatusVersion != expectedStatusVersion {
		return ErrCancellationVersionConflict
	}
	tx.repository.task.Status = taskmodule.StatusCancelled
	tx.repository.task.StatusVersion++
	tx.repository.task.SyncVersion++
	return nil
}

func (tx *cancellationFakeTransaction) UpdateAssignmentCancelled(_ context.Context, assignmentID, expectedStatusVersion uint64, fromStatus taskmodule.AssignmentStatus, cancellationID, _ string, _ time.Time) error {
	if tx.repository.assignment == nil || tx.repository.assignment.ID != assignmentID || tx.repository.assignment.Status != fromStatus || tx.repository.assignment.StatusVersion != expectedStatusVersion {
		return ErrCancellationInvalidState
	}
	tx.repository.assignment.Status = taskmodule.AssignmentCancelled
	tx.repository.assignment.StatusVersion++
	return nil
}

func (tx *cancellationFakeTransaction) ReleasePersonnelIdle(_ context.Context, personnelID, expectedStatusVersion uint64, fromState personnelmodule.WorkState, changedAt time.Time) error {
	value, ok := tx.repository.people[personnelID]
	if !ok || value.WorkState != fromState || value.StatusVersion != expectedStatusVersion {
		return ErrCancellationInvalidState
	}
	value.WorkState = personnelmodule.WorkStateIdle
	value.StatusVersion++
	value.LastStateChangedAt = changedAt
	tx.repository.people[personnelID] = value
	return nil
}

func (tx *cancellationFakeTransaction) CreateTaskStatusHistory(_ context.Context, value taskmodule.StatusHistory) error {
	tx.repository.taskHistories = append(tx.repository.taskHistories, value)
	return nil
}

func (tx *cancellationFakeTransaction) CreateAssignmentStatusHistory(_ context.Context, value taskmodule.AssignmentStatusHistory) error {
	tx.repository.assignmentHistories = append(tx.repository.assignmentHistories, value)
	return nil
}

func (tx *cancellationFakeTransaction) CreatePersonnelStatusHistory(_ context.Context, value personnelmodule.StatusHistory) error {
	tx.repository.personnelHistories = append(tx.repository.personnelHistories, value)
	return nil
}

func (tx *cancellationFakeTransaction) CreateBusinessIdempotency(_ context.Context, value IdempotencyRecord) error {
	key := value.OperationType + "\x00" + value.IdempotencyKey
	if _, exists := tx.repository.idempotency[key]; exists {
		return ErrDuplicate
	}
	tx.repository.idempotency[key] = value
	return nil
}

func (tx *cancellationFakeTransaction) AppendAudit(_ context.Context, value coresync.AuditRecord) error {
	tx.repository.audits = append(tx.repository.audits, value)
	return nil
}

func (tx *cancellationFakeTransaction) AppendOutbox(_ context.Context, value sharedEvent.EventEnvelope) error {
	tx.repository.outbox = append(tx.repository.outbox, value)
	return nil
}
