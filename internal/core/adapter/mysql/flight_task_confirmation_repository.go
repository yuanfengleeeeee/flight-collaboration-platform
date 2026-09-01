package mysql

import (
	"context"
	"fmt"
	"time"

	flighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ flighttask.ConfirmationRepository = (*Repository)(nil)

func (r *Repository) WithinConfirmationTransaction(ctx context.Context, fn func(flighttask.ConfirmationTransaction) error) error {
	if r == nil || r.db == nil || fn == nil {
		return flighttask.ErrRepositoryNotConfigured
	}
	if ctx == nil {
		return fmt.Errorf("task confirmation transaction context is nil")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&transaction{db: tx})
	})
}

func (tx *transaction) FindTaskForUpdate(ctx context.Context, publicID string) (taskmodule.Instance, error) {
	var row taskConfirmationRow
	err := tx.db.WithContext(ctx).
		Table("task_instance AS ti").
		Select(`ti.id, ti.public_id, ti.flight_id, ti.template_id, ti.area_id, ti.team_id,
			ti.trigger_type, ti.generation_key, ti.source_event_id, ti.template_version,
			ti.task_name, ti.message, ti.planned_at, ti.status, ti.status_version, ti.sync_version,
			f.public_id AS flight_public_id, f.flight_display_no,
			oa.name AS area_name, tt.required_position_code, tt.required_capabilities`).
		Joins("JOIN flight AS f ON f.id = ti.flight_id").
		Joins("JOIN operation_area AS oa ON oa.id = ti.area_id").
		Joins("JOIN task_template AS tt ON tt.id = ti.template_id").
		Where("ti.public_id = ?", publicID).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&row).Error
	if err != nil {
		return taskmodule.Instance{}, normalizeNotFound(err)
	}
	return row.toDomain()
}

func (tx *transaction) FindCandidateForUpdate(ctx context.Context, taskID uint64, publicID string) (taskmodule.Candidate, error) {
	var row taskConfirmationCandidateRow
	err := tx.db.WithContext(ctx).
		Table("task_candidate AS tc").
		Select(`tc.id, tc.public_id, tc.task_id, tc.personnel_id, tc.candidate_rank, tc.status,
			tc.matched_position_code, tc.matched_capabilities, tc.personnel_work_state_snapshot,
			tc.personnel_state_changed_at_snapshot, tc.rejection_reason, tc.selected_at, tc.invalidated_at,
			p.public_id AS personnel_public_id`).
		Joins("JOIN personnel AS p ON p.id = tc.personnel_id").
		Where("tc.task_id = ? AND tc.public_id = ?", taskID, publicID).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&row).Error
	if err != nil {
		return taskmodule.Candidate{}, normalizeNotFound(err)
	}
	return row.toDomain()
}

func (tx *transaction) FindPersonnelForUpdate(ctx context.Context, personnelID uint64) (personnelmodule.CandidateRecord, error) {
	var row personnelConfirmationRow
	err := tx.db.WithContext(ctx).
		Table("personnel AS p").
		Select(`p.id, p.public_id, p.user_public_id, p.team_id, t.area_id, p.position_code,
			p.capabilities, p.work_state, p.status_version, p.last_state_changed_at, p.enabled`).
		Joins("JOIN team AS t ON t.id = p.team_id").
		Joins("JOIN operation_area AS oa ON oa.id = t.area_id").
		Where("p.id = ?", personnelID).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&row).Error
	if err != nil {
		return personnelmodule.CandidateRecord{}, normalizeNotFound(err)
	}
	return row.toDomain()
}

func (tx *transaction) HasPersonnelScheduleConflict(ctx context.Context, personnelID uint64, plannedAt time.Time) (bool, error) {
	var count int64
	err := tx.db.WithContext(ctx).
		Table("task_assignment AS a").
		Joins("JOIN task_instance AS ti ON ti.id = a.task_id").
		Where("a.personnel_id = ? AND a.status IN (?, ?) AND ti.planned_at = ?", personnelID, string(taskmodule.AssignmentConfirmed), string(taskmodule.AssignmentAccepted), plannedAt.UTC()).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("count personnel assignment conflicts: %w", err)
	}
	return count > 0, nil
}

