// Package sync contains Edge Inbox, Projection and Command Store contracts.
package sync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

var (
	ErrDuplicate                 = errors.New("duplicate edge synchronization record")
	ErrNotFound                  = errors.New("edge synchronization record not found")
	ErrProjectionVersionConflict = errors.New("edge task projection version conflicts with existing payload")
	ErrCommandIDConflict         = errors.New("edge command id was reused with different content")
)

type TaskProjection struct {
	PublicID           string    `json:"public_id"`
	AssignmentPublicID string    `json:"assignment_public_id,omitempty"`
	EmployeePublicID   string    `json:"employee_public_id"`
	FlightDisplayNo    string    `json:"flight_display_no"`
	TaskName           string    `json:"task_name"`
	AreaName           string    `json:"area_name"`
	PlannedAt          time.Time `json:"planned_at"`
	// Status is retained for the Foundation probe and old clients. Business
	// events use BusinessStatus, but both fields are kept aligned on write.
	Status         string    `json:"status,omitempty"`
	BusinessStatus string    `json:"business_status,omitempty"`
	Message        string    `json:"message"`
	SyncVersion    uint64    `json:"sync_version"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type InboxRecord struct {
	Envelope   sharedEvent.EventEnvelope
	Status     string
	Error      string
	ReceivedAt time.Time
	AppliedAt  *time.Time
}

type CommandRecord struct {
	ID          uint64                      `json:"id"`
	Envelope    sharedEvent.CommandEnvelope `json:"envelope"`
	Status      string                      `json:"status"`
	Attempts    int                         `json:"attempts"`
	NextAttempt time.Time                   `json:"next_attempt_at"`
	LastError   string                      `json:"last_error"`
	CreatedAt   time.Time                   `json:"created_at"`
}

type EventProjection func(context.Context, sharedEvent.EventEnvelope) error

type Store interface {
	ApplyEvent(ctx context.Context, envelope sharedEvent.EventEnvelope, project EventProjection) (duplicate bool, err error)
	PutCommand(ctx context.Context, command sharedEvent.CommandEnvelope) (duplicate bool, err error)
	ClaimPendingCommands(ctx context.Context, limit int, now time.Time) ([]CommandRecord, error)
	MarkCommandSent(ctx context.Context, commandID string) error
	MarkCommandRetry(ctx context.Context, commandID string, next time.Time, reason string) error
	MarkCommandFailed(ctx context.Context, commandID string, reason string) error
	UpsertTaskProjection(ctx context.Context, projection TaskProjection) error
	ListTaskProjections(ctx context.Context, employeePublicID string) ([]TaskProjection, error)
}

type MemoryStore struct {
	mu          sync.Mutex
	nextID      uint64
	inbox       map[string]InboxRecord
	commands    map[string]CommandRecord
	projections map[string]TaskProjection
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{inbox: make(map[string]InboxRecord), commands: make(map[string]CommandRecord), projections: make(map[string]TaskProjection)}
}

func (s *MemoryStore) ApplyEvent(ctx context.Context, envelope sharedEvent.EventEnvelope, project EventProjection) (bool, error) {
	if err := envelope.Validate(); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	if existing, exists := s.inbox[envelope.EventID]; exists {
		if existing.Status == sharedEvent.StatusApplied {
			s.mu.Unlock()
			return true, nil
		}
		if existing.Status == sharedEvent.StatusProcessing && time.Since(existing.ReceivedAt) < time.Minute {
			s.mu.Unlock()
			return true, nil
		}
	}
	received := time.Now().UTC()
	s.inbox[envelope.EventID] = InboxRecord{Envelope: envelope, Status: sharedEvent.StatusProcessing, ReceivedAt: received}
	s.mu.Unlock()
	if project == nil {
		s.mu.Lock()
		s.inbox[envelope.EventID] = InboxRecord{Envelope: envelope, Status: sharedEvent.StatusApplied, ReceivedAt: received, AppliedAt: &received}
		s.mu.Unlock()
		return false, nil
	}
	if err := project(ctx, envelope); err != nil {
		s.mu.Lock()
		s.inbox[envelope.EventID] = InboxRecord{Envelope: envelope, Status: sharedEvent.StatusFailed, Error: err.Error(), ReceivedAt: received}
		s.mu.Unlock()
		return false, err
	}
	s.mu.Lock()
	s.inbox[envelope.EventID] = InboxRecord{Envelope: envelope, Status: sharedEvent.StatusApplied, ReceivedAt: received, AppliedAt: &received}
	s.mu.Unlock()
	return false, nil
}

func (s *MemoryStore) PutCommand(ctx context.Context, command sharedEvent.CommandEnvelope) (bool, error) {
	if err := command.Validate(); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.commands[command.CommandID]; exists {
		if !sameCommand(existing.Envelope, command) {
			return false, ErrCommandIDConflict
		}
		return true, nil
	}
	s.nextID++
	now := time.Now().UTC()
	s.commands[command.CommandID] = CommandRecord{ID: s.nextID, Envelope: command, Status: sharedEvent.StatusPending, NextAttempt: now, CreatedAt: now}
	return false, nil
}

func sameCommand(left, right sharedEvent.CommandEnvelope) bool {
	return left.CommandID == right.CommandID && left.CommandType == right.CommandType && left.SchemaVersion == right.SchemaVersion && left.ActorPublicID == right.ActorPublicID && left.AggregateID == right.AggregateID && sharedEvent.EqualPersistedTime(left.OccurredAt, right.OccurredAt) && left.TraceID == right.TraceID && sharedEvent.EquivalentJSON(left.Payload, right.Payload)
}

func (s *MemoryStore) ClaimPendingCommands(ctx context.Context, limit int, now time.Time) ([]CommandRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]CommandRecord, 0, len(s.commands))
	for _, value := range s.commands {
		if (value.Status == sharedEvent.StatusPending || value.Status == sharedEvent.StatusRetry || value.Status == sharedEvent.StatusProcessing) && !value.NextAttempt.After(now) {
			value.Status = sharedEvent.StatusProcessing
			value.NextAttempt = now.UTC()
			s.commands[value.Envelope.CommandID] = value
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	if len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func (s *MemoryStore) MarkCommandSent(ctx context.Context, commandID string) error {
	return s.updateCommand(ctx, commandID, func(value *CommandRecord) { value.Status = sharedEvent.StatusSent; value.LastError = "" })
}

func (s *MemoryStore) MarkCommandRetry(ctx context.Context, commandID string, next time.Time, reason string) error {
	return s.updateCommand(ctx, commandID, func(value *CommandRecord) {
		value.Status = sharedEvent.StatusRetry
		value.NextAttempt = next.UTC()
		value.Attempts++
		value.LastError = reason
	})
}

func (s *MemoryStore) MarkCommandFailed(ctx context.Context, commandID string, reason string) error {
	return s.updateCommand(ctx, commandID, func(value *CommandRecord) {
		value.Status = sharedEvent.StatusFailed
		value.Attempts++
		value.LastError = reason
	})
}

func (s *MemoryStore) UpsertTaskProjection(ctx context.Context, projection TaskProjection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	projection, err := normalizeTaskProjection(projection)
	if err != nil {
		return err
	}
	if projection.PublicID == "" || projection.EmployeePublicID == "" || projection.SyncVersion == 0 {
		return fmt.Errorf("task projection public_id, employee_public_id and sync_version are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, exists := s.projections[projection.PublicID]; exists && previous.SyncVersion > projection.SyncVersion {
		return nil
	}
	if previous, exists := s.projections[projection.PublicID]; exists && previous.SyncVersion == projection.SyncVersion {
		if !sameTaskProjection(previous, projection) {
			return ErrProjectionVersionConflict
		}
		return nil
	}
	if projection.UpdatedAt.IsZero() {
		projection.UpdatedAt = time.Now().UTC()
	}
	s.projections[projection.PublicID] = projection
	return nil
}

func normalizeTaskProjection(projection TaskProjection) (TaskProjection, error) {
	if projection.BusinessStatus == "" {
		projection.BusinessStatus = projection.Status
	}
	if projection.Status == "" {
		projection.Status = projection.BusinessStatus
	}
	if projection.UpdatedAt.IsZero() {
		projection.UpdatedAt = time.Now().UTC()
	}
	return projection, nil
}

func sameTaskProjection(left, right TaskProjection) bool {
	return left.PublicID == right.PublicID &&
		left.AssignmentPublicID == right.AssignmentPublicID &&
		left.EmployeePublicID == right.EmployeePublicID &&
		left.FlightDisplayNo == right.FlightDisplayNo &&
		left.TaskName == right.TaskName &&
		left.AreaName == right.AreaName &&
		left.PlannedAt.Equal(right.PlannedAt) &&
		left.Status == right.Status &&
		left.BusinessStatus == right.BusinessStatus &&
		left.Message == right.Message &&
		left.SyncVersion == right.SyncVersion
}

func (s *MemoryStore) ListTaskProjections(ctx context.Context, employeePublicID string) ([]TaskProjection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]TaskProjection, 0)
	for _, value := range s.projections {
		if employeePublicID == "" || value.EmployeePublicID == employeePublicID {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.Before(values[j].UpdatedAt) })
	return values, nil
}

func (s *MemoryStore) InboxRecord(eventID string) (InboxRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.inbox[eventID]
	return value, ok
}

func (s *MemoryStore) CommandRecord(commandID string) (CommandRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.commands[commandID]
	return value, ok
}

func (s *MemoryStore) updateCommand(ctx context.Context, commandID string, update func(*CommandRecord)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.commands[commandID]
	if !ok {
		return ErrNotFound
	}
	update(&value)
	s.commands[commandID] = value
	return nil
}
