package sync

import (
	"context"
	"errors"
	"testing"
	"time"

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
	retry := command
	retry.TraceID = id.MustPublicID()
	retry.OccurredAt = command.OccurredAt.Add(time.Minute)
	if duplicate, err := store.PutCommand(context.Background(), retry); err != nil || !duplicate {
		t.Fatalf("metadata-only retry result duplicate=%v err=%v", duplicate, err)
	}
	record, err := store.FindCommand(context.Background(), command.CommandID)
	if err != nil || record.Envelope.CommandID != command.CommandID || record.Status != sharedEvent.StatusPending || record.UpdatedAt.IsZero() {
		t.Fatalf("unexpected command status record: %#v err=%v", record, err)
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

func TestMemoryStoreTaskSnapshotTracksEmployeeRevisionAndLag(t *testing.T) {
	store := NewMemoryStore()
	employeePublicID := "employee-snapshot-1"
	projection := TaskProjection{
		PublicID:         "task-snapshot-1",
		EmployeePublicID: employeePublicID,
		FlightDisplayNo:  "CA1234",
		TaskName:         "Snapshot task",
		AreaName:         "A1",
		Status:           "assigned",
		BusinessStatus:   "assigned",
		Message:          "assigned",
		SyncVersion:      1,
		UpdatedAt:        time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
		t.Fatal(err)
	}
	event, err := sharedEvent.NewEvent("task.snapshot.updated.v1", "task", projection.PublicID, "core-test", map[string]string{"status": "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	event.OccurredAt = time.Date(2026, 9, 2, 7, 59, 0, 0, time.UTC)
	if _, err := store.ApplyEvent(context.Background(), event, nil); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 2, 8, 1, 0, 0, time.UTC)
	snapshot, err := store.ListTaskSnapshot(context.Background(), employeePublicID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].PublicID != projection.PublicID {
		t.Fatalf("unexpected snapshot items: %#v", snapshot.Items)
	}
	if snapshot.ProjectionRevision != 1 {
		t.Fatalf("projection revision=%d, want 1", snapshot.ProjectionRevision)
	}
	if !snapshot.SnapshotAt.Equal(now) {
		t.Fatalf("snapshot_at=%s, want %s", snapshot.SnapshotAt, now)
	}
	if !snapshot.ProjectionLagKnown || snapshot.ProjectionLag != 2*time.Minute {
		t.Fatalf("unexpected projection lag: known=%v lag=%s", snapshot.ProjectionLagKnown, snapshot.ProjectionLag)
	}

	if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.ListTaskSnapshot(context.Background(), employeePublicID, now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProjectionRevision != 1 {
		t.Fatalf("same-version replay changed revision to %d", snapshot.ProjectionRevision)
	}

	updated := projection
	updated.SyncVersion = 2
	updated.Status = "in_progress"
	updated.BusinessStatus = "in_progress"
	if err := store.UpsertTaskProjection(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.ListTaskSnapshot(context.Background(), employeePublicID, now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProjectionRevision != 2 || snapshot.Items[0].SyncVersion != 2 {
		t.Fatalf("higher-version update did not advance snapshot: %#v", snapshot)
	}
}

func TestMemoryStoreReportsSyncQueueStats(t *testing.T) {
	store := NewMemoryStore()
	command, err := sharedEvent.NewCommand("probe.queue.v1", "employee-1", "task-1", "trace-1", map[string]string{"value": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	event, err := sharedEvent.NewEvent("probe.queue.event.v1", "task", "task-1", "core", map[string]string{"value": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyEvent(context.Background(), event, nil); err != nil {
		t.Fatal(err)
	}

	stats, err := store.SyncQueueStats(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if stats.PendingCommandCount != 1 {
		t.Fatalf("expected one pending command, got %#v", stats)
	}
	if stats.PendingInboxCount != 0 || stats.FailedInboxCount != 0 {
		t.Fatalf("expected no pending or failed inbox records, got %#v", stats)
	}
}
