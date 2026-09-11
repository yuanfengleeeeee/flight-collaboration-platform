package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	flighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"gorm.io/gorm"
)

var _ flighttask.TaskQueryRepository = (*Repository)(nil)

func (r *Repository) ListTaskReadModels(ctx context.Context, filter flighttask.TaskQueryFilter) ([]flighttask.TaskReadModel, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, flighttask.ErrRepositoryNotConfigured
	}
	query := r.taskReadQuery(ctx, filter)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}
	var rows []taskConfirmationRow
	if err := query.Order("ti.planned_at ASC, ti.public_id ASC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("query tasks: %w", err)
	}
	models := make([]flighttask.TaskReadModel, 0, len(rows))
	for _, row := range rows {
		model, err := r.taskReadModel(ctx, row)
		if err != nil {
			return nil, 0, err
		}
		models = append(models, model)
	}
	return models, total, nil
}

func (r *Repository) FindTaskReadModel(ctx context.Context, publicID string, scope security.AccessScope) (flighttask.TaskReadModel, error) {
	if r == nil || r.db == nil {
		return flighttask.TaskReadModel{}, flighttask.ErrRepositoryNotConfigured
	}
	var row taskConfirmationRow
	filter := flighttask.TaskQueryFilter{Scope: scope}
	if err := r.taskReadQuery(ctx, filter).Where("ti.public_id = ?", publicID).First(&row).Error; err != nil {
		return flighttask.TaskReadModel{}, normalizeNotFound(err)
	}
	return r.taskReadModel(ctx, row)
}

func (r *Repository) FindTaskHistory(ctx context.Context, publicID string, scope security.AccessScope) (flighttask.TaskHistoryReadModel, error) {
	if r == nil || r.db == nil {
		return flighttask.TaskHistoryReadModel{}, flighttask.ErrRepositoryNotConfigured
	}
	var taskRow taskConfirmationRow
	if err := r.taskReadQuery(ctx, flighttask.TaskQueryFilter{Scope: scope}).Where("ti.public_id = ?", publicID).First(&taskRow).Error; err != nil {
		return flighttask.TaskHistoryReadModel{}, normalizeNotFound(err)
	}
	task, err := taskRow.toDomain()
	if err != nil {
		return flighttask.TaskHistoryReadModel{}, err
	}
	result := flighttask.TaskHistoryReadModel{TaskPublicID: task.PublicID, CurrentStatus: task.Status, CurrentStatusVersion: task.StatusVersion, TaskHistories: make([]taskmodule.StatusHistory, 0), AssignmentHistories: make([]flighttask.AssignmentHistoryReadModel, 0), PersonnelHistories: make([]flighttask.PersonnelHistoryReadModel, 0)}

	var taskRows []taskStatusHistoryQueryRow
	if err := r.db.WithContext(ctx).Where("task_id = ?", task.ID).Order("occurred_at ASC, id ASC").Find(&taskRows).Error; err != nil {
		return flighttask.TaskHistoryReadModel{}, fmt.Errorf("query task history %s: %w", publicID, err)
	}
	for _, row := range taskRows {
		result.TaskHistories = append(result.TaskHistories, row.toDomain())
	}

	var assignmentRows []assignmentStatusHistoryQueryRow
	if err := r.db.WithContext(ctx).
		Table("task_assignment_status_history AS h").
		Select(`h.public_id, h.assignment_id, h.status_version, h.from_status, h.to_status,
			h.reason, h.actor_type, h.actor_public_id, h.command_id, h.confirmation_id,
			h.cancellation_id, h.occurred_at, a.public_id AS assignment_public_id,
			p.public_id AS personnel_public_id`).
		Joins("JOIN task_assignment AS a ON a.id = h.assignment_id").
		Joins("JOIN personnel AS p ON p.id = a.personnel_id").
		Where("a.task_id = ?", task.ID).
		Order("h.occurred_at ASC, h.id ASC").
		Find(&assignmentRows).Error; err != nil {
		return flighttask.TaskHistoryReadModel{}, fmt.Errorf("query assignment history %s: %w", publicID, err)
	}
	for _, row := range assignmentRows {
		result.AssignmentHistories = append(result.AssignmentHistories, flighttask.AssignmentHistoryReadModel{AssignmentPublicID: row.AssignmentPublicID, PersonnelPublicID: row.PersonnelPublicID, History: row.toDomain()})
	}

	var personnelRows []personnelStatusHistoryQueryRow
	if err := r.db.WithContext(ctx).
		Table("personnel_status_history AS h").
		Select(`h.public_id, h.personnel_id, h.status_version, h.from_state, h.to_state,
			h.reason, h.actor_type, h.actor_public_id, h.assignment_id, h.command_id,
			h.occurred_at, p.public_id AS personnel_public_id`).
		Joins("JOIN personnel AS p ON p.id = h.personnel_id").
		Joins("JOIN task_assignment AS a ON a.id = h.assignment_id").
		Where("a.task_id = ?", task.ID).
		Order("h.occurred_at ASC, h.id ASC").
		Find(&personnelRows).Error; err != nil {
		return flighttask.TaskHistoryReadModel{}, fmt.Errorf("query personnel history %s: %w", publicID, err)
	}
	for _, row := range personnelRows {
		result.PersonnelHistories = append(result.PersonnelHistories, flighttask.PersonnelHistoryReadModel{PersonnelPublicID: row.PersonnelPublicID, History: row.toDomain()})
	}
	return result, nil
}

