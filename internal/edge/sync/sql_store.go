package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"gorm.io/gorm"
)

type transactionContextKey struct{}

type SQLStore struct{ db *gorm.DB }

func NewSQLStore(db *gorm.DB) *SQLStore { return &SQLStore{db: db} }

func (s *SQLStore) ApplyEvent(ctx context.Context, envelope sharedEvent.EventEnvelope, project EventProjection) (bool, error) {
	// Edge 先以 event_id 写 Inbox，再在同一个 Edge DB 事务里更新 Projection。
	// 这样重复 Event 不会重复产生最终投影效果。
	if err := envelope.Validate(); err != nil {
		return false, err
	}
	if s == nil || s.db == nil {
		return false, fmt.Errorf("edge sql store is not configured")
	}
	duplicate := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing syncInboxRow
		findErr := tx.Where("event_id = ?", envelope.EventID).First(&existing).Error
		if findErr == nil {
			now := time.Now().UTC()
			if existing.Status == sharedEvent.StatusApplied || (existing.Status == sharedEvent.StatusProcessing && time.Since(existing.ReceivedAt) < time.Minute) {
				duplicate = true
				return nil
			}
			if err := tx.Model(&syncInboxRow{}).Where("event_id = ?", envelope.EventID).Updates(map[string]any{"status": sharedEvent.StatusProcessing, "error_message": nil, "applied_at": nil, "received_at": now}).Error; err != nil {
				return err
			}
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			if findErr != nil {
				return findErr
			}
		} else {
			row := newSyncInboxRow(envelope)
			if err := tx.Create(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrDuplicatedKey) {
					duplicate = true
					return nil
				}
				return err
			}
		}
		if project != nil {
			projectCtx := context.WithValue(ctx, transactionContextKey{}, tx)
			if err := project(projectCtx, envelope); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		return tx.Model(&syncInboxRow{}).Where("event_id = ?", envelope.EventID).Updates(map[string]any{"status": sharedEvent.StatusApplied, "applied_at": now}).Error
	})
	if err != nil {
		if recordErr := s.recordFailedEvent(ctx, envelope, err); recordErr != nil {
			return duplicate, fmt.Errorf("%w (record failed event: %v)", err, recordErr)
		}
		return duplicate, err
	}
	return duplicate, nil
}

func (s *SQLStore) recordFailedEvent(ctx context.Context, envelope sharedEvent.EventEnvelope, cause error) error {
	// Projection 事务失败后，单独保留 failed Inbox，避免失败原因随着回滚丢失。
	message := cause.Error()
	row := newSyncInboxRow(envelope)
	row.Status = sharedEvent.StatusFailed
	row.ErrorMessage = &message
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
		return err
	}
	result := s.db.WithContext(ctx).Model(&syncInboxRow{}).
		Where("event_id = ? AND status IN ?", envelope.EventID, []string{sharedEvent.StatusPending, sharedEvent.StatusProcessing}).
		Updates(map[string]any{"status": sharedEvent.StatusFailed, "error_message": message, "applied_at": nil})
	return result.Error
}

func (s *SQLStore) PutCommand(ctx context.Context, command sharedEvent.CommandEnvelope) (bool, error) {
	// 移动端 Command 先落 Edge DB，再由 Core Worker 拉取；这里不直接修改
	// Core 业务事实。
	if err := command.Validate(); err != nil {
		return false, err
	}
	if s == nil || s.db == nil {
		return false, fmt.Errorf("edge sql store is not configured")
	}
	var existing mobileCommandRow
	findErr := s.db.WithContext(ctx).Where("command_id = ?", command.CommandID).First(&existing).Error
	if findErr == nil {
		if !sameCommand(existing.commandRecord().Envelope, command) {
			return false, ErrCommandIDConflict
		}
		return true, nil
	}
	if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("find existing mobile command: %w", findErr)
	}
	row := newMobileCommandRow(command)
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			if findErr := s.db.WithContext(ctx).Where("command_id = ?", command.CommandID).First(&existing).Error; findErr != nil {
				return false, fmt.Errorf("read concurrently stored mobile command: %w", findErr)
			}
			if !sameCommand(existing.commandRecord().Envelope, command) {
				return false, ErrCommandIDConflict
			}
			return true, nil
		}
		return false, fmt.Errorf("store mobile command: %w", err)
	}
	return false, nil
}

