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

func TestConfirmTaskAtomicallyAssignsCandidate(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	repository := newConfirmationFakeRepository(now)
	service := NewConfirmationService(repository, iam.NewAuthorizer(), fixedClock{value: now})

	result, err := service.ConfirmTask(context.Background(), confirmationInput("candidate-1", "confirmation-1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.ResultCode != ResultTaskAssigned || result.Duplicate || result.TaskStatus != string(taskmodule.StatusAssigned) || result.TaskVersion != 1 || result.SyncVersion != 1 {
		t.Fatalf("unexpected confirmation result: %#v", result)
	}
	if repository.task.Status != taskmodule.StatusAssigned || repository.task.StatusVersion != 1 || repository.task.SyncVersion != 1 {
		t.Fatalf("task was not assigned: %#v", repository.task)
	}
	if person := repository.people[11]; person.WorkState != personnelmodule.WorkStateReserved || person.StatusVersion != 1 {
		t.Fatalf("personnel was not reserved: %#v", person)
	}
	if repository.candidates[0].Status != taskmodule.CandidateSelected || repository.candidates[1].Status != taskmodule.CandidateProposed {
		t.Fatalf("candidate statuses = %#v", repository.candidates)
	}
	if len(repository.assignments) != 1 || repository.assignments[0].ConfirmationID != "confirmation-1" || len(repository.taskHistories) != 1 || len(repository.assignmentHistories) != 1 || len(repository.personnelHistories) != 1 || len(repository.audits) != 1 || len(repository.outbox) != 1 {
		t.Fatalf("transaction side effects: assignments=%d task_history=%d assignment_history=%d personnel_history=%d audits=%d outbox=%d", len(repository.assignments), len(repository.taskHistories), len(repository.assignmentHistories), len(repository.personnelHistories), len(repository.audits), len(repository.outbox))
	}
	if repository.outbox[0].EventType != "task.assigned.v1" || repository.outbox[0].CorrelationID == "" {
		t.Fatalf("unexpected assigned event: %#v", repository.outbox[0])
	}
	var payload TaskAssignedEventPayload
	if err := json.Unmarshal(repository.outbox[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TaskPublicID != "task-1" || payload.ConfirmationID != "confirmation-1" || payload.EmployeePublicID != "person-1" || payload.BusinessStatus != string(taskmodule.StatusAssigned) || payload.SyncVersion != 1 {
		t.Fatalf("unexpected assigned payload: %#v", payload)
	}
}

func TestConfirmTaskRequiresHumanScope(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	repository := newConfirmationFakeRepository(now)
	service := NewConfirmationService(repository, iam.NewAuthorizer(), fixedClock{value: now})
	input := confirmationInput("candidate-1", "confirmation-forbidden")
	input.Principal.Scopes = security.AccessScope{TeamIDs: []uint64{999}, AreaIDs: []uint64{10}}

	result, err := service.ConfirmTask(context.Background(), input)
	if CodeOf(err) != ResultConfirmationForbidden || result.ResultCode != ResultConfirmationForbidden {
		t.Fatalf("unexpected forbidden confirmation: result=%#v err=%v", result, err)
	}
	if repository.task.Status != taskmodule.StatusAwaitingConfirmation || repository.people[11].WorkState != personnelmodule.WorkStateIdle || repository.candidates[0].Status != taskmodule.CandidateProposed {
		t.Fatalf("forbidden command changed business facts: task=%#v person=%#v candidate=%#v", repository.task, repository.people[11], repository.candidates[0])
	}
	if len(repository.idempotency) != 1 || len(repository.audits) != 1 || repository.audits[0].Result != ResultConfirmationForbidden {
		t.Fatalf("forbidden command audit/idempotency: idem=%d audits=%d", len(repository.idempotency), len(repository.audits))
	}
}

func TestConfirmTaskInvalidatesCandidateWhenPersonnelChanged(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	repository := newConfirmationFakeRepository(now)
	person := repository.people[11]
	person.WorkState = personnelmodule.WorkStateBusy
	repository.people[11] = person
	service := NewConfirmationService(repository, iam.NewAuthorizer(), fixedClock{value: now})

	result, err := service.ConfirmTask(context.Background(), confirmationInput("candidate-1", "confirmation-invalid"))
	if CodeOf(err) != ResultCandidateNoLongerEligible || result.ResultCode != ResultCandidateNoLongerEligible {
		t.Fatalf("unexpected invalid candidate result: result=%#v err=%v", result, err)
	}
	if repository.candidates[0].Status != taskmodule.CandidateInvalidated || repository.candidates[0].RejectionReason != ResultCandidateNoLongerEligible || len(repository.assignments) != 0 || repository.task.Status != taskmodule.StatusAwaitingConfirmation {
		t.Fatalf("invalid candidate changed wrong facts: candidate=%#v assignments=%d task=%#v", repository.candidates[0], len(repository.assignments), repository.task)
	}
}

func TestConfirmTaskIdempotencyAndConflict(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	repository := newConfirmationFakeRepository(now)
	service := NewConfirmationService(repository, iam.NewAuthorizer(), fixedClock{value: now})
	input := confirmationInput("candidate-1", "confirmation-replay")

	first, err := service.ConfirmTask(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ConfirmTask(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.AssignmentPublicID != first.AssignmentPublicID || len(repository.assignments) != 1 || len(repository.outbox) != 1 || len(repository.audits) != 1 {
		t.Fatalf("idempotent replay created new effects: first=%#v second=%#v assignments=%d outbox=%d audits=%d", first, second, len(repository.assignments), len(repository.outbox), len(repository.audits))
	}

	conflicting := input
	conflicting.CandidatePublicID = "candidate-2"
	if _, err := service.ConfirmTask(context.Background(), conflicting); CodeOf(err) != ResultConfirmationIDConflict {
		t.Fatalf("confirmation id conflict error = %v", err)
	}
}

func TestConfirmTaskRejectsAlreadyAssignedTask(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	repository := newConfirmationFakeRepository(now)
	repository.task.Status = taskmodule.StatusAssigned
	repository.task.StatusVersion = 1
	service := NewConfirmationService(repository, iam.NewAuthorizer(), fixedClock{value: now})
	input := confirmationInput("candidate-1", "confirmation-already-assigned")

	result, err := service.ConfirmTask(context.Background(), input)
	if CodeOf(err) != ResultTaskAlreadyAssigned || result.ResultCode != ResultTaskAlreadyAssigned {
		t.Fatalf("unexpected already-assigned result: result=%#v err=%v", result, err)
	}
	if len(repository.assignments) != 0 || repository.people[11].WorkState != personnelmodule.WorkStateIdle {
		t.Fatalf("already-assigned command changed facts: assignments=%d person=%#v", len(repository.assignments), repository.people[11])
	}
}

func TestAutomaticDispatcherAssignsWithoutEmployeeApproval(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	repository := newConfirmationFakeRepository(now)
	repository.task.Status = taskmodule.StatusPendingDispatch
	confirmation := NewConfirmationService(repository, iam.NewAuthorizer(), fixedClock{value: now})
	dispatcher := NewAutomaticDispatcher(confirmation, nil)

	result, err := dispatcher.Dispatch(context.Background(), ArrivalResult{
		TaskPublicID: "task-1",
		Candidates: []CandidateView{
			{PublicID: "candidate-1", PersonnelPublicID: "person-1", Rank: 1},
			{PublicID: "candidate-2", PersonnelPublicID: "person-2", Rank: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskStatus != string(taskmodule.StatusAssigned) || result.AssignmentStatus != string(taskmodule.AssignmentConfirmed) || len(repository.assignments) != 1 || repository.assignments[0].ConfirmedByPublicID != AutomaticDispatchActorID {
		t.Fatalf("automatic dispatch did not assign: result=%#v assignments=%#v", result, repository.assignments)
	}
}

func TestAutomaticDispatcherRetriesNextCandidateAfterConflict(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	repository := newConfirmationFakeRepository(now)
	repository.task.Status = taskmodule.StatusPendingDispatch
	firstPerson := repository.people[11]
	firstPerson.WorkState = personnelmodule.WorkStateBusy
	repository.people[11] = firstPerson
	confirmation := NewConfirmationService(repository, iam.NewAuthorizer(), fixedClock{value: now})
	dispatcher := NewAutomaticDispatcher(confirmation, nil)

	result, err := dispatcher.Dispatch(context.Background(), ArrivalResult{
		TaskPublicID: "task-1",
		Candidates: []CandidateView{
			{PublicID: "candidate-1", PersonnelPublicID: "person-1", Rank: 1},
			{PublicID: "candidate-2", PersonnelPublicID: "person-2", Rank: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskStatus != string(taskmodule.StatusAssigned) || result.PersonnelPublicID != "person-2" || len(repository.assignments) != 1 {
		t.Fatalf("dispatcher did not fall back to the next candidate: result=%#v assignments=%#v", result, repository.assignments)
	}
	if repository.candidates[0].Status != taskmodule.CandidateInvalidated || repository.candidates[1].Status != taskmodule.CandidateSelected {
		t.Fatalf("candidate fallback statuses = %#v", repository.candidates)
	}
}

func confirmationInput(candidateID, confirmationID string) ConfirmationInput {
	return ConfirmationInput{
		Principal:    security.Principal{Type: security.HumanPrincipal, PublicID: "leader-1", Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{Global: true}},
		TaskPublicID: "task-1", CandidatePublicID: candidateID, ConfirmationID: confirmationID, ExpectedTaskVersion: 0,
		RequestID: "request-1", TraceID: "trace-1", SourceIP: "127.0.0.1",
	}
}

type confirmationFakeRepository struct {
	task                taskmodule.Instance
	candidates          []taskmodule.Candidate
	people              map[uint64]personnelmodule.CandidateRecord
	assignments         []taskmodule.Assignment
	taskHistories       []taskmodule.StatusHistory
	assignmentHistories []taskmodule.AssignmentStatusHistory
	personnelHistories  []personnelmodule.StatusHistory
	audits              []coresync.AuditRecord
	outbox              []sharedEvent.EventEnvelope
	idempotency         map[string]IdempotencyRecord
	scheduleConflict    bool
}

func newConfirmationFakeRepository(now time.Time) *confirmationFakeRepository {
	return &confirmationFakeRepository{
		task: taskmodule.Instance{ID: 100, PublicID: "task-1", FlightID: 1, FlightPublicID: "flight-1", FlightDisplayNo: "CA1234", AreaID: 10, AreaName: "到达区", TeamID: 20, RequiredPositionCode: "ramp", RequiredCapabilities: []string{"ramp"}, Name: "到达保障", Message: "请前往到达区", PlannedAt: now.Add(15 * time.Minute), Status: taskmodule.StatusAwaitingConfirmation, StatusVersion: 0, SyncVersion: 0},
		candidates: []taskmodule.Candidate{
			{ID: 201, PublicID: "candidate-1", TaskID: 100, PersonnelID: 11, PersonnelPublicID: "person-1", Rank: 1, Status: taskmodule.CandidateProposed, MatchedPositionCode: "ramp", MatchedCapabilities: []string{"ramp"}, PersonnelWorkStateSnapshot: string(personnelmodule.WorkStateIdle), PersonnelStateChangedAt: now.Add(-10 * time.Minute)},
			{ID: 202, PublicID: "candidate-2", TaskID: 100, PersonnelID: 12, PersonnelPublicID: "person-2", Rank: 2, Status: taskmodule.CandidateProposed, MatchedPositionCode: "ramp", MatchedCapabilities: []string{"ramp"}, PersonnelWorkStateSnapshot: string(personnelmodule.WorkStateIdle), PersonnelStateChangedAt: now.Add(-5 * time.Minute)},
		},
		people: map[uint64]personnelmodule.CandidateRecord{
			11: {ID: 11, PublicID: "person-1", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp", "radio"}, WorkState: personnelmodule.WorkStateIdle, StatusVersion: 0, LastStateChangedAt: now.Add(-10 * time.Minute), Enabled: true},
			12: {ID: 12, PublicID: "person-2", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp"}, WorkState: personnelmodule.WorkStateIdle, StatusVersion: 0, LastStateChangedAt: now.Add(-5 * time.Minute), Enabled: true},
		},
		idempotency: make(map[string]IdempotencyRecord),
	}
}

func (r *confirmationFakeRepository) FindBusinessIdempotency(_ context.Context, operationType, key string) (IdempotencyRecord, error) {
	value, ok := r.idempotency[operationType+"\x00"+key]
	if !ok {
		return IdempotencyRecord{}, ErrNotFound
	}
	return value, nil
}

func (r *confirmationFakeRepository) WithinConfirmationTransaction(_ context.Context, fn func(ConfirmationTransaction) error) error {
	snapshot := r.clone()
	err := fn(&confirmationFakeTransaction{repository: r})
	if err != nil {
		*r = *snapshot
	}
	return err
}

func (r *confirmationFakeRepository) clone() *confirmationFakeRepository {
	clone := *r
	clone.candidates = append([]taskmodule.Candidate(nil), r.candidates...)
	clone.people = make(map[uint64]personnelmodule.CandidateRecord, len(r.people))
	for key, value := range r.people {
		value.Capabilities = append([]string(nil), value.Capabilities...)
		clone.people[key] = value
	}
	clone.assignments = append([]taskmodule.Assignment(nil), r.assignments...)
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

type confirmationFakeTransaction struct{ repository *confirmationFakeRepository }

func (tx *confirmationFakeTransaction) FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error) {
	return tx.repository.FindBusinessIdempotency(ctx, operationType, key)
}

func (tx *confirmationFakeTransaction) FindTaskForUpdate(_ context.Context, publicID string) (taskmodule.Instance, error) {
	if tx.repository.task.PublicID != publicID {
		return taskmodule.Instance{}, ErrNotFound
	}
	return tx.repository.task, nil
}

func (tx *confirmationFakeTransaction) FindCandidateForUpdate(_ context.Context, taskID uint64, publicID string) (taskmodule.Candidate, error) {
	for _, value := range tx.repository.candidates {
		if value.TaskID == taskID && value.PublicID == publicID {
			return value, nil
		}
	}
	return taskmodule.Candidate{}, ErrNotFound
}

func (tx *confirmationFakeTransaction) FindPersonnelForUpdate(_ context.Context, personnelID uint64) (personnelmodule.CandidateRecord, error) {
	value, ok := tx.repository.people[personnelID]
	if !ok {
		return personnelmodule.CandidateRecord{}, ErrNotFound
	}
	return value, nil
}

func (tx *confirmationFakeTransaction) HasPersonnelScheduleConflict(context.Context, uint64, time.Time) (bool, error) {
	return tx.repository.scheduleConflict, nil
}

func (tx *confirmationFakeTransaction) ReservePersonnel(_ context.Context, personnelID, expectedStatusVersion uint64, changedAt time.Time) error {
	value, ok := tx.repository.people[personnelID]
	if !ok || value.WorkState != personnelmodule.WorkStateIdle || value.StatusVersion != expectedStatusVersion {
		return ErrPersonnelReservationConflict
	}
	value.WorkState = personnelmodule.WorkStateReserved
	value.StatusVersion++
	value.LastStateChangedAt = changedAt
	tx.repository.people[personnelID] = value
	return nil
}

func (tx *confirmationFakeTransaction) InvalidateCandidate(_ context.Context, candidateID uint64, reason string, changedAt time.Time) error {
	for index := range tx.repository.candidates {
		if tx.repository.candidates[index].ID == candidateID {
			tx.repository.candidates[index].Status = taskmodule.CandidateInvalidated
			tx.repository.candidates[index].RejectionReason = reason
			tx.repository.candidates[index].InvalidatedAt = &changedAt
			return nil
		}
	}
	return ErrNotFound
}

func (tx *confirmationFakeTransaction) MarkCandidatesConfirmed(_ context.Context, taskID, selectedCandidateID uint64, changedAt time.Time) error {
	selected := false
	for index := range tx.repository.candidates {
		candidate := &tx.repository.candidates[index]
		if candidate.TaskID != taskID {
			continue
		}
		if candidate.ID == selectedCandidateID {
			if candidate.Status != taskmodule.CandidateProposed {
				return ErrCandidateNoLongerEligible
			}
			candidate.Status = taskmodule.CandidateSelected
			candidate.SelectedAt = &changedAt
			selected = true
		}
	}
	if !selected {
		return ErrCandidateNoLongerEligible
	}
	return nil
}

func (tx *confirmationFakeTransaction) UpdateTaskAssigned(_ context.Context, taskID, expectedStatusVersion uint64, changedAt time.Time) error {
	if tx.repository.task.ID != taskID || (tx.repository.task.Status != taskmodule.StatusPendingDispatch && tx.repository.task.Status != taskmodule.StatusAwaitingConfirmation) || tx.repository.task.StatusVersion != expectedStatusVersion {
		return ErrTaskVersionConflict
	}
	tx.repository.task.Status = taskmodule.StatusAssigned
	tx.repository.task.StatusVersion++
	tx.repository.task.SyncVersion++
	return nil
}

func (tx *confirmationFakeTransaction) CreateAssignment(_ context.Context, value *taskmodule.Assignment) error {
	value.ID = 500 + uint64(len(tx.repository.assignments))
	tx.repository.assignments = append(tx.repository.assignments, *value)
	return nil
}

func (tx *confirmationFakeTransaction) CreateTaskStatusHistory(_ context.Context, value taskmodule.StatusHistory) error {
	tx.repository.taskHistories = append(tx.repository.taskHistories, value)
	return nil
}

func (tx *confirmationFakeTransaction) CreateAssignmentStatusHistory(_ context.Context, value taskmodule.AssignmentStatusHistory) error {
	tx.repository.assignmentHistories = append(tx.repository.assignmentHistories, value)
	return nil
}

func (tx *confirmationFakeTransaction) CreatePersonnelStatusHistory(_ context.Context, value personnelmodule.StatusHistory) error {
	tx.repository.personnelHistories = append(tx.repository.personnelHistories, value)
	return nil
}

func (tx *confirmationFakeTransaction) CreateBusinessIdempotency(_ context.Context, value IdempotencyRecord) error {
	key := value.OperationType + "\x00" + value.IdempotencyKey
	if _, exists := tx.repository.idempotency[key]; exists {
		return ErrDuplicate
	}
	tx.repository.idempotency[key] = value
	return nil
}

func (tx *confirmationFakeTransaction) AppendAudit(_ context.Context, value coresync.AuditRecord) error {
	tx.repository.audits = append(tx.repository.audits, value)
	return nil
}

func (tx *confirmationFakeTransaction) AppendOutbox(_ context.Context, value sharedEvent.EventEnvelope) error {
	tx.repository.outbox = append(tx.repository.outbox, value)
	return nil
}
