package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"gorm.io/gorm"
)

type SQLStore struct{ db *gorm.DB }

func NewSQLStore(db *gorm.DB) *SQLStore { return &SQLStore{db: db} }

func (s *SQLStore) RunTransaction(ctx context.Context, fn func(CoreTransaction) error) error {
	// 业务模块通过 CoreTransaction 写业务事实、Audit 和 Outbox；三者必须
	// 使用同一个 MySQL 事务，避免业务成功但同步消息丢失。
	if s == nil || s.db == nil || fn == nil {
		return fmt.Errorf("core sql transaction is not configured")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&sqlTransaction{db: tx})
	})
}

func (s *SQLStore) ClaimPendingOutbox(ctx context.Context, limit int, now time.Time) ([]OutboxRecord, error) {
	return s.ClaimPendingOutboxWithLease(ctx, limit, now, "legacy", time.Nanosecond)
}

func (s *SQLStore) ClaimPendingOutboxWithLease(ctx context.Context, limit int, now time.Time, owner string, leaseDuration time.Duration) ([]OutboxRecord, error) {
	// SKIP LOCKED 只保护领取事务本身；lease_owner/lease_expires_at 负责
	// 让多个 Worker 在事务结束后仍能区分所有权，并回收崩溃 Worker 的记录。
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("core sql store is not configured")
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
	if now.IsZero() {
		now = time.Now().UTC()
	}
	leaseExpiresAt := now.UTC().Add(leaseDuration)
	if owner == "legacy" {
		leaseExpiresAt = now.UTC()
	}
	var rows []outboxRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").Where("(status IN ? AND next_attempt_at <= ?) OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?))", []string{sharedEvent.StatusPending, sharedEvent.StatusRetry}, now.UTC(), sharedEvent.StatusProcessing, now.UTC()).Order("id ASC").Limit(limit).Find(&rows)
		if query.Error != nil {
			return query.Error
		}
		for i := range rows {
			if err := tx.Model(&outboxRow{}).Where("event_id = ?", rows[i].EventID).Updates(map[string]any{"status": sharedEvent.StatusProcessing, "lease_owner": owner, "lease_expires_at": leaseExpiresAt, "next_attempt_at": now.UTC(), "updated_at": now.UTC()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim core outbox: %w", err)
	}
	values := make([]OutboxRecord, 0, len(rows))
	for _, row := range rows {
		value, err := row.outboxRecord()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (s *SQLStore) OutboxStats(ctx context.Context, now time.Time) (OutboxStats, error) {
	if s == nil || s.db == nil {
		return OutboxStats{}, fmt.Errorf("core sql store is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var stats OutboxStats
	if err := s.db.WithContext(ctx).Model(&outboxRow{}).Where("status IN ?", []string{sharedEvent.StatusPending, sharedEvent.StatusRetry, sharedEvent.StatusProcessing}).Count(&stats.PendingCount).Error; err != nil {
		return OutboxStats{}, fmt.Errorf("count pending core outbox: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&outboxRow{}).Where("status = ?", sharedEvent.StatusFailed).Count(&stats.FailedCount).Error; err != nil {
		return OutboxStats{}, fmt.Errorf("count failed core outbox: %w", err)
	}
	var oldest outboxRow
	if err := s.db.WithContext(ctx).Select("created_at").Where("status IN ?", []string{sharedEvent.StatusPending, sharedEvent.StatusRetry, sharedEvent.StatusProcessing}).Order("created_at ASC").First(&oldest).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return OutboxStats{}, fmt.Errorf("find oldest core outbox: %w", err)
	}
	if !oldest.CreatedAt.IsZero() && now.After(oldest.CreatedAt) {
		stats.OldestPendingAge = now.Sub(oldest.CreatedAt)
	}
	return stats, nil
}

func (s *SQLStore) MarkOutboxSent(ctx context.Context, eventID string) error {
	return s.updateOutbox(ctx, eventID, map[string]any{"status": sharedEvent.StatusSent, "last_error": "", "lease_owner": nil, "lease_expires_at": nil})
}

func (s *SQLStore) MarkOutboxRetry(ctx context.Context, eventID string, next time.Time, reason string) error {
	return s.updateOutbox(ctx, eventID, map[string]any{"status": sharedEvent.StatusRetry, "next_attempt_at": next.UTC(), "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1), "lease_owner": nil, "lease_expires_at": nil})
}

func (s *SQLStore) MarkOutboxFailed(ctx context.Context, eventID string, reason string) error {
	return s.updateOutbox(ctx, eventID, map[string]any{"status": sharedEvent.StatusFailed, "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1), "lease_owner": nil, "lease_expires_at": nil})
}

func (s *SQLStore) MarkOutboxSentWithLease(ctx context.Context, eventID, owner string) error {
	return s.updateOutboxWithLease(ctx, eventID, owner, map[string]any{"status": sharedEvent.StatusSent, "last_error": ""})
}

func (s *SQLStore) MarkOutboxRetryWithLease(ctx context.Context, eventID, owner string, next time.Time, reason string) error {
	return s.updateOutboxWithLease(ctx, eventID, owner, map[string]any{"status": sharedEvent.StatusRetry, "next_attempt_at": next.UTC(), "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1)})
}

func (s *SQLStore) MarkOutboxFailedWithLease(ctx context.Context, eventID, owner string, reason string) error {
	return s.updateOutboxWithLease(ctx, eventID, owner, map[string]any{"status": sharedEvent.StatusFailed, "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1)})
}

func (s *SQLStore) ProcessCommand(ctx context.Context, command sharedEvent.CommandEnvelope, execute CommandExecution) (bool, error) {
	// Command 的接收记录、业务处理结果、Audit 和新 Outbox 都在同一个
	// Core 事务中完成；重复或短时间 processing 的 command_id 直接幂等返回。
	if err := command.Validate(); err != nil {
		return false, err
	}
	if s == nil || s.db == nil || execute == nil {
		return false, fmt.Errorf("core command processor is not configured")
	}
	var duplicate bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing coreInboxRow
		findErr := tx.Where("command_id = ?", command.CommandID).First(&existing).Error
		if findErr == nil {
			if !sameCoreInboxCommand(existing, command) {
				return CommandIDConflictError{}
			}
			now := time.Now().UTC()
			if existing.Status == sharedEvent.StatusApplied || (existing.Status == sharedEvent.StatusProcessing && time.Since(existing.ReceivedAt) < time.Minute) {
				duplicate = true
				return nil
			}
			if err := tx.Model(&coreInboxRow{}).Where("command_id = ?", command.CommandID).Updates(map[string]any{"status": sharedEvent.StatusProcessing, "error_message": nil, "processed_at": nil, "received_at": now}).Error; err != nil {
				return err
			}
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			row := newCoreInboxRow(command)
			if err := tx.Create(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrDuplicatedKey) {
					duplicate = true
					return nil
				}
				return err
			}
		}
		if err := execute(ctx, &sqlTransaction{db: tx}, command); err != nil {
			return err
		}
		return tx.Model(&coreInboxRow{}).Where("command_id = ?", command.CommandID).Updates(map[string]any{"status": sharedEvent.StatusApplied, "processed_at": time.Now().UTC()}).Error
	})
	if err != nil {
		// The business transaction is rolled back, so persist the failure in a
		// separate transaction. A later delivery can retry a failed command.
		if recordErr := s.recordFailedCommand(ctx, command, err); recordErr != nil {
			return duplicate, fmt.Errorf("%w (record failed command: %v)", err, recordErr)
		}
		return duplicate, err
	}
	return duplicate, nil
}

func sameCoreInboxCommand(row coreInboxRow, command sharedEvent.CommandEnvelope) bool {
	return sharedEvent.EquivalentCommand(sharedEvent.CommandEnvelope{CommandID: row.CommandID, CommandType: row.CommandType, SchemaVersion: row.SchemaVersion, ActorPublicID: row.ActorPublicID, AggregateID: row.AggregateID, Payload: row.Payload}, command)
}

func (s *SQLStore) recordFailedCommand(ctx context.Context, command sharedEvent.CommandEnvelope, cause error) error {
	// 业务事务失败时，core_inbox 也会随事务回滚，所以必须在独立事务中
	// 保存 failed 记录，之后才能依据错误和重试策略再次领取。
	message := cause.Error()
	row := newCoreInboxRow(command)
	row.Status = sharedEvent.StatusFailed
	row.ErrorMessage = message
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
		return err
	}
	result := s.db.WithContext(ctx).Model(&coreInboxRow{}).
		Where("command_id = ? AND status IN ?", command.CommandID, []string{sharedEvent.StatusPending, sharedEvent.StatusProcessing}).
		Updates(map[string]any{"status": sharedEvent.StatusFailed, "error_message": message, "processed_at": nil})
	return result.Error
}

func (s *SQLStore) updateOutbox(ctx context.Context, eventID string, values map[string]any) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("core sql store is not configured")
	}
	result := s.db.WithContext(ctx).Model(&outboxRow{}).Where("event_id = ?", eventID).Updates(values)
	if result.Error != nil {
		return fmt.Errorf("update outbox %s: %w", eventID, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) updateOutboxWithLease(ctx context.Context, eventID, owner string, values map[string]any) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("core sql store is not configured")
	}
	if owner == "" {
		return fmt.Errorf("outbox lease owner is required")
	}
	values["lease_owner"] = nil
	values["lease_expires_at"] = nil
	result := s.db.WithContext(ctx).Model(&outboxRow{}).Where("event_id = ? AND status = ? AND lease_owner = ?", eventID, sharedEvent.StatusProcessing, owner).Updates(values)
	if result.Error != nil {
		return fmt.Errorf("update leased outbox %s: %w", eventID, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrLeaseLost
	}
	return nil
}

type sqlTransaction struct{ db *gorm.DB }

func (tx *sqlTransaction) CreateProbeEvent(ctx context.Context, value ProbeEvent) error {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	row := probeEventRow{PublicID: value.PublicID, EventType: value.EventType, Payload: value.Payload, CreatedAt: value.CreatedAt}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

func (tx *sqlTransaction) AppendAudit(ctx context.Context, value AuditRecord) error {
	if value.OccurredAt.IsZero() {
		value.OccurredAt = time.Now().UTC()
	}
	row := auditRow{ActorType: value.ActorType, ActorID: value.ActorID, Action: value.Action, ResourceType: value.ResourceType, ResourceID: value.ResourceID, Result: value.Result, RequestID: value.RequestID, TraceID: value.TraceID, SourceIP: value.SourceIP, OccurredAt: value.OccurredAt}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}

func (tx *sqlTransaction) AppendOutbox(ctx context.Context, envelope sharedEvent.EventEnvelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	row, err := newOutboxRow(envelope)
	if err != nil {
		return err
	}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

type auditRow struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	ActorType    string    `gorm:"column:actor_type"`
	ActorID      string    `gorm:"column:actor_id"`
	Action       string    `gorm:"column:action"`
	ResourceType string    `gorm:"column:resource_type"`
	ResourceID   string    `gorm:"column:resource_id"`
	Result       string    `gorm:"column:result"`
	RequestID    string    `gorm:"column:request_id"`
	TraceID      string    `gorm:"column:trace_id"`
	SourceIP     string    `gorm:"column:source_ip"`
	OccurredAt   time.Time `gorm:"column:occurred_at"`
}

func (auditRow) TableName() string { return "audit_log" }

type probeEventRow struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID  string    `gorm:"column:public_id"`
	EventType string    `gorm:"column:event_type"`
	Payload   []byte    `gorm:"column:payload"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (probeEventRow) TableName() string { return "architecture_probe_event" }

type outboxRow struct {
	ID             uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	EventID        string     `gorm:"column:event_id"`
	EventType      string     `gorm:"column:event_type"`
	SchemaVersion  int        `gorm:"column:schema_version"`
	AggregateType  string     `gorm:"column:aggregate_type"`
	AggregateID    string     `gorm:"column:aggregate_id"`
	OccurredAt     time.Time  `gorm:"column:occurred_at"`
	Producer       string     `gorm:"column:producer"`
	CorrelationID  string     `gorm:"column:correlation_id"`
	TraceID        string     `gorm:"column:trace_id"`
	Payload        []byte     `gorm:"column:payload"`
	Status         string     `gorm:"column:status"`
	Attempts       int        `gorm:"column:attempts"`
	NextAttemptAt  time.Time  `gorm:"column:next_attempt_at"`
	LeaseOwner     *string    `gorm:"column:lease_owner"`
	LeaseExpiresAt *time.Time `gorm:"column:lease_expires_at"`
	LastError      *string    `gorm:"column:last_error"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (outboxRow) TableName() string { return "outbox_event" }

func newOutboxRow(envelope sharedEvent.EventEnvelope) (outboxRow, error) {
	payload, err := json.Marshal(envelope.Payload)
	if err != nil {
		return outboxRow{}, fmt.Errorf("marshal outbox payload: %w", err)
	}
	now := time.Now().UTC()
	return outboxRow{EventID: envelope.EventID, EventType: envelope.EventType, SchemaVersion: envelope.SchemaVersion, AggregateType: envelope.AggregateType, AggregateID: envelope.AggregateID, OccurredAt: envelope.OccurredAt.UTC(), Producer: envelope.Producer, CorrelationID: envelope.CorrelationID, TraceID: envelope.TraceID, Payload: payload, Status: sharedEvent.StatusPending, NextAttemptAt: now, CreatedAt: now}, nil
}

func (row outboxRow) outboxRecord() (OutboxRecord, error) {
	lastError := ""
	if row.LastError != nil {
		lastError = *row.LastError
	}
	leaseOwner := ""
	if row.LeaseOwner != nil {
		leaseOwner = *row.LeaseOwner
	}
	leaseExpiresAt := time.Time{}
	if row.LeaseExpiresAt != nil {
		leaseExpiresAt = row.LeaseExpiresAt.UTC()
	}
	return OutboxRecord{ID: row.ID, Envelope: sharedEvent.EventEnvelope{EventID: row.EventID, EventType: row.EventType, SchemaVersion: row.SchemaVersion, AggregateType: row.AggregateType, AggregateID: row.AggregateID, OccurredAt: row.OccurredAt.UTC(), Producer: row.Producer, CorrelationID: row.CorrelationID, TraceID: row.TraceID, Payload: append([]byte(nil), row.Payload...)}, Status: row.Status, Attempts: row.Attempts, NextAttempt: row.NextAttemptAt.UTC(), LastError: lastError, CreatedAt: row.CreatedAt.UTC(), LeaseOwner: leaseOwner, LeaseExpiresAt: leaseExpiresAt}, nil
}

type coreInboxRow struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	CommandID     string     `gorm:"column:command_id"`
	CommandType   string     `gorm:"column:command_type"`
	SchemaVersion int        `gorm:"column:schema_version"`
	ActorPublicID string     `gorm:"column:actor_public_id"`
	AggregateID   string     `gorm:"column:aggregate_id"`
	OccurredAt    time.Time  `gorm:"column:occurred_at"`
	TraceID       string     `gorm:"column:trace_id"`
	Payload       []byte     `gorm:"column:payload"`
	Status        string     `gorm:"column:status"`
	ErrorMessage  string     `gorm:"column:error_message"`
	ReceivedAt    time.Time  `gorm:"column:received_at"`
	ProcessedAt   *time.Time `gorm:"column:processed_at"`
}

func (coreInboxRow) TableName() string { return "core_inbox" }

func newCoreInboxRow(command sharedEvent.CommandEnvelope) coreInboxRow {
	return coreInboxRow{CommandID: command.CommandID, CommandType: command.CommandType, SchemaVersion: command.SchemaVersion, ActorPublicID: command.ActorPublicID, AggregateID: command.AggregateID, OccurredAt: command.OccurredAt.UTC().Round(time.Microsecond), TraceID: command.TraceID, Payload: append([]byte(nil), command.Payload...), Status: sharedEvent.StatusProcessing, ReceivedAt: time.Now().UTC()}
}

func mapDatabaseError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicate
	}
	return err
}
