package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	flighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ flighttask.EmployeeCommandTransaction = (*transaction)(nil)

func (tx *transaction) FindAssignmentForUpdate(ctx context.Context, taskID uint64, publicID string) (taskmodule.Assignment, error) {
	var row taskCommandAssignmentRow
	err := tx.db.WithContext(ctx).
		Table("task_assignment AS a").
		Select(`a.id, a.public_id, a.task_id, a.candidate_id, a.personnel_id, a.status,
			a.status_version, a.confirmation_id, a.confirmed_by_public_id, a.confirmed_at,
			p.public_id AS personnel_public_id`).
		Joins("JOIN personnel AS p ON p.id = a.personnel_id").
		Where("a.task_id = ? AND a.public_id = ?", taskID, publicID).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&row).Error
	if err != nil {
		return taskmodule.Assignment{}, normalizeNotFound(err)
	}
	return row.toDomain(), nil
}

func (tx *transaction) UpdateTaskInProgress(ctx context.Context, taskID, expectedStatusVersion uint64, changedAt time.Time) error {
	return tx.updateTaskStatus(ctx, taskID, taskmodule.StatusAssigned, taskmodule.StatusInProgress, expectedStatusVersion, changedAt, false)
}

func (tx *transaction) UpdateTaskCompleted(ctx context.Context, taskID, expectedStatusVersion uint64, changedAt time.Time) error {
	return tx.updateTaskStatus(ctx, taskID, taskmodule.StatusInProgress, taskmodule.StatusCompleted, expectedStatusVersion, changedAt, true)
}

func (tx *transaction) updateTaskStatus(ctx context.Context, taskID uint64, from, to taskmodule.Status, expectedStatusVersion uint64, changedAt time.Time, completed bool) error {
	values := map[string]any{"status": string(to), "status_version": expectedStatusVersion + 1, "sync_version": gorm.Expr("sync_version + ?", 1), "updated_at": changedAt.UTC()}
	if completed {
		values["completed_at"] = changedAt.UTC()
	}
	result := tx.db.WithContext(ctx).Table("task_instance").Where("id = ? AND status = ? AND status_version = ?", taskID, string(from), expectedStatusVersion).Updates(values)
	if result.Error != nil {
		return fmt.Errorf("update task %s status: %w", to, result.Error)
	}
	if result.RowsAffected != 1 {
		return &flighttask.CommandError{Code: flighttask.ResultStaleAssignment, Message: "task changed before employee command was applied"}
	}
	return nil
}

func (tx *transaction) UpdateAssignmentAccepted(ctx context.Context, assignmentID, expectedStatusVersion uint64, changedAt time.Time) error {
	return tx.updateAssignmentStatus(ctx, assignmentID, taskmodule.AssignmentConfirmed, taskmodule.AssignmentAccepted, expectedStatusVersion, changedAt, false)
}

func (tx *transaction) UpdateAssignmentCompleted(ctx context.Context, assignmentID, expectedStatusVersion uint64, changedAt time.Time) error {
	return tx.updateAssignmentStatus(ctx, assignmentID, taskmodule.AssignmentAccepted, taskmodule.AssignmentCompleted, expectedStatusVersion, changedAt, true)
}

func (tx *transaction) updateAssignmentStatus(ctx context.Context, assignmentID uint64, from, to taskmodule.AssignmentStatus, expectedStatusVersion uint64, changedAt time.Time, completed bool) error {
	values := map[string]any{"status": string(to), "status_version": expectedStatusVersion + 1, "updated_at": changedAt.UTC()}
	if completed {
		values["completed_at"] = changedAt.UTC()
	} else {
		values["accepted_at"] = changedAt.UTC()
	}
	result := tx.db.WithContext(ctx).Table("task_assignment").Where("id = ? AND status = ? AND status_version = ?", assignmentID, string(from), expectedStatusVersion).Updates(values)
	if result.Error != nil {
		return fmt.Errorf("update assignment %s status: %w", to, result.Error)
	}
	if result.RowsAffected != 1 {
		return &flighttask.CommandError{Code: flighttask.ResultStaleAssignment, Message: "assignment changed before employee command was applied"}
	}
	return nil
}

