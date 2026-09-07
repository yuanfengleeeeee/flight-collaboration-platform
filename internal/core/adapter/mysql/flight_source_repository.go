package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	flightsync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flightsync"
	flightmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/flight"
	flightintegration "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/flight"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ flightsync.Repository = (*FlightSourceRepository)(nil)

type FlightSourceRepository struct{ db *gorm.DB }

func NewFlightSourceRepository(db *gorm.DB) *FlightSourceRepository {
	return &FlightSourceRepository{db: db}
}

func (r *FlightSourceRepository) RecordSourceAttempt(ctx context.Context, provider string, at time.Time) error {
	if r == nil || r.db == nil {
		return flightsync.ErrRepositoryNotConfigured
	}
	return r.upsertSourceHealth(ctx, provider, map[string]any{"state": string(flightsync.SourceStale), "last_attempt_at": at.UTC(), "updated_at": at.UTC()})
}

func (r *FlightSourceRepository) RecordSourceSuccess(ctx context.Context, provider string, at time.Time) error {
	if r == nil || r.db == nil {
		return flightsync.ErrRepositoryNotConfigured
	}
	if err := r.upsertSourceHealth(ctx, provider, map[string]any{"state": string(flightsync.SourceFresh), "last_attempt_at": at.UTC(), "last_success_at": at.UTC(), "last_error": nil, "updated_at": at.UTC()}); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&flightRow{}).Where("source_provider = ?", provider).Updates(map[string]any{"source_state": string(flightsync.SourceFresh), "source_last_attempt_at": at.UTC(), "source_last_error": nil, "updated_at": at.UTC()}).Error
}

func (r *FlightSourceRepository) RecordSourceFailure(ctx context.Context, provider string, at time.Time, reason string) error {
	if r == nil || r.db == nil {
		return flightsync.ErrRepositoryNotConfigured
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&flightRow{}).Where("source_provider = ?", provider).Count(&count).Error; err != nil {
		return err
	}
	state := flightsync.SourceFailed
	if count > 0 {
		state = flightsync.SourceFallback
	}
	if err := r.upsertSourceHealth(ctx, provider, map[string]any{"state": string(state), "last_attempt_at": at.UTC(), "last_failure_at": at.UTC(), "last_error": reason, "updated_at": at.UTC()}); err != nil {
		return err
	}
	flightState := string(flightsync.SourceFailed)
	if count > 0 {
		flightState = string(flightsync.SourceFallback)
	}
	return r.db.WithContext(ctx).Model(&flightRow{}).Where("source_provider = ?", provider).Updates(map[string]any{"source_state": flightState, "source_last_attempt_at": at.UTC(), "source_last_error": reason, "updated_at": at.UTC()}).Error
}

func (r *FlightSourceRepository) GetSourceHealth(ctx context.Context, provider string) (flightsync.SourceHealth, error) {
	if r == nil || r.db == nil {
		return flightsync.SourceHealth{}, flightsync.ErrRepositoryNotConfigured
	}
	var row flightSourceHealthRow
	if err := r.db.WithContext(ctx).Where("provider = ?", provider).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return flightsync.SourceHealth{Provider: provider, State: flightsync.SourceStale, FallbackEnabled: true}, nil
		}
		return flightsync.SourceHealth{}, err
	}
	return row.toDomain(), nil
}

func (r *FlightSourceRepository) upsertSourceHealth(ctx context.Context, provider string, updates map[string]any) error {
	var row flightSourceHealthRow
	if err := r.db.WithContext(ctx).Where("provider = ?", provider).First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		publicID, idErr := id.NewPublicID()
		if idErr != nil {
			return idErr
		}
		row = flightSourceHealthRow{PublicID: publicID, Provider: provider, State: string(flightsync.SourceStale)}
		if err := r.db.WithContext(ctx).Create(&row).Error; err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
			return err
		}
	} else if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&flightSourceHealthRow{}).Where("provider = ?", provider).Updates(updates).Error
}

