package event

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/model"
)

func TestBuildIdempotencyKey(t *testing.T) {
	occurrence := time.Date(2026, 8, 7, 20, 30, 0, 0, time.FixedZone("UTC-8", -8*60*60))

	if got, want := BuildIdempotencyKey(101, " source-42 ", occurrence), "flight_arrived:source:source-42"; got != want {
		t.Fatalf("source key = %q, want %q", got, want)
	}
	if got, want := BuildIdempotencyKey(101, "", occurrence), "flight_arrived:flight:101:date:2026-08-08"; got != want {
		t.Fatalf("fallback key = %q, want %q", got, want)
	}
}

func TestNormalizeInput(t *testing.T) {
	occurrence := time.Date(2026, 8, 7, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	normalized, err := normalizeInput(RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301, SourceEventID: " source-1 ", OccurrenceTime: occurrence,
	})
	if err != nil {
		t.Fatalf("normalizeInput() error = %v", err)
	}
	if normalized.SourceEventID != "source-1" || !normalized.OccurrenceTime.Equal(occurrence.UTC()) {
		t.Fatalf("normalized input = %+v", normalized)
	}

	for _, input := range []RecordInput{
		{TeamID: 201, TemplateID: 301, OccurrenceTime: occurrence},
		{FlightID: 101, TeamID: 201, TemplateID: 301},
		{FlightID: 101, TeamID: 201, TemplateID: 301, SourceEventID: strings.Repeat("x", 129), OccurrenceTime: occurrence},
	} {
		if _, err := normalizeInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("normalizeInput(%+v) error = %v, want ErrInvalidInput", input, err)
		}
	}
}

func TestNewFlightArrivedEventAndPendingTask(t *testing.T) {
	input := RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301, SourceEventID: "source-2",
		OccurrenceTime: time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	}
	key := BuildIdempotencyKey(input.FlightID, input.SourceEventID, input.OccurrenceTime)
	arrivalEvent := newFlightArrivedEvent(input, key)
	if arrivalEvent.Type != model.EventFlightArrived || arrivalEvent.Source != model.EventSourceManual || arrivalEvent.Status != model.EventStatusPending || arrivalEvent.IdempotencyKey != key {
		t.Fatalf("unexpected B3 event: %+v", arrivalEvent)
	}
	if arrivalEvent.FlightID == nil || *arrivalEvent.FlightID != input.FlightID || arrivalEvent.AffectedPositions != "[]" || arrivalEvent.HandleLogs != "[]" {
		t.Fatalf("unexpected event defaults: %+v", arrivalEvent)
	}

	task := newPendingTask(input, model.TaskTemplate{Version: 9, TimeoutSeconds: 0}, 401)
	if task.TriggerEventID != 401 || task.TemplateVersion != 9 || task.Status != model.TaskStatusPending || task.AssignedCount != 0 {
		t.Fatalf("unexpected B3 task: %+v", task)
	}
	if !task.PlannedStart.Equal(input.OccurrenceTime) || !task.PlannedEnd.Equal(input.OccurrenceTime.Add(time.Duration(DefaultTaskTimeoutSeconds)*time.Second)) {
		t.Fatalf("unexpected default task plan: start=%s end=%s", task.PlannedStart, task.PlannedEnd)
	}
}

func TestRecordFlightArrivedRejectsUnavailableRepository(t *testing.T) {
	_, err := NewService(nil).Record(context.Background(), RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301, OccurrenceTime: time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("error = %v, want ErrRepositoryUnavailable", err)
	}
}

