// Package mysql contains the GORM adapter for Core business use cases. It is
// an infrastructure package: application services depend on its ports, not
// on GORM or these row mappings.
package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	flighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	flightmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/flight"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ flighttask.Repository = (*Repository)(nil)

type Repository struct {
	db *gorm.DB
}

func NewFlightTaskRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) FindBusinessIdempotency(ctx context.Context, operationType, key string) (flighttask.IdempotencyRecord, error) {
	if r == nil || r.db == nil {
		return flighttask.IdempotencyRecord{}, flighttask.ErrRepositoryNotConfigured
	}
	return findBusinessIdempotency(ctx, r.db, operationType, key)
}

func (r *Repository) WithinTransaction(ctx context.Context, fn func(flighttask.Transaction) error) error {
	if r == nil || r.db == nil || fn == nil {
		return flighttask.ErrRepositoryNotConfigured
	}
	if ctx == nil {
		return fmt.Errorf("flight task transaction context is nil")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&transaction{db: tx})
	})
}

type transaction struct {
	db *gorm.DB
}

func (tx *transaction) FindFlightForUpdate(ctx context.Context, publicID string) (flightmodule.Record, error) {
	var row flightRow
	err := tx.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", publicID).First(&row).Error
	if err != nil {
		return flightmodule.Record{}, normalizeNotFound(err)
	}
	return row.toDomain(), nil
}

func (tx *transaction) FindBusinessIdempotency(ctx context.Context, operationType, key string) (flighttask.IdempotencyRecord, error) {
	return findBusinessIdempotency(ctx, tx.db, operationType, key)
}

func (tx *transaction) FindTaskByGenerationKey(ctx context.Context, generationKey string) (taskmodule.Instance, error) {
	var row taskRow
	err := tx.db.WithContext(ctx).Where("generation_key = ?", generationKey).First(&row).Error
	if err != nil {
		return taskmodule.Instance{}, normalizeNotFound(err)
	}
	return row.toDomain(), nil
}

func (tx *transaction) FindActiveTemplate(ctx context.Context, triggerType taskmodule.TriggerType) (taskmodule.Template, error) {
	var row taskTemplateRow
	err := tx.db.WithContext(ctx).Where("trigger_type = ? AND enabled = ?", string(triggerType), true).Order("template_version DESC").First(&row).Error
	if err != nil {
		return taskmodule.Template{}, normalizeNotFound(err)
	}
	return row.toDomain()
}

