package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	flighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	taskchange "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/taskchange"
	flightmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/flight"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ taskchange.Repository = (*TaskChangeRepository)(nil)
var _ flighttask.ReceiptTimeoutRepository = (*Repository)(nil)

type TaskChangeRepository struct{ db *gorm.DB }

func NewTaskChangeRepository(db *gorm.DB) *TaskChangeRepository { return &TaskChangeRepository{db: db} }

// ReassignUnreceived performs a bounded scan and then rechecks every row under
// a task/assignment lock. The second check makes overlapping workers and
// delayed ticks harmless: only the worker that still owns the pending receipt
// can create the replacement assignment and Outbox event.
func (r *Repository) ReassignUnreceived(ctx context.Context, now time.Time, timeout time.Duration, limit int, workerID string) (flighttask.ReceiptReassignmentResult, error) {
	if r == nil || r.db == nil {
		return flighttask.ReceiptReassignmentResult{}, flighttask.ErrRepositoryNotConfigured
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if timeout <= 0 {
		timeout = flighttask.DefaultAssignmentReceiptTimeout
	}
	if limit <= 0 {
		limit = 50
	}
	cutoff := now.UTC().Add(-timeout)
	var tasks []taskChangeTaskRow
	query := r.db.WithContext(ctx).Table("task_instance AS ti").Select(taskChangeTaskSelect).
		Joins("JOIN flight AS f ON f.id = ti.flight_id").Joins("JOIN operation_area AS oa ON oa.id = ti.area_id").Joins("JOIN task_template AS tt ON tt.id = ti.template_id").
		Joins("JOIN task_assignment AS ta ON ta.task_id = ti.id").
		Where("ti.status = ? AND ta.status = ? AND ta.receipt_status = ? AND ta.confirmed_at <= ?", string(taskmodule.StatusAssigned), string(taskmodule.AssignmentConfirmed), string(taskmodule.AssignmentReceiptPending), cutoff).
		Order("ta.confirmed_at ASC, ti.id ASC").Limit(limit)
	if err := query.Find(&tasks).Error; err != nil {
		return flighttask.ReceiptReassignmentResult{}, fmt.Errorf("scan unreceived assignments: %w", err)
	}
	result := flighttask.ReceiptReassignmentResult{Scanned: len(tasks)}
	actor := "system:auto-reassign"
	if strings.TrimSpace(workerID) == "" {
		workerID = "worker"
	}
	for _, candidateTask := range tasks {
		reassignedBefore := result.Reassigned
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var task taskChangeTaskRow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("task_instance AS ti").Select(taskChangeTaskSelect).Joins("JOIN flight AS f ON f.id = ti.flight_id").Joins("JOIN operation_area AS oa ON oa.id = ti.area_id").Joins("JOIN task_template AS tt ON tt.id = ti.template_id").Where("ti.id = ?", candidateTask.ID).First(&task).Error; err != nil {
				return normalizeTaskChangeNotFound(err)
			}
			var assignment taskAssignmentRow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_id = ? AND status = ?", task.ID, string(taskmodule.AssignmentConfirmed)).First(&assignment).Error; err != nil {
				return nil
			}
			if assignment.ReceiptStatus != string(taskmodule.AssignmentReceiptPending) || assignment.ConfirmedAt.After(cutoff) {
				return nil
			}
			commandID := "timeout-reassign:" + task.PublicID + ":" + fmt.Sprint(assignment.StatusVersion)
			if err := reassignTask(ctx, tx, &task, "", "assignment receipt timeout", actor, commandID, commandID, now.UTC()); err != nil {
				if errors.Is(err, taskchange.ErrNotFound) || errors.Is(err, taskchange.ErrConflict) {
					return nil
				}
				return err
			}
			result.Reassigned++
			return nil
		})
		if err != nil {
			return result, fmt.Errorf("reassign unreceived task %s: %w", candidateTask.PublicID, err)
		}
		// A task with no currently eligible proposed candidate stays assigned so
		// the manager can see the shortage; it is not silently cancelled.
		if result.Reassigned == reassignedBefore {
			result.NoCandidate++
		}
	}
	return result, nil
}

