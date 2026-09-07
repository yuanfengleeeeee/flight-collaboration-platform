package exception

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	exceptionmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/exception"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

func TestServiceRecordsAssignedEmployeeException(t *testing.T) {
	command, err := sharedEvent.NewCommand(CommandReportTaskException, "person-1", "task-1", "trace-1", commandPayload{AssignmentPublicID: "assignment-1", ExpectedSyncVersion: 3, Category: "equipment", Severity: "high", Description: "belt stopped", ClientOccurredAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	fake := &exceptionTransaction{task: taskmodule.Instance{ID: 1, PublicID: "task-1", SyncVersion: 3}, assignment: taskmodule.Assignment{ID: 2, PublicID: "assignment-1", PersonnelID: 3, PersonnelPublicID: "person-1"}, person: personnelmodule.CandidateRecord{ID: 3, PublicID: "person-1"}}
	service := NewService(nil, fixedClock{value: time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)})
	if err := service.Handle(context.Background(), fake, command); err != nil {
		t.Fatal(err)
	}
	if fake.record.Status != exceptionmodule.StatusOpen || fake.record.Severity != exceptionmodule.SeverityHigh || fake.record.TaskPublicID != "task-1" || fake.audit.Result != string(exceptionmodule.StatusOpen) {
		t.Fatalf("unexpected exception record: %#v audit=%#v", fake.record, fake.audit)
	}
}

func TestServiceRejectsForeignOrStaleExceptionReports(t *testing.T) {
	base := commandPayload{AssignmentPublicID: "assignment-1", ExpectedSyncVersion: 3, Category: "equipment", Severity: "low", Description: "minor"}
	command, err := sharedEvent.NewCommand(CommandReportTaskException, "other-person", "task-1", "trace-1", base)
	if err != nil {
		t.Fatal(err)
	}
	fake := &exceptionTransaction{task: taskmodule.Instance{ID: 1, PublicID: "task-1", SyncVersion: 4}, assignment: taskmodule.Assignment{ID: 2, PublicID: "assignment-1", PersonnelID: 3, PersonnelPublicID: "person-1"}, person: personnelmodule.CandidateRecord{ID: 3, PublicID: "person-1"}}
	service := NewService(nil, fixedClock{value: time.Now().UTC()})
	if err := service.Handle(context.Background(), fake, command); err == nil || !errors.Is(err, ErrNotAssigned) {
		t.Fatalf("foreign actor error=%v, want not assigned", err)
	}
	command.ActorPublicID = "person-1"
	command.Payload, _ = json.Marshal(commandPayload{AssignmentPublicID: "assignment-1", ExpectedSyncVersion: 3, Category: "equipment", Severity: "low", Description: "minor"})
	if err := service.Handle(context.Background(), fake, command); err == nil || !errors.Is(err, ErrStaleTask) {
		t.Fatalf("stale task error=%v, want stale task", err)
	}
}

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

var _ clock.Clock = fixedClock{}

type exceptionTransaction struct {
	task       taskmodule.Instance
	assignment taskmodule.Assignment
	person     personnelmodule.CandidateRecord
	record     exceptionmodule.Record
	audit      coresync.AuditRecord
}

func (f *exceptionTransaction) FindTaskForUpdate(context.Context, string) (taskmodule.Instance, error) {
	return f.task, nil
}

func (f *exceptionTransaction) FindAssignmentForUpdate(context.Context, uint64, string) (taskmodule.Assignment, error) {
	return f.assignment, nil
}

func (f *exceptionTransaction) FindPersonnelForUpdate(context.Context, uint64) (personnelmodule.CandidateRecord, error) {
	return f.person, nil
}

func (f *exceptionTransaction) CreateTaskException(_ context.Context, value exceptionmodule.Record) error {
	f.record = value
	return nil
}

func (f *exceptionTransaction) AppendAudit(_ context.Context, value coresync.AuditRecord) error {
	f.audit = value
	return nil
}

var _ Transaction = (*exceptionTransaction)(nil)
