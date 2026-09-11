package flighttask

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	flightmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/flight"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

func TestRecordFlightArrivedGeneratesTaskAndCandidatesAtomically(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	service := NewService(repository, fixedClock{value: now})

	result, err := service.RecordFlightArrived(context.Background(), arrivalInput(now, "source-1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.ResultCode != ResultCreated || result.Duplicate || result.CandidateCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if repository.flight.Status != flightmodule.StatusArrived || repository.flight.StatusVersion != 1 {
		t.Fatalf("flight was not transitioned: %#v", repository.flight)
	}
	if len(repository.tasks) != 1 || repository.tasks[0].Status != taskmodule.StatusPendingDispatch {
		t.Fatalf("unexpected tasks: %#v", repository.tasks)
	}
	if len(repository.candidates) != 1 || repository.candidates[0].PersonnelPublicID != "person-1" || repository.candidates[0].Rank != 1 {
		t.Fatalf("unexpected candidates: %#v", repository.candidates)
	}
	if len(repository.flightHistories) != 1 || len(repository.taskHistories) != 1 || len(repository.audits) != 1 || len(repository.outbox) != 1 {
		t.Fatalf("transaction side effects: flights=%d tasks=%d audits=%d outbox=%d", len(repository.flightHistories), len(repository.taskHistories), len(repository.audits), len(repository.outbox))
	}
	if repository.outbox[0].EventType != "task.generated.v1" {
		t.Fatalf("outbox event type = %q", repository.outbox[0].EventType)
	}

	duplicate, err := service.RecordFlightArrived(context.Background(), arrivalInput(now, "source-1"))
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.ResultCode != ResultCreated || len(repository.tasks) != 1 || len(repository.candidates) != 1 || len(repository.audits) != 1 || len(repository.outbox) != 1 {
		t.Fatalf("duplicate created side effects: result=%#v tasks=%d candidates=%d audits=%d outbox=%d", duplicate, len(repository.tasks), len(repository.candidates), len(repository.audits), len(repository.outbox))
	}
}

func TestRecordFlightArrivedRejectsSourceEventContentConflict(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	service := NewService(repository, fixedClock{value: now})

	if _, err := service.RecordFlightArrived(context.Background(), arrivalInput(now, "source-1")); err != nil {
		t.Fatal(err)
	}
	conflicting := arrivalInput(now.Add(time.Minute), "source-1")
	if _, err := service.RecordFlightArrived(context.Background(), conflicting); CodeOf(err) != ErrSourceEventConflict.Code {
		t.Fatalf("conflicting source event error = %v", err)
	}
	if len(repository.tasks) != 1 || len(repository.outbox) != 1 || repository.flight.StatusVersion != 1 {
		t.Fatalf("conflict changed business facts: tasks=%d outbox=%d flight_version=%d", len(repository.tasks), len(repository.outbox), repository.flight.StatusVersion)
	}
}

func TestRecordFlightArrivedReturnsExistingGenerationForDifferentSourceEvent(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	service := NewService(repository, fixedClock{value: now})

	first, err := service.RecordFlightArrived(context.Background(), arrivalInput(now, "source-1"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RecordFlightArrived(context.Background(), arrivalInput(now, "source-2"))
	if err != nil {
		t.Fatal(err)
	}
	if second.ResultCode != ResultAlreadyProcessed || second.Duplicate || second.TaskPublicID != first.TaskPublicID {
		t.Fatalf("unexpected generation replay: first=%#v second=%#v", first, second)
	}
	if len(repository.tasks) != 1 || len(repository.outbox) != 1 || len(repository.flightHistories) != 1 {
		t.Fatalf("generation replay created duplicate facts: tasks=%d outbox=%d histories=%d", len(repository.tasks), len(repository.outbox), len(repository.flightHistories))
	}
}

func TestRecordFlightArrivedWithoutTemplateKeepsFlightFact(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	repository.template = nil
	service := NewService(repository, fixedClock{value: now})

	result, err := service.RecordFlightArrived(context.Background(), arrivalInput(now, "source-no-template"))
	if err != nil {
		t.Fatal(err)
	}
	if result.ResultCode != ResultNoActiveTemplate || result.TaskPublicID != "" || repository.flight.Status != flightmodule.StatusArrived {
		t.Fatalf("unexpected no-template result: %#v flight=%#v", result, repository.flight)
	}
	if len(repository.tasks) != 0 || len(repository.candidates) != 0 || len(repository.outbox) != 0 || len(repository.audits) != 1 || len(repository.idempotency) != 1 {
		t.Fatalf("no-template side effects: tasks=%d candidates=%d outbox=%d audits=%d idempotency=%d", len(repository.tasks), len(repository.candidates), len(repository.outbox), len(repository.audits), len(repository.idempotency))
	}
}

func TestRecordFlightArrivedRejectsInvalidFlightTransition(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	repository.flight.Status = flightmodule.StatusDeparted
	service := NewService(repository, fixedClock{value: now})

	result, err := service.RecordFlightArrived(context.Background(), arrivalInput(now, "source-departed"))
	if CodeOf(err) != ErrFlightStatusConflict.Code || result.ResultCode != ResultFlightStatusConflict {
		t.Fatalf("unexpected transition result=%#v err=%v", result, err)
	}
	if repository.flight.Status != flightmodule.StatusDeparted || len(repository.tasks) != 0 || len(repository.outbox) != 0 {
		t.Fatalf("invalid transition changed facts: flight=%#v tasks=%d outbox=%d", repository.flight, len(repository.tasks), len(repository.outbox))
	}
	if record := repository.idempotency[OperationFlightArrived+"\x00source-departed"]; record.Status != idempotencyRejected || record.ResultCode != ResultFlightStatusConflict {
		t.Fatalf("rejected result was not persisted: %#v", record)
	}
	result, err = service.RecordFlightArrived(context.Background(), arrivalInput(now, "source-departed"))
	if CodeOf(err) != ErrFlightStatusConflict.Code || !result.Duplicate {
		t.Fatalf("replayed rejected transition result=%#v err=%v", result, err)
	}
}

func TestRecordArrivalHandlerReturnsStableEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	service := NewService(repository, fixedClock{value: now})
	router := gin.New()
	RegisterRoutes(router, service)

	body := `{"source_event_id":"http-source","occurred_at":"2026-08-31T08:00:00Z","actual_arrival_at":"2026-08-31T08:00:00Z","source":"flight-system"}`
	request := httptest.NewRequest(http.MethodPost, "/internal/integration/v1/flights/flight-1/arrival", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ArrivalResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.ResultCode != ResultCreated || response.Data.TaskPublicID == "" {
		t.Fatalf("unexpected handler response: %#v", response)
	}
}

func TestRecordFlightArrivedRejectsManualOrMalformedProviderSource(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	service := NewService(newFakeRepository(now), fixedClock{value: now})
	for _, source := range []string{"manual", "Flight-System", "provider source"} {
		t.Run(source, func(t *testing.T) {
			input := arrivalInput(now, "source-"+source)
			input.Source = source
			if _, err := service.RecordFlightArrived(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("source %q returned %v, want invalid input", source, err)
			}
		})
	}
}

func TestFilterAndSortCandidatesAppliesFrozenRules(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	template := taskmodule.Template{AreaID: 10, TeamID: 20, RequiredPositionCode: "ramp", RequiredCapabilities: []string{"ramp"}}
	values := []personnelmodule.CandidateRecord{
		{PublicID: "person-later", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-5 * time.Minute), Enabled: true},
		{PublicID: "person-earlier", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp", "radio"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-10 * time.Minute), Enabled: true},
		{PublicID: "person-tie-b", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-10 * time.Minute), Enabled: true},
		{PublicID: "person-busy", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp"}, WorkState: personnelmodule.WorkStateBusy, LastStateChangedAt: now.Add(-20 * time.Minute), Enabled: true},
		{PublicID: "person-wrong-team", TeamID: 21, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-30 * time.Minute), Enabled: true},
		{PublicID: "person-no-capability", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"radio"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-40 * time.Minute), Enabled: true},
		{PublicID: "person-disabled", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-50 * time.Minute), Enabled: false},
	}

	eligible := filterAndSortCandidates(values, template)
	if len(eligible) != 3 {
		t.Fatalf("eligible candidate count = %d, want 3: %#v", len(eligible), eligible)
	}
	got := []string{eligible[0].PublicID, eligible[1].PublicID, eligible[2].PublicID}
	want := []string{"person-earlier", "person-tie-b", "person-later"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("candidate order = %v, want %v", got, want)
	}
}

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

