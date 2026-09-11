package flightsync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	flighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	flightintegration "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/flight"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fakeSourceRepository struct {
	stageInput     IngestInput
	stageResult    StageResult
	claimed        []PendingRecord
	stageCalls     int
	scheduleCalls  int
	eventCalls     int
	lastEvent      flightintegration.Event
	flightPublicID string
	applyError     error
	appliedID      uint64
	appliedOwner   string
	retriedID      uint64
	retriedOwner   string
	retryAt        time.Time
	health         SourceHealth
	attempts       int
	successes      int
	failures       int
}

func (r *fakeSourceRepository) Stage(_ context.Context, input IngestInput) (StageResult, error) {
	r.stageCalls++
	r.stageInput = input
	return r.stageResult, nil
}

func (r *fakeSourceRepository) ClaimPending(context.Context, int, time.Time, string, time.Duration) ([]PendingRecord, error) {
	return r.claimed, nil
}

func (r *fakeSourceRepository) MarkApplied(_ context.Context, id uint64, owner string, _ time.Time) error {
	r.appliedID, r.appliedOwner = id, owner
	return nil
}

func (r *fakeSourceRepository) MarkRetry(_ context.Context, id uint64, owner string, next time.Time, _ string) error {
	r.retriedID, r.retriedOwner, r.retryAt = id, owner, next
	return nil
}

func (r *fakeSourceRepository) MarkFailed(context.Context, uint64, string, string) error { return nil }

func (r *fakeSourceRepository) ApplySchedule(context.Context, PendingRecord, flightintegration.Schedule, time.Time) error {
	r.scheduleCalls++
	return r.applyError
}

func (r *fakeSourceRepository) FindFlightPublicID(context.Context, string, string) (string, error) {
	if r.flightPublicID == "" {
		return "", errors.New("flight not found")
	}
	return r.flightPublicID, nil
}

func (r *fakeSourceRepository) ApplyEvent(_ context.Context, _ PendingRecord, value flightintegration.Event, _ time.Time) error {
	r.eventCalls++
	r.lastEvent = value
	return r.applyError
}

func (r *fakeSourceRepository) RecordSourceAttempt(_ context.Context, provider string, at time.Time) error {
	r.attempts++
	r.health.Provider = provider
	r.health.LastAttemptAt = &at
	r.health.FallbackEnabled = true
	return nil
}

func (r *fakeSourceRepository) RecordSourceSuccess(_ context.Context, provider string, at time.Time) error {
	r.successes++
	r.health.Provider, r.health.State = provider, SourceFresh
	r.health.LastSuccessAt, r.health.LastAttemptAt = &at, &at
	r.health.LastError = ""
	r.health.FallbackEnabled = true
	return nil
}

func (r *fakeSourceRepository) RecordSourceFailure(_ context.Context, provider string, at time.Time, reason string) error {
	r.failures++
	r.health.Provider, r.health.State = provider, SourceFallback
	r.health.LastFailureAt, r.health.LastAttemptAt = &at, &at
	r.health.LastError = reason
	r.health.FallbackEnabled = true
	return nil
}

func (r *fakeSourceRepository) GetSourceHealth(context.Context, string) (SourceHealth, error) {
	return r.health, nil
}

type fakeArrivalRecorder struct {
	input flighttask.ArrivalInput
	calls int
}

func (r *fakeArrivalRecorder) RecordFlightArrived(_ context.Context, input flighttask.ArrivalInput) (flighttask.ArrivalResult, error) {
	r.calls++
	r.input = input
	return flighttask.ArrivalResult{}, nil
}

