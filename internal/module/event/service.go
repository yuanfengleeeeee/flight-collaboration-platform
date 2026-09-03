// Package event implements the flight-arrival event and task-generation slice.
package event

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/model"
)

const (
	// DefaultTaskTimeoutSeconds is used when a template has no timeout configured.
	// B3 treats zero (and invalid negative legacy values) as a 30-minute deadline.
	DefaultTaskTimeoutSeconds = 30 * 60
)

var (
	// ErrInvalidInput identifies input that cannot represent a flight-arrival event.
	ErrInvalidInput = errors.New("invalid flight arrival input")

	// Resource errors intentionally stay distinct so a later HTTP handler can map
	// them to stable API errors without matching error strings.
	ErrFlightNotFound          = errors.New("flight not found")
	ErrFlightDisabled          = errors.New("flight disabled")
	ErrTeamNotFound            = errors.New("team not found")
	ErrTeamDisabled            = errors.New("team disabled")
	ErrTemplateNotFound        = errors.New("task template not found")
	ErrTemplateDisabled        = errors.New("task template disabled")
	ErrTemplateTriggerMismatch = errors.New("task template trigger mismatch")

	// ErrNotFound and ErrDuplicateKey are repository-level signals. They are used
	// internally by the service to make uniqueness races safe without exposing
	// database-driver details to callers.
	ErrNotFound              = errors.New("repository record not found")
	ErrDuplicateKey          = errors.New("duplicate key")
	ErrRepositoryUnavailable = errors.New("event repository unavailable")
	ErrEventTaskLinkMissing  = errors.New("event task link missing")
)

// RecordInput is the complete input required to record one flight-arrival event.
// SourceEventID is optional for manual/test events; OccurrenceTime is required.
type RecordInput struct {
	FlightID       int64
	TeamID         int64
	TemplateID     int64
	SourceEventID  string
	OccurrenceTime time.Time
}

// RecordResult contains the durable identifiers for an event and its one task.
// Duplicate is true when a prior request already recorded the same idempotency key.
type RecordResult struct {
	EventID   int64
	TaskID    int64
	Duplicate bool
}

// Repository is deliberately small so service behavior can be covered with a
// pure in-memory fake. The production GORM adapter is in repository.go.
type Repository interface {
	FindEventByIdempotencyKey(ctx context.Context, key string) (model.Event, error)
	InTransaction(ctx context.Context, fn func(Transaction) error) error
}

// Transaction exposes exactly the reads and writes B3 needs to keep event and
// task creation in one MySQL transaction.
type Transaction interface {
	FindEventByIdempotencyKey(ctx context.Context, key string) (model.Event, error)
	FindFlight(ctx context.Context, id int64) (model.Flight, error)
	FindTeam(ctx context.Context, id int64) (model.Team, error)
	FindTaskTemplate(ctx context.Context, id int64) (model.TaskTemplate, error)
	CreateEvent(ctx context.Context, value *model.Event) error
	CreateTaskInstance(ctx context.Context, value *model.TaskInstance) error
	LinkEventTask(ctx context.Context, eventID, taskID int64) error
}

// Service records the arrival event and generates its initial pending task.
type Service struct {
	repository Repository
}

// NewService constructs a B3 service over the supplied repository.
func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// Record is an alias for RecordFlightArrived for callers that only handle this
// event type in B3.
func (s *Service) Record(ctx context.Context, input RecordInput) (RecordResult, error) {
	return s.RecordFlightArrived(ctx, input)
}