func (r *FlightSourceRepository) Stage(ctx context.Context, input flightsync.IngestInput) (flightsync.StageResult, error) {
	if r == nil || r.db == nil {
		return flightsync.StageResult{}, flightsync.ErrRepositoryNotConfigured
	}
	now := input.Now.UTC()
	result := flightsync.StageResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, schedule := range input.Schedules {
			payload, err := json.Marshal(schedule)
			if err != nil {
				return fmt.Errorf("marshal flight schedule: %w", err)
			}
			row, err := sourceInboxRowFromSchedule(schedule, payload, now)
			if err != nil {
				return err
			}
			wasNew, wasUpdated, err := stageSourceRecord(tx, row, now)
			if err != nil {
				return err
			}
			if wasNew {
				result.Accepted++
			} else if wasUpdated {
				result.Updated++
			} else {
				result.Duplicate++
			}
		}
		for _, sourceEvent := range input.Events {
			payload, err := json.Marshal(sourceEvent)
			if err != nil {
				return fmt.Errorf("marshal flight source event: %w", err)
			}
			row, err := sourceInboxRowFromEvent(sourceEvent, payload, now)
			if err != nil {
				return err
			}
			wasNew, wasUpdated, err := stageSourceRecord(tx, row, now)
			if err != nil {
				return err
			}
			if wasNew {
				result.Accepted++
			} else if wasUpdated {
				result.Updated++
			} else {
				result.Duplicate++
			}
		}
		return nil
	})
	if err != nil {
		return flightsync.StageResult{}, fmt.Errorf("stage flight source records: %w", err)
	}
	return result, nil
}

func stageSourceRecord(tx *gorm.DB, row flightSourceInboxRow, now time.Time) (bool, bool, error) {
	var existing flightSourceInboxRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ? AND record_type = ? AND external_record_id = ?", row.Provider, row.RecordType, row.ExternalRecordID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Create(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return false, false, nil
			}
			return false, false, fmt.Errorf("create flight source inbox record: %w", err)
		}
		return true, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("find flight source inbox record: %w", err)
	}
	if existing.PayloadHash == row.PayloadHash {
		return false, false, nil
	}
	updates := map[string]any{
		"external_flight_id": row.ExternalFlightID, "flight_display_no": row.FlightDisplayNo,
		"operating_date": row.OperatingDate, "scheduled_at": row.ScheduledAt,
		"event_status": row.EventStatus, "occurred_at": row.OccurredAt,
		"actual_arrival_at": row.ActualArrivalAt, "actual_departure_at": row.ActualDepartureAt,
		"reason": row.Reason, "payload": row.Payload, "payload_hash": row.PayloadHash,
		"status": sharedEvent.StatusPending, "attempts": 0, "next_attempt_at": now,
		"lease_owner": nil, "lease_expires_at": nil, "applied_at": nil, "last_error": nil, "updated_at": now,
	}
	if err := tx.Model(&flightSourceInboxRow{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return false, false, fmt.Errorf("update flight source inbox record: %w", err)
	}
	return false, true, nil
}

func (r *FlightSourceRepository) ClaimPending(ctx context.Context, limit int, now time.Time, owner string, leaseDuration time.Duration) ([]flightsync.PendingRecord, error) {
	if r == nil || r.db == nil {
		return nil, flightsync.ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = 50
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("flight source lease owner is required")
	}
	if leaseDuration <= 0 {
		leaseDuration = time.Minute
	}
	leaseExpiresAt := now.UTC().Add(leaseDuration)
	var rows []flightSourceInboxRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").Where("(status IN ? AND next_attempt_at <= ?) OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?))", []string{sharedEvent.StatusPending, sharedEvent.StatusRetry}, now.UTC(), sharedEvent.StatusProcessing, now.UTC()).Order("id ASC").Limit(limit).Find(&rows)
		if query.Error != nil {
			return query.Error
		}
		for _, row := range rows {
			if err := tx.Model(&flightSourceInboxRow{}).Where("id = ?", row.ID).Updates(map[string]any{"status": sharedEvent.StatusProcessing, "lease_owner": owner, "lease_expires_at": leaseExpiresAt, "updated_at": now.UTC()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim flight source inbox records: %w", err)
	}
	result := make([]flightsync.PendingRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toPendingRecord())
	}
	return result, nil
}

