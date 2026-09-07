package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	coreoperations "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/operations"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"gorm.io/gorm"
)

var _ coreoperations.Repository = (*OperationsRepository)(nil)

type OperationsRepository struct{ db *gorm.DB }

func NewOperationsRepository(db *gorm.DB) *OperationsRepository {
	return &OperationsRepository{db: db}
}

func (r *OperationsRepository) ListPersonnelStatus(ctx context.Context, filter coreoperations.StatusFilter) ([]coreoperations.PersonnelStatus, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, coreoperations.ErrRepositoryNotConfigured
	}
	query := r.db.WithContext(ctx).Table("personnel AS p").Joins("JOIN team AS t ON t.id = p.team_id").Joins("JOIN operation_area AS oa ON oa.id = t.area_id")
	query = applyPersonnelScope(query, filter.Scope)
	if filter.WorkState != "" {
		query = query.Where("p.work_state = ?", filter.WorkState)
	}
	if filter.TeamPublicID != "" {
		query = query.Where("t.public_id = ?", filter.TeamPublicID)
	}
	if filter.AreaPublicID != "" {
		query = query.Where("oa.public_id = ?", filter.AreaPublicID)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count personnel status: %w", err)
	}
	var rows []operationsPersonnelStatusRow
	if err := query.Select(`p.public_id, p.employee_no, p.display_name, oa.public_id AS area_public_id, oa.name AS area_name,
		t.public_id AS team_public_id, t.name AS team_name, p.position_code, p.capabilities, p.work_state,
		p.status_version, p.last_state_changed_at, p.unavailable_reason, p.enabled`).
		Order("p.work_state ASC, p.display_name ASC, p.public_id ASC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list personnel status: %w", err)
	}
	items := make([]coreoperations.PersonnelStatus, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDomain())
	}
	return items, total, nil
}

func (r *OperationsRepository) ListPersonnelStatusHistory(ctx context.Context, filter coreoperations.StatusHistoryFilter) ([]coreoperations.StatusHistory, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, coreoperations.ErrRepositoryNotConfigured
	}
	query := r.db.WithContext(ctx).Table("personnel_status_history AS h").Joins("JOIN personnel AS p ON p.id = h.personnel_id").Joins("JOIN team AS t ON t.id = p.team_id").Joins("JOIN operation_area AS oa ON oa.id = t.area_id").Where("p.public_id = ?", filter.PersonnelPublicID)
	query = applyPersonnelScope(query, filter.Scope)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count personnel status history: %w", err)
	}
	var rows []operationsStatusHistoryRow
	if err := query.Select(`h.public_id, p.public_id AS personnel_public_id, h.status_version, h.from_state, h.to_state,
		h.reason, h.actor_type, h.actor_public_id, h.assignment_id, h.command_id, h.occurred_at`).
		Order("h.occurred_at DESC, h.id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list personnel status history: %w", err)
	}
	items := make([]coreoperations.StatusHistory, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDomain())
	}
	return items, total, nil
}

func (r *OperationsRepository) ListEvents(ctx context.Context, filter coreoperations.EventFilter) ([]coreoperations.Event, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, coreoperations.ErrRepositoryNotConfigured
	}
	query := r.db.WithContext(ctx).Table("outbox_event AS e").
		Joins("LEFT JOIN task_instance AS ti ON e.aggregate_type = 'task' AND ti.public_id = e.aggregate_id").
		Joins("LEFT JOIN flight AS f ON (e.aggregate_type = 'flight' AND f.public_id = e.aggregate_id) OR (e.aggregate_type = 'task' AND f.id = ti.flight_id)")
	query = applyEventScope(query, filter.Scope)
	if filter.EventType != "" {
		query = query.Where("e.event_type = ?", filter.EventType)
	}
	if filter.Status != "" {
		query = query.Where("e.status = ?", filter.Status)
	}
	if filter.FlightPublicID != "" {
		query = query.Where("f.public_id = ?", filter.FlightPublicID)
	}
	if !filter.From.IsZero() {
		query = query.Where("e.occurred_at >= ?", filter.From.UTC())
	}
	if !filter.To.IsZero() {
		query = query.Where("e.occurred_at <= ?", filter.To.UTC())
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count operations events: %w", err)
	}
	var rows []operationsEventRow
	if err := query.Select(`e.event_id AS public_id, e.event_type, e.status, e.aggregate_type, e.aggregate_id AS aggregate_public_id,
		f.public_id AS flight_public_id, f.flight_display_no, e.producer AS source, e.occurred_at, e.last_error`).
		Order("e.occurred_at DESC, e.id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list operations events: %w", err)
	}
	items := make([]coreoperations.Event, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDomain())
	}
	return items, total, nil
}