func (tx *transaction) UpdatePersonnelBusy(ctx context.Context, personnelID, expectedStatusVersion uint64, changedAt time.Time) error {
	return tx.updatePersonnelState(ctx, personnelID, personnelmodule.WorkStateReserved, personnelmodule.WorkStateBusy, expectedStatusVersion, changedAt)
}

func (tx *transaction) UpdatePersonnelIdle(ctx context.Context, personnelID, expectedStatusVersion uint64, changedAt time.Time) error {
	return tx.updatePersonnelState(ctx, personnelID, personnelmodule.WorkStateBusy, personnelmodule.WorkStateIdle, expectedStatusVersion, changedAt)
}

func (tx *transaction) updatePersonnelState(ctx context.Context, personnelID uint64, from, to personnelmodule.WorkState, expectedStatusVersion uint64, changedAt time.Time) error {
	result := tx.db.WithContext(ctx).Table("personnel").Where("id = ? AND work_state = ? AND status_version = ?", personnelID, string(from), expectedStatusVersion).Updates(map[string]any{
		"work_state": string(to), "status_version": expectedStatusVersion + 1, "last_state_changed_at": changedAt.UTC(), "updated_at": changedAt.UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("update personnel %s state: %w", to, result.Error)
	}
	if result.RowsAffected != 1 {
		return &flighttask.CommandError{Code: flighttask.ResultStaleAssignment, Message: "personnel changed before employee command was applied"}
	}
	return nil
}

// ProcessCommand is the command processor used by the Worker for business
// commands. It keeps Core Inbox, domain changes, histories, audit and Outbox
// in the same MySQL transaction.
func (r *Repository) ProcessCommand(ctx context.Context, command sharedEvent.CommandEnvelope, execute coresync.CommandExecution) (bool, error) {
	if err := command.Validate(); err != nil {
		return false, err
	}
	if r == nil || r.db == nil || execute == nil {
		return false, fmt.Errorf("core business command processor is not configured")
	}
	var duplicate bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing coreInboxCommandRow
		findErr := tx.Where("command_id = ?", command.CommandID).First(&existing).Error
		if findErr == nil {
			if !sameCommandEnvelope(existing, command) {
				return &flighttask.CommandError{Code: "command_id_conflict", Message: "command id was already used with different content"}
			}
			if existing.Status == sharedEvent.StatusApplied || (existing.Status == sharedEvent.StatusProcessing && time.Since(existing.ReceivedAt) < time.Minute) {
				duplicate = true
				return nil
			}
			now := time.Now().UTC()
			if err := tx.Model(&coreInboxCommandRow{}).Where("command_id = ?", command.CommandID).Updates(map[string]any{"status": sharedEvent.StatusProcessing, "error_message": nil, "processed_at": nil, "received_at": now}).Error; err != nil {
				return err
			}
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		} else {
			row := newCoreInboxCommandRow(command)
			if err := tx.Create(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrDuplicatedKey) {
					duplicate = true
					return nil
				}
				return err
			}
		}
		if err := execute(ctx, &transaction{db: tx}, command); err != nil {
			return err
		}
		return tx.Model(&coreInboxCommandRow{}).Where("command_id = ?", command.CommandID).Updates(map[string]any{"status": sharedEvent.StatusApplied, "processed_at": time.Now().UTC()}).Error
	})
	if err != nil {
		if recordErr := r.recordFailedBusinessCommand(ctx, command, err); recordErr != nil {
			return duplicate, fmt.Errorf("%w (record failed command: %v)", err, recordErr)
		}
		return duplicate, err
	}
	return duplicate, nil
}

