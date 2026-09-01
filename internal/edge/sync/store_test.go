package sync

import (
	"context"
	"errors"
	"testing"

	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

func TestMemoryStoreAppliesDuplicateEventOnce(t *testing.T) {
	store := NewMemoryStore()
	event, err := sharedEvent.NewEvent("probe.task_projection.updated.v1", "task", id.MustPublicID(), "core-test", map[string]string{"status": "pending"})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	project := func(context.Context, sharedEvent.EventEnvelope) error { count++; return nil }
	duplicate, err := store.ApplyEvent(context.Background(), event, project)
	if err != nil || duplicate {
		t.Fatalf("first event result duplicate=%v err=%v", duplicate, err)
	}
	duplicate, err = store.ApplyEvent(context.Background(), event, project)
	if err != nil || !duplicate {
		t.Fatalf("second event result duplicate=%v err=%v", duplicate, err)
	}
	if count != 1 {
		t.Fatalf("projector ran %d times", count)
	}
}

func TestMemoryStoreRetriesFailedEvent(t *testing.T) {
	store := NewMemoryStore()
	event, err := sharedEvent.NewEvent("probe.task_projection.updated.v1", "task", id.MustPublicID(), "core-test", map[string]string{"status": "retry"})
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	project := func(context.Context, sharedEvent.EventEnvelope) error {
		attempts++
		if attempts == 1 {
			return errors.New("projection temporarily unavailable")
		}
		return nil
	}
	if _, err := store.ApplyEvent(context.Background(), event, project); err == nil {
		t.Fatal("first projection attempt unexpectedly succeeded")
	}
	duplicate, err := store.ApplyEvent(context.Background(), event, project)
	if err != nil || duplicate {
		t.Fatalf("retry event duplicate=%v err=%v", duplicate, err)
	}
	if attempts != 2 {
		t.Fatalf("projection attempts=%d, want 2", attempts)
	}
}

func TestMemoryStoreCommandDeduplicationAndProjectionVisibility(t *testing.T) {
	store := NewMemoryStore()
	command, err := sharedEvent.NewCommand("probe.complete.v1", id.MustPublicID(), id.MustPublicID(), id.MustPublicID(), map[string]string{"ok": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := store.PutCommand(context.Background(), command); err != nil || duplicate {
		t.Fatalf("put command result duplicate=%v err=%v", duplicate, err)
	}
	if duplicate, err := store.PutCommand(context.Background(), command); err != nil || !duplicate {
		t.Fatalf("duplicate command result duplicate=%v err=%v", duplicate, err)
	}
	projection := TaskProjection{PublicID: id.MustPublicID(), EmployeePublicID: command.ActorPublicID, FlightDisplayNo: "TEST-001", TaskName: "Probe", AreaName: "Area", Status: "pending", Message: "probe", SyncVersion: 1}
	if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
		t.Fatal(err)
	}
	values, err := store.ListTaskProjections(context.Background(), command.ActorPublicID)
	if err != nil || len(values) != 1 || values[0].PublicID != projection.PublicID {
		t.Fatalf("unexpected projection values: %#v err=%v", values, err)
	}
}

func TestMemoryStoreProjectionVersionsConvergeAndRejectConflicts(t *testing.T) {
	store := NewMemoryStore()
	projection := TaskProjection{PublicID: "task-1", AssignmentPublicID: "assignment-1", EmployeePublicID: "employee-1", FlightDisplayNo: "CA1234", TaskName: "Ramp", AreaName: "A1", Status: "assigned", BusinessStatus: "assigned", Message: "do work", SyncVersion: 2}
	if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTaskProjection(context.Background(), TaskProjection{PublicID: projection.PublicID, EmployeePublicID: projection.EmployeePublicID, Status: "assigned", BusinessStatus: "assigned", Message: projection.Message, SyncVersion: 1}); err != nil {
		t.Fatalf("stale projection should be ignored: %v", err)
	}
	if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
		t.Fatalf("same version and payload should be idempotent: %v", err)
	}
	conflicting := projection
	conflicting.Message = "changed at same version"
	if !errors.Is(store.UpsertTaskProjection(context.Background(), conflicting), ErrProjectionVersionConflict) {
		t.Fatal("expected same-version projection conflict")
	}
}