func (r *OperationsRepository) ListAudit(ctx context.Context, filter coreoperations.AuditFilter) ([]coreoperations.AuditEntry, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, coreoperations.ErrRepositoryNotConfigured
	}
	query := r.db.WithContext(ctx).Table("audit_log")
	if filter.ActorID != "" {
		query = query.Where("actor_id = ?", filter.ActorID)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.ResourceType != "" {
		query = query.Where("resource_type = ?", filter.ResourceType)
	}
	if !filter.From.IsZero() {
		query = query.Where("occurred_at >= ?", filter.From.UTC())
	}
	if !filter.To.IsZero() {
		query = query.Where("occurred_at <= ?", filter.To.UTC())
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count audit records: %w", err)
	}
	var rows []operationsAuditRow
	if err := query.Select("id, actor_type, actor_id, action, resource_type, resource_id, result, request_id, trace_id, source_ip, occurred_at").Order("occurred_at DESC, id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list audit records: %w", err)
	}
	items := make([]coreoperations.AuditEntry, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDomain())
	}
	return items, total, nil
}

func (r *OperationsRepository) GetDiagnostics(ctx context.Context) (coreoperations.SyncDiagnostics, error) {
	if r == nil || r.db == nil {
		return coreoperations.SyncDiagnostics{}, coreoperations.ErrRepositoryNotConfigured
	}
	var result coreoperations.SyncDiagnostics
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM flight_source_inbox WHERE status IN ?", []string{"pending", "processing"}).Scan(&result.FlightSourcePending).Error; err != nil {
		return coreoperations.SyncDiagnostics{}, fmt.Errorf("count flight source pending: %w", err)
	}
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM flight_source_inbox WHERE status = ?", "retry").Scan(&result.FlightSourceRetry).Error; err != nil {
		return coreoperations.SyncDiagnostics{}, fmt.Errorf("count flight source retry: %w", err)
	}
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM flight_source_inbox WHERE status = ?", "failed").Scan(&result.FlightSourceFailed).Error; err != nil {
		return coreoperations.SyncDiagnostics{}, fmt.Errorf("count flight source failed: %w", err)
	}
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM outbox_event WHERE status IN ?", []string{"pending", "retry", "processing"}).Scan(&result.OutboxPending).Error; err != nil {
		return coreoperations.SyncDiagnostics{}, fmt.Errorf("count outbox pending: %w", err)
	}
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM outbox_event WHERE status = ?", "failed").Scan(&result.OutboxFailed).Error; err != nil {
		return coreoperations.SyncDiagnostics{}, fmt.Errorf("count outbox failed: %w", err)
	}
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM core_inbox WHERE status = ?", "failed").Scan(&result.CoreInboxFailed).Error; err != nil {
		return coreoperations.SyncDiagnostics{}, fmt.Errorf("count core inbox failed: %w", err)
	}
	return result, nil
}

func applyEventScope(query *gorm.DB, scope security.AccessScope) *gorm.DB {
	if scope.Global {
		return query
	}
	if scope.UserID != 0 {
		return query.Where(`
			(e.aggregate_type = 'task' AND EXISTS (
				SELECT 1 FROM task_instance AS scope_task
				JOIN task_assignment AS scope_assignment ON scope_assignment.task_id = scope_task.id
				WHERE scope_task.public_id = e.aggregate_id AND scope_assignment.personnel_id = ?
			)) OR
			(e.aggregate_type = 'flight' AND EXISTS (
				SELECT 1 FROM task_instance AS scope_task
				JOIN task_assignment AS scope_assignment ON scope_assignment.task_id = scope_task.id
				WHERE scope_task.flight_id = f.id AND scope_assignment.personnel_id = ?
			))`, scope.UserID, scope.UserID)
	}
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if len(scope.TeamIDs) > 0 {
		conditions = append(conditions, `(e.aggregate_type = 'task' AND ti.team_id IN ?) OR
			(e.aggregate_type = 'flight' AND EXISTS (
				SELECT 1 FROM task_instance AS scope_task
				WHERE scope_task.flight_id = f.id AND scope_task.team_id IN ?
			))`)
		args = append(args, scope.TeamIDs, scope.TeamIDs)
	}
	if len(scope.AreaIDs) > 0 {
		conditions = append(conditions, `(e.aggregate_type = 'task' AND ti.area_id IN ?) OR
			(e.aggregate_type = 'flight' AND EXISTS (
				SELECT 1 FROM task_instance AS scope_task
				WHERE scope_task.flight_id = f.id AND scope_task.area_id IN ?
			))`)
		args = append(args, scope.AreaIDs, scope.AreaIDs)
	}
	if len(conditions) == 0 {
		return query.Where("1 = 0")
	}
	return query.Where("("+stringsJoin(conditions, " OR ")+")", args...)
}

func stringsJoin(values []string, separator string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += separator
		}
		result += value
	}
	return result
}