func (r *FlightSourceRepository) MarkApplied(ctx context.Context, recordID uint64, owner string, appliedAt time.Time) error {
	return r.updateClaimed(ctx, recordID, owner, map[string]any{"status": sharedEvent.StatusApplied, "applied_at": appliedAt.UTC(), "last_error": nil})
}

func (r *FlightSourceRepository) MarkRetry(ctx context.Context, recordID uint64, owner string, next time.Time, reason string) error {
	return r.updateClaimed(ctx, recordID, owner, map[string]any{"status": sharedEvent.StatusRetry, "next_attempt_at": next.UTC(), "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1)})
}

func (r *FlightSourceRepository) MarkFailed(ctx context.Context, recordID uint64, owner, reason string) error {
	return r.updateClaimed(ctx, recordID, owner, map[string]any{"status": sharedEvent.StatusFailed, "last_error": reason, "attempts": gorm.Expr("attempts + ?", 1)})
}

func (r *FlightSourceRepository) updateClaimed(ctx context.Context, recordID uint64, owner string, values map[string]any) error {
	if r == nil || r.db == nil {
		return flightsync.ErrRepositoryNotConfigured
	}
	values["lease_owner"] = nil
	values["lease_expires_at"] = nil
	values["updated_at"] = time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&flightSourceInboxRow{}).Where("id = ? AND status = ? AND lease_owner = ?", recordID, sharedEvent.StatusProcessing, owner).Updates(values)
	if result.Error != nil {
		return fmt.Errorf("update flight source inbox record %d: %w", recordID, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("flight source inbox lease lost for record %d", recordID)
	}
	return nil
}

func (r *FlightSourceRepository) ApplySchedule(ctx context.Context, record flightsync.PendingRecord, schedule flightintegration.Schedule, now time.Time) error {
	if r == nil || r.db == nil {
		return flightsync.ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row flightRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_provider = ? AND external_flight_id = ?", schedule.Provider, schedule.ExternalFlightID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operating_date = ? AND flight_display_no = ?", schedule.OperatingDate.UTC().Format("2006-01-02"), schedule.FlightDisplayNo).First(&row).Error
			if err == nil && row.ExternalFlightID != "" && (row.SourceProvider != schedule.Provider || row.ExternalFlightID != schedule.ExternalFlightID) {
				return fmt.Errorf("flight display number %s on %s belongs to another external flight", schedule.FlightDisplayNo, schedule.OperatingDate.Format("2006-01-02"))
			}
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find flight for schedule: %w", err)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			publicID, err := id.NewPublicID()
			if err != nil {
				return fmt.Errorf("generate synchronized flight public id: %w", err)
			}
			syncedAt := now.UTC()
			row = flightRow{PublicID: publicID, DisplayNo: schedule.FlightDisplayNo, SourceProvider: schedule.Provider, ExternalFlightID: schedule.ExternalFlightID, OperatingDate: schedule.OperatingDate.UTC(), ScheduledAt: schedule.ScheduledAt.UTC(), SourceLastSyncedAt: &syncedAt, SourceState: string(flightsync.SourceFresh), SourceLastAttemptAt: &syncedAt, Status: string(flightmodule.StatusScheduled), LastStatusChangedAt: now.UTC(), CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
			if err := tx.Create(&row).Error; err != nil {
				return mapWriteError(err)
			}
			return nil
		}
		updates := map[string]any{"flight_display_no": schedule.FlightDisplayNo, "source_provider": schedule.Provider, "external_flight_id": schedule.ExternalFlightID, "operating_date": schedule.OperatingDate.UTC(), "scheduled_at": schedule.ScheduledAt.UTC(), "source_last_synced_at": now.UTC(), "source_state": string(flightsync.SourceFresh), "source_last_attempt_at": now.UTC(), "source_last_error": nil, "updated_at": now.UTC()}
		if err := tx.Model(&flightRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return mapWriteError(err)
		}
		return nil
	})
}