func (r *Repository) recordFailedBusinessCommand(ctx context.Context, command sharedEvent.CommandEnvelope, cause error) error {
	message := cause.Error()
	row := newCoreInboxCommandRow(command)
	row.Status = sharedEvent.StatusFailed
	row.ErrorMessage = &message
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
		return err
	}
	return r.db.WithContext(ctx).Model(&coreInboxCommandRow{}).Where("command_id = ? AND status IN ?", command.CommandID, []string{sharedEvent.StatusPending, sharedEvent.StatusProcessing}).Updates(map[string]any{"status": sharedEvent.StatusFailed, "error_message": message, "processed_at": nil}).Error
}

func (tx *transaction) CreateProbeEvent(ctx context.Context, value coresync.ProbeEvent) error {
	row := architectureProbeEventRow{PublicID: value.PublicID, EventType: value.EventType, Payload: value.Payload, CreatedAt: value.CreatedAt.UTC()}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	if err := tx.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapWriteError(err)
	}
	return nil
}

type taskCommandAssignmentRow struct {
	ID                  uint64    `gorm:"column:id"`
	PublicID            string    `gorm:"column:public_id"`
	TaskID              uint64    `gorm:"column:task_id"`
	CandidateID         uint64    `gorm:"column:candidate_id"`
	PersonnelID         uint64    `gorm:"column:personnel_id"`
	Status              string    `gorm:"column:status"`
	StatusVersion       uint64    `gorm:"column:status_version"`
	ConfirmationID      string    `gorm:"column:confirmation_id"`
	ConfirmedByPublicID string    `gorm:"column:confirmed_by_public_id"`
	ConfirmedAt         time.Time `gorm:"column:confirmed_at"`
	PersonnelPublicID   string    `gorm:"column:personnel_public_id"`
}

func (row taskCommandAssignmentRow) toDomain() taskmodule.Assignment {
	return taskmodule.Assignment{ID: row.ID, PublicID: row.PublicID, TaskID: row.TaskID, CandidateID: row.CandidateID, PersonnelID: row.PersonnelID, PersonnelPublicID: row.PersonnelPublicID, Status: taskmodule.AssignmentStatus(row.Status), StatusVersion: row.StatusVersion, ConfirmationID: row.ConfirmationID, ConfirmedByPublicID: row.ConfirmedByPublicID, ConfirmedAt: row.ConfirmedAt.UTC()}
}

type coreInboxCommandRow struct {
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
	ErrorMessage  *string    `gorm:"column:error_message"`
	ReceivedAt    time.Time  `gorm:"column:received_at"`
	ProcessedAt   *time.Time `gorm:"column:processed_at"`
}

func (coreInboxCommandRow) TableName() string { return "core_inbox" }

func newCoreInboxCommandRow(command sharedEvent.CommandEnvelope) coreInboxCommandRow {
	return coreInboxCommandRow{CommandID: command.CommandID, CommandType: command.CommandType, SchemaVersion: command.SchemaVersion, ActorPublicID: command.ActorPublicID, AggregateID: command.AggregateID, OccurredAt: command.OccurredAt.UTC().Round(time.Microsecond), TraceID: command.TraceID, Payload: append([]byte(nil), command.Payload...), Status: sharedEvent.StatusProcessing, ReceivedAt: time.Now().UTC()}
}

func sameCommandEnvelope(row coreInboxCommandRow, command sharedEvent.CommandEnvelope) bool {
	return sharedEvent.EquivalentCommand(sharedEvent.CommandEnvelope{CommandID: row.CommandID, CommandType: row.CommandType, SchemaVersion: row.SchemaVersion, ActorPublicID: row.ActorPublicID, AggregateID: row.AggregateID, Payload: row.Payload}, command)
}

type architectureProbeEventRow struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID  string    `gorm:"column:public_id"`
	EventType string    `gorm:"column:event_type"`
	Payload   []byte    `gorm:"column:payload"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (architectureProbeEventRow) TableName() string { return "architecture_probe_event" }