func TestRecordFlightArrivedCreatesPendingEventAndTask(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository)
	occurrence := time.Date(2026, 8, 7, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	input := RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301,
		SourceEventID: " source-arrival-1 ", OccurrenceTime: occurrence,
	}

	result, err := service.RecordFlightArrived(context.Background(), input)
	if err != nil {
		t.Fatalf("RecordFlightArrived() error = %v", err)
	}
	if result.Duplicate || result.EventID == 0 || result.TaskID == 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	key := BuildIdempotencyKey(input.FlightID, input.SourceEventID, occurrence)
	arrivalEvent, ok := repository.event(key)
	if !ok {
		t.Fatalf("event with key %q was not saved", key)
	}
	if arrivalEvent.Type != model.EventFlightArrived || arrivalEvent.Source != model.EventSourceManual || arrivalEvent.Status != model.EventStatusPending {
		t.Fatalf("unexpected event identity/state: %+v", arrivalEvent)
	}
	if arrivalEvent.SourceEventID != "source-arrival-1" || arrivalEvent.FlightID == nil || *arrivalEvent.FlightID != input.FlightID {
		t.Fatalf("unexpected event source/flight fields: %+v", arrivalEvent)
	}
	if arrivalEvent.TaskID == nil || *arrivalEvent.TaskID != result.TaskID {
		t.Fatalf("event task link = %v, want %d", arrivalEvent.TaskID, result.TaskID)
	}
	if !arrivalEvent.TriggerTime.Equal(occurrence.UTC()) {
		t.Fatalf("event trigger time = %s, want %s", arrivalEvent.TriggerTime, occurrence.UTC())
	}

	task, ok := repository.task(result.TaskID)
	if !ok {
		t.Fatalf("task %d was not saved", result.TaskID)
	}
	if task.FlightID != input.FlightID || task.TeamID != input.TeamID || task.TemplateID != input.TemplateID || task.TriggerEventID != result.EventID {
		t.Fatalf("unexpected task linkage: %+v", task)
	}
	if task.TemplateVersion != 7 || task.Status != model.TaskStatusPending || task.AssignedCount != 0 {
		t.Fatalf("unexpected task version/state: %+v", task)
	}
	if !task.PlannedStart.Equal(occurrence.UTC()) || !task.PlannedEnd.Equal(occurrence.UTC().Add(15*time.Minute)) {
		t.Fatalf("unexpected task plan: start=%s end=%s", task.PlannedStart, task.PlannedEnd)
	}
	if repository.eventCount() != 1 || repository.taskCount() != 1 {
		t.Fatalf("B3 must create only one event and task, got events=%d tasks=%d", repository.eventCount(), repository.taskCount())
	}
}

func TestRecordFlightArrivedUsesDefaultTimeoutForZeroTemplateTimeout(t *testing.T) {
	repository := newFakeRepository()
	template := repository.templates[301]
	template.TimeoutSeconds = 0
	repository.templates[301] = template
	occurrence := time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)

	result, err := NewService(repository).Record(context.Background(), RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301, OccurrenceTime: occurrence,
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	task, ok := repository.task(result.TaskID)
	if !ok {
		t.Fatal("task was not saved")
	}
	wantEnd := occurrence.Add(time.Duration(DefaultTaskTimeoutSeconds) * time.Second)
	if !task.PlannedEnd.Equal(wantEnd) {
		t.Fatalf("planned end = %s, want zero-timeout fallback %s", task.PlannedEnd, wantEnd)
	}
}

func TestRecordFlightArrivedReturnsStableDomainErrors(t *testing.T) {
	baseInput := RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301, SourceEventID: "domain-error", OccurrenceTime: time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		name   string
		mutate func(*fakeRepository, *RecordInput)
		want   error
	}{
		{
			name: "invalid input",
			mutate: func(_ *fakeRepository, input *RecordInput) {
				input.OccurrenceTime = time.Time{}
			},
			want: ErrInvalidInput,
		},
		{
			name: "flight not found",
			mutate: func(repository *fakeRepository, _ *RecordInput) {
				delete(repository.flights, 101)
			},
			want: ErrFlightNotFound,
		},
		{
			name: "flight disabled",
			mutate: func(repository *fakeRepository, _ *RecordInput) {
				flight := repository.flights[101]
				flight.Status = model.FlightStatusDisabled
				repository.flights[101] = flight
			},
			want: ErrFlightDisabled,
		},
		{
			name: "team not found",
			mutate: func(repository *fakeRepository, _ *RecordInput) {
				delete(repository.teams, 201)
			},
			want: ErrTeamNotFound,
		},
		{
			name: "team disabled",
			mutate: func(repository *fakeRepository, _ *RecordInput) {
				team := repository.teams[201]
				team.Enabled = false
				repository.teams[201] = team
			},
			want: ErrTeamDisabled,
		},
		{
			name: "template not found",
			mutate: func(repository *fakeRepository, _ *RecordInput) {
				delete(repository.templates, 301)
			},
			want: ErrTemplateNotFound,
		},
		{
			name: "template disabled",
			mutate: func(repository *fakeRepository, _ *RecordInput) {
				template := repository.templates[301]
				template.Enabled = false
				repository.templates[301] = template
			},
			want: ErrTemplateDisabled,
		},
		{
			name: "template trigger mismatch",
			mutate: func(repository *fakeRepository, _ *RecordInput) {
				template := repository.templates[301]
				template.TriggerEventType = model.EventTaskExecution
				repository.templates[301] = template
			},
			want: ErrTemplateTriggerMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newFakeRepository()
			input := baseInput
			test.mutate(repository, &input)

			_, err := NewService(repository).Record(context.Background(), input)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, test.want)
			}
			if repository.eventCount() != 0 || repository.taskCount() != 0 {
				t.Fatalf("failed request must not persist data, got events=%d tasks=%d", repository.eventCount(), repository.taskCount())
			}
		})
	}
}