func (r *TaskChangeRepository) CreateRequest(ctx context.Context, value taskmodule.ChangeRequest, scope platformsecurity.AccessScope) (taskmodule.ChangeRequest, error) {
	if r == nil || r.db == nil {
		return taskmodule.ChangeRequest{}, taskchange.ErrRepositoryNotConfigured
	}
	if value.PublicID == "" || value.RequestID == "" || value.TaskPublicID == "" {
		return taskmodule.ChangeRequest{}, taskchange.ErrInvalidInput
	}
	var row taskChangeRequestRow
	var taskPublicID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := findScopedTask(ctx, tx, value.TaskPublicID, scope, false)
		if err != nil {
			return err
		}
		taskPublicID = task.PublicID
		var exceptionID uint64
		if value.ExceptionPublicID != "" {
			var exception taskExceptionRow
			if err := tx.Where("public_id = ? AND task_id = ?", value.ExceptionPublicID, task.ID).First(&exception).Error; err != nil {
				return normalizeTaskChangeNotFound(err)
			}
			exceptionID = exception.ID
		}
		row = taskChangeRequestRowFromDomain(value, task.ID, exceptionID)
		if err := tx.Create(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return taskchange.ErrConflict
			}
			return fmt.Errorf("create task change request row: %w", err)
		}
		if err := appendTaskChangeAudit(ctx, tx, value.RequestedByPublicID, "task.change.requested", value.TaskPublicID, string(value.Status), value.TraceID, value.RequestID, "internal", value.RequestedAt); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return taskmodule.ChangeRequest{}, err
	}
	result := row.toDomain()
	result.TaskPublicID = taskPublicID
	return result, nil
}

func (r *TaskChangeRepository) ListRequests(ctx context.Context, filter taskchange.ListFilter) ([]taskmodule.ChangeRequest, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, taskchange.ErrRepositoryNotConfigured
	}
	query := r.db.WithContext(ctx).Table("task_change_request AS r").Joins("JOIN task_instance AS ti ON ti.id = r.task_id")
	query = applyTaskChangeScope(query, filter.Scope)
	if query == nil {
		return nil, 0, taskchange.ErrForbidden
	}
	if filter.TaskPublicID != "" {
		query = query.Where("ti.public_id = ?", filter.TaskPublicID)
	}
	if filter.Status != "" {
		query = query.Where("r.status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count task change requests: %w", err)
	}
	var rows []taskChangeRequestRow
	if err := query.Select(taskChangeRequestSelect).Order("r.requested_at DESC, r.id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list task change requests: %w", err)
	}
	items := make([]taskmodule.ChangeRequest, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDomain())
	}
	return items, total, nil
}

func (r *TaskChangeRepository) ReviewRequest(ctx context.Context, input taskchange.ReviewInput) (taskmodule.ChangeRequest, error) {
	if r == nil || r.db == nil {
		return taskmodule.ChangeRequest{}, taskchange.ErrRepositoryNotConfigured
	}
	var result taskmodule.ChangeRequest
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row taskChangeRequestRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", input.PublicID).First(&row).Error; err != nil {
			return normalizeTaskChangeNotFound(err)
		}
		var task taskChangeTaskRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("task_instance AS ti").Select(taskChangeTaskSelect).Joins("JOIN flight AS f ON f.id = ti.flight_id").Joins("JOIN operation_area AS oa ON oa.id = ti.area_id").Joins("JOIN task_template AS tt ON tt.id = ti.template_id").Where("ti.id = ?", row.TaskID).First(&task).Error; err != nil {
			return normalizeTaskChangeNotFound(err)
		}
		if !scopeMatches(input.Scope, task.TeamID, task.AreaID) {
			return taskchange.ErrForbidden
		}
		if row.Status != string(taskmodule.ChangeRequestPending) {
			result = row.toDomain()
			result.TaskPublicID = task.PublicID
			return nil
		}
		now := input.ReviewedAt.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		updates := map[string]any{"reviewed_by_public_id": input.ReviewedBy, "reviewed_at": now, "review_note": input.ReviewNote, "updated_at": now}
		if input.Decision == taskchange.ReviewReject {
			updates["status"] = string(taskmodule.ChangeRequestRejected)
			if err := tx.Model(&taskChangeRequestRow{}).Where("id = ? AND status = ?", row.ID, string(taskmodule.ChangeRequestPending)).Updates(updates).Error; err != nil {
				return fmt.Errorf("reject task change request: %w", err)
			}
			row.Status = string(taskmodule.ChangeRequestRejected)
			row.ReviewedByPublicID, row.ReviewedAt, row.ReviewNote = input.ReviewedBy, &now, input.ReviewNote
			result = row.toDomain()
			result.TaskPublicID = task.PublicID
			return appendTaskChangeAudit(ctx, tx, input.ReviewedBy, "task.change.rejected", task.PublicID, row.Status, input.TraceID, input.RequestID, input.SourceIP, now)
		}
		if input.Decision != taskchange.ReviewApprove {
			return taskchange.ErrInvalidInput
		}
		if applyErr := applyTaskChange(ctx, tx, &task, &row, input, now); applyErr != nil {
			failure := trimTaskChangeError(applyErr)
			updates["status"] = string(taskmodule.ChangeRequestFailed)
			updates["failure_reason"] = failure
			if err := tx.Model(&taskChangeRequestRow{}).Where("id = ? AND status = ?", row.ID, string(taskmodule.ChangeRequestPending)).Updates(updates).Error; err != nil {
				return fmt.Errorf("persist failed task change request: %w", err)
			}
			row.Status, row.FailureReason = string(taskmodule.ChangeRequestFailed), failure
			row.ReviewedByPublicID, row.ReviewedAt, row.ReviewNote = input.ReviewedBy, &now, input.ReviewNote
			result = row.toDomain()
			result.TaskPublicID = task.PublicID
			return appendTaskChangeAudit(ctx, tx, input.ReviewedBy, "task.change.failed", task.PublicID, row.Status, input.TraceID, input.RequestID, input.SourceIP, now)
		}
		updates["status"] = string(taskmodule.ChangeRequestApplied)
		updates["applied_at"] = now
		if err := tx.Model(&taskChangeRequestRow{}).Where("id = ? AND status = ?", row.ID, string(taskmodule.ChangeRequestPending)).Updates(updates).Error; err != nil {
			return fmt.Errorf("mark task change applied: %w", err)
		}
		row.Status, row.AppliedAt = string(taskmodule.ChangeRequestApplied), &now
		row.ReviewedByPublicID, row.ReviewedAt, row.ReviewNote = input.ReviewedBy, &now, input.ReviewNote
		result = row.toDomain()
		result.TaskPublicID = task.PublicID
		return appendTaskChangeAudit(ctx, tx, input.ReviewedBy, "task.change.applied", task.PublicID, row.Status, input.TraceID, input.RequestID, input.SourceIP, now)
	})
	if err != nil {
		return taskmodule.ChangeRequest{}, err
	}
	return result, nil
}