func (r *FlightSourceRepository) FindFlightPublicID(ctx context.Context, provider, externalFlightID string) (string, error) {
	if r == nil || r.db == nil {
		return "", flightsync.ErrRepositoryNotConfigured
	}
	var row flightRow
	if err := r.db.WithContext(ctx).Where("source_provider = ? AND external_flight_id = ?", provider, externalFlightID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", flightsync.ErrInvalidInput
		}
		return "", err
	}
	return row.PublicID, nil
}

func (r *FlightSourceRepository) ApplyEvent(ctx context.Context, record flightsync.PendingRecord, sourceEvent flightintegration.Event, now time.Time) error {
	if r == nil || r.db == nil {
		return flightsync.ErrRepositoryNotConfigured
	}
	if sourceEvent.Status == "arrived" {
		return flightsync.ErrUnsupportedEvent
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row flightRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_provider = ? AND external_flight_id = ?", sourceEvent.Provider, sourceEvent.ExternalFlightID).First(&row).Error; err != nil {
			return normalizeNotFound(err)
		}
		sourceID := sourceEventID(sourceEvent.Provider, sourceEvent.ExternalEventID)
		var history flightStatusHistoryRow
		if err := tx.Where("source_event_id = ?", sourceID).First(&history).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if sourceEvent.Status == "delayed" {
			if sourceEvent.ScheduledAt == nil || sourceEvent.ScheduledAt.IsZero() {
				return flightsync.ErrInvalidInput
			}
			oldScheduledAt := row.ScheduledAt
			updates := map[string]any{"scheduled_at": sourceEvent.ScheduledAt.UTC(), "source_last_synced_at": now.UTC(), "source_state": string(flightsync.SourceFresh), "source_last_attempt_at": now.UTC(), "source_last_error": nil, "updated_at": now.UTC()}
			if err := tx.Model(&flightRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{"flight_public_id": row.PublicID, "flight_display_no": row.DisplayNo, "source_event_id": sourceID, "from_scheduled_at": oldScheduledAt.UTC(), "to_scheduled_at": sourceEvent.ScheduledAt.UTC(), "reason": sourceEvent.Reason})
			envelope, err := sharedEvent.NewEvent("flight.schedule.changed.v1", "flight", row.PublicID, "core-flight-source-sync", json.RawMessage(payload))
			if err != nil {
				return err
			}
			envelope.OccurredAt = sourceEvent.OccurredAt.UTC()
			envelope.Payload = payload
			outbox := outboxRow{EventID: envelope.EventID, EventType: envelope.EventType, SchemaVersion: envelope.SchemaVersion, AggregateType: envelope.AggregateType, AggregateID: envelope.AggregateID, OccurredAt: envelope.OccurredAt, Producer: envelope.Producer, CorrelationID: envelope.CorrelationID, TraceID: envelope.TraceID, Payload: payload, Status: sharedEvent.StatusPending, NextAttemptAt: now.UTC(), CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
			return tx.Create(&outbox).Error
		}
		target := flightmodule.Status(sourceEvent.Status)
		if row.Status == string(target) {
			return tx.Model(&flightRow{}).Where("id = ?", row.ID).Updates(map[string]any{"source_last_synced_at": now.UTC(), "source_state": string(flightsync.SourceFresh), "source_last_attempt_at": now.UTC(), "source_last_error": nil, "updated_at": now.UTC()}).Error
		}
		if target == flightmodule.StatusCancelled && row.Status == string(flightmodule.StatusDeparted) {
			return fmt.Errorf("cancelled event cannot follow departed flight")
		}
		if target == flightmodule.StatusDeparted && row.Status == string(flightmodule.StatusCancelled) {
			return fmt.Errorf("departed event cannot follow cancelled flight")
		}
		from := row.Status
		updates := map[string]any{"status": string(target), "status_version": row.StatusVersion + 1, "source_last_synced_at": now.UTC(), "source_state": string(flightsync.SourceFresh), "source_last_attempt_at": now.UTC(), "source_last_error": nil, "last_status_changed_at": sourceEvent.OccurredAt.UTC(), "updated_at": now.UTC()}
		if sourceEvent.ActualDepartureAt != nil {
			updates["departed_at"] = sourceEvent.ActualDepartureAt.UTC()
		}
		if err := tx.Model(&flightRow{}).Where("id = ? AND status_version = ?", row.ID, row.StatusVersion).Updates(updates).Error; err != nil {
			return err
		}
		historyID, err := id.NewPublicID()
		if err != nil {
			return err
		}
		if err := tx.Create(&flightStatusHistoryRow{PublicID: historyID, FlightID: row.ID, FromStatus: &from, ToStatus: string(target), TransitionType: "flight_source_event", SourceEventID: sourceID, ActorType: "machine", ActorPublicID: sourceEvent.Provider, Reason: sourceEvent.Reason, OccurredAt: sourceEvent.OccurredAt.UTC(), CreatedAt: now.UTC()}).Error; err != nil {
			return mapWriteError(err)
		}
		payload, _ := json.Marshal(map[string]any{"flight_public_id": row.PublicID, "flight_display_no": row.DisplayNo, "from_status": from, "to_status": target, "source_event_id": sourceID})
		envelope, err := sharedEvent.NewEvent("flight.status.changed.v1", "flight", row.PublicID, "core-flight-source-sync", json.RawMessage(payload))
		if err != nil {
			return err
		}
		envelope.OccurredAt = sourceEvent.OccurredAt.UTC()
		envelope.Payload = payload
		outbox := outboxRow{EventID: envelope.EventID, EventType: envelope.EventType, SchemaVersion: envelope.SchemaVersion, AggregateType: envelope.AggregateType, AggregateID: envelope.AggregateID, OccurredAt: envelope.OccurredAt, Producer: envelope.Producer, CorrelationID: envelope.CorrelationID, TraceID: envelope.TraceID, Payload: payload, Status: sharedEvent.StatusPending, NextAttemptAt: now.UTC(), CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
		if err := tx.Create(&outbox).Error; err != nil {
			return mapWriteError(err)
		}
		return nil
	})
}