// RecordFlightArrived atomically creates a pending flight_arrived event, its
// pending task instance, and the event-to-task link. It does not create a
// candidate, notification, or assignment; those are later business steps.
func (s *Service) RecordFlightArrived(ctx context.Context, input RecordInput) (RecordResult, error) {
	if s == nil || s.repository == nil {
		return RecordResult{}, ErrRepositoryUnavailable
	}

	input, err := normalizeInput(input)
	if err != nil {
		return RecordResult{}, err
	}
	key := BuildIdempotencyKey(input.FlightID, input.SourceEventID, input.OccurrenceTime)

	// This is only a fast path. Correctness under concurrent requests is provided
	// by the event unique key plus the duplicate-key recovery below.
	if existing, err := s.repository.FindEventByIdempotencyKey(ctx, key); err == nil {
		return duplicateResult(existing)
	} else if !errors.Is(err, ErrNotFound) {
		return RecordResult{}, fmt.Errorf("find existing flight arrival event: %w", err)
	}

	var result RecordResult
	err = s.repository.InTransaction(ctx, func(tx Transaction) error {
		// Recheck inside the transaction for the ordinary sequential duplicate case.
		// A racing insert is still resolved by the event unique key on CreateEvent.
		if existing, findErr := tx.FindEventByIdempotencyKey(ctx, key); findErr == nil {
			duplicate, resultErr := duplicateResult(existing)
			if resultErr != nil {
				return resultErr
			}
			result = duplicate
			return nil
		} else if !errors.Is(findErr, ErrNotFound) {
			return fmt.Errorf("find event in transaction: %w", findErr)
		}

		flight, findErr := tx.FindFlight(ctx, input.FlightID)
		if findErr != nil {
			return resourceNotFound(ErrFlightNotFound, input.FlightID, findErr)
		}
		// The current flight schema has no enabled column. Until its state machine
		// is expanded, only the explicit disabled status is rejected by B3.
		if flight.Status == model.FlightStatusDisabled {
			return fmt.Errorf("flight %d: %w", input.FlightID, ErrFlightDisabled)
		}

		team, findErr := tx.FindTeam(ctx, input.TeamID)
		if findErr != nil {
			return resourceNotFound(ErrTeamNotFound, input.TeamID, findErr)
		}
		if !team.Enabled {
			return fmt.Errorf("team %d: %w", input.TeamID, ErrTeamDisabled)
		}

		template, findErr := tx.FindTaskTemplate(ctx, input.TemplateID)
		if findErr != nil {
			return resourceNotFound(ErrTemplateNotFound, input.TemplateID, findErr)
		}
		if !template.Enabled {
			return fmt.Errorf("task template %d: %w", input.TemplateID, ErrTemplateDisabled)
		}
		if template.TriggerEventType != model.EventFlightArrived {
			return fmt.Errorf("task template %d trigger %q: %w", input.TemplateID, template.TriggerEventType, ErrTemplateTriggerMismatch)
		}

		arrivalEvent := newFlightArrivedEvent(input, key)
		if createErr := tx.CreateEvent(ctx, &arrivalEvent); createErr != nil {
			return fmt.Errorf("create flight arrival event: %w", createErr)
		}
		if arrivalEvent.ID <= 0 {
			return fmt.Errorf("create flight arrival event: missing generated ID")
		}

		task := newPendingTask(input, template, arrivalEvent.ID)
		if createErr := tx.CreateTaskInstance(ctx, &task); createErr != nil {
			return fmt.Errorf("create task instance: %w", createErr)
		}
		if task.ID <= 0 {
			return fmt.Errorf("create task instance: missing generated ID")
		}

		if linkErr := tx.LinkEventTask(ctx, arrivalEvent.ID, task.ID); linkErr != nil {
			return fmt.Errorf("link event %d to task %d: %w", arrivalEvent.ID, task.ID, linkErr)
		}
		result = RecordResult{EventID: arrivalEvent.ID, TaskID: task.ID}
		return nil
	})
	if err == nil {
		return result, nil
	}

	// A unique-key error means another transaction won the race. Its transaction
	// is the source of truth; read it after our transaction has rolled back.
	if errors.Is(err, ErrDuplicateKey) {
		existing, findErr := s.repository.FindEventByIdempotencyKey(ctx, key)
		if findErr != nil {
			return RecordResult{}, fmt.Errorf("load event after duplicate key: %w", findErr)
		}
		return duplicateResult(existing)
	}
	return RecordResult{}, err
}

// BuildIdempotencyKey makes a stable key for the one B3 event type. External
// source IDs take precedence; fallback keys deliberately use the UTC date.
func BuildIdempotencyKey(flightID int64, sourceEventID string, occurrenceTime time.Time) string {
	if sourceEventID = strings.TrimSpace(sourceEventID); sourceEventID != "" {
		return fmt.Sprintf("%s:source:%s", model.EventFlightArrived, sourceEventID)
	}
	return fmt.Sprintf("%s:flight:%d:date:%s", model.EventFlightArrived, flightID, occurrenceTime.UTC().Format("2006-01-02"))
}

func normalizeInput(input RecordInput) (RecordInput, error) {
	if input.FlightID <= 0 || input.TeamID <= 0 || input.TemplateID <= 0 || input.OccurrenceTime.IsZero() {
		return RecordInput{}, fmt.Errorf("flight_id, team_id, template_id and occurrence_time are required: %w", ErrInvalidInput)
	}
	input.SourceEventID = strings.TrimSpace(input.SourceEventID)
	if len(input.SourceEventID) > 128 {
		return RecordInput{}, fmt.Errorf("source_event_id exceeds 128 characters: %w", ErrInvalidInput)
	}
	input.OccurrenceTime = input.OccurrenceTime.UTC()
	return input, nil
}

func duplicateResult(value model.Event) (RecordResult, error) {
	if value.ID <= 0 || value.TaskID == nil || *value.TaskID <= 0 {
		return RecordResult{}, fmt.Errorf("event %d: %w", value.ID, ErrEventTaskLinkMissing)
	}
	return RecordResult{EventID: value.ID, TaskID: *value.TaskID, Duplicate: true}, nil
}

func resourceNotFound(domainErr error, id int64, err error) error {
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("resource %d: %w", id, domainErr)
	}
	return fmt.Errorf("load resource %d: %w", id, err)
}

func newFlightArrivedEvent(input RecordInput, key string) model.Event {
	flightID := input.FlightID
	return model.Event{
		Type:              model.EventFlightArrived,
		Level:             model.EventLevelInfo,
		Source:            model.EventSourceManual,
		SourceEventID:     input.SourceEventID,
		IdempotencyKey:    key,
		TriggerTime:       input.OccurrenceTime,
		FlightID:          &flightID,
		AffectedPositions: "[]",
		AffectedUsers:     "[]",
		Status:            model.EventStatusPending,
		Title:             "Flight arrived",
		Description:       "Flight arrival event recorded manually",
		HandleLogs:        "[]",
	}
}

func newPendingTask(input RecordInput, template model.TaskTemplate, triggerEventID int64) model.TaskInstance {
	timeoutSeconds := template.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = DefaultTaskTimeoutSeconds
	}
	return model.TaskInstance{
		FlightID:        input.FlightID,
		TemplateID:      input.TemplateID,
		TeamID:          input.TeamID,
		TriggerEventID:  triggerEventID,
		TemplateVersion: template.Version,
		PlannedStart:    input.OccurrenceTime,
		PlannedEnd:      input.OccurrenceTime.Add(time.Duration(timeoutSeconds) * time.Second),
		Status:          model.TaskStatusPending,
	}
}
