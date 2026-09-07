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
	ErrLeaseLost                 = errors.New("edge synchronization lease is no longer owned")
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
	Status         string     `json:"status,omitempty"`
	BusinessStatus string     `json:"business_status,omitempty"`
	ReceiptStatus  string     `json:"receipt_status,omitempty"`
	ReceivedAt     *time.Time `json:"received_at,omitempty"`
	Message        string     `json:"message"`
	SyncVersion    uint64     `json:"sync_version"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type NotificationProjection struct {
	PublicID         string    `json:"public_id"`
	EmployeePublicID string    `json:"employee_public_id"`
	Title            string    `json:"title"`
	Message          string    `json:"message"`
	Status           string    `json:"status"`
	SyncVersion      uint64    `json:"sync_version"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type NotificationFilter struct {
	EmployeePublicID string
	Status           string
	Page             int
	PageSize         int
}

// TaskHistoryFilter is separate from TaskSnapshot on purpose: the task list
// is an authoritative employee-scoped recovery snapshot, while history is a
// paged read model and must not load every projection into application memory.
type TaskHistoryFilter struct {
	EmployeePublicID string
	Status           string
	Page             int
	PageSize         int
}

type TaskHistoryStore interface {
	ListTaskHistory(context.Context, TaskHistoryFilter) ([]TaskProjection, int64, error)
}

type InboxRecord struct {
	Envelope   sharedEvent.EventEnvelope
	Status     string
	Error      string
	ReceivedAt time.Time
	AppliedAt  *time.Time
}