func (tx *transaction) ListCandidatePersonnel(ctx context.Context, filter flighttask.CandidateFilter) ([]personnelmodule.CandidateRecord, error) {
	if filter.TeamID == 0 || filter.AreaID == 0 || strings.TrimSpace(filter.PositionCode) == "" || filter.PlannedAt.IsZero() {
		return nil, fmt.Errorf("candidate filter is incomplete")
	}
	var rows []personnelCandidateRow
	query := tx.db.WithContext(ctx).
		Table("personnel AS p").
		Select("p.id, p.public_id, p.user_public_id, p.team_id, t.area_id, p.position_code, p.capabilities, p.work_state, p.status_version, p.last_state_changed_at, p.enabled").
		Joins("JOIN team AS t ON t.id = p.team_id").
		Joins("JOIN operation_area AS oa ON oa.id = t.area_id").
		Where("p.team_id = ? AND t.area_id = ? AND t.enabled = ? AND oa.enabled = ? AND p.position_code = ? AND p.work_state = ? AND p.enabled = ?", filter.TeamID, filter.AreaID, true, true, filter.PositionCode, string(personnelmodule.WorkStateIdle), true).
		Where("EXISTS (SELECT 1 FROM team_member AS tm WHERE tm.personnel_id = p.id AND tm.team_id = p.team_id AND tm.left_at IS NULL AND tm.is_primary = ?)", true).
		// BVS2-02 models the first slice's planned time as a point in time.
		// Until a scheduling duration is explicitly designed, an active
		// assignment at the same planned_at is the deterministic conflict rule.
		Where("NOT EXISTS (SELECT 1 FROM task_assignment AS a JOIN task_instance AS ti ON ti.id = a.task_id WHERE a.personnel_id = p.id AND a.status IN (?, ?) AND ti.planned_at = ?)", "confirmed", "accepted", filter.PlannedAt.UTC()).
		Order("p.last_state_changed_at ASC, p.public_id ASC").
		Find(&rows)
	if query.Error != nil {
		return nil, fmt.Errorf("query candidate personnel: %w", query.Error)
	}
	values := make([]personnelmodule.CandidateRecord, 0, len(rows))
	for _, row := range rows {
		value, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (tx *transaction) UpdateFlightArrived(ctx context.Context, flightID, expectedVersion uint64, actualArrivalAt, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Model(&flightRow{}).Where("id = ? AND status = ? AND status_version = ?", flightID, string(flightmodule.StatusScheduled), expectedVersion).Updates(map[string]any{
		"actual_arrival_at":      actualArrivalAt.UTC(),
		"status":                 string(flightmodule.StatusArrived),
		"status_version":         expectedVersion + 1,
		"last_status_changed_at": changedAt.UTC(),
		"updated_at":             changedAt.UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("update flight arrived: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("flight %d optimistic update did not affect one row", flightID)
	}
	return nil
}

func (tx *transaction) CreateFlightStatusHistory(ctx context.Context, value flightmodule.StatusHistory) error {
	var fromStatus *string
	if value.FromStatus != nil {
		from := string(*value.FromStatus)
		fromStatus = &from
	}
	now := time.Now().UTC()
	row := flightStatusHistoryRow{PublicID: value.PublicID, FlightID: value.FlightID, FromStatus: fromStatus, ToStatus: string(value.ToStatus), TransitionType: value.TransitionType, SourceEventID: value.SourceEventID, ActorType: value.ActorType, ActorPublicID: value.ActorPublicID, Reason: value.Reason, OccurredAt: value.OccurredAt.UTC(), CreatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return nil
}

func (tx *transaction) CreateTask(ctx context.Context, value *taskmodule.Instance) error {
	if value == nil {
		return fmt.Errorf("task instance is nil")
	}
	now := time.Now().UTC()
	row := taskRow{PublicID: value.PublicID, FlightID: value.FlightID, TemplateID: value.TemplateID, AreaID: value.AreaID, TeamID: value.TeamID, TriggerType: string(value.TriggerType), GenerationKey: value.GenerationKey, SourceEventID: value.SourceEventID, TemplateVersion: value.TemplateVersion, Name: value.Name, Message: value.Message, PlannedAt: value.PlannedAt.UTC(), Status: string(value.Status), StatusVersion: value.StatusVersion, SyncVersion: value.SyncVersion, CreatedAt: now, UpdatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	value.ID = row.ID
	return nil
}

func (tx *transaction) CreateTaskStatusHistory(ctx context.Context, value taskmodule.StatusHistory) error {
	var fromStatus *string
	if value.FromStatus != nil {
		from := string(*value.FromStatus)
		fromStatus = &from
	}
	now := time.Now().UTC()
	row := taskStatusHistoryRow{PublicID: value.PublicID, TaskID: value.TaskID, StatusVersion: value.StatusVersion, FromStatus: fromStatus, ToStatus: string(value.ToStatus), Reason: value.Reason, ActorType: value.ActorType, ActorPublicID: value.ActorPublicID, CommandID: value.CommandID, SourceEventID: value.SourceEventID, OccurredAt: value.OccurredAt.UTC(), CreatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return nil
}

func (tx *transaction) CreateCandidate(ctx context.Context, value *taskmodule.Candidate) error {
	if value == nil {
		return fmt.Errorf("task candidate is nil")
	}
	capabilities, err := json.Marshal(value.MatchedCapabilities)
	if err != nil {
		return fmt.Errorf("marshal candidate capabilities: %w", err)
	}
	now := time.Now().UTC()
	row := taskCandidateRow{PublicID: value.PublicID, TaskID: value.TaskID, PersonnelID: value.PersonnelID, Rank: value.Rank, Status: string(value.Status), MatchedPositionCode: value.MatchedPositionCode, MatchedCapabilities: capabilities, PersonnelWorkStateSnapshot: value.PersonnelWorkStateSnapshot, PersonnelStateChangedAt: value.PersonnelStateChangedAt.UTC(), CreatedAt: now, UpdatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	value.ID = row.ID
	return nil
}

func (tx *transaction) CreateBusinessIdempotency(ctx context.Context, value flighttask.IdempotencyRecord) error {
	now := time.Now().UTC()
	row := businessIdempotencyRow{PublicID: value.PublicID, OperationType: value.OperationType, IdempotencyKey: value.IdempotencyKey, RequestHash: value.RequestHash, Status: value.Status, AggregateType: value.AggregateType, AggregatePublicID: value.AggregatePublicID, ActorType: value.ActorType, ActorPublicID: value.ActorPublicID, CommandType: value.CommandType, ResultCode: value.ResultCode, ResultStatus: value.ResultStatus, ResultPayload: append([]byte(nil), value.ResultPayload...), ErrorSummary: value.ErrorSummary, RequestID: value.RequestID, TraceID: value.TraceID, CreatedAt: now, UpdatedAt: now}
	if !value.FirstProcessedAt.IsZero() {
		first := value.FirstProcessedAt.UTC()
		row.FirstProcessedAt = &first
	}
	if !value.LastProcessedAt.IsZero() {
		last := value.LastProcessedAt.UTC()
		row.LastProcessedAt = &last
	}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return nil
}

func (tx *transaction) AppendAudit(ctx context.Context, value coresync.AuditRecord) error {
	now := time.Now().UTC()
	if value.OccurredAt.IsZero() {
		value.OccurredAt = now
	}
	row := auditRow{ActorType: value.ActorType, ActorID: value.ActorID, Action: value.Action, ResourceType: value.ResourceType, ResourceID: value.ResourceID, Result: value.Result, RequestID: value.RequestID, TraceID: value.TraceID, SourceIP: value.SourceIP, OccurredAt: value.OccurredAt.UTC(), CreatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create business audit: %w", err)
	}
	return nil
}

func (tx *transaction) AppendOutbox(ctx context.Context, value sharedEvent.EventEnvelope) error {
	if err := value.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	row := outboxRow{EventID: value.EventID, EventType: value.EventType, SchemaVersion: value.SchemaVersion, AggregateType: value.AggregateType, AggregateID: value.AggregateID, OccurredAt: value.OccurredAt.UTC(), Producer: value.Producer, CorrelationID: value.CorrelationID, TraceID: value.TraceID, Payload: append([]byte(nil), value.Payload...), Status: sharedEvent.StatusPending, Attempts: 0, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return nil
}

func findBusinessIdempotency(ctx context.Context, db *gorm.DB, operationType, key string) (flighttask.IdempotencyRecord, error) {
	var row businessIdempotencyRow
	err := db.WithContext(ctx).Where("operation_type = ? AND idempotency_key = ?", operationType, key).First(&row).Error
	if err != nil {
		return flighttask.IdempotencyRecord{}, normalizeNotFound(err)
	}
	return row.toDomain(), nil
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %v", flighttask.ErrNotFound, err)
	}
	return err
}

func mapWriteError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return flighttask.ErrDuplicate
	}
	return err
}

type flightRow struct {
	ID                  uint64     `gorm:"column:id;primaryKey"`
	PublicID            string     `gorm:"column:public_id"`
	DisplayNo           string     `gorm:"column:flight_display_no"`
	SourceProvider      string     `gorm:"column:source_provider"`
	ExternalFlightID    string     `gorm:"column:external_flight_id"`
	OperatingDate       time.Time  `gorm:"column:operating_date"`
	ScheduledAt         time.Time  `gorm:"column:scheduled_at"`
	SourceLastSyncedAt  *time.Time `gorm:"column:source_last_synced_at"`
	SourceState         string     `gorm:"column:source_state"`
	SourceLastAttemptAt *time.Time `gorm:"column:source_last_attempt_at"`
	SourceLastError     string     `gorm:"column:source_last_error"`
	ActualArrivalAt     *time.Time `gorm:"column:actual_arrival_at"`
	Status              string     `gorm:"column:status"`
	StatusVersion       uint64     `gorm:"column:status_version"`
	LastStatusChangedAt time.Time  `gorm:"column:last_status_changed_at"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

func (flightRow) TableName() string { return "flight" }

func (row flightRow) toDomain() flightmodule.Record {
	return flightmodule.Record{ID: row.ID, PublicID: row.PublicID, DisplayNo: row.DisplayNo, SourceProvider: row.SourceProvider, ExternalFlightID: row.ExternalFlightID, OperatingDate: row.OperatingDate.UTC(), ScheduledAt: row.ScheduledAt.UTC(), SourceLastSyncedAt: row.SourceLastSyncedAt, SourceState: flightmodule.SourceState(row.SourceState), SourceLastAttemptAt: row.SourceLastAttemptAt, SourceLastError: row.SourceLastError, ActualArrivalAt: row.ActualArrivalAt, Status: flightmodule.Status(row.Status), StatusVersion: row.StatusVersion, LastStatusChangedAt: row.LastStatusChangedAt.UTC()}
}

type flightStatusHistoryRow struct {
	ID             uint64    `gorm:"column:id;primaryKey"`
	PublicID       string    `gorm:"column:public_id"`
	FlightID       uint64    `gorm:"column:flight_id"`
	FromStatus     *string   `gorm:"column:from_status"`
	ToStatus       string    `gorm:"column:to_status"`
	TransitionType string    `gorm:"column:transition_type"`
	SourceEventID  string    `gorm:"column:source_event_id"`
	ActorType      string    `gorm:"column:actor_type"`
	ActorPublicID  string    `gorm:"column:actor_public_id"`
	Reason         string    `gorm:"column:reason"`
	OccurredAt     time.Time `gorm:"column:occurred_at"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (flightStatusHistoryRow) TableName() string { return "flight_status_history" }

type taskTemplateRow struct {
	ID                   uint64 `gorm:"column:id;primaryKey"`
	PublicID             string `gorm:"column:public_id"`
	Name                 string `gorm:"column:name"`
	TriggerType          string `gorm:"column:trigger_type"`
	Version              uint   `gorm:"column:template_version"`
	Enabled              bool   `gorm:"column:enabled"`
	AreaID               uint64 `gorm:"column:target_area_id"`
	TeamID               uint64 `gorm:"column:target_team_id"`
	RequiredPositionCode string `gorm:"column:required_position_code"`
	RequiredCapabilities []byte `gorm:"column:required_capabilities"`
	PlannedOffsetSeconds int    `gorm:"column:planned_offset_seconds"`
	DefaultMessage       string `gorm:"column:default_message"`
}

func (taskTemplateRow) TableName() string { return "task_template" }

func (row taskTemplateRow) toDomain() (taskmodule.Template, error) {
	capabilities, err := decodeCapabilities(row.RequiredCapabilities)
	if err != nil {
		return taskmodule.Template{}, fmt.Errorf("decode task template %s capabilities: %w", row.PublicID, err)
	}
	return taskmodule.Template{ID: row.ID, PublicID: row.PublicID, Name: row.Name, TriggerType: taskmodule.TriggerType(row.TriggerType), Version: row.Version, Enabled: row.Enabled, AreaID: row.AreaID, TeamID: row.TeamID, RequiredPositionCode: row.RequiredPositionCode, RequiredCapabilities: capabilities, PlannedOffsetSeconds: row.PlannedOffsetSeconds, DefaultMessage: row.DefaultMessage}, nil
}

type taskRow struct {
	ID                   uint64    `gorm:"column:id;primaryKey"`
	PublicID             string    `gorm:"column:public_id"`
	FlightID             uint64    `gorm:"column:flight_id"`
	TemplateID           uint64    `gorm:"column:template_id"`
	AreaID               uint64    `gorm:"column:area_id"`
	TeamID               uint64    `gorm:"column:team_id"`
	TriggerType          string    `gorm:"column:trigger_type"`
	GenerationKey        string    `gorm:"column:generation_key"`
	SourceEventID        string    `gorm:"column:source_event_id"`
	TemplateVersion      uint      `gorm:"column:template_version"`
	RequiredPositionCode string    `gorm:"-"`
	RequiredCapabilities []string  `gorm:"-"`
	Name                 string    `gorm:"column:task_name"`
	Message              string    `gorm:"column:message"`
	PlannedAt            time.Time `gorm:"column:planned_at"`
	Status               string    `gorm:"column:status"`
	StatusVersion        uint64    `gorm:"column:status_version"`
	SyncVersion          uint64    `gorm:"column:sync_version"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
}

func (taskRow) TableName() string { return "task_instance" }

func (row taskRow) toDomain() taskmodule.Instance {
	return taskmodule.Instance{ID: row.ID, PublicID: row.PublicID, FlightID: row.FlightID, TemplateID: row.TemplateID, AreaID: row.AreaID, TeamID: row.TeamID, TriggerType: taskmodule.TriggerType(row.TriggerType), GenerationKey: row.GenerationKey, SourceEventID: row.SourceEventID, TemplateVersion: row.TemplateVersion, RequiredPositionCode: row.RequiredPositionCode, RequiredCapabilities: append([]string(nil), row.RequiredCapabilities...), Name: row.Name, Message: row.Message, PlannedAt: row.PlannedAt.UTC(), Status: taskmodule.Status(row.Status), StatusVersion: row.StatusVersion, SyncVersion: row.SyncVersion}
}

type taskStatusHistoryRow struct {
	ID            uint64    `gorm:"column:id;primaryKey"`
	PublicID      string    `gorm:"column:public_id"`
	TaskID        uint64    `gorm:"column:task_id"`
	StatusVersion uint64    `gorm:"column:status_version"`
	FromStatus    *string   `gorm:"column:from_status"`
	ToStatus      string    `gorm:"column:to_status"`
	Reason        string    `gorm:"column:reason"`
	ActorType     string    `gorm:"column:actor_type"`
	ActorPublicID string    `gorm:"column:actor_public_id"`
	CommandID     string    `gorm:"column:command_id"`
	SourceEventID string    `gorm:"column:source_event_id"`
	OccurredAt    time.Time `gorm:"column:occurred_at"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (taskStatusHistoryRow) TableName() string { return "task_status_history" }

type taskCandidateRow struct {
	ID                         uint64    `gorm:"column:id;primaryKey"`
	PublicID                   string    `gorm:"column:public_id"`
	TaskID                     uint64    `gorm:"column:task_id"`
	PersonnelID                uint64    `gorm:"column:personnel_id"`
	Rank                       int       `gorm:"column:candidate_rank"`
	Status                     string    `gorm:"column:status"`
	MatchedPositionCode        string    `gorm:"column:matched_position_code"`
	MatchedCapabilities        []byte    `gorm:"column:matched_capabilities"`
	PersonnelWorkStateSnapshot string    `gorm:"column:personnel_work_state_snapshot"`
	PersonnelStateChangedAt    time.Time `gorm:"column:personnel_state_changed_at_snapshot"`
	CreatedAt                  time.Time `gorm:"column:created_at"`
	UpdatedAt                  time.Time `gorm:"column:updated_at"`
}

func (taskCandidateRow) TableName() string { return "task_candidate" }

type personnelCandidateRow struct {
	ID                 uint64    `gorm:"column:id"`
	PublicID           string    `gorm:"column:public_id"`
	UserPublicID       string    `gorm:"column:user_public_id"`
	TeamID             uint64    `gorm:"column:team_id"`
	AreaID             uint64    `gorm:"column:area_id"`
	PositionCode       string    `gorm:"column:position_code"`
	Capabilities       []byte    `gorm:"column:capabilities"`
	WorkState          string    `gorm:"column:work_state"`
	StatusVersion      uint64    `gorm:"column:status_version"`
	LastStateChangedAt time.Time `gorm:"column:last_state_changed_at"`
	Enabled            bool      `gorm:"column:enabled"`
}

func (row personnelCandidateRow) toDomain() (personnelmodule.CandidateRecord, error) {
	capabilities, err := decodeCapabilities(row.Capabilities)
	if err != nil {
		return personnelmodule.CandidateRecord{}, fmt.Errorf("decode personnel %s capabilities: %w", row.PublicID, err)
	}
	return personnelmodule.CandidateRecord{ID: row.ID, PublicID: row.PublicID, UserPublicID: row.UserPublicID, TeamID: row.TeamID, AreaID: row.AreaID, PositionCode: row.PositionCode, Capabilities: capabilities, WorkState: personnelmodule.WorkState(row.WorkState), LastStateChangedAt: row.LastStateChangedAt.UTC(), Enabled: row.Enabled}, nil
}

type businessIdempotencyRow struct {
	ID                uint64     `gorm:"column:id;primaryKey"`
	PublicID          string     `gorm:"column:public_id"`
	OperationType     string     `gorm:"column:operation_type"`
	IdempotencyKey    string     `gorm:"column:idempotency_key"`
	RequestHash       string     `gorm:"column:request_hash"`
	Status            string     `gorm:"column:status"`
	AggregateType     string     `gorm:"column:aggregate_type"`
	AggregatePublicID string     `gorm:"column:aggregate_public_id"`
	ActorType         string     `gorm:"column:actor_type"`
	ActorPublicID     string     `gorm:"column:actor_public_id"`
	CommandType       string     `gorm:"column:command_type"`
	ResultCode        string     `gorm:"column:result_code"`
	ResultStatus      string     `gorm:"column:result_status"`
	ResultPayload     []byte     `gorm:"column:result_payload"`
	ErrorSummary      string     `gorm:"column:error_summary"`
	RequestID         string     `gorm:"column:request_id"`
	TraceID           string     `gorm:"column:trace_id"`
	FirstProcessedAt  *time.Time `gorm:"column:first_processed_at"`
	LastProcessedAt   *time.Time `gorm:"column:last_processed_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (businessIdempotencyRow) TableName() string { return "business_idempotency_record" }

func (row businessIdempotencyRow) toDomain() flighttask.IdempotencyRecord {
	var firstProcessedAt, lastProcessedAt time.Time
	if row.FirstProcessedAt != nil {
		firstProcessedAt = row.FirstProcessedAt.UTC()
	}
	if row.LastProcessedAt != nil {
		lastProcessedAt = row.LastProcessedAt.UTC()
	}
	return flighttask.IdempotencyRecord{PublicID: row.PublicID, OperationType: row.OperationType, IdempotencyKey: row.IdempotencyKey, RequestHash: row.RequestHash, Status: row.Status, AggregateType: row.AggregateType, AggregatePublicID: row.AggregatePublicID, ActorType: row.ActorType, ActorPublicID: row.ActorPublicID, CommandType: row.CommandType, ResultCode: row.ResultCode, ResultStatus: row.ResultStatus, ResultPayload: append(json.RawMessage(nil), row.ResultPayload...), ErrorSummary: row.ErrorSummary, RequestID: row.RequestID, TraceID: row.TraceID, FirstProcessedAt: firstProcessedAt, LastProcessedAt: lastProcessedAt}
}

type auditRow struct {
	ID           uint64    `gorm:"column:id;primaryKey"`
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
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (auditRow) TableName() string { return "audit_log" }

type outboxRow struct {
	ID            uint64    `gorm:"column:id;primaryKey"`
	EventID       string    `gorm:"column:event_id"`
	EventType     string    `gorm:"column:event_type"`
	SchemaVersion int       `gorm:"column:schema_version"`
	AggregateType string    `gorm:"column:aggregate_type"`
	AggregateID   string    `gorm:"column:aggregate_id"`
	OccurredAt    time.Time `gorm:"column:occurred_at"`
	Producer      string    `gorm:"column:producer"`
	CorrelationID string    `gorm:"column:correlation_id"`
	TraceID       string    `gorm:"column:trace_id"`
	Payload       []byte    `gorm:"column:payload"`
	Status        string    `gorm:"column:status"`
	Attempts      int       `gorm:"column:attempts"`
	NextAttemptAt time.Time `gorm:"column:next_attempt_at"`
	LastError     *string   `gorm:"column:last_error"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (outboxRow) TableName() string { return "outbox_event" }

func decodeCapabilities(raw []byte) ([]string, error) {
	if strings.TrimSpace(string(raw)) == "" {
		return nil, fmt.Errorf("capabilities JSON is empty")
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("capabilities contain an empty value")
		}
	}
	return values, nil
}