const taskChangeRequestSelect = `r.id, r.public_id, r.task_id, ti.public_id AS task_public_id, r.exception_id, r.action, r.reason,
 r.target_candidate_public_id, r.target_planned_at, r.status, r.requested_by_public_id,
 r.requested_at, r.reviewed_by_public_id, r.reviewed_at, r.review_note, r.applied_at,
 r.failure_reason, r.request_id, r.trace_id`

type taskChangeRequestRow struct {
	ID                      uint64     `gorm:"column:id;primaryKey"`
	PublicID                string     `gorm:"column:public_id"`
	TaskID                  uint64     `gorm:"column:task_id"`
	TaskPublicID            string     `gorm:"column:task_public_id"`
	ExceptionID             *uint64    `gorm:"column:exception_id"`
	Action                  string     `gorm:"column:action"`
	Reason                  string     `gorm:"column:reason"`
	TargetCandidatePublicID string     `gorm:"column:target_candidate_public_id"`
	TargetPlannedAt         *time.Time `gorm:"column:target_planned_at"`
	Status                  string     `gorm:"column:status"`
	RequestedByPublicID     string     `gorm:"column:requested_by_public_id"`
	RequestedAt             time.Time  `gorm:"column:requested_at"`
	ReviewedByPublicID      string     `gorm:"column:reviewed_by_public_id"`
	ReviewedAt              *time.Time `gorm:"column:reviewed_at"`
	ReviewNote              string     `gorm:"column:review_note"`
	AppliedAt               *time.Time `gorm:"column:applied_at"`
	FailureReason           string     `gorm:"column:failure_reason"`
	RequestID               string     `gorm:"column:request_id"`
	TraceID                 string     `gorm:"column:trace_id"`
}

func (taskChangeRequestRow) TableName() string { return "task_change_request" }

func taskChangeRequestRowFromDomain(value taskmodule.ChangeRequest, taskID, exceptionID uint64) taskChangeRequestRow {
	var exception *uint64
	if exceptionID != 0 {
		exception = &exceptionID
	}
	return taskChangeRequestRow{PublicID: value.PublicID, TaskID: taskID, ExceptionID: exception, Action: string(value.Action), Reason: value.Reason, TargetCandidatePublicID: value.TargetCandidatePublicID, TargetPlannedAt: value.TargetPlannedAt, Status: string(value.Status), RequestedByPublicID: value.RequestedByPublicID, RequestedAt: value.RequestedAt.UTC(), RequestID: value.RequestID, TraceID: value.TraceID}
}

func (row taskChangeRequestRow) toDomain() taskmodule.ChangeRequest {
	var exceptionID uint64
	if row.ExceptionID != nil {
		exceptionID = *row.ExceptionID
	}
	return taskmodule.ChangeRequest{ID: row.ID, PublicID: row.PublicID, TaskID: row.TaskID, TaskPublicID: row.TaskPublicID, ExceptionID: exceptionID, Action: taskmodule.ChangeAction(row.Action), Reason: row.Reason, TargetCandidatePublicID: row.TargetCandidatePublicID, TargetPlannedAt: row.TargetPlannedAt, Status: taskmodule.ChangeRequestStatus(row.Status), RequestedByPublicID: row.RequestedByPublicID, RequestedAt: row.RequestedAt.UTC(), ReviewedByPublicID: row.ReviewedByPublicID, ReviewedAt: row.ReviewedAt, ReviewNote: row.ReviewNote, AppliedAt: row.AppliedAt, FailureReason: row.FailureReason, RequestID: row.RequestID, TraceID: row.TraceID}
}

