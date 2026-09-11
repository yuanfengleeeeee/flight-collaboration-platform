// Package sync contains Core-side durable synchronization contracts and a
// memory implementation used by the Architecture Probe.
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
	ErrDuplicate         = errors.New("duplicate synchronization record")
	ErrNotFound          = errors.New("synchronization record not found")
	ErrLeaseLost         = errors.New("synchronization lease is no longer owned")
	ErrCommandIDConflict = errors.New("command id was reused with different content")
)

type CommandIDConflictError struct{}

func (CommandIDConflictError) Error() string  { return ErrCommandIDConflict.Error() }
func (CommandIDConflictError) Unwrap() error  { return ErrCommandIDConflict }
func (CommandIDConflictError) Terminal() bool { return true }

type AuditRecord struct {
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	RequestID    string
	TraceID      string
	SourceIP     string
	OccurredAt   time.Time
}

type ProbeEvent struct {
	PublicID  string
	EventType string
	Payload   []byte
	CreatedAt time.Time
}

type OutboxRecord struct {
	ID             uint64
	Envelope       sharedEvent.EventEnvelope
	Status         string
	Attempts       int
	NextAttempt    time.Time
	LastError      string
	CreatedAt      time.Time
	LeaseOwner     string    `json:"-"`
	LeaseExpiresAt time.Time `json:"-"`
}

type OutboxStats struct {
	PendingCount     int64
	FailedCount      int64
	OldestPendingAge time.Duration
}

type OutboxStatsProvider interface {
	OutboxStats(ctx context.Context, now time.Time) (OutboxStats, error)
}

type CoreTransaction interface {
	CreateProbeEvent(ctx context.Context, value ProbeEvent) error
	AppendAudit(ctx context.Context, value AuditRecord) error
	AppendOutbox(ctx context.Context, envelope sharedEvent.EventEnvelope) error
}

type CommandExecution func(context.Context, CoreTransaction, sharedEvent.CommandEnvelope) error

type Store interface {
	RunTransaction(ctx context.Context, fn func(CoreTransaction) error) error
	ClaimPendingOutbox(ctx context.Context, limit int, now time.Time) ([]OutboxRecord, error)
	MarkOutboxSent(ctx context.Context, eventID string) error
	MarkOutboxRetry(ctx context.Context, eventID string, next time.Time, reason string) error
	MarkOutboxFailed(ctx context.Context, eventID string, reason string) error
	ProcessCommand(ctx context.Context, command sharedEvent.CommandEnvelope, execute CommandExecution) (duplicate bool, err error)
}

// LeasedStore is implemented by durable stores that coordinate multiple
// workers with a database-backed lease. Store remains source-compatible with
// the architecture probe and small test doubles; production workers prefer
// this interface whenever it is available.
type LeasedStore interface {
	ClaimPendingOutboxWithLease(ctx context.Context, limit int, now time.Time, owner string, leaseDuration time.Duration) ([]OutboxRecord, error)
	MarkOutboxSentWithLease(ctx context.Context, eventID, owner string) error
	MarkOutboxRetryWithLease(ctx context.Context, eventID, owner string, next time.Time, reason string) error
	MarkOutboxFailedWithLease(ctx context.Context, eventID, owner string, reason string) error
}

type MemoryStore struct {
	mu     sync.Mutex
	nextID uint64
	probe  map[string]ProbeEvent
	audits []AuditRecord
	outbox map[string]OutboxRecord
	inbox  map[string]commandInboxRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{probe: make(map[string]ProbeEvent), outbox: make(map[string]OutboxRecord), inbox: make(map[string]commandInboxRecord)}
}