func (s *SQLStore) FindCommand(ctx context.Context, commandID string) (CommandRecord, error) {
	if s == nil || s.db == nil {
		return CommandRecord{}, fmt.Errorf("edge sql store is not configured")
	}
	var row mobileCommandRow
	if err := s.db.WithContext(ctx).Where("command_id = ?", commandID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CommandRecord{}, ErrNotFound
		}
		return CommandRecord{}, fmt.Errorf("find mobile command: %w", err)
	}
	return row.commandRecord(), nil
}

func (s *SQLStore) ClaimPendingCommands(ctx context.Context, limit int, now time.Time) ([]CommandRecord, error) {
	return s.ClaimPendingCommandsWithLease(ctx, limit, now, "legacy", time.Nanosecond)
}

func (s *SQLStore) ClaimPendingCommandsWithLease(ctx context.Context, limit int, now time.Time, owner string, leaseDuration time.Duration) ([]CommandRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("edge sql store is not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	if owner == "" {
		return nil, fmt.Errorf("command lease owner is required")
	}
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("command lease duration must be positive")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	leaseExpiresAt := now.Add(leaseDuration)
	if owner == "legacy" {
		leaseExpiresAt = now
	}
	var rows []mobileCommandRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		readyStatuses := []string{sharedEvent.StatusPending, sharedEvent.StatusRetry}
		query := tx.Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").Where(
			"(status IN ? AND next_attempt_at <= ?) OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?))",
			readyStatuses, now, sharedEvent.StatusProcessing, now,
		).Order("id ASC").Limit(limit)
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Model(&mobileCommandRow{}).Where("command_id = ?", row.CommandID).Updates(map[string]any{
				"status":           sharedEvent.StatusProcessing,
				"lease_owner":      owner,
				"lease_expires_at": leaseExpiresAt,
				"next_attempt_at":  now,
				"updated_at":       now,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim edge commands: %w", err)
	}
	values := make([]CommandRecord, 0, len(rows))
	for _, row := range rows {
		leaseOwner := owner
		leaseExpiry := leaseExpiresAt
		row.LeaseOwner = &leaseOwner
		row.LeaseExpiresAt = &leaseExpiry
		row.Status = sharedEvent.StatusProcessing
		values = append(values, row.commandRecord())
	}
	return values, nil
}

func (s *SQLStore) claimPendingCommandsLegacy(ctx context.Context, limit int, now time.Time) ([]CommandRecord, error) {
	// Core Worker 主动拉取 pending/retry/过期 processing Command。行锁和
	// SKIP LOCKED 防止多个 Worker 同时处理同一批记录。
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("edge sql store is not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	var rows []mobileCommandRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").Where("status IN ? AND next_attempt_at <= ?", []string{sharedEvent.StatusPending, sharedEvent.StatusRetry, sharedEvent.StatusProcessing}, now.UTC()).Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Model(&mobileCommandRow{}).Where("command_id = ?", row.CommandID).Updates(map[string]any{"status": sharedEvent.StatusProcessing, "updated_at": now.UTC()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim edge commands: %w", err)
	}
	values := make([]CommandRecord, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.commandRecord())
	}
	return values, nil
}