type taskChangeTaskRow struct {
	ID                   uint64    `gorm:"column:id;primaryKey"`
	PublicID             string    `gorm:"column:public_id"`
	FlightPublicID       string    `gorm:"column:flight_public_id"`
	FlightDisplayNo      string    `gorm:"column:flight_display_no"`
	AreaID               uint64    `gorm:"column:area_id"`
	AreaName             string    `gorm:"column:area_name"`
	TeamID               uint64    `gorm:"column:team_id"`
	RequiredPositionCode string    `gorm:"column:required_position_code"`
	RequiredCapabilities []byte    `gorm:"column:required_capabilities"`
	Name                 string    `gorm:"column:task_name"`
	Message              string    `gorm:"column:message"`
	PlannedAt            time.Time `gorm:"column:planned_at"`
	Status               string    `gorm:"column:status"`
	StatusVersion        uint64    `gorm:"column:status_version"`
	SyncVersion          uint64    `gorm:"column:sync_version"`
}

const taskChangeTaskSelect = `ti.id, ti.public_id, f.public_id AS flight_public_id,
 f.flight_display_no, ti.area_id, oa.name AS area_name, ti.team_id, ti.task_name,
 tt.required_position_code, tt.required_capabilities, ti.message, ti.planned_at,
 ti.status, ti.status_version, ti.sync_version`

func findScopedTask(ctx context.Context, db *gorm.DB, publicID string, scope platformsecurity.AccessScope, lock bool) (taskChangeTaskRow, error) {
	query := db.WithContext(ctx).Table("task_instance AS ti").Select(taskChangeTaskSelect).Joins("JOIN flight AS f ON f.id = ti.flight_id").Joins("JOIN operation_area AS oa ON oa.id = ti.area_id").Joins("JOIN task_template AS tt ON tt.id = ti.template_id").Where("ti.public_id = ?", publicID)
	query = applyTaskChangeScope(query, scope)
	if query == nil {
		return taskChangeTaskRow{}, taskchange.ErrForbidden
	}
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var task taskChangeTaskRow
	if err := query.First(&task).Error; err != nil {
		return taskChangeTaskRow{}, normalizeTaskChangeNotFound(err)
	}
	return task, nil
}

func applyTaskChangeScope(query *gorm.DB, scope platformsecurity.AccessScope) *gorm.DB {
	if scope.Global {
		return query
	}
	if len(scope.TeamIDs) == 0 && len(scope.AreaIDs) == 0 {
		return nil
	}
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if len(scope.TeamIDs) > 0 {
		conditions = append(conditions, "ti.team_id IN ?")
		args = append(args, scope.TeamIDs)
	}
	if len(scope.AreaIDs) > 0 {
		conditions = append(conditions, "ti.area_id IN ?")
		args = append(args, scope.AreaIDs)
	}
	return query.Where(strings.Join(conditions, " OR "), args...)
}

func scopeMatches(scope platformsecurity.AccessScope, teamID, areaID uint64) bool {
	if scope.Global {
		return true
	}
	for _, value := range scope.TeamIDs {
		if value == teamID {
			return true
		}
	}
	for _, value := range scope.AreaIDs {
		if value == areaID {
			return true
		}
	}
	return false
}

func applyTaskChange(ctx context.Context, tx *gorm.DB, task *taskChangeTaskRow, request *taskChangeRequestRow, input taskchange.ReviewInput, now time.Time) error {
	if task == nil || request == nil {
		return taskchange.ErrInvalidInput
	}
	action := taskmodule.ChangeAction(request.Action)
	switch action {
	case taskmodule.ChangeActionPause:
		return pauseTask(ctx, tx, task, request.Reason, input.ReviewedBy, request.RequestID, input.TraceID, now)
	case taskmodule.ChangeActionCancel:
		return cancelTaskForChange(ctx, tx, task, request.Reason, input.ReviewedBy, request.RequestID, input.TraceID, now)
	case taskmodule.ChangeActionReschedule:
		if request.TargetPlannedAt == nil || request.TargetPlannedAt.IsZero() {
			return taskchange.ErrInvalidInput
		}
		return rescheduleTask(ctx, tx, task, *request.TargetPlannedAt, request.Reason, input.ReviewedBy, request.RequestID, input.TraceID, now)
	case taskmodule.ChangeActionReassign, taskmodule.ChangeActionResume:
		return reassignTask(ctx, tx, task, request.TargetCandidatePublicID, request.Reason, input.ReviewedBy, request.RequestID, input.TraceID, now)
	default:
		return taskchange.ErrInvalidInput
	}
}