func (s *MemoryStore) RunTransaction(ctx context.Context, fn func(CoreTransaction) error) error {
	if s == nil || fn == nil {
		return fmt.Errorf("core memory transaction is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	probeSnapshot := cloneProbe(s.probe)
	auditSnapshot := append([]AuditRecord(nil), s.audits...)
	outboxSnapshot := cloneOutbox(s.outbox)
	tx := &memoryTransaction{store: s}
	if err := fn(tx); err != nil {
		s.probe, s.audits, s.outbox = probeSnapshot, auditSnapshot, outboxSnapshot
		return err
	}
	return nil
}

func (s *MemoryStore) ClaimPendingOutbox(ctx context.Context, limit int, now time.Time) ([]OutboxRecord, error) {
	return s.ClaimPendingOutboxWithLease(ctx, limit, now, "legacy", time.Nanosecond)
}

func (s *MemoryStore) ClaimPendingOutboxWithLease(ctx context.Context, limit int, now time.Time, owner string, leaseDuration time.Duration) ([]OutboxRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if owner == "" {
		return nil, fmt.Errorf("outbox lease owner is required")
	}
	if leaseDuration <= 0 {
		leaseDuration = time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]OutboxRecord, 0, len(s.outbox))
	for _, value := range s.outbox {
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
		value.LeaseOwner = owner
		value.LeaseExpiresAt = now.UTC().Add(leaseDuration)
		if owner == "legacy" {
			value.LeaseExpiresAt = now.UTC()
		}
		s.outbox[value.Envelope.EventID] = value
		values[index] = value
	}
	return values, nil
}

func (s *MemoryStore) MarkOutboxSent(ctx context.Context, eventID string) error {
	return s.updateOutbox(ctx, eventID, func(value *OutboxRecord) {
		value.Status, value.LastError = sharedEvent.StatusSent, ""
		value.LeaseOwner, value.LeaseExpiresAt = "", time.Time{}
	})
}

func (s *MemoryStore) MarkOutboxRetry(ctx context.Context, eventID string, next time.Time, reason string) error {
	return s.updateOutbox(ctx, eventID, func(value *OutboxRecord) {
		value.Status = sharedEvent.StatusRetry
		value.NextAttempt = next.UTC()
		value.Attempts++
		value.LastError = reason
		value.LeaseOwner, value.LeaseExpiresAt = "", time.Time{}
	})
}

func (s *MemoryStore) MarkOutboxFailed(ctx context.Context, eventID string, reason string) error {
	return s.updateOutbox(ctx, eventID, func(value *OutboxRecord) {
		value.Status = sharedEvent.StatusFailed
		value.Attempts++
		value.LastError = reason
		value.LeaseOwner, value.LeaseExpiresAt = "", time.Time{}
	})
}

func (s *MemoryStore) MarkOutboxSentWithLease(ctx context.Context, eventID, owner string) error {
	return s.updateOutboxWithLease(ctx, eventID, owner, func(value *OutboxRecord) {
		value.Status, value.LastError = sharedEvent.StatusSent, ""
	})
}

func (s *MemoryStore) MarkOutboxRetryWithLease(ctx context.Context, eventID, owner string, next time.Time, reason string) error {
	return s.updateOutboxWithLease(ctx, eventID, owner, func(value *OutboxRecord) {
		value.Status, value.NextAttempt, value.Attempts, value.LastError = sharedEvent.StatusRetry, next.UTC(), value.Attempts+1, reason
	})
}

func (s *MemoryStore) MarkOutboxFailedWithLease(ctx context.Context, eventID, owner string, reason string) error {
	return s.updateOutboxWithLease(ctx, eventID, owner, func(value *OutboxRecord) {
		value.Status, value.Attempts, value.LastError = sharedEvent.StatusFailed, value.Attempts+1, reason
	})
}

func (s *MemoryStore) ProcessCommand(ctx context.Context, command sharedEvent.CommandEnvelope, execute CommandExecution) (bool, error) {
	if err := command.Validate(); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.inbox[command.CommandID]; exists {
		if !sameCommandEnvelope(existing.Envelope, command) {
			return false, CommandIDConflictError{}
		}
		if existing.Status != sharedEvent.StatusFailed {
			return true, nil
		}
	}
	probeSnapshot := cloneProbe(s.probe)
	auditSnapshot := append([]AuditRecord(nil), s.audits...)
	outboxSnapshot := cloneOutbox(s.outbox)
	s.inbox[command.CommandID] = commandInboxRecord{Envelope: command, Status: sharedEvent.StatusProcessing}
	tx := &memoryTransaction{store: s}
	if err := execute(ctx, tx, command); err != nil {
		s.probe, s.audits, s.outbox = probeSnapshot, auditSnapshot, outboxSnapshot
		s.inbox[command.CommandID] = commandInboxRecord{Envelope: command, Status: sharedEvent.StatusFailed}
		return false, err
	}
	s.inbox[command.CommandID] = commandInboxRecord{Envelope: command, Status: sharedEvent.StatusApplied}
	return false, nil
}

type commandInboxRecord struct {
	Envelope sharedEvent.CommandEnvelope
	Status   string
}

func sameCommandEnvelope(left, right sharedEvent.CommandEnvelope) bool {
	return sharedEvent.EquivalentCommand(left, right)
}

func (s *MemoryStore) PendingOutbox() []OutboxRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]OutboxRecord, 0, len(s.outbox))
	for _, value := range s.outbox {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values
}