func sourceEventID(provider, externalID string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + ":" + strings.TrimSpace(externalID)
}

type flightSourceInboxRow struct {
	ID                uint64     `gorm:"column:id;primaryKey"`
	PublicID          string     `gorm:"column:public_id"`
	Provider          string     `gorm:"column:provider"`
	RecordType        string     `gorm:"column:record_type"`
	ExternalRecordID  string     `gorm:"column:external_record_id"`
	ExternalFlightID  string     `gorm:"column:external_flight_id"`
	FlightDisplayNo   *string    `gorm:"column:flight_display_no"`
	OperatingDate     *time.Time `gorm:"column:operating_date"`
	ScheduledAt       *time.Time `gorm:"column:scheduled_at"`
	EventStatus       *string    `gorm:"column:event_status"`
	OccurredAt        *time.Time `gorm:"column:occurred_at"`
	ActualArrivalAt   *time.Time `gorm:"column:actual_arrival_at"`
	ActualDepartureAt *time.Time `gorm:"column:actual_departure_at"`
	Reason            *string    `gorm:"column:reason"`
	Payload           []byte     `gorm:"column:payload"`
	PayloadHash       string     `gorm:"column:payload_hash"`
	Status            string     `gorm:"column:status"`
	Attempts          int        `gorm:"column:attempts"`
	NextAttemptAt     time.Time  `gorm:"column:next_attempt_at"`
	LeaseOwner        *string    `gorm:"column:lease_owner"`
	LeaseExpiresAt    *time.Time `gorm:"column:lease_expires_at"`
	AppliedAt         *time.Time `gorm:"column:applied_at"`
	LastError         *string    `gorm:"column:last_error"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

type flightSourceHealthRow struct {
	ID            uint64     `gorm:"column:id;primaryKey"`
	PublicID      string     `gorm:"column:public_id"`
	Provider      string     `gorm:"column:provider"`
	State         string     `gorm:"column:state"`
	LastAttemptAt *time.Time `gorm:"column:last_attempt_at"`
	LastSuccessAt *time.Time `gorm:"column:last_success_at"`
	LastFailureAt *time.Time `gorm:"column:last_failure_at"`
	LastError     string     `gorm:"column:last_error"`
}

func (flightSourceHealthRow) TableName() string { return "flight_source_health" }

func (row flightSourceHealthRow) toDomain() flightsync.SourceHealth {
	return flightsync.SourceHealth{Provider: row.Provider, State: flightsync.SourceState(row.State), LastAttemptAt: row.LastAttemptAt, LastSuccessAt: row.LastSuccessAt, LastFailureAt: row.LastFailureAt, LastError: row.LastError, FallbackEnabled: true}
}

func (flightSourceInboxRow) TableName() string { return "flight_source_inbox" }

func sourceInboxRowFromSchedule(value flightintegration.Schedule, payload []byte, now time.Time) (flightSourceInboxRow, error) {
	displayNo := value.FlightDisplayNo
	operatingDate := value.OperatingDate.UTC()
	scheduledAt := value.ScheduledAt.UTC()
	publicID, err := id.NewPublicID()
	if err != nil {
		return flightSourceInboxRow{}, fmt.Errorf("generate flight source inbox public id: %w", err)
	}
	return flightSourceInboxRow{PublicID: publicID, Provider: value.Provider, RecordType: flightsync.RecordTypeSchedule, ExternalRecordID: value.ExternalFlightID, ExternalFlightID: value.ExternalFlightID, FlightDisplayNo: &displayNo, OperatingDate: &operatingDate, ScheduledAt: &scheduledAt, Payload: payload, PayloadHash: hashPayload(payload), Status: sharedEvent.StatusPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}, nil
}

func sourceInboxRowFromEvent(value flightintegration.Event, payload []byte, now time.Time) (flightSourceInboxRow, error) {
	status, reason := value.Status, value.Reason
	occurredAt := value.OccurredAt.UTC()
	publicID, err := id.NewPublicID()
	if err != nil {
		return flightSourceInboxRow{}, fmt.Errorf("generate flight source inbox public id: %w", err)
	}
	var scheduledAt *time.Time
	if value.ScheduledAt != nil {
		scheduled := value.ScheduledAt.UTC()
		scheduledAt = &scheduled
	}
	return flightSourceInboxRow{PublicID: publicID, Provider: value.Provider, RecordType: flightsync.RecordTypeEvent, ExternalRecordID: value.ExternalEventID, ExternalFlightID: value.ExternalFlightID, ScheduledAt: scheduledAt, EventStatus: &status, OccurredAt: &occurredAt, ActualArrivalAt: value.ActualArrivalAt, ActualDepartureAt: value.ActualDepartureAt, Reason: &reason, Payload: payload, PayloadHash: hashPayload(payload), Status: sharedEvent.StatusPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}, nil
}

func (row flightSourceInboxRow) toPendingRecord() flightsync.PendingRecord {
	return flightsync.PendingRecord{ID: row.ID, PublicID: row.PublicID, Provider: row.Provider, RecordType: row.RecordType, ExternalRecordID: row.ExternalRecordID, ExternalFlightID: row.ExternalFlightID, FlightDisplayNo: optionalString(row.FlightDisplayNo), OperatingDate: row.OperatingDate, ScheduledAt: row.ScheduledAt, EventStatus: optionalString(row.EventStatus), OccurredAt: row.OccurredAt, ActualArrivalAt: row.ActualArrivalAt, ActualDepartureAt: row.ActualDepartureAt, Reason: optionalString(row.Reason), Payload: append([]byte(nil), row.Payload...), Attempts: row.Attempts}
}

func hashPayload(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