func pauseTask(ctx context.Context, tx *gorm.DB, task *taskChangeTaskRow, reason, actor, commandID, traceID string, now time.Time) error {
	if task.Status == string(taskmodule.StatusCompleted) || task.Status == string(taskmodule.StatusCancelled) || task.Status == string(taskmodule.StatusPaused) {
		return taskchange.ErrConflict
	}
	fromStatus := taskmodule.Status(task.Status)
	assignment, person, err := lockActiveAssignment(ctx, tx, task.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, taskchange.ErrNotFound) {
		return err
	}
	if assignment != nil {
		if err := releaseAssignment(ctx, tx, assignment, person, reason, actor, commandID, now); err != nil {
			return err
		}
	}
	if err := tx.Model(&taskRow{}).Where("id = ? AND status_version = ?", task.ID, task.StatusVersion).Updates(map[string]any{"status": string(taskmodule.StatusPaused), "status_version": task.StatusVersion + 1, "sync_version": task.SyncVersion + 1, "updated_at": now}).Error; err != nil {
		return err
	}
	task.Status, task.StatusVersion, task.SyncVersion = string(taskmodule.StatusPaused), task.StatusVersion+1, task.SyncVersion+1
	if err := writeTaskHistory(ctx, tx, task, fromStatus, taskmodule.StatusPaused, reason, actor, commandID, now); err != nil {
		return err
	}
	return appendTaskChangeEvent(ctx, tx, task, assignment, taskmodule.StatusPaused, reason, traceID, now)
}

func cancelTaskForChange(ctx context.Context, tx *gorm.DB, task *taskChangeTaskRow, reason, actor, commandID, traceID string, now time.Time) error {
	if task.Status == string(taskmodule.StatusCompleted) || task.Status == string(taskmodule.StatusCancelled) {
		return taskchange.ErrConflict
	}
	fromStatus := taskmodule.Status(task.Status)
	assignment, person, err := lockActiveAssignment(ctx, tx, task.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, taskchange.ErrNotFound) {
		return err
	}
	if assignment != nil {
		if err := releaseAssignment(ctx, tx, assignment, person, reason, actor, commandID, now); err != nil {
			return err
		}
	}
	if err := tx.Model(&taskRow{}).Where("id = ? AND status_version = ?", task.ID, task.StatusVersion).Updates(map[string]any{"status": string(taskmodule.StatusCancelled), "status_version": task.StatusVersion + 1, "sync_version": task.SyncVersion + 1, "cancelled_at": now, "cancel_reason": reason, "updated_at": now}).Error; err != nil {
		return err
	}
	task.Status, task.StatusVersion, task.SyncVersion = string(taskmodule.StatusCancelled), task.StatusVersion+1, task.SyncVersion+1
	if err := writeTaskHistory(ctx, tx, task, fromStatus, taskmodule.StatusCancelled, reason, actor, commandID, now); err != nil {
		return err
	}
	return appendTaskChangeEvent(ctx, tx, task, assignment, taskmodule.StatusCancelled, reason, traceID, now)
}