type operationsPersonnelStatusRow struct {
	PublicID           string    `gorm:"column:public_id"`
	EmployeeNo         string    `gorm:"column:employee_no"`
	DisplayName        string    `gorm:"column:display_name"`
	AreaPublicID       string    `gorm:"column:area_public_id"`
	AreaName           string    `gorm:"column:area_name"`
	TeamPublicID       string    `gorm:"column:team_public_id"`
	TeamName           string    `gorm:"column:team_name"`
	PositionCode       string    `gorm:"column:position_code"`
	Capabilities       []byte    `gorm:"column:capabilities"`
	WorkState          string    `gorm:"column:work_state"`
	StatusVersion      uint64    `gorm:"column:status_version"`
	LastStateChangedAt time.Time `gorm:"column:last_state_changed_at"`
	UnavailableReason  string    `gorm:"column:unavailable_reason"`
	Enabled            bool      `gorm:"column:enabled"`
}

func (row operationsPersonnelStatusRow) toDomain() coreoperations.PersonnelStatus {
	var capabilities []string
	_ = json.Unmarshal(row.Capabilities, &capabilities)
	capability := ""
	if len(capabilities) > 0 {
		capability = capabilities[0]
	}
	return coreoperations.PersonnelStatus{PublicID: row.PublicID, EmployeeNo: row.EmployeeNo, DisplayName: row.DisplayName, AreaPublicID: row.AreaPublicID, AreaName: row.AreaName, TeamPublicID: row.TeamPublicID, TeamName: row.TeamName, PositionCode: row.PositionCode, CapabilityCode: capability, WorkState: row.WorkState, StatusVersion: row.StatusVersion, LastStateChangedAt: row.LastStateChangedAt.UTC().Format(time.RFC3339Nano), UnavailableReason: row.UnavailableReason, Enabled: row.Enabled}
}

type operationsStatusHistoryRow struct {
	PublicID          string    `gorm:"column:public_id"`
	PersonnelPublicID string    `gorm:"column:personnel_public_id"`
	StatusVersion     uint64    `gorm:"column:status_version"`
	FromState         *string   `gorm:"column:from_state"`
	ToState           string    `gorm:"column:to_state"`
	Reason            string    `gorm:"column:reason"`
	ActorType         string    `gorm:"column:actor_type"`
	ActorPublicID     string    `gorm:"column:actor_public_id"`
	AssignmentID      *uint64   `gorm:"column:assignment_id"`
	CommandID         string    `gorm:"column:command_id"`
	OccurredAt        time.Time `gorm:"column:occurred_at"`
}

func (row operationsStatusHistoryRow) toDomain() coreoperations.StatusHistory {
	return coreoperations.StatusHistory{PublicID: row.PublicID, PersonnelPublicID: row.PersonnelPublicID, StatusVersion: row.StatusVersion, FromState: row.FromState, ToState: row.ToState, Reason: row.Reason, ActorType: row.ActorType, ActorPublicID: row.ActorPublicID, AssignmentID: row.AssignmentID, CommandID: row.CommandID, OccurredAt: row.OccurredAt.UTC().Format(time.RFC3339Nano)}
}

type operationsEventRow struct {
	PublicID          string    `gorm:"column:public_id"`
	EventType         string    `gorm:"column:event_type"`
	Status            string    `gorm:"column:status"`
	AggregateType     string    `gorm:"column:aggregate_type"`
	AggregatePublicID string    `gorm:"column:aggregate_public_id"`
	FlightPublicID    string    `gorm:"column:flight_public_id"`
	FlightDisplayNo   string    `gorm:"column:flight_display_no"`
	Source            string    `gorm:"column:source"`
	OccurredAt        time.Time `gorm:"column:occurred_at"`
	LastError         string    `gorm:"column:last_error"`
}

func (row operationsEventRow) toDomain() coreoperations.Event {
	return coreoperations.Event{PublicID: row.PublicID, EventType: row.EventType, Status: row.Status, AggregateType: row.AggregateType, AggregatePublicID: row.AggregatePublicID, FlightPublicID: row.FlightPublicID, FlightDisplayNo: row.FlightDisplayNo, Source: row.Source, OccurredAt: row.OccurredAt.UTC().Format(time.RFC3339Nano), LastError: row.LastError}
}

type operationsAuditRow struct {
	ID           uint64    `gorm:"column:id"`
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

func (row operationsAuditRow) toDomain() coreoperations.AuditEntry {
	return coreoperations.AuditEntry{ID: row.ID, ActorType: row.ActorType, ActorID: row.ActorID, Action: row.Action, ResourceType: row.ResourceType, ResourceID: row.ResourceID, Result: row.Result, RequestID: row.RequestID, TraceID: row.TraceID, SourceIP: row.SourceIP, OccurredAt: row.OccurredAt.UTC().Format(time.RFC3339Nano)}
}