func (s *MemoryStore) OutboxStats(ctx context.Context, now time.Time) (OutboxStats, error) {
	if err := ctx.Err(); err != nil {
		return OutboxStats{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := OutboxStats{}
	var oldest time.Time
	for _, value := range s.outbox {
		switch value.Status {
		case sharedEvent.StatusPending, sharedEvent.StatusRetry, sharedEvent.StatusProcessing:
			stats.PendingCount++
			if oldest.IsZero() || value.CreatedAt.Before(oldest) {
				oldest = value.CreatedAt
			}
		case sharedEvent.StatusFailed:
			stats.FailedCount++
		}
	}
	if !oldest.IsZero() && now.After(oldest) {
		stats.OldestPendingAge = now.Sub(oldest)
	}
	return stats, nil
}

func (s *MemoryStore) ProbeEvents() []ProbeEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]ProbeEvent, 0, len(s.probe))
	for _, value := range s.probe {
		values = append(values, value)
	}
	return values
}

func (s *MemoryStore) AuditRecords() []AuditRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AuditRecord(nil), s.audits...)
}

func (s *MemoryStore) updateOutbox(ctx context.Context, eventID string, update func(*OutboxRecord)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.outbox[eventID]
	if !ok {
		return ErrNotFound
	}
	update(&value)
	s.outbox[eventID] = value
	return nil
}

func (s *MemoryStore) updateOutboxWithLease(ctx context.Context, eventID, owner string, update func(*OutboxRecord)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if owner == "" {
		return fmt.Errorf("outbox lease owner is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.outbox[eventID]
	if !ok {
		return ErrNotFound
	}
	if value.Status != sharedEvent.StatusProcessing || value.LeaseOwner != owner {
		return ErrLeaseLost
	}
	update(&value)
	value.LeaseOwner, value.LeaseExpiresAt = "", time.Time{}
	s.outbox[eventID] = value
	return nil
}

type memoryTransaction struct{ store *MemoryStore }

func (tx *memoryTransaction) CreateProbeEvent(ctx context.Context, value ProbeEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if value.PublicID == "" || value.EventType == "" || len(value.Payload) == 0 {
		return fmt.Errorf("probe event fields are required")
	}
	if _, exists := tx.store.probe[value.PublicID]; exists {
		return ErrDuplicate
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	tx.store.probe[value.PublicID] = value
	return nil
}

func (tx *memoryTransaction) AppendAudit(ctx context.Context, value AuditRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if value.Action == "" || value.ResourceType == "" || value.Result == "" {
		return fmt.Errorf("audit action, resource_type and result are required")
	}
	if value.OccurredAt.IsZero() {
		value.OccurredAt = time.Now().UTC()
	}
	tx.store.audits = append(tx.store.audits, value)
	return nil
}

func (tx *memoryTransaction) AppendOutbox(ctx context.Context, envelope sharedEvent.EventEnvelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	if _, exists := tx.store.outbox[envelope.EventID]; exists {
		return ErrDuplicate
	}
	tx.store.nextID++
	now := time.Now().UTC()
	tx.store.outbox[envelope.EventID] = OutboxRecord{ID: tx.store.nextID, Envelope: envelope, Status: sharedEvent.StatusPending, NextAttempt: now, CreatedAt: now}
	return nil
}

func cloneProbe(source map[string]ProbeEvent) map[string]ProbeEvent {
	result := make(map[string]ProbeEvent, len(source))
	for key, value := range source {
		value.Payload = append([]byte(nil), value.Payload...)
		result[key] = value
	}
	return result
}

func cloneOutbox(source map[string]OutboxRecord) map[string]OutboxRecord {
	result := make(map[string]OutboxRecord, len(source))
	for key, value := range source {
		value.Envelope.Payload = append([]byte(nil), value.Envelope.Payload...)
		result[key] = value
	}
	return result
}