func (s *SQLStore) SyncQueueStats(ctx context.Context, now time.Time) (SyncQueueStats, error) {
	if s == nil || s.db == nil {
		return SyncQueueStats{}, fmt.Errorf("edge sql store is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var stats SyncQueueStats
	if err := s.db.WithContext(ctx).Model(&mobileCommandRow{}).Where("status IN ?", []string{sharedEvent.StatusPending, sharedEvent.StatusRetry, sharedEvent.StatusProcessing}).Count(&stats.PendingCommandCount).Error; err != nil {
		return SyncQueueStats{}, fmt.Errorf("count pending edge commands: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&syncInboxRow{}).Where("status IN ?", []string{sharedEvent.StatusPending, sharedEvent.StatusProcessing, sharedEvent.StatusRetry}).Count(&stats.PendingInboxCount).Error; err != nil {
		return SyncQueueStats{}, fmt.Errorf("count pending edge inbox: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&syncInboxRow{}).Where("status = ?", sharedEvent.StatusFailed).Count(&stats.FailedInboxCount).Error; err != nil {
		return SyncQueueStats{}, fmt.Errorf("count failed edge inbox: %w", err)
	}
	var oldest mobileCommandRow
	if err := s.db.WithContext(ctx).Select("created_at").Where("status IN ?", []string{sharedEvent.StatusPending, sharedEvent.StatusRetry, sharedEvent.StatusProcessing}).Order("created_at ASC").First(&oldest).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return SyncQueueStats{}, fmt.Errorf("find oldest edge command: %w", err)
	}
	if !oldest.CreatedAt.IsZero() && now.After(oldest.CreatedAt) {
		stats.OldestPendingCommandAge = now.Sub(oldest.CreatedAt)
	}
	var oldestInbox syncInboxRow
	if err := s.db.WithContext(ctx).Select("received_at").Where("status IN ?", []string{sharedEvent.StatusPending, sharedEvent.StatusProcessing, sharedEvent.StatusRetry}).Order("received_at ASC").First(&oldestInbox).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return SyncQueueStats{}, fmt.Errorf("find oldest edge inbox: %w", err)
	}
	if !oldestInbox.ReceivedAt.IsZero() && now.After(oldestInbox.ReceivedAt) {
		stats.OldestPendingInboxAge = now.Sub(oldestInbox.ReceivedAt)
	}
	var latestApplied syncInboxRow
	if err := s.db.WithContext(ctx).Select("occurred_at").Where("status = ?", sharedEvent.StatusApplied).Order("occurred_at DESC").First(&latestApplied).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return SyncQueueStats{}, fmt.Errorf("find latest applied edge event: %w", err)
	}
	if !latestApplied.OccurredAt.IsZero() && now.After(latestApplied.OccurredAt) {
		stats.ProjectionLag = now.Sub(latestApplied.OccurredAt)
	}
	return stats, nil
}

func (s *SQLStore) MarkCommandSent(ctx context.Context, commandID string) error {
	return s.updateCommand(ctx, commandID, map[string]any{"status": sharedEvent.StatusSent, "last_error": "", "lease_owner": nil, "lease_expires_at": nil})
}

func (s *SQLStore) MarkCommandRetry(ctx context.Context, commandID string, next time.Time, reason string) error {
	return s.updateCommand(ctx, commandID, map[string]any{"status": sharedEvent.StatusRetry, "next_attempt_at": next.UTC(), "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1), "lease_owner": nil, "lease_expires_at": nil})
}

func (s *SQLStore) MarkCommandFailed(ctx context.Context, commandID string, reason string) error {
	return s.updateCommand(ctx, commandID, map[string]any{"status": sharedEvent.StatusFailed, "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1), "lease_owner": nil, "lease_expires_at": nil})
}

func (s *SQLStore) MarkCommandSentWithLease(ctx context.Context, commandID, owner string) error {
	return s.updateCommandWithLease(ctx, commandID, owner, map[string]any{"status": sharedEvent.StatusSent, "last_error": ""})
}

func (s *SQLStore) MarkCommandRetryWithLease(ctx context.Context, commandID, owner string, next time.Time, reason string) error {
	return s.updateCommandWithLease(ctx, commandID, owner, map[string]any{"status": sharedEvent.StatusRetry, "next_attempt_at": next.UTC(), "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1)})
}

func (s *SQLStore) MarkCommandFailedWithLease(ctx context.Context, commandID, owner, reason string) error {
	return s.updateCommandWithLease(ctx, commandID, owner, map[string]any{"status": sharedEvent.StatusFailed, "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1)})
}

func (s *SQLStore) UpsertTaskProjection(ctx context.Context, projection TaskProjection) error {
	// Projection 是可重放的读取模型，只接受不低于当前 sync_version 的数据；
	// 如果调用来自 Inbox 事务，则复用该事务。
	if s == nil || s.db == nil {
		return fmt.Errorf("edge sql store is not configured")
	}
	projection, err := normalizeTaskProjection(projection)
	if err != nil {
		return err
	}
	if projection.PublicID == "" || projection.EmployeePublicID == "" || projection.SyncVersion == 0 {
		return fmt.Errorf("task projection public_id, employee_public_id and sync_version are required")
	}
	if tx, ok := ctx.Value(transactionContextKey{}).(*gorm.DB); ok {
		return s.upsertTaskProjection(ctx, tx.WithContext(ctx), projection)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.upsertTaskProjection(ctx, tx.WithContext(ctx), projection)
	})
}

func (s *SQLStore) upsertTaskProjection(ctx context.Context, db *gorm.DB, projection TaskProjection) error {
	var existing taskProjectionRow
	findErr := db.Where("public_id = ?", projection.PublicID).First(&existing).Error
	if findErr == nil {
		if existing.SyncVersion > projection.SyncVersion {
			return nil
		}
		if existing.SyncVersion == projection.SyncVersion {
			current := existing.projection()
			if !sameTaskProjection(current, projection) {
				return ErrProjectionVersionConflict
			}
			return nil
		}
	}
	row := taskProjectionRow{PublicID: projection.PublicID, AssignmentPublicID: projection.AssignmentPublicID, EmployeePublicID: projection.EmployeePublicID, FlightDisplayNo: projection.FlightDisplayNo, TaskName: projection.TaskName, AreaName: projection.AreaName, PlannedAt: projection.PlannedAt.UTC(), Status: projection.Status, Message: projection.Message, SyncVersion: projection.SyncVersion, UpdatedAt: projection.UpdatedAt.UTC()}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		if err := db.Create(&row).Error; err != nil {
			return err
		}
		return s.bumpProjectionRevision(db, projection.EmployeePublicID)
	}
	if findErr != nil {
		return findErr
	}
	if err := db.Model(&taskProjectionRow{}).Where("public_id = ?", projection.PublicID).Updates(map[string]any{"assignment_public_id": row.AssignmentPublicID, "employee_public_id": row.EmployeePublicID, "flight_display_no": row.FlightDisplayNo, "task_name": row.TaskName, "area_name": row.AreaName, "planned_at": row.PlannedAt, "status": row.Status, "message": row.Message, "sync_version": row.SyncVersion, "updated_at": row.UpdatedAt}).Error; err != nil {
		return err
	}
	if err := s.bumpProjectionRevision(db, projection.EmployeePublicID); err != nil {
		return err
	}
	if existing.EmployeePublicID != projection.EmployeePublicID {
		return s.bumpProjectionRevision(db, existing.EmployeePublicID)
	}
	return nil
}

func (s *SQLStore) ListTaskProjections(ctx context.Context, employeePublicID string) ([]TaskProjection, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("edge sql store is not configured")
	}
	var rows []taskProjectionRow
	query := s.db.WithContext(ctx).Order("updated_at ASC, public_id ASC")
	if employeePublicID != "" {
		query = query.Where("employee_public_id = ?", employeePublicID)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list task projections: %w", err)
	}
	values := make([]TaskProjection, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.projection())
	}
	return values, nil
}

func (s *SQLStore) ListTaskSnapshot(ctx context.Context, employeePublicID string, now time.Time) (TaskSnapshot, error) {
	if s == nil || s.db == nil {
		return TaskSnapshot{}, fmt.Errorf("edge sql store is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var (
		rows          []taskProjectionRow
		cursor        employeeProjectionCursorRow
		latestApplied syncInboxRow
	)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Order("updated_at ASC, public_id ASC")
		if employeePublicID != "" {
			query = query.Where("employee_public_id = ?", employeePublicID)
		}
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		if err := tx.Where("employee_public_id = ?", employeePublicID).First(&cursor).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read employee projection revision: %w", err)
		}
		if err := tx.Select("occurred_at").Where("status = ?", sharedEvent.StatusApplied).Order("occurred_at DESC").First(&latestApplied).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read latest applied projection event: %w", err)
		}
		return nil
	})
	if err != nil {
		return TaskSnapshot{}, err
	}
	values := make([]TaskProjection, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.projection())
	}
	snapshot := TaskSnapshot{Items: values, ProjectionRevision: cursor.Revision, SnapshotAt: now.UTC()}
	if !latestApplied.OccurredAt.IsZero() {
		snapshot.ProjectionLagKnown = true
		if now.After(latestApplied.OccurredAt) {
			snapshot.ProjectionLag = now.Sub(latestApplied.OccurredAt)
		}
	}
	return snapshot, nil
}

func (s *SQLStore) bumpProjectionRevision(db *gorm.DB, employeePublicID string) error {
	now := time.Now().UTC()
	return db.Exec(`INSERT INTO employee_projection_cursor (employee_public_id, revision, updated_at) VALUES (?, 1, ?) ON DUPLICATE KEY UPDATE revision = revision + 1, updated_at = ?`, employeePublicID, now, now).Error
}

func (s *SQLStore) updateCommand(ctx context.Context, commandID string, values map[string]any) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("edge sql store is not configured")
	}
	values["updated_at"] = time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&mobileCommandRow{}).Where("command_id = ?", commandID).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) updateCommandWithLease(ctx context.Context, commandID, owner string, values map[string]any) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("edge sql store is not configured")
	}
	if owner == "" {
		return fmt.Errorf("command lease owner is required")
	}
	values["lease_owner"] = nil
	values["lease_expires_at"] = nil
	values["updated_at"] = time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&mobileCommandRow{}).Where("command_id = ? AND status = ? AND lease_owner = ?", commandID, sharedEvent.StatusProcessing, owner).Updates(values)
	if result.Error != nil {
		return fmt.Errorf("update leased command %s: %w", commandID, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrLeaseLost
	}
	return nil
}