var _ clock.Clock = fixedClock{}

type fakeRepository struct {
	flight          flightmodule.Record
	template        *taskmodule.Template
	people          []personnelmodule.CandidateRecord
	tasks           []taskmodule.Instance
	candidates      []taskmodule.Candidate
	flightHistories []flightmodule.StatusHistory
	taskHistories   []taskmodule.StatusHistory
	audits          []coresync.AuditRecord
	outbox          []sharedEvent.EventEnvelope
	idempotency     map[string]IdempotencyRecord
	now             time.Time
}

func newFakeRepository(now time.Time) *fakeRepository {
	return &fakeRepository{
		flight:   flightmodule.Record{ID: 1, PublicID: "flight-1", DisplayNo: "CA1234", Status: flightmodule.StatusScheduled, StatusVersion: 0, ScheduledAt: now},
		template: &taskmodule.Template{ID: 2, PublicID: "template-1", Name: "到达保障", TriggerType: taskmodule.TriggerFlightArrived, Version: 1, Enabled: true, AreaID: 10, TeamID: 20, RequiredPositionCode: "ramp", RequiredCapabilities: []string{"ramp"}, PlannedOffsetSeconds: 900, DefaultMessage: "请前往到达区"},
		people: []personnelmodule.CandidateRecord{
			{ID: 11, PublicID: "person-1", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp", "radio"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-10 * time.Minute), Enabled: true},
			{ID: 12, PublicID: "person-2", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"radio"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-20 * time.Minute), Enabled: true},
			{ID: 13, PublicID: "person-3", TeamID: 20, AreaID: 10, PositionCode: "ramp", Capabilities: []string{"ramp"}, WorkState: personnelmodule.WorkStateBusy, LastStateChangedAt: now.Add(-30 * time.Minute), Enabled: true},
			{ID: 14, PublicID: "person-4", TeamID: 21, AreaID: 11, PositionCode: "ramp", Capabilities: []string{"ramp"}, WorkState: personnelmodule.WorkStateIdle, LastStateChangedAt: now.Add(-40 * time.Minute), Enabled: true},
		},
		idempotency: make(map[string]IdempotencyRecord),
		now:         now,
	}
}

