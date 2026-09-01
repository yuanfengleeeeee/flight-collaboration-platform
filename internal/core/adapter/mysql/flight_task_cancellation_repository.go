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

var _ flighttask.CancellationRepository = (*Repository)(nil)

func (r *Repository) WithinCancellationTransaction(ctx context.Context, fn func(flighttask.CancellationTransaction) error) error {
	if r == nil || r.db == nil || fn == nil {
		return flighttask.ErrRepositoryNotConfigured
	}
	if ctx == nil {
		return fmt.Errorf("task cancellation transaction context is nil")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&transaction{db: tx})
	})
}

func (tx *transaction) FindActiveAssignmentForUpdate(ctx context.Context, taskID uint64) (taskmodule.Assignment, error) {
	var row taskCommandAssignmentRow
	err := tx.db.WithContext(ctx).
		Table("task_assignment AS a").
		Select(`a.id, a.public_id, a.task_id, a.candidate_id, a.personnel_id, a.status, a.status_version,
			a.confirmation_id, a.confirmed_by_public_id, a.confirmed_at, p.public_id AS personnel_public_id`).
		Joins("JOIN personnel AS p ON p.id = a.personnel_id").
		Where("a.task_id = ? AND a.status IN ?", taskID, []string{string(taskmodule.AssignmentConfirmed), string(taskmodule.AssignmentAccepted)}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&row).Error
	if err != nil {
		return taskmodule.Assignment{}, normalizeNotFound(err)
	}
	return row.toDomain(), nil
}

func (tx *transaction) InvalidateProposedCandidates(ctx context.Context, taskID uint64, reason string, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Table("task_candidate").
		Where("task_id = ? AND status = ?", taskID, string(taskmodule.CandidateProposed)).
		Updates(map[string]any{
			"status":           string(taskmodule.CandidateInvalidated),
			"rejection_reason": reason,
			"invalidated_at":   changedAt.UTC(),
			"updated_at":       changedAt.UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("invalidate proposed candidates: %w", result.Error)
	}
	return nil
}

func (tx *transaction) UpdateTaskCancelled(ctx context.Context, taskID, expectedStatusVersion uint64, fromStatus taskmodule.Status, reason string, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Table("task_instance").
		Where("id = ? AND status = ? AND status_version = ?", taskID, string(fromStatus), expectedStatusVersion).
		Updates(map[string]any{
			"status":         string(taskmodule.StatusCancelled),
			"status_version": expectedStatusVersion + 1,
			"sync_version":   gorm.Expr("sync_version + ?", 1),
			"cancelled_at":   changedAt.UTC(),
			"cancel_reason":  reason,
			"updated_at":     changedAt.UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update task cancelled: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return flighttask.ErrCancellationVersionConflict
	}
	return nil
}

func (tx *transaction) UpdateAssignmentCancelled(ctx context.Context, assignmentID, expectedStatusVersion uint64, fromStatus taskmodule.AssignmentStatus, cancellationID, reason string, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Table("task_assignment").
		Where("id = ? AND status = ? AND status_version = ?", assignmentID, string(fromStatus), expectedStatusVersion).
		Updates(map[string]any{
			"status":          string(taskmodule.AssignmentCancelled),
			"status_version":  expectedStatusVersion + 1,
			"cancelled_at":    changedAt.UTC(),
			"cancellation_id": cancellationID,
			"cancel_reason":   reason,
			"updated_at":      changedAt.UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update assignment cancelled: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return flighttask.ErrCancellationInvalidState
	}
	return nil
}

func (tx *transaction) ReleasePersonnelIdle(ctx context.Context, personnelID, expectedStatusVersion uint64, fromState personnelmodule.WorkState, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Table("personnel").
		Where("id = ? AND work_state = ? AND status_version = ?", personnelID, string(fromState), expectedStatusVersion).
		Updates(map[string]any{
			"work_state":            string(personnelmodule.WorkStateIdle),
			"status_version":        expectedStatusVersion + 1,
			"last_state_changed_at": changedAt.UTC(),
			"updated_at":            changedAt.UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("release personnel idle: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return flighttask.ErrCancellationInvalidState
	}
	return nil
}