type CommandRecord struct {
	ID             uint64                      `json:"id"`
	Envelope       sharedEvent.CommandEnvelope `json:"envelope"`
	Status         string                      `json:"status"`
	Attempts       int                         `json:"attempts"`
	NextAttempt    time.Time                   `json:"next_attempt_at"`
	LastError      string                      `json:"last_error"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
	LeaseOwner     string                      `json:"-"`
	LeaseExpiresAt time.Time                   `json:"-"`
}

type SyncQueueStats struct {
	PendingCommandCount     int64
	PendingInboxCount       int64
	FailedInboxCount        int64
	OldestPendingCommandAge time.Duration
	OldestPendingInboxAge   time.Duration
	ProjectionLag           time.Duration
}

type SyncQueueStatsProvider interface {
	SyncQueueStats(ctx context.Context, now time.Time) (SyncQueueStats, error)
}

// TaskSnapshot is the employee-scoped recovery read model. ProjectionRevision
// is a durable per-employee change sequence, not any individual task's
// sync_version. The current API still returns a complete snapshot; the
// revision gives a client a stable point to compare after refresh or reconnect.
type TaskSnapshot struct {
	Items              []TaskProjection
	ProjectionRevision uint64
	SnapshotAt         time.Time
	ProjectionLag      time.Duration
	ProjectionLagKnown bool
}

type TaskSnapshotProvider interface {
	ListTaskSnapshot(ctx context.Context, employeePublicID string, now time.Time) (TaskSnapshot, error)
}

// NotificationStore is optional so the foundation MemoryStore and existing
// projection-only test doubles remain source-compatible. Durable Edge stores
// implement it for the employee notification read surface.
type NotificationStore interface {
	ListNotifications(context.Context, NotificationFilter) ([]NotificationProjection, int64, error)
	MarkNotificationRead(context.Context, string, string) error
}

type NotificationProjectionWriter interface {
	UpsertNotificationProjection(context.Context, NotificationProjection) error
}

type EventProjection func(context.Context, sharedEvent.EventEnvelope) error

type Store interface {
	ApplyEvent(ctx context.Context, envelope sharedEvent.EventEnvelope, project EventProjection) (duplicate bool, err error)
	PutCommand(ctx context.Context, command sharedEvent.CommandEnvelope) (duplicate bool, err error)
	FindCommand(ctx context.Context, commandID string) (CommandRecord, error)
	ClaimPendingCommands(ctx context.Context, limit int, now time.Time) ([]CommandRecord, error)
	MarkCommandSent(ctx context.Context, commandID string) error
	MarkCommandRetry(ctx context.Context, commandID string, next time.Time, reason string) error
	MarkCommandFailed(ctx context.Context, commandID string, reason string) error
	UpsertTaskProjection(ctx context.Context, projection TaskProjection) error
	ListTaskProjections(ctx context.Context, employeePublicID string) ([]TaskProjection, error)
}

// LeasedStore is implemented by durable Edge stores so multiple Core Worker
// processes can claim commands safely and stale acknowledgements are ignored.
type LeasedStore interface {
	ClaimPendingCommandsWithLease(ctx context.Context, limit int, now time.Time, owner string, leaseDuration time.Duration) ([]CommandRecord, error)
	MarkCommandSentWithLease(ctx context.Context, commandID, owner string) error
	MarkCommandRetryWithLease(ctx context.Context, commandID, owner string, next time.Time, reason string) error
	MarkCommandFailedWithLease(ctx context.Context, commandID, owner string, reason string) error
}

type MemoryStore struct {
	mu                  sync.Mutex
	nextID              uint64
	inbox               map[string]InboxRecord
	commands            map[string]CommandRecord
	projections         map[string]TaskProjection
	projectionRevisions map[string]uint64
	notifications       map[string]NotificationProjection
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{inbox: make(map[string]InboxRecord), commands: make(map[string]CommandRecord), projections: make(map[string]TaskProjection), projectionRevisions: make(map[string]uint64), notifications: make(map[string]NotificationProjection)}
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
	s.commands[command.CommandID] = CommandRecord{ID: s.nextID, Envelope: command, Status: sharedEvent.StatusPending, NextAttempt: now, CreatedAt: now, UpdatedAt: now}
	return false, nil
}

func sameCommand(left, right sharedEvent.CommandEnvelope) bool {
	return sharedEvent.EquivalentCommand(left, right)
}

func (s *MemoryStore) ClaimPendingCommands(ctx context.Context, limit int, now time.Time) ([]CommandRecord, error) {
	return s.ClaimPendingCommandsWithLease(ctx, limit, now, "legacy", time.Nanosecond)
}

func (s *MemoryStore) ClaimPendingCommandsWithLease(ctx context.Context, limit int, now time.Time, owner string, leaseDuration time.Duration) ([]CommandRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if owner == "" {
		return nil, fmt.Errorf("command lease owner is required")
	}
	if leaseDuration <= 0 {
		leaseDuration = time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]CommandRecord, 0, len(s.commands))
	for _, value := range s.commands {
		ready := (value.Status == sharedEvent.StatusPending || value.Status == sharedEvent.StatusRetry) && !value.NextAttempt.After(now)
		expired := value.Status == sharedEvent.StatusProcessing && (value.LeaseExpiresAt.IsZero() || !value.LeaseExpiresAt.After(now))
		if ready || expired {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	if len(values) > limit {
		values = values[:limit]
	}
	for index, value := range values {
		value.Status = sharedEvent.StatusProcessing
		value.NextAttempt = now.UTC()
		value.UpdatedAt = now.UTC()
		value.LeaseOwner = owner
		value.LeaseExpiresAt = now.UTC().Add(leaseDuration)
		if owner == "legacy" {
			value.LeaseExpiresAt = now.UTC()
		}
		s.commands[value.Envelope.CommandID] = value
		values[index] = value
	}
	return values, nil
}

func (s *MemoryStore) MarkCommandSent(ctx context.Context, commandID string) error {
	return s.updateCommand(ctx, commandID, func(value *CommandRecord) {
		value.Status, value.LastError = sharedEvent.StatusSent, ""
		value.LeaseOwner, value.LeaseExpiresAt = "", time.Time{}
	})
}

func (s *MemoryStore) MarkCommandRetry(ctx context.Context, commandID string, next time.Time, reason string) error {
	return s.updateCommand(ctx, commandID, func(value *CommandRecord) {
		value.Status = sharedEvent.StatusRetry
		value.NextAttempt = next.UTC()
		value.Attempts++
		value.LastError = reason
		value.LeaseOwner, value.LeaseExpiresAt = "", time.Time{}
	})
}

func (s *MemoryStore) MarkCommandFailed(ctx context.Context, commandID string, reason string) error {
	return s.updateCommand(ctx, commandID, func(value *CommandRecord) {
		value.Status = sharedEvent.StatusFailed
		value.Attempts++
		value.LastError = reason
		value.LeaseOwner, value.LeaseExpiresAt = "", time.Time{}
	})
}

func (s *MemoryStore) MarkCommandSentWithLease(ctx context.Context, commandID, owner string) error {
	return s.updateCommandWithLease(ctx, commandID, owner, func(value *CommandRecord) {
		value.Status, value.LastError = sharedEvent.StatusSent, ""
	})
}

func (s *MemoryStore) MarkCommandRetryWithLease(ctx context.Context, commandID, owner string, next time.Time, reason string) error {
	return s.updateCommandWithLease(ctx, commandID, owner, func(value *CommandRecord) {
		value.Status, value.NextAttempt, value.Attempts, value.LastError = sharedEvent.StatusRetry, next.UTC(), value.Attempts+1, reason
	})
}

func (s *MemoryStore) MarkCommandFailedWithLease(ctx context.Context, commandID, owner string, reason string) error {
	return s.updateCommandWithLease(ctx, commandID, owner, func(value *CommandRecord) {
		value.Status, value.Attempts, value.LastError = sharedEvent.StatusFailed, value.Attempts+1, reason
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
	previous, exists := s.projections[projection.PublicID]
	if exists && previous.SyncVersion > projection.SyncVersion {
		return nil
	}
	if exists && previous.SyncVersion == projection.SyncVersion {
		if !sameTaskProjection(previous, projection) {
			return ErrProjectionVersionConflict
		}
		return nil
	}
	if projection.UpdatedAt.IsZero() {
		projection.UpdatedAt = time.Now().UTC()
	}
	s.projections[projection.PublicID] = projection
	s.projectionRevisions[projection.EmployeePublicID]++
	if exists && previous.EmployeePublicID != projection.EmployeePublicID {
		// Reassignment removes the task from the previous employee's snapshot as
		// well, so both employee-scoped revisions must advance.
		s.projectionRevisions[previous.EmployeePublicID]++
	}
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

func (s *MemoryStore) ListNotifications(ctx context.Context, filter NotificationFilter) ([]NotificationProjection, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]NotificationProjection, 0)
	for _, value := range s.notifications {
		if filter.EmployeePublicID != "" && value.EmployeePublicID != filter.EmployeePublicID {
			continue
		}
		if filter.Status != "" && value.Status != filter.Status {
			continue
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].UpdatedAt.Equal(values[j].UpdatedAt) {
			return values[i].PublicID < values[j].PublicID
		}
		return values[i].UpdatedAt.Before(values[j].UpdatedAt)
	})
	total := int64(len(values))
	page, pageSize := normalizeNotificationPagination(filter.Page, filter.PageSize)
	start := (page - 1) * pageSize
	if start >= len(values) {
		return []NotificationProjection{}, total, nil
	}
	end := start + pageSize
	if end > len(values) {
		end = len(values)
	}
	return append([]NotificationProjection(nil), values[start:end]...), total, nil
}

func (s *MemoryStore) MarkNotificationRead(ctx context.Context, employeePublicID, publicID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.notifications[publicID]
	if !ok || value.EmployeePublicID != employeePublicID {
		return ErrNotFound
	}
	value.Status = "read"
	value.UpdatedAt = time.Now().UTC()
	s.notifications[publicID] = value
	return nil
}

func (s *MemoryStore) UpsertNotificationProjection(ctx context.Context, notification NotificationProjection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if notification.PublicID == "" || notification.EmployeePublicID == "" || notification.SyncVersion == 0 {
		return fmt.Errorf("notification public_id, employee_public_id and sync_version are required")
	}
	if notification.UpdatedAt.IsZero() {
		notification.UpdatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.notifications[notification.PublicID]; ok && previous.SyncVersion > notification.SyncVersion {
		return nil
	}
	s.notifications[notification.PublicID] = notification
	return nil
}

func normalizeNotificationPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
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
		left.ReceiptStatus == right.ReceiptStatus &&
		timesEqual(left.ReceivedAt, right.ReceivedAt) &&
		left.Message == right.Message &&
		left.SyncVersion == right.SyncVersion
}

func timesEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

func (s *MemoryStore) ListTaskProjections(ctx context.Context, employeePublicID string) ([]TaskProjection, error) {
	snapshot, err := s.ListTaskSnapshot(ctx, employeePublicID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return snapshot.Items, nil
}

func (s *MemoryStore) ListTaskHistory(ctx context.Context, filter TaskHistoryFilter) ([]TaskProjection, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]TaskProjection, 0)
	for _, value := range s.projections {
		if filter.EmployeePublicID != "" && value.EmployeePublicID != filter.EmployeePublicID {
			continue
		}
		status := value.BusinessStatus
		if status == "" {
			status = value.Status
		}
		if status != "completed" && status != "cancelled" {
			continue
		}
		if filter.Status != "" && filter.Status != "all" && status != filter.Status {
			continue
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].UpdatedAt.Equal(values[j].UpdatedAt) {
			return values[i].PublicID > values[j].PublicID
		}
		return values[i].UpdatedAt.After(values[j].UpdatedAt)
	})
	total := int64(len(values))
	page, pageSize := normalizeNotificationPagination(filter.Page, filter.PageSize)
	start := (page - 1) * pageSize
	if start >= len(values) {
		return []TaskProjection{}, total, nil
	}
	end := start + pageSize
	if end > len(values) {
		end = len(values)
	}
	return append([]TaskProjection(nil), values[start:end]...), total, nil
}

func (s *MemoryStore) ListTaskSnapshot(ctx context.Context, employeePublicID string, now time.Time) (TaskSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return TaskSnapshot{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]TaskProjection, 0)
	for _, value := range s.projections {
		if employeePublicID == "" || value.EmployeePublicID == employeePublicID {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].UpdatedAt.Equal(values[j].UpdatedAt) {
			return values[i].PublicID < values[j].PublicID
		}
		return values[i].UpdatedAt.Before(values[j].UpdatedAt)
	})
	latestApplied := s.latestAppliedEventAtLocked()
	snapshot := TaskSnapshot{Items: values, ProjectionRevision: s.projectionRevisions[employeePublicID], SnapshotAt: now.UTC()}
	if !latestApplied.IsZero() {
		snapshot.ProjectionLagKnown = true
		if now.After(latestApplied) {
			snapshot.ProjectionLag = now.Sub(latestApplied)
		}
	}
	return snapshot, nil
}

func (s *MemoryStore) SyncQueueStats(ctx context.Context, now time.Time) (SyncQueueStats, error) {
	if err := ctx.Err(); err != nil {
		return SyncQueueStats{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := SyncQueueStats{}
	var oldestCommand time.Time
	for _, value := range s.commands {
		switch value.Status {
		case sharedEvent.StatusPending, sharedEvent.StatusRetry, sharedEvent.StatusProcessing:
			stats.PendingCommandCount++
			if oldestCommand.IsZero() || value.CreatedAt.Before(oldestCommand) {
				oldestCommand = value.CreatedAt
			}
		}
	}
	if !oldestCommand.IsZero() && now.After(oldestCommand) {
		stats.OldestPendingCommandAge = now.Sub(oldestCommand)
	}
	var oldestInbox time.Time
	for _, value := range s.inbox {
		switch value.Status {
		case sharedEvent.StatusPending, sharedEvent.StatusProcessing, sharedEvent.StatusRetry:
			stats.PendingInboxCount++
			receivedAt := value.ReceivedAt
			if !receivedAt.IsZero() && (oldestInbox.IsZero() || receivedAt.Before(oldestInbox)) {
				oldestInbox = receivedAt
			}
		case sharedEvent.StatusFailed:
			stats.FailedInboxCount++
		}
	}
	if !oldestInbox.IsZero() && now.After(oldestInbox) {
		stats.OldestPendingInboxAge = now.Sub(oldestInbox)
	}
	latestApplied := s.latestAppliedEventAtLocked()
	if !latestApplied.IsZero() && now.After(latestApplied) {
		stats.ProjectionLag = now.Sub(latestApplied)
	}
	return stats, nil
}

func (s *MemoryStore) latestAppliedEventAtLocked() time.Time {
	var latestApplied time.Time
	for _, value := range s.inbox {
		if value.Status == sharedEvent.StatusApplied && value.Envelope.OccurredAt.After(latestApplied) {
			latestApplied = value.Envelope.OccurredAt
		}
	}
	return latestApplied
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

func (s *MemoryStore) FindCommand(ctx context.Context, commandID string) (CommandRecord, error) {
	if err := ctx.Err(); err != nil {
		return CommandRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.commands[commandID]
	if !ok {
		return CommandRecord{}, ErrNotFound
	}
	return value, nil
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
	value.UpdatedAt = time.Now().UTC()
	s.commands[commandID] = value
	return nil
}

func (s *MemoryStore) updateCommandWithLease(ctx context.Context, commandID, owner string, update func(*CommandRecord)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if owner == "" {
		return fmt.Errorf("command lease owner is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.commands[commandID]
	if !ok {
		return ErrNotFound
	}
	if value.Status != sharedEvent.StatusProcessing || value.LeaseOwner != owner {
		return ErrLeaseLost
	}
	update(&value)
	value.LeaseOwner, value.LeaseExpiresAt = "", time.Time{}
	value.UpdatedAt = time.Now().UTC()
	s.commands[commandID] = value
	return nil
}
