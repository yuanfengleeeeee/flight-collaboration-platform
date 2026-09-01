package flighttask

import (
	"context"
	"testing"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

func TestEmployeeAcceptAndCompleteAreAtomicStateTransitions(t *testing.T) {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	fake := &employeeCommandFakeTransaction{
		task:       taskmodule.Instance{ID: 1, PublicID: "task-1", FlightDisplayNo: "CA1234", AreaName: "A1", Name: "Ramp", Message: "do work", PlannedAt: now, Status: taskmodule.StatusAssigned, StatusVersion: 1, SyncVersion: 1},
		assignment: taskmodule.Assignment{ID: 2, PublicID: "assignment-1", TaskID: 1, PersonnelID: 3, PersonnelPublicID: "employee-1", Status: taskmodule.AssignmentConfirmed},
		person:     personnelmodule.CandidateRecord{ID: 3, PublicID: "employee-1", WorkState: personnelmodule.WorkStateReserved, StatusVersion: 1},
	}
	service := NewEmployeeCommandService(iam.NewAuthorizer(), fixedClock{value: now})
	accept := newEmployeeCommand(t, CommandEmployeeAcceptTask, "employee-1", 1, "accept-1")
	if err := service.Handle(context.Background(), fake, accept); err != nil {
		t.Fatal(err)
	}
	if fake.task.Status != taskmodule.StatusInProgress || fake.assignment.Status != taskmodule.AssignmentAccepted || fake.person.WorkState != personnelmodule.WorkStateBusy || fake.task.SyncVersion != 2 || len(fake.outbox) != 1 || fake.outbox[0].EventType != EventTaskAccepted {
		t.Fatalf("accept transition incomplete: task=%#v assignment=%#v person=%#v outbox=%#v", fake.task, fake.assignment, fake.person, fake.outbox)
	}
	complete := newEmployeeCommand(t, CommandEmployeeCompleteTask, "employee-1", 2, "complete-1")
	if err := service.Handle(context.Background(), fake, complete); err != nil {
		t.Fatal(err)
	}
	if fake.task.Status != taskmodule.StatusCompleted || fake.assignment.Status != taskmodule.AssignmentCompleted || fake.person.WorkState != personnelmodule.WorkStateIdle || fake.task.SyncVersion != 3 || len(fake.outbox) != 2 || fake.outbox[1].EventType != EventTaskCompleted {
		t.Fatalf("complete transition incomplete: task=%#v assignment=%#v person=%#v outbox=%#v", fake.task, fake.assignment, fake.person, fake.outbox)
	}
	if len(fake.taskHistories) != 2 || len(fake.assignmentHistories) != 2 || len(fake.personnelHistories) != 2 || len(fake.audits) != 2 {
		t.Fatalf("transition side effects incomplete: task=%d assignment=%d personnel=%d audit=%d", len(fake.taskHistories), len(fake.assignmentHistories), len(fake.personnelHistories), len(fake.audits))
	}
}

func TestEmployeeCommandRejectsDifferentActor(t *testing.T) {
	fake := &employeeCommandFakeTransaction{task: taskmodule.Instance{ID: 1, PublicID: "task-1", Status: taskmodule.StatusAssigned, SyncVersion: 1}, assignment: taskmodule.Assignment{ID: 2, PublicID: "assignment-1", TaskID: 1, PersonnelID: 3, PersonnelPublicID: "employee-1", Status: taskmodule.AssignmentConfirmed}, person: personnelmodule.CandidateRecord{ID: 3, PublicID: "employee-1", WorkState: personnelmodule.WorkStateReserved, StatusVersion: 1}}
	command := newEmployeeCommand(t, CommandEmployeeAcceptTask, "employee-2", 1, "accept-forbidden")
	err := NewEmployeeCommandService(iam.NewAuthorizer(), fixedClock{value: time.Now().UTC()}).Handle(context.Background(), fake, command)
	if CommandCode(err) != ResultNotAssignedToActor || len(fake.outbox) != 0 {
		t.Fatalf("unexpected actor rejection: code=%s err=%v outbox=%d", CommandCode(err), err, len(fake.outbox))
	}
}

func newEmployeeCommand(t *testing.T, commandType, actor string, expectedVersion uint64, commandID string) sharedEvent.CommandEnvelope {
	t.Helper()
	command, err := sharedEvent.NewCommand(commandType, actor, "task-1", "trace-1", employeeTaskCommandPayload{AssignmentPublicID: "assignment-1", ExpectedSyncVersion: expectedVersion, ClientOccurredAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	command.CommandID = commandID
	return command
}

type employeeCommandFakeTransaction struct {
	task                taskmodule.Instance
	assignment          taskmodule.Assignment
	person              personnelmodule.CandidateRecord
	taskHistories       []taskmodule.StatusHistory
	assignmentHistories []taskmodule.AssignmentStatusHistory
	personnelHistories  []personnelmodule.StatusHistory
	audits              []coresync.AuditRecord
	outbox              []sharedEvent.EventEnvelope
}

func (f *employeeCommandFakeTransaction) FindTaskForUpdate(context.Context, string) (taskmodule.Instance, error) {
	return f.task, nil
}
func (f *employeeCommandFakeTransaction) FindAssignmentForUpdate(context.Context, uint64, string) (taskmodule.Assignment, error) {
	return f.assignment, nil
}
func (f *employeeCommandFakeTransaction) FindPersonnelForUpdate(context.Context, uint64) (personnelmodule.CandidateRecord, error) {
	return f.person, nil
}
func (f *employeeCommandFakeTransaction) UpdateTaskInProgress(_ context.Context, _ uint64, expected uint64, _ time.Time) error {
	if f.task.StatusVersion != expected {
		return ErrNotFound
	}
	f.task.Status, f.task.StatusVersion, f.task.SyncVersion = taskmodule.StatusInProgress, expected+1, f.task.SyncVersion+1
	return nil
}
func (f *employeeCommandFakeTransaction) UpdateTaskCompleted(_ context.Context, _ uint64, expected uint64, _ time.Time) error {
	if f.task.StatusVersion != expected {
		return ErrNotFound
	}
	f.task.Status, f.task.StatusVersion, f.task.SyncVersion = taskmodule.StatusCompleted, expected+1, f.task.SyncVersion+1
	return nil
}
func (f *employeeCommandFakeTransaction) UpdateAssignmentAccepted(_ context.Context, _ uint64, expected uint64, _ time.Time) error {
	if f.assignment.StatusVersion != expected {
		return ErrNotFound
	}
	f.assignment.Status, f.assignment.StatusVersion = taskmodule.AssignmentAccepted, expected+1
	return nil
}
func (f *employeeCommandFakeTransaction) UpdateAssignmentCompleted(_ context.Context, _ uint64, expected uint64, _ time.Time) error {
	if f.assignment.StatusVersion != expected {
		return ErrNotFound
	}
	f.assignment.Status, f.assignment.StatusVersion = taskmodule.AssignmentCompleted, expected+1
	return nil
}
func (f *employeeCommandFakeTransaction) UpdatePersonnelBusy(_ context.Context, _ uint64, expected uint64, _ time.Time) error {
	if f.person.StatusVersion != expected {
		return ErrNotFound
	}
	f.person.WorkState, f.person.StatusVersion = personnelmodule.WorkStateBusy, expected+1
	return nil
}
func (f *employeeCommandFakeTransaction) UpdatePersonnelIdle(_ context.Context, _ uint64, expected uint64, _ time.Time) error {
	if f.person.StatusVersion != expected {
		return ErrNotFound
	}
	f.person.WorkState, f.person.StatusVersion = personnelmodule.WorkStateIdle, expected+1
	return nil
}
func (f *employeeCommandFakeTransaction) CreateTaskStatusHistory(_ context.Context, value taskmodule.StatusHistory) error {
	f.taskHistories = append(f.taskHistories, value)
	return nil
}
func (f *employeeCommandFakeTransaction) CreateAssignmentStatusHistory(_ context.Context, value taskmodule.AssignmentStatusHistory) error {
	f.assignmentHistories = append(f.assignmentHistories, value)
	return nil
}
func (f *employeeCommandFakeTransaction) CreatePersonnelStatusHistory(_ context.Context, value personnelmodule.StatusHistory) error {
	f.personnelHistories = append(f.personnelHistories, value)
	return nil
}
func (f *employeeCommandFakeTransaction) AppendAudit(_ context.Context, value coresync.AuditRecord) error {
	f.audits = append(f.audits, value)
	return nil
}
func (f *employeeCommandFakeTransaction) AppendOutbox(_ context.Context, value sharedEvent.EventEnvelope) error {
	f.outbox = append(f.outbox, value)
	return nil
}

var _ security.Authorizer = iam.NewAuthorizer()