func arrivalInput(at time.Time, sourceEventID string) ArrivalInput {
	return ArrivalInput{FlightPublicID: "flight-1", SourceEventID: sourceEventID, OccurredAt: at, ActualArrivalAt: at, Source: "flight-system", ActorType: "machine", ActorPublicID: "actor-1", RequestID: "request-1", TraceID: "trace-1", SourceIP: "127.0.0.1"}
}

func (r *fakeRepository) FindBusinessIdempotency(_ context.Context, operationType, key string) (IdempotencyRecord, error) {
	value, ok := r.idempotency[operationType+"\x00"+key]
	if !ok {
		return IdempotencyRecord{}, ErrNotFound
	}
	return value, nil
}

func (r *fakeRepository) WithinTransaction(_ context.Context, fn func(Transaction) error) error {
	return fn(&fakeTransaction{repository: r})
}

type fakeTransaction struct{ repository *fakeRepository }

func (tx *fakeTransaction) FindFlightForUpdate(_ context.Context, publicID string) (flightmodule.Record, error) {
	if tx.repository.flight.PublicID != publicID {
		return flightmodule.Record{}, ErrNotFound
	}
	return tx.repository.flight, nil
}

func (tx *fakeTransaction) FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error) {
	return tx.repository.FindBusinessIdempotency(ctx, operationType, key)
}

func (tx *fakeTransaction) FindTaskByGenerationKey(_ context.Context, key string) (taskmodule.Instance, error) {
	for _, value := range tx.repository.tasks {
		if value.GenerationKey == key {
			return value, nil
		}
	}
	return taskmodule.Instance{}, ErrNotFound
}

func (tx *fakeTransaction) FindActiveTemplate(context.Context, taskmodule.TriggerType) (taskmodule.Template, error) {
	if tx.repository.template == nil {
		return taskmodule.Template{}, ErrNotFound
	}
	return *tx.repository.template, nil
}

func (tx *fakeTransaction) ListCandidatePersonnel(context.Context, CandidateFilter) ([]personnelmodule.CandidateRecord, error) {
	return append([]personnelmodule.CandidateRecord(nil), tx.repository.people...), nil
}

func (tx *fakeTransaction) UpdateFlightArrived(_ context.Context, _, expectedVersion uint64, actualArrivalAt, changedAt time.Time) error {
	if tx.repository.flight.StatusVersion != expectedVersion || tx.repository.flight.Status != flightmodule.StatusScheduled {
		return errors.New("optimistic lock failed")
	}
	tx.repository.flight.Status = flightmodule.StatusArrived
	tx.repository.flight.StatusVersion++
	tx.repository.flight.ActualArrivalAt = &actualArrivalAt
	tx.repository.flight.LastStatusChangedAt = changedAt
	return nil
}

func (tx *fakeTransaction) CreateFlightStatusHistory(_ context.Context, value flightmodule.StatusHistory) error {
	tx.repository.flightHistories = append(tx.repository.flightHistories, value)
	return nil
}

func (tx *fakeTransaction) CreateTask(_ context.Context, value *taskmodule.Instance) error {
	value.ID = uint64(len(tx.repository.tasks) + 100)
	tx.repository.tasks = append(tx.repository.tasks, *value)
	return nil
}

func (tx *fakeTransaction) CreateTaskStatusHistory(_ context.Context, value taskmodule.StatusHistory) error {
	tx.repository.taskHistories = append(tx.repository.taskHistories, value)
	return nil
}

func (tx *fakeTransaction) CreateCandidate(_ context.Context, value *taskmodule.Candidate) error {
	value.ID = uint64(len(tx.repository.candidates) + 200)
	tx.repository.candidates = append(tx.repository.candidates, *value)
	return nil
}

func (tx *fakeTransaction) CreateBusinessIdempotency(_ context.Context, value IdempotencyRecord) error {
	key := value.OperationType + "\x00" + value.IdempotencyKey
	if _, exists := tx.repository.idempotency[key]; exists {
		return ErrDuplicate
	}
	tx.repository.idempotency[key] = value
	return nil
}

func (tx *fakeTransaction) AppendAudit(_ context.Context, value coresync.AuditRecord) error {
	tx.repository.audits = append(tx.repository.audits, value)
	return nil
}

func (tx *fakeTransaction) AppendOutbox(_ context.Context, value sharedEvent.EventEnvelope) error {
	tx.repository.outbox = append(tx.repository.outbox, value)
	return nil
}