type syncInboxRow struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	EventID       string     `gorm:"column:event_id"`
	EventType     string     `gorm:"column:event_type"`
	SchemaVersion int        `gorm:"column:schema_version"`
	AggregateType string     `gorm:"column:aggregate_type"`
	AggregateID   string     `gorm:"column:aggregate_id"`
	OccurredAt    time.Time  `gorm:"column:occurred_at"`
	Producer      string     `gorm:"column:producer"`
	CorrelationID string     `gorm:"column:correlation_id"`
	TraceID       string     `gorm:"column:trace_id"`
	Payload       []byte     `gorm:"column:payload"`
	Status        string     `gorm:"column:status"`
	ErrorMessage  *string    `gorm:"column:error_message"`
	ReceivedAt    time.Time  `gorm:"column:received_at"`
	AppliedAt     *time.Time `gorm:"column:applied_at"`
}

func (syncInboxRow) TableName() string { return "sync_inbox" }

func newSyncInboxRow(envelope sharedEvent.EventEnvelope) syncInboxRow {
	return syncInboxRow{EventID: envelope.EventID, EventType: envelope.EventType, SchemaVersion: envelope.SchemaVersion, AggregateType: envelope.AggregateType, AggregateID: envelope.AggregateID, OccurredAt: envelope.OccurredAt.UTC(), Producer: envelope.Producer, CorrelationID: envelope.CorrelationID, TraceID: envelope.TraceID, Payload: append([]byte(nil), envelope.Payload...), Status: sharedEvent.StatusProcessing, ReceivedAt: time.Now().UTC()}
}