func rescheduleTask(ctx context.Context, tx *gorm.DB, task *taskChangeTaskRow, plannedAt time.Time, reason, actor, commandID, traceID string, now time.Time) error {
	if task.Status == string(taskmodule.StatusCompleted) || task.Status == string(taskmodule.StatusCancelled) {
		return taskchange.ErrConflict
	}
	plannedAt = plannedAt.UTC()
	assignment, _, err := lockActiveAssignment(ctx, tx, task.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if assignment != nil {
		var conflictCount int64
		if err := tx.WithContext(ctx).Table("task_assignment AS a").Joins("JOIN task_instance AS ti ON ti.id = a.task_id").Where("a.personnel_id = ? AND a.status IN ? AND ti.id <> ? AND ti.planned_at = ?", assignment.PersonnelID, []string{string(taskmodule.AssignmentConfirmed), string(taskmodule.AssignmentAccepted)}, task.ID, plannedAt).Count(&conflictCount).Error; err != nil {
			return fmt.Errorf("check rescheduled personnel conflict: %w", err)
		}
		if conflictCount > 0 {
			return taskchange.ErrConflict
		}
	}
	if err := tx.Model(&taskRow{}).Where("id = ? AND status_version = ?", task.ID, task.StatusVersion).Updates(map[string]any{"planned_at": plannedAt, "status_version": task.StatusVersion + 1, "sync_version": task.SyncVersion + 1, "updated_at": now}).Error; err != nil {
		return err
	}
	task.PlannedAt, task.StatusVersion, task.SyncVersion = plannedAt, task.StatusVersion+1, task.SyncVersion+1
	if err := writeTaskHistory(ctx, tx, task, taskmodule.Status(task.Status), taskmodule.Status(task.Status), reason, actor, commandID, now); err != nil {
		return err
	}
	return appendTaskChangeEvent(ctx, tx, task, assignment, taskmodule.Status(task.Status), reason, traceID, now)
}

func reassignTask(ctx context.Context, tx *gorm.DB, task *taskChangeTaskRow, candidatePublicID, reason, actor, commandID, traceID string, now time.Time) error {
	if task.Status == string(taskmodule.StatusCompleted) || task.Status == string(taskmodule.StatusCancelled) || task.Status == string(taskmodule.StatusInProgress) {
		return taskchange.ErrConflict
	}
	fromStatus := taskmodule.Status(task.Status)
	var candidate taskCandidateRow
	candidateQuery := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_id = ? AND status = ?", task.ID, string(taskmodule.CandidateProposed))
	if strings.TrimSpace(candidatePublicID) != "" {
		candidateQuery = candidateQuery.Where("public_id = ?", candidatePublicID)
	}
	if err := candidateQuery.Order("candidate_rank ASC, id ASC").First(&candidate).Error; err != nil {
		return taskchange.ErrNotFound
	}
	var person personnelCandidateRow
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("p.id = ?", candidate.PersonnelID).Table("personnel AS p").Select("p.id, p.public_id, p.user_public_id, p.team_id, t.area_id, p.position_code, p.capabilities, p.work_state, p.status_version, p.last_state_changed_at, p.enabled").Joins("JOIN team AS t ON t.id = p.team_id").First(&person).Error; err != nil {
		return err
	}
	eligible, err := taskChangeCandidateEligible(ctx, tx, task, &candidate, &person)
	if err != nil {
		return err
	}
	if !eligible {
		return taskchange.ErrConflict
	}
	assignment, oldPerson, err := lockActiveAssignment(ctx, tx, task.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, taskchange.ErrNotFound) {
		return err
	}
	if assignment != nil && assignment.PersonnelID == person.ID {
		return taskchange.ErrConflict
	}
	if assignment != nil {
		if err := releaseAssignment(ctx, tx, assignment, oldPerson, reason, actor, commandID, now); err != nil {
			return err
		}
	}
	if err := tx.Model(&taskChangePersonnelRow{}).Where("id = ? AND status_version = ? AND work_state = ?", person.ID, person.StatusVersion, string(personnelmodule.WorkStateIdle)).Updates(map[string]any{"work_state": string(personnelmodule.WorkStateReserved), "status_version": person.StatusVersion + 1, "last_state_changed_at": now, "updated_at": now, "unavailable_reason": nil}).Error; err != nil {
		return err
	}
	if err := tx.Model(&taskCandidateRow{}).Where("task_id = ? AND status = ?", task.ID, string(taskmodule.CandidateProposed)).Updates(map[string]any{"status": string(taskmodule.CandidateInvalidated), "rejection_reason": "reassignment selected another candidate", "invalidated_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&taskCandidateRow{}).Where("id = ?", candidate.ID).Updates(map[string]any{"status": string(taskmodule.CandidateSelected), "selected_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	confirmationID := "change:" + commandID
	if assignment == nil {
		publicID, err := id.NewPublicID()
		if err != nil {
			return err
		}
		row := taskAssignmentRow{PublicID: publicID, TaskID: task.ID, CandidateID: candidate.ID, PersonnelID: person.ID, Status: string(taskmodule.AssignmentConfirmed), StatusVersion: 0, ReceiptStatus: string(taskmodule.AssignmentReceiptPending), ConfirmationID: confirmationID, ConfirmedByPublicID: actor, ConfirmedAt: now, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		assignment = &row
	} else {
		if err := tx.Model(&taskAssignmentRow{}).Where("id = ?", assignment.ID).Updates(map[string]any{"candidate_id": candidate.ID, "personnel_id": person.ID, "status": string(taskmodule.AssignmentConfirmed), "status_version": assignment.StatusVersion + 1, "receipt_status": string(taskmodule.AssignmentReceiptPending), "received_at": nil, "confirmation_id": confirmationID, "confirmed_by_public_id": actor, "confirmed_at": now, "accepted_at": nil, "cancelled_at": nil, "cancellation_id": nil, "cancel_reason": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		assignment.CandidateID, assignment.PersonnelID, assignment.Status = candidate.ID, person.ID, string(taskmodule.AssignmentConfirmed)
		assignment.StatusVersion++
		assignment.ReceiptStatus, assignment.ConfirmationID, assignment.ConfirmedByPublicID, assignment.ConfirmedAt = string(taskmodule.AssignmentReceiptPending), confirmationID, actor, now
	}
	assignment.PersonnelPublicID = person.PublicID
	if err := tx.Create(&taskAssignmentStatusHistoryRow{PublicID: mustTaskChangeID(), AssignmentID: assignment.ID, StatusVersion: assignment.StatusVersion, ToStatus: string(taskmodule.AssignmentConfirmed), Reason: reason, ActorType: "human", ActorPublicID: actor, ConfirmationID: confirmationID, OccurredAt: now, CreatedAt: now}).Error; err != nil {
		return err
	}
	fromPerson := string(personnelmodule.WorkStateIdle)
	assignmentID := assignment.ID
	if err := tx.Create(&personnelStatusHistoryRow{PublicID: mustTaskChangeID(), PersonnelID: person.ID, StatusVersion: person.StatusVersion + 1, FromState: &fromPerson, ToState: string(personnelmodule.WorkStateReserved), Reason: reason, ActorType: "human", ActorPublicID: actor, AssignmentID: &assignmentID, CommandID: commandID, OccurredAt: now, CreatedAt: now}).Error; err != nil {
		return err
	}
	newStatus := taskmodule.StatusAssigned
	if err := tx.Model(&taskRow{}).Where("id = ? AND status_version = ?", task.ID, task.StatusVersion).Updates(map[string]any{"status": string(newStatus), "status_version": task.StatusVersion + 1, "sync_version": task.SyncVersion + 1, "updated_at": now}).Error; err != nil {
		return err
	}
	task.Status, task.StatusVersion, task.SyncVersion = string(newStatus), task.StatusVersion+1, task.SyncVersion+1
	if err := writeTaskHistory(ctx, tx, task, fromStatus, newStatus, reason, actor, commandID, now); err != nil {
		return err
	}
	return appendTaskChangeEvent(ctx, tx, task, assignment, newStatus, reason, traceID, now)
}

type taskChangePersonnelRow struct {
	ID                 uint64    `gorm:"column:id;primaryKey"`
	PublicID           string    `gorm:"column:public_id"`
	WorkState          string    `gorm:"column:work_state"`
	StatusVersion      uint64    `gorm:"column:status_version"`
	LastStateChangedAt time.Time `gorm:"column:last_state_changed_at"`
}

func (taskChangePersonnelRow) TableName() string { return "personnel" }

func taskChangeCandidateEligible(ctx context.Context, tx *gorm.DB, task *taskChangeTaskRow, candidate *taskCandidateRow, person *personnelCandidateRow) (bool, error) {
	if task == nil || candidate == nil || person == nil || candidate.TaskID != task.ID || candidate.Status != string(taskmodule.CandidateProposed) {
		return false, nil
	}
	if person.TeamID != task.TeamID || person.AreaID != task.AreaID || person.PositionCode != task.RequiredPositionCode || person.WorkState != string(personnelmodule.WorkStateIdle) || !person.Enabled {
		return false, nil
	}
	taskCapabilities, err := decodeCapabilities(task.RequiredCapabilities)
	if err != nil {
		return false, fmt.Errorf("decode task %s required capabilities: %w", task.PublicID, err)
	}
	personCapabilities, err := decodeCapabilities(person.Capabilities)
	if err != nil {
		return false, fmt.Errorf("decode personnel %s capabilities: %w", person.PublicID, err)
	}
	if !containsAllCapabilities(personCapabilities, taskCapabilities) {
		return false, nil
	}
	var membershipCount int64
	if err := tx.WithContext(ctx).Table("team_member").Where("personnel_id = ? AND team_id = ? AND left_at IS NULL AND is_primary = ?", person.ID, task.TeamID, true).Count(&membershipCount).Error; err != nil {
		return false, fmt.Errorf("check personnel %s primary team membership: %w", person.PublicID, err)
	}
	if membershipCount == 0 {
		return false, nil
	}
	var conflictCount int64
	if err := tx.WithContext(ctx).Table("task_assignment AS a").Joins("JOIN task_instance AS ti ON ti.id = a.task_id").Where("a.personnel_id = ? AND a.status IN ? AND ti.id <> ? AND ti.planned_at = ?", person.ID, []string{string(taskmodule.AssignmentConfirmed), string(taskmodule.AssignmentAccepted)}, task.ID, task.PlannedAt.UTC()).Count(&conflictCount).Error; err != nil {
		return false, fmt.Errorf("check personnel %s schedule conflict: %w", person.PublicID, err)
	}
	return conflictCount == 0, nil
}

func containsAllCapabilities(actual, required []string) bool {
	available := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		available[strings.TrimSpace(value)] = struct{}{}
	}
	for _, value := range required {
		if _, ok := available[strings.TrimSpace(value)]; !ok {
			return false
		}
	}
	return true
}

func lockActiveAssignment(ctx context.Context, tx *gorm.DB, taskID uint64) (*taskAssignmentRow, *taskChangePersonnelRow, error) {
	var assignment taskAssignmentRow
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_id = ? AND status IN ?", taskID, []string{string(taskmodule.AssignmentConfirmed), string(taskmodule.AssignmentAccepted)}).First(&assignment).Error
	if err != nil {
		return nil, nil, err
	}
	var person taskChangePersonnelRow
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", assignment.PersonnelID).First(&person).Error; err != nil {
		return nil, nil, err
	}
	assignment.PersonnelPublicID = person.PublicID
	return &assignment, &person, nil
}

func releaseAssignment(ctx context.Context, tx *gorm.DB, assignment *taskAssignmentRow, person *taskChangePersonnelRow, reason, actor, commandID string, now time.Time) error {
	if assignment == nil || person == nil {
		return nil
	}
	if err := tx.Model(&taskAssignmentRow{}).Where("id = ? AND status_version = ?", assignment.ID, assignment.StatusVersion).Updates(map[string]any{"status": string(taskmodule.AssignmentCancelled), "status_version": assignment.StatusVersion + 1, "cancelled_at": now, "cancellation_id": commandID, "cancel_reason": reason, "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&taskChangePersonnelRow{}).Where("id = ? AND status_version = ?", person.ID, person.StatusVersion).Updates(map[string]any{"work_state": string(personnelmodule.WorkStateIdle), "status_version": person.StatusVersion + 1, "last_state_changed_at": now, "updated_at": now, "unavailable_reason": nil}).Error; err != nil {
		return err
	}
	fromAssignment := assignment.Status
	if err := tx.Create(&taskAssignmentStatusHistoryRow{PublicID: mustTaskChangeID(), AssignmentID: assignment.ID, StatusVersion: assignment.StatusVersion + 1, FromStatus: &fromAssignment, ToStatus: string(taskmodule.AssignmentCancelled), Reason: reason, ActorType: string("human"), ActorPublicID: actor, CancellationID: commandID, OccurredAt: now, CreatedAt: now}).Error; err != nil {
		return err
	}
	fromPerson := person.WorkState
	assignmentID := assignment.ID
	return tx.Create(&personnelStatusHistoryRow{PublicID: mustTaskChangeID(), PersonnelID: person.ID, StatusVersion: person.StatusVersion + 1, FromState: &fromPerson, ToState: string(personnelmodule.WorkStateIdle), Reason: reason, ActorType: string("human"), ActorPublicID: actor, AssignmentID: &assignmentID, CommandID: commandID, OccurredAt: now, CreatedAt: now}).Error
}

func writeTaskHistory(ctx context.Context, tx *gorm.DB, task *taskChangeTaskRow, fromStatus, toStatus taskmodule.Status, reason, actor, commandID string, now time.Time) error {
	from := string(fromStatus)
	return tx.WithContext(ctx).Create(&taskStatusHistoryRow{PublicID: mustTaskChangeID(), TaskID: task.ID, StatusVersion: task.StatusVersion, FromStatus: &from, ToStatus: string(toStatus), Reason: reason, ActorType: string("human"), ActorPublicID: actor, CommandID: commandID, OccurredAt: now, CreatedAt: now}).Error
}

func appendTaskChangeEvent(ctx context.Context, tx *gorm.DB, task *taskChangeTaskRow, assignment *taskAssignmentRow, status taskmodule.Status, reason, traceID string, now time.Time) error {
	// The task row has already been updated in this transaction. Emit that
	// committed version so Edge can reject stale events deterministically.
	payload := map[string]any{"task_public_id": task.PublicID, "assignment_public_id": "", "employee_public_id": "", "flight_display_no": task.FlightDisplayNo, "task_name": task.Name, "area_name": task.AreaName, "planned_at": task.PlannedAt.UTC(), "business_status": string(status), "message": reason, "sync_version": task.SyncVersion}
	if assignment != nil {
		payload["assignment_public_id"] = assignment.PublicID
		payload["employee_public_id"] = assignment.PersonnelPublicID
		payload["receipt_status"] = assignment.ReceiptStatus
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	envelope, err := sharedEvent.NewEvent("task."+string(status)+".v1", "task", task.PublicID, "core-task-change", json.RawMessage(encoded))
	if err != nil {
		return err
	}
	envelope.TraceID = traceID
	envelope.OccurredAt = now.UTC()
	row := outboxRow{EventID: envelope.EventID, EventType: envelope.EventType, SchemaVersion: envelope.SchemaVersion, AggregateType: envelope.AggregateType, AggregateID: envelope.AggregateID, OccurredAt: envelope.OccurredAt, Producer: envelope.Producer, CorrelationID: envelope.CorrelationID, TraceID: envelope.TraceID, Payload: encoded, Status: sharedEvent.StatusPending, NextAttemptAt: now.UTC(), CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	return tx.WithContext(ctx).Create(&row).Error
}

func appendTaskChangeAudit(ctx context.Context, tx *gorm.DB, actor, action, resource, result, traceID, requestID, sourceIP string, occurredAt time.Time) error {
	return tx.WithContext(ctx).Create(&auditRow{ActorType: "human", ActorID: actor, Action: action, ResourceType: "task", ResourceID: resource, Result: result, RequestID: requestID, TraceID: traceID, SourceIP: sourceIP, OccurredAt: occurredAt.UTC(), CreatedAt: occurredAt.UTC()}).Error
}

func normalizeTaskChangeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return taskchange.ErrNotFound
	}
	return err
}

func trimTaskChangeError(err error) string {
	value := strings.TrimSpace(err.Error())
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}

func mustTaskChangeID() string {
	value, err := id.NewPublicID()
	if err != nil {
		return fmt.Sprintf("task-change-%d", time.Now().UnixNano())
	}
	return value
}

// Keep these references local to the adapter so future status additions do
// not accidentally remove the domain validation from this transaction.
var _ = flightmodule.StatusScheduled
var _ = coresync.AuditRecord{}