func TestRecordFlightArrivedReturnsOriginalIDsForDuplicate(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository)
	input := RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301, SourceEventID: "repeat-source", OccurrenceTime: time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
	}
	first, err := service.Record(context.Background(), input)
	if err != nil {
		t.Fatalf("first Record() error = %v", err)
	}

	// Duplicates return the durable original result even if a referenced resource
	// becomes invalid later; the event unique key remains the source of truth.
	delete(repository.flights, input.FlightID)
	template := repository.templates[input.TemplateID]
	template.Enabled = false
	repository.templates[input.TemplateID] = template
	transactionsBefore := repository.transactionCalls

	second, err := service.Record(context.Background(), input)
	if err != nil {
		t.Fatalf("second Record() error = %v", err)
	}
	if !second.Duplicate || second.EventID != first.EventID || second.TaskID != first.TaskID {
		t.Fatalf("duplicate result = %+v, first = %+v", second, first)
	}
	if repository.transactionCalls != transactionsBefore || repository.eventCount() != 1 || repository.taskCount() != 1 {
		t.Fatalf("duplicate request must not create another transaction/data: tx=%d events=%d tasks=%d", repository.transactionCalls, repository.eventCount(), repository.taskCount())
	}
}

func TestRecordFlightArrivedRecoversUniqueKeyRace(t *testing.T) {
	repository := newFakeRepository()
	input := RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301, SourceEventID: "racing-source", OccurrenceTime: time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
	}
	key := BuildIdempotencyKey(input.FlightID, input.SourceEventID, input.OccurrenceTime)
	externalTaskID := int64(802)
	repository.forceDuplicateEvent = &model.Event{ID: 801, IdempotencyKey: key, TaskID: &externalTaskID}

	result, err := NewService(repository).Record(context.Background(), input)
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if !result.Duplicate || result.EventID != 801 || result.TaskID != externalTaskID {
		t.Fatalf("unique-key race result = %+v", result)
	}
	if repository.createEventCalls != 1 || repository.createTaskCalls != 0 || repository.eventCount() != 1 || repository.taskCount() != 0 {
		t.Fatalf("race must recover the winner without creating a task: event calls=%d task calls=%d events=%d tasks=%d", repository.createEventCalls, repository.createTaskCalls, repository.eventCount(), repository.taskCount())
	}
}

func TestRecordFlightArrivedRollsBackWhenTaskCreationFails(t *testing.T) {
	repository := newFakeRepository()
	injectedErr := errors.New("injected task insert failure")
	repository.createTaskErr = injectedErr
	input := RecordInput{
		FlightID: 101, TeamID: 201, TemplateID: 301, SourceEventID: "rollback-source", OccurrenceTime: time.Date(2026, 8, 7, 14, 0, 0, 0, time.UTC),
	}

	_, err := NewService(repository).Record(context.Background(), input)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("error = %v, want wrapped injected error", err)
	}
	if repository.eventCount() != 0 || repository.taskCount() != 0 || repository.linkCalls != 0 {
		t.Fatalf("failed task insert must rollback event and avoid link: events=%d tasks=%d links=%d", repository.eventCount(), repository.taskCount(), repository.linkCalls)
	}
}

func TestIsDuplicateKeyError(t *testing.T) {
	if !isDuplicateKeyError(&drivermysql.MySQLError{Number: 1062}) {
		t.Fatal("MySQL 1062 should be recognized as a duplicate key")
	}
	if isDuplicateKeyError(&drivermysql.MySQLError{Number: 1064}) {
		t.Fatal("non-duplicate MySQL error must not be recognized")
	}
}
