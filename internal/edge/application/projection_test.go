package application

import (
	"context"
	"testing"
	"time"

	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

func TestProjectTaskAcceptedEventUsesCoreSnapshot(t *testing.T) {
	store := edgesync.NewMemoryStore()
	envelope, err := sharedEvent.NewEvent("task.accepted.v1", "task", "task-1", "core-flight-task", taskProjectionEventPayload{
		TaskPublicID: "task-1", AssignmentPublicID: "assignment-1", EmployeePublicID: "employee-1", FlightDisplayNo: "CA1234", TaskName: "Ramp", AreaName: "A1", PlannedAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), BusinessStatus: "in_progress", Message: "accepted", SyncVersion: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := projectEvent(context.Background(), store, envelope); err != nil {
		t.Fatal(err)
	}
	values, err := store.ListTaskProjections(context.Background(), "employee-1")
	if err != nil || len(values) != 1 {
		t.Fatalf("unexpected projections: %#v err=%v", values, err)
	}
	if values[0].BusinessStatus != "in_progress" || values[0].Status != "in_progress" || values[0].AssignmentPublicID != "assignment-1" || values[0].SyncVersion != 2 {
		t.Fatalf("unexpected task projection: %#v", values[0])
	}
}
