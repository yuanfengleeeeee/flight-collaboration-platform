package mysql

import (
	"context"
	"fmt"
	"time"

	coreexception "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/exception"
	exceptionmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/exception"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
)

var _ coreexception.Transaction = (*transaction)(nil)

func (tx *transaction) CreateTaskException(ctx context.Context, value exceptionmodule.Record) error {
	if value.PublicID == "" || value.TaskID == 0 || value.AssignmentID == 0 || value.PersonnelID == 0 {
		return fmt.Errorf("task exception identity is incomplete")
	}
	row := taskExceptionRow{PublicID: value.PublicID, TaskID: value.TaskID, AssignmentID: value.AssignmentID, PersonnelID: value.PersonnelID, Category: value.Category, Severity: string(value.Severity), Description: value.Description, Status: string(value.Status), ReportedByPublicID: value.ReportedByPublicID, ReportedAt: value.ReportedAt.UTC(), ResolvedByPublicID: value.ResolvedByPublicID, ResolutionNote: value.ResolutionNote}
	if value.ResolvedAt != nil {
		resolvedAt := value.ResolvedAt.UTC()
		row.ResolvedAt = &resolvedAt
	}
	if row.ReportedAt.IsZero() {
		return fmt.Errorf("task exception reported_at is required")
	}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return nil
}

type taskExceptionRow struct {
	ID                 uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID           string     `gorm:"column:public_id"`
	TaskID             uint64     `gorm:"column:task_id"`
	AssignmentID       uint64     `gorm:"column:assignment_id"`
	PersonnelID        uint64     `gorm:"column:personnel_id"`
	Category           string     `gorm:"column:category"`
	Severity           string     `gorm:"column:severity"`
	Description        string     `gorm:"column:description"`
	Status             string     `gorm:"column:status"`
	ReportedByPublicID string     `gorm:"column:reported_by_public_id"`
	ReportedAt         time.Time  `gorm:"column:reported_at"`
	ResolvedByPublicID string     `gorm:"column:resolved_by_public_id"`
	ResolvedAt         *time.Time `gorm:"column:resolved_at"`
	ResolutionNote     string     `gorm:"column:resolution_note"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (taskExceptionRow) TableName() string { return "task_exception" }

// CreateTaskChangeRequest is called from the employee command transaction so
// an exception report cannot be acknowledged without its requested change
// being durably recorded as pending manager approval.
func (tx *transaction) CreateTaskChangeRequest(ctx context.Context, value taskmodule.ChangeRequest) error {
	if value.PublicID == "" || value.TaskID == 0 || value.ExceptionPublicID == "" || value.RequestID == "" || value.RequestedByPublicID == "" {
		return fmt.Errorf("task change request identity is incomplete")
	}
	var exception taskExceptionRow
	if err := tx.db.WithContext(ctx).Where("public_id = ? AND task_id = ?", value.ExceptionPublicID, value.TaskID).First(&exception).Error; err != nil {
		return fmt.Errorf("find task exception for change request: %w", err)
	}
	row := taskChangeRequestRowFromDomain(value, value.TaskID, exception.ID)
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: "human", ActorID: value.RequestedByPublicID, Action: "task.change.requested", ResourceType: "task", ResourceID: value.TaskPublicID, Result: string(taskmodule.ChangeRequestPending), RequestID: value.RequestID, TraceID: value.TraceID, OccurredAt: value.RequestedAt})
}