func (tx *transaction) ReservePersonnel(ctx context.Context, personnelID, expectedStatusVersion uint64, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Table("personnel").
		Where("id = ? AND work_state = ? AND status_version = ?", personnelID, string(personnelmodule.WorkStateIdle), expectedStatusVersion).
		Updates(map[string]any{
			"work_state":            string(personnelmodule.WorkStateReserved),
			"status_version":        expectedStatusVersion + 1,
			"last_state_changed_at": changedAt.UTC(),
			"updated_at":            changedAt.UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update personnel reserved: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return flighttask.ErrPersonnelReservationConflict
	}
	return nil
}

func (tx *transaction) InvalidateCandidate(ctx context.Context, candidateID uint64, reason string, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Table("task_candidate").
		Where("id = ?", candidateID).
		Updates(map[string]any{"status": string(taskmodule.CandidateInvalidated), "rejection_reason": reason, "invalidated_at": changedAt.UTC(), "updated_at": changedAt.UTC()})
	if result.Error != nil {
		return fmt.Errorf("update candidate invalidated: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("candidate %d invalidation did not affect one row", candidateID)
	}
	return nil
}

func (tx *transaction) MarkCandidatesConfirmed(ctx context.Context, taskID, selectedCandidateID uint64, changedAt time.Time) error {
	selected := tx.db.WithContext(ctx).Table("task_candidate").
		Where("id = ? AND task_id = ? AND status = ?", selectedCandidateID, taskID, string(taskmodule.CandidateProposed)).
		Updates(map[string]any{"status": string(taskmodule.CandidateSelected), "selected_at": changedAt.UTC(), "updated_at": changedAt.UTC()})
	if selected.Error != nil {
		return fmt.Errorf("select task candidate: %w", selected.Error)
	}
	if selected.RowsAffected != 1 {
		return flighttask.ErrCandidateNoLongerEligible
	}
	rejected := tx.db.WithContext(ctx).Table("task_candidate").
		Where("task_id = ? AND status = ?", taskID, string(taskmodule.CandidateProposed)).
		Updates(map[string]any{"status": string(taskmodule.CandidateRejected), "rejection_reason": "not_selected", "updated_at": changedAt.UTC()})
	if rejected.Error != nil {
		return fmt.Errorf("reject unselected task candidates: %w", rejected.Error)
	}
	return nil
}

func (tx *transaction) UpdateTaskAssigned(ctx context.Context, taskID, expectedStatusVersion uint64, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Table("task_instance").
		Where("id = ? AND status = ? AND status_version = ?", taskID, string(taskmodule.StatusAwaitingConfirmation), expectedStatusVersion).
		Updates(map[string]any{"status": string(taskmodule.StatusAssigned), "status_version": expectedStatusVersion + 1, "sync_version": clause.Expr{SQL: "sync_version + 1"}, "updated_at": changedAt.UTC()})
	if result.Error != nil {
		return fmt.Errorf("update task assigned: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return flighttask.ErrTaskVersionConflict
	}
	return nil
}

func (tx *transaction) CreateAssignment(ctx context.Context, value *taskmodule.Assignment) error {
	if value == nil {
		return fmt.Errorf("task assignment is nil")
	}
	now := time.Now().UTC()
	row := taskAssignmentRow{PublicID: value.PublicID, TaskID: value.TaskID, CandidateID: value.CandidateID, PersonnelID: value.PersonnelID, Status: string(value.Status), StatusVersion: value.StatusVersion, ConfirmationID: value.ConfirmationID, ConfirmedByPublicID: value.ConfirmedByPublicID, ConfirmedAt: value.ConfirmedAt.UTC(), CreatedAt: now, UpdatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	value.ID = row.ID
	return nil
}

func (tx *transaction) CreateAssignmentStatusHistory(ctx context.Context, value taskmodule.AssignmentStatusHistory) error {
	var fromStatus *string
	if value.FromStatus != nil {
		from := string(*value.FromStatus)
		fromStatus = &from
	}
	now := time.Now().UTC()
	row := taskAssignmentStatusHistoryRow{PublicID: value.PublicID, AssignmentID: value.AssignmentID, StatusVersion: value.StatusVersion, FromStatus: fromStatus, ToStatus: string(value.ToStatus), Reason: value.Reason, ActorType: value.ActorType, ActorPublicID: value.ActorPublicID, CommandID: value.CommandID, ConfirmationID: value.ConfirmationID, CancellationID: value.CancellationID, OccurredAt: value.OccurredAt.UTC(), CreatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return nil
}

func (tx *transaction) CreatePersonnelStatusHistory(ctx context.Context, value personnelmodule.StatusHistory) error {
	var fromState *string
	if value.FromState != nil {
		from := string(*value.FromState)
		fromState = &from
	}
	now := time.Now().UTC()
	row := personnelStatusHistoryRow{PublicID: value.PublicID, PersonnelID: value.PersonnelID, StatusVersion: value.StatusVersion, FromState: fromState, ToState: string(value.ToState), Reason: value.Reason, ActorType: value.ActorType, ActorPublicID: value.ActorPublicID, AssignmentID: nullableUint64(value.AssignmentID), CommandID: value.CommandID, OccurredAt: value.OccurredAt.UTC(), CreatedAt: now}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return nil
}

type taskConfirmationRow struct {
	ID                   uint64    `gorm:"column:id"`
	PublicID             string    `gorm:"column:public_id"`
	FlightID             uint64    `gorm:"column:flight_id"`
	FlightPublicID       string    `gorm:"column:flight_public_id"`
	FlightDisplayNo      string    `gorm:"column:flight_display_no"`
	TemplateID           uint64    `gorm:"column:template_id"`
	TemplatePublicID     string    `gorm:"column:template_public_id"`
	AreaID               uint64    `gorm:"column:area_id"`
	AreaPublicID         string    `gorm:"column:area_public_id"`
	AreaName             string    `gorm:"column:area_name"`
	TeamID               uint64    `gorm:"column:team_id"`
	TeamPublicID         string    `gorm:"column:team_public_id"`
	TriggerType          string    `gorm:"column:trigger_type"`
	GenerationKey        string    `gorm:"column:generation_key"`
	SourceEventID        string    `gorm:"column:source_event_id"`
	TemplateVersion      uint      `gorm:"column:template_version"`
	Name                 string    `gorm:"column:task_name"`
	Message              string    `gorm:"column:message"`
	PlannedAt            time.Time `gorm:"column:planned_at"`
	Status               string    `gorm:"column:status"`
	StatusVersion        uint64    `gorm:"column:status_version"`
	SyncVersion          uint64    `gorm:"column:sync_version"`
	RequiredPositionCode string    `gorm:"column:required_position_code"`
	RequiredCapabilities []byte    `gorm:"column:required_capabilities"`
}

func (row taskConfirmationRow) toDomain() (taskmodule.Instance, error) {
	capabilities, err := decodeCapabilities(row.RequiredCapabilities)
	if err != nil {
		return taskmodule.Instance{}, fmt.Errorf("decode task %s required capabilities: %w", row.PublicID, err)
	}
	return taskmodule.Instance{ID: row.ID, PublicID: row.PublicID, FlightID: row.FlightID, FlightPublicID: row.FlightPublicID, FlightDisplayNo: row.FlightDisplayNo, TemplateID: row.TemplateID, AreaID: row.AreaID, AreaName: row.AreaName, TeamID: row.TeamID, TriggerType: taskmodule.TriggerType(row.TriggerType), GenerationKey: row.GenerationKey, SourceEventID: row.SourceEventID, TemplateVersion: row.TemplateVersion, RequiredPositionCode: row.RequiredPositionCode, RequiredCapabilities: capabilities, Name: row.Name, Message: row.Message, PlannedAt: row.PlannedAt.UTC(), Status: taskmodule.Status(row.Status), StatusVersion: row.StatusVersion, SyncVersion: row.SyncVersion}, nil
}

type taskConfirmationCandidateRow struct {
	ID                         uint64     `gorm:"column:id"`
	PublicID                   string     `gorm:"column:public_id"`
	TaskID                     uint64     `gorm:"column:task_id"`
	PersonnelID                uint64     `gorm:"column:personnel_id"`
	Rank                       int        `gorm:"column:candidate_rank"`
	Status                     string     `gorm:"column:status"`
	MatchedPositionCode        string     `gorm:"column:matched_position_code"`
	MatchedCapabilities        []byte     `gorm:"column:matched_capabilities"`
	PersonnelWorkStateSnapshot string     `gorm:"column:personnel_work_state_snapshot"`
	PersonnelStateChangedAt    time.Time  `gorm:"column:personnel_state_changed_at_snapshot"`
	RejectionReason            string     `gorm:"column:rejection_reason"`
	SelectedAt                 *time.Time `gorm:"column:selected_at"`
	InvalidatedAt              *time.Time `gorm:"column:invalidated_at"`
	PersonnelPublicID          string     `gorm:"column:personnel_public_id"`
}

func (row taskConfirmationCandidateRow) toDomain() (taskmodule.Candidate, error) {
	capabilities, err := decodeCapabilities(row.MatchedCapabilities)
	if err != nil {
		return taskmodule.Candidate{}, fmt.Errorf("decode candidate %s capabilities: %w", row.PublicID, err)
	}
	return taskmodule.Candidate{ID: row.ID, PublicID: row.PublicID, TaskID: row.TaskID, PersonnelID: row.PersonnelID, PersonnelPublicID: row.PersonnelPublicID, Rank: row.Rank, Status: taskmodule.CandidateStatus(row.Status), MatchedPositionCode: row.MatchedPositionCode, MatchedCapabilities: capabilities, PersonnelWorkStateSnapshot: row.PersonnelWorkStateSnapshot, PersonnelStateChangedAt: row.PersonnelStateChangedAt.UTC(), RejectionReason: row.RejectionReason, SelectedAt: row.SelectedAt, InvalidatedAt: row.InvalidatedAt}, nil
}

type personnelConfirmationRow struct {
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

func (row personnelConfirmationRow) toDomain() (personnelmodule.CandidateRecord, error) {
	capabilities, err := decodeCapabilities(row.Capabilities)
	if err != nil {
		return personnelmodule.CandidateRecord{}, fmt.Errorf("decode personnel %s capabilities: %w", row.PublicID, err)
	}
	return personnelmodule.CandidateRecord{ID: row.ID, PublicID: row.PublicID, UserPublicID: row.UserPublicID, TeamID: row.TeamID, AreaID: row.AreaID, PositionCode: row.PositionCode, Capabilities: capabilities, WorkState: personnelmodule.WorkState(row.WorkState), StatusVersion: row.StatusVersion, LastStateChangedAt: row.LastStateChangedAt.UTC(), Enabled: row.Enabled}, nil
}

type taskAssignmentRow struct {
	ID                  uint64    `gorm:"column:id;primaryKey"`
	PublicID            string    `gorm:"column:public_id"`
	TaskID              uint64    `gorm:"column:task_id"`
	CandidateID         uint64    `gorm:"column:candidate_id"`
	PersonnelID         uint64    `gorm:"column:personnel_id"`
	Status              string    `gorm:"column:status"`
	StatusVersion       uint64    `gorm:"column:status_version"`
	ConfirmationID      string    `gorm:"column:confirmation_id"`
	ConfirmedByPublicID string    `gorm:"column:confirmed_by_public_id"`
	ConfirmedAt         time.Time `gorm:"column:confirmed_at"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (taskAssignmentRow) TableName() string { return "task_assignment" }

type taskAssignmentStatusHistoryRow struct {
	ID             uint64    `gorm:"column:id;primaryKey"`
	PublicID       string    `gorm:"column:public_id"`
	AssignmentID   uint64    `gorm:"column:assignment_id"`
	StatusVersion  uint64    `gorm:"column:status_version"`
	FromStatus     *string   `gorm:"column:from_status"`
	ToStatus       string    `gorm:"column:to_status"`
	Reason         string    `gorm:"column:reason"`
	ActorType      string    `gorm:"column:actor_type"`
	ActorPublicID  string    `gorm:"column:actor_public_id"`
	CommandID      string    `gorm:"column:command_id"`
	ConfirmationID string    `gorm:"column:confirmation_id"`
	CancellationID string    `gorm:"column:cancellation_id"`
	OccurredAt     time.Time `gorm:"column:occurred_at"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (taskAssignmentStatusHistoryRow) TableName() string { return "task_assignment_status_history" }

type personnelStatusHistoryRow struct {
	ID            uint64    `gorm:"column:id;primaryKey"`
	PublicID      string    `gorm:"column:public_id"`
	PersonnelID   uint64    `gorm:"column:personnel_id"`
	StatusVersion uint64    `gorm:"column:status_version"`
	FromState     *string   `gorm:"column:from_state"`
	ToState       string    `gorm:"column:to_state"`
	Reason        string    `gorm:"column:reason"`
	ActorType     string    `gorm:"column:actor_type"`
	ActorPublicID string    `gorm:"column:actor_public_id"`
	AssignmentID  *uint64   `gorm:"column:assignment_id"`
	CommandID     string    `gorm:"column:command_id"`
	OccurredAt    time.Time `gorm:"column:occurred_at"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (personnelStatusHistoryRow) TableName() string { return "personnel_status_history" }

func nullableUint64(value uint64) *uint64 {
	if value == 0 {
		return nil
	}
	return &value
}