type taskProjectionRow struct {
	ID                 uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID           string    `gorm:"column:public_id"`
	AssignmentPublicID string    `gorm:"column:assignment_public_id"`
	EmployeePublicID   string    `gorm:"column:employee_public_id"`
	FlightDisplayNo    string    `gorm:"column:flight_display_no"`
	TaskName           string    `gorm:"column:task_name"`
	AreaName           string    `gorm:"column:area_name"`
	PlannedAt          time.Time `gorm:"column:planned_at"`
	Status             string    `gorm:"column:status"`
	Message            string    `gorm:"column:message"`
	SyncVersion        uint64    `gorm:"column:sync_version"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

type employeeProjectionCursorRow struct {
	EmployeePublicID string    `gorm:"column:employee_public_id;primaryKey"`
	Revision         uint64    `gorm:"column:revision"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (employeeProjectionCursorRow) TableName() string { return "employee_projection_cursor" }

func (taskProjectionRow) TableName() string { return "task_projection" }

func (row taskProjectionRow) projection() TaskProjection {
	return TaskProjection{PublicID: row.PublicID, AssignmentPublicID: row.AssignmentPublicID, EmployeePublicID: row.EmployeePublicID, FlightDisplayNo: row.FlightDisplayNo, TaskName: row.TaskName, AreaName: row.AreaName, PlannedAt: row.PlannedAt.UTC(), Status: row.Status, BusinessStatus: row.Status, Message: row.Message, SyncVersion: row.SyncVersion, UpdatedAt: row.UpdatedAt.UTC()}
}

type mobileCommandRow struct {
	ID             uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	CommandID      string     `gorm:"column:command_id"`
	CommandType    string     `gorm:"column:command_type"`
	SchemaVersion  int        `gorm:"column:schema_version"`
	ActorPublicID  string     `gorm:"column:actor_public_id"`
	AggregateID    string     `gorm:"column:aggregate_id"`
	OccurredAt     time.Time  `gorm:"column:occurred_at"`
	TraceID        string     `gorm:"column:trace_id"`
	Payload        []byte     `gorm:"column:payload"`
	Status         string     `gorm:"column:status"`
	Attempts       int        `gorm:"column:attempts"`
	NextAttemptAt  time.Time  `gorm:"column:next_attempt_at"`
	LastError      *string    `gorm:"column:last_error"`
	LeaseOwner     *string    `gorm:"column:lease_owner"`
	LeaseExpiresAt *time.Time `gorm:"column:lease_expires_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (mobileCommandRow) TableName() string { return "mobile_command" }

func newMobileCommandRow(command sharedEvent.CommandEnvelope) mobileCommandRow {
	now := time.Now().UTC()
	return mobileCommandRow{CommandID: command.CommandID, CommandType: command.CommandType, SchemaVersion: command.SchemaVersion, ActorPublicID: command.ActorPublicID, AggregateID: command.AggregateID, OccurredAt: command.OccurredAt.UTC().Round(time.Microsecond), TraceID: command.TraceID, Payload: append([]byte(nil), command.Payload...), Status: sharedEvent.StatusPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}
}

func (row mobileCommandRow) commandRecord() CommandRecord {
	lastError := ""
	if row.LastError != nil {
		lastError = *row.LastError
	}
	var leaseOwner string
	if row.LeaseOwner != nil {
		leaseOwner = *row.LeaseOwner
	}
	var leaseExpiresAt time.Time
	if row.LeaseExpiresAt != nil {
		leaseExpiresAt = row.LeaseExpiresAt.UTC()
	}
	return CommandRecord{ID: row.ID, Envelope: sharedEvent.CommandEnvelope{CommandID: row.CommandID, CommandType: row.CommandType, SchemaVersion: row.SchemaVersion, ActorPublicID: row.ActorPublicID, AggregateID: row.AggregateID, OccurredAt: row.OccurredAt.UTC(), TraceID: row.TraceID, Payload: append([]byte(nil), row.Payload...)}, Status: row.Status, Attempts: row.Attempts, NextAttempt: row.NextAttemptAt.UTC(), LastError: lastError, LeaseOwner: leaseOwner, LeaseExpiresAt: leaseExpiresAt, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
}