func TestServiceIngestNormalizesProviderBeforeStaging(t *testing.T) {
	now := time.Date(2026, 9, 4, 5, 0, 0, 0, time.UTC)
	repository := &fakeSourceRepository{stageResult: StageResult{Accepted: 2}}
	service := NewService(repository, nil, fixedClock{now: now})
	operatingDate := now.AddDate(0, 0, 1)
	result, err := service.Ingest(context.Background(), IngestInput{
		Provider: " AODB ",
		Schedules: []flightintegration.Schedule{{
			ExternalFlightID: "ext-1",
			FlightDisplayNo:  "CA123",
			OperatingDate:    operatingDate,
			ScheduledAt:      operatingDate.Add(8 * time.Hour),
		}},
		Events: []flightintegration.Event{{
			ExternalEventID:  "evt-1",
			ExternalFlightID: "ext-1",
			Status:           "departed",
			OccurredAt:       operatingDate.Add(9 * time.Hour),
		}},
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if result.Accepted != 2 || repository.stageCalls != 1 {
		t.Fatalf("unexpected stage result: %+v, calls=%d", result, repository.stageCalls)
	}
	if repository.stageInput.Provider != "aodb" || repository.stageInput.Now != now {
		t.Fatalf("provider/now were not normalized: %+v", repository.stageInput)
	}
	if repository.stageInput.Schedules[0].Provider != "aodb" || repository.stageInput.Events[0].Provider != "aodb" {
		t.Fatal("normalized provider was not copied to every record")
	}
}

func TestServiceIngestRejectsManualProviderAndUnsupportedEvent(t *testing.T) {
	service := NewService(&fakeSourceRepository{}, nil, fixedClock{now: time.Now()})
	if _, err := service.Ingest(context.Background(), IngestInput{Provider: "manual", Schedules: []flightintegration.Schedule{{ExternalFlightID: "x", FlightDisplayNo: "CA1", OperatingDate: time.Now(), ScheduledAt: time.Now()}}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("manual provider error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.Ingest(context.Background(), IngestInput{Provider: "aodb", Events: []flightintegration.Event{{ExternalEventID: "evt", ExternalFlightID: "flight", Status: "unknown", OccurredAt: time.Now()}}}); !errors.Is(err, ErrUnsupportedEvent) {
		t.Fatalf("unsupported event error = %v, want ErrUnsupportedEvent", err)
	}
}

func TestServiceApplyPendingRoutesArrivalAndMarksApplied(t *testing.T) {
	now := time.Date(2026, 9, 4, 5, 0, 0, 0, time.UTC)
	occurredAt := now.Add(time.Hour)
	payload, err := json.Marshal(flightintegration.Event{Provider: "aodb", ExternalEventID: "evt-1", ExternalFlightID: "ext-1", Status: "arrived", OccurredAt: occurredAt})
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakeSourceRepository{claimed: []PendingRecord{{ID: 7, RecordType: RecordTypeEvent, Attempts: 0, Payload: payload}}, flightPublicID: "flight-1"}
	arrival := &fakeArrivalRecorder{}
	service := NewService(repository, arrival, fixedClock{now: now})
	result, err := service.ApplyPending(context.Background(), 10, "worker-1")
	if err != nil {
		t.Fatalf("ApplyPending() error = %v", err)
	}
	if result != (ApplyResult{Claimed: 1, Applied: 1}) || arrival.calls != 1 {
		t.Fatalf("unexpected apply result: %+v, arrival calls=%d", result, arrival.calls)
	}
	if repository.appliedID != 7 || repository.appliedOwner != "worker-1" {
		t.Fatalf("record was not marked applied: id=%d owner=%s", repository.appliedID, repository.appliedOwner)
	}
	if arrival.input.FlightPublicID != "flight-1" || arrival.input.SourceEventID != "aodb:evt-1" || arrival.input.ActualArrivalAt != occurredAt {
		t.Fatalf("arrival input was not mapped from source event: %+v", arrival.input)
	}
}

func TestServiceApplyPendingRetriesFailedNonArrival(t *testing.T) {
	now := time.Date(2026, 9, 4, 5, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(flightintegration.Event{Provider: "aodb", ExternalEventID: "evt-2", ExternalFlightID: "ext-1", Status: "departed", OccurredAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakeSourceRepository{claimed: []PendingRecord{{ID: 8, RecordType: RecordTypeEvent, Attempts: 1, Payload: payload}}, applyError: errors.New("flight is not ready")}
	service := NewService(repository, nil, fixedClock{now: now})
	result, err := service.ApplyPending(context.Background(), 10, "worker-1")
	if err != nil {
		t.Fatalf("ApplyPending() error = %v", err)
	}
	if result != (ApplyResult{Claimed: 1, Retried: 1}) {
		t.Fatalf("unexpected retry result: %+v", result)
	}
	if repository.retriedID != 8 || repository.retriedOwner != "worker-1" || !repository.retryAt.Equal(now.Add(2*time.Second)) {
		t.Fatalf("record was not scheduled with exponential backoff: id=%d owner=%s next=%s", repository.retriedID, repository.retriedOwner, repository.retryAt)
	}
}

func TestServiceSourceHealthMarksFreshDataStaleAfterWindow(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-11 * time.Minute)
	repository := &fakeSourceRepository{health: SourceHealth{Provider: "aodb", State: SourceFresh, LastSuccessAt: &lastSuccess, FallbackEnabled: true}}
	service := NewService(repository, nil, fixedClock{now: now})
	value, err := service.SourceHealth(context.Background(), "aodb")
	if err != nil {
		t.Fatal(err)
	}
	if value.State != SourceStale || !value.FallbackEnabled {
		t.Fatalf("stale source health = %#v", value)
	}
}

func TestServiceAcceptsDelayEventWithNewScheduledTime(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	newScheduledAt := now.Add(4 * time.Hour)
	repository := &fakeSourceRepository{stageResult: StageResult{Accepted: 1}}
	service := NewService(repository, nil, fixedClock{now: now})
	_, err := service.Ingest(context.Background(), IngestInput{Provider: "aodb", Events: []flightintegration.Event{{
		ExternalEventID: "delay-1", ExternalFlightID: "flight-1", Status: "delayed", OccurredAt: now, ScheduledAt: &newScheduledAt,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if repository.stageInput.Events[0].ScheduledAt == nil || !repository.stageInput.Events[0].ScheduledAt.Equal(newScheduledAt) {
		t.Fatalf("delay scheduled time was not staged: %#v", repository.stageInput.Events[0])
	}
}