func (r *Repository) taskReadQuery(ctx context.Context, filter flighttask.TaskQueryFilter) *gorm.DB {
	query := r.db.WithContext(ctx).
		Table("task_instance AS ti").
		Select(`ti.id, ti.public_id, ti.flight_id, ti.template_id, ti.area_id, ti.team_id,
			ti.trigger_type, ti.generation_key, ti.source_event_id, ti.template_version,
			ti.task_name, ti.message, ti.planned_at, ti.status, ti.status_version, ti.sync_version,
			f.public_id AS flight_public_id, f.flight_display_no,
			tt.public_id AS template_public_id,
			oa.public_id AS area_public_id, oa.name AS area_name,
			t.public_id AS team_public_id,
			tt.required_position_code, tt.required_capabilities`).
		Joins("JOIN flight AS f ON f.id = ti.flight_id").
		Joins("JOIN task_template AS tt ON tt.id = ti.template_id").
		Joins("JOIN operation_area AS oa ON oa.id = ti.area_id").
		Joins("JOIN team AS t ON t.id = ti.team_id")
	if filter.Status != "" {
		query = query.Where("ti.status = ?", filter.Status)
	}
	if filter.FlightPublicID != "" {
		query = query.Where("f.public_id = ?", filter.FlightPublicID)
	}
	if !filter.Scope.Global {
		switch {
		case filter.Scope.UserID != 0 && len(filter.Scope.TeamIDs) == 0 && len(filter.Scope.AreaIDs) == 0:
			query = query.Where(`EXISTS (
				SELECT 1 FROM task_assignment AS scoped_assignment
				JOIN personnel AS scoped_personnel ON scoped_personnel.id = scoped_assignment.personnel_id
				WHERE scoped_assignment.task_id = ti.id AND scoped_personnel.id = ?
			)`, filter.Scope.UserID)
		default:
			if len(filter.Scope.TeamIDs) > 0 {
				query = query.Where("ti.team_id IN ?", filter.Scope.TeamIDs)
			}
			if len(filter.Scope.AreaIDs) > 0 {
				query = query.Where("ti.area_id IN ?", filter.Scope.AreaIDs)
			}
		}
	}
	return query
}

func (r *Repository) taskReadModel(ctx context.Context, row taskConfirmationRow) (flighttask.TaskReadModel, error) {
	task, err := row.toDomain()
	if err != nil {
		return flighttask.TaskReadModel{}, err
	}
	model := flighttask.TaskReadModel{Task: task, TemplatePublicID: row.TemplatePublicID, AreaPublicID: row.AreaPublicID, TeamPublicID: row.TeamPublicID, Candidates: make([]taskmodule.Candidate, 0)}
	var candidateRows []taskConfirmationCandidateRow
	if err := r.db.WithContext(ctx).
		Table("task_candidate AS tc").
		Select(`tc.id, tc.public_id, tc.task_id, tc.personnel_id, tc.candidate_rank, tc.status,
			tc.matched_position_code, tc.matched_capabilities, tc.personnel_work_state_snapshot,
			tc.personnel_state_changed_at_snapshot, tc.rejection_reason, tc.selected_at, tc.invalidated_at,
			p.public_id AS personnel_public_id`).
		Joins("JOIN personnel AS p ON p.id = tc.personnel_id").
		Where("tc.task_id = ?", task.ID).
		Order("tc.candidate_rank ASC, tc.public_id ASC").
		Find(&candidateRows).Error; err != nil {
		return flighttask.TaskReadModel{}, fmt.Errorf("query candidates for task %s: %w", task.PublicID, err)
	}
	for _, candidateRow := range candidateRows {
		candidate, err := candidateRow.toDomain()
		if err != nil {
			return flighttask.TaskReadModel{}, err
		}
		model.Candidates = append(model.Candidates, candidate)
	}
	var assignmentRow taskCommandAssignmentRow
	err = r.db.WithContext(ctx).
		Table("task_assignment AS a").
		Select(`a.id, a.public_id, a.task_id, a.candidate_id, a.personnel_id, a.status, a.status_version,
			a.receipt_status, a.received_at, a.confirmation_id, a.confirmed_by_public_id, a.confirmed_at, p.public_id AS personnel_public_id`).
		Joins("JOIN personnel AS p ON p.id = a.personnel_id").
		Where("a.task_id = ?", task.ID).
		First(&assignmentRow).Error
	if err == nil {
		assignment := assignmentRow.toDomain()
		model.Assignment = &assignment
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return flighttask.TaskReadModel{}, fmt.Errorf("query assignment for task %s: %w", task.PublicID, err)
	}
	return model, nil
}

type taskStatusHistoryQueryRow struct {
	ID            uint64    `gorm:"column:id"`
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
}

func (row taskStatusHistoryQueryRow) toDomain() taskmodule.StatusHistory {
	var fromStatus *taskmodule.Status
	if row.FromStatus != nil {
		value := taskmodule.Status(*row.FromStatus)
		fromStatus = &value
	}
	return taskmodule.StatusHistory{PublicID: row.PublicID, TaskID: row.TaskID, StatusVersion: row.StatusVersion, FromStatus: fromStatus, ToStatus: taskmodule.Status(row.ToStatus), Reason: row.Reason, ActorType: row.ActorType, ActorPublicID: row.ActorPublicID, CommandID: row.CommandID, SourceEventID: row.SourceEventID, OccurredAt: row.OccurredAt.UTC()}
}

type assignmentStatusHistoryQueryRow struct {
	ID                 uint64    `gorm:"column:id"`
	PublicID           string    `gorm:"column:public_id"`
	AssignmentID       uint64    `gorm:"column:assignment_id"`
	StatusVersion      uint64    `gorm:"column:status_version"`
	FromStatus         *string   `gorm:"column:from_status"`
	ToStatus           string    `gorm:"column:to_status"`
	Reason             string    `gorm:"column:reason"`
	ActorType          string    `gorm:"column:actor_type"`
	ActorPublicID      string    `gorm:"column:actor_public_id"`
	CommandID          string    `gorm:"column:command_id"`
	ConfirmationID     string    `gorm:"column:confirmation_id"`
	CancellationID     string    `gorm:"column:cancellation_id"`
	OccurredAt         time.Time `gorm:"column:occurred_at"`
	AssignmentPublicID string    `gorm:"column:assignment_public_id"`
	PersonnelPublicID  string    `gorm:"column:personnel_public_id"`
}

func (row assignmentStatusHistoryQueryRow) toDomain() taskmodule.AssignmentStatusHistory {
	var fromStatus *taskmodule.AssignmentStatus
	if row.FromStatus != nil {
		value := taskmodule.AssignmentStatus(*row.FromStatus)
		fromStatus = &value
	}
	return taskmodule.AssignmentStatusHistory{PublicID: row.PublicID, AssignmentID: row.AssignmentID, StatusVersion: row.StatusVersion, FromStatus: fromStatus, ToStatus: taskmodule.AssignmentStatus(row.ToStatus), Reason: row.Reason, ActorType: row.ActorType, ActorPublicID: row.ActorPublicID, CommandID: row.CommandID, ConfirmationID: row.ConfirmationID, CancellationID: row.CancellationID, OccurredAt: row.OccurredAt.UTC()}
}

type personnelStatusHistoryQueryRow struct {
	ID                uint64    `gorm:"column:id"`
	PublicID          string    `gorm:"column:public_id"`
	PersonnelID       uint64    `gorm:"column:personnel_id"`
	StatusVersion     uint64    `gorm:"column:status_version"`
	FromState         *string   `gorm:"column:from_state"`
	ToState           string    `gorm:"column:to_state"`
	Reason            string    `gorm:"column:reason"`
	ActorType         string    `gorm:"column:actor_type"`
	ActorPublicID     string    `gorm:"column:actor_public_id"`
	AssignmentID      *uint64   `gorm:"column:assignment_id"`
	CommandID         string    `gorm:"column:command_id"`
	OccurredAt        time.Time `gorm:"column:occurred_at"`
	PersonnelPublicID string    `gorm:"column:personnel_public_id"`
}

func (row personnelStatusHistoryQueryRow) toDomain() personnelmodule.StatusHistory {
	var fromState *personnelmodule.WorkState
	if row.FromState != nil {
		value := personnelmodule.WorkState(*row.FromState)
		fromState = &value
	}
	var assignmentID uint64
	if row.AssignmentID != nil {
		assignmentID = *row.AssignmentID
	}
	return personnelmodule.StatusHistory{PublicID: row.PublicID, PersonnelID: row.PersonnelID, StatusVersion: row.StatusVersion, FromState: fromState, ToState: personnelmodule.WorkState(row.ToState), Reason: row.Reason, ActorType: row.ActorType, ActorPublicID: row.ActorPublicID, AssignmentID: assignmentID, CommandID: row.CommandID, OccurredAt: row.OccurredAt.UTC()}
}
