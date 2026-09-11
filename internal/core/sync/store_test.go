package sync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

func TestMemoryStoreTransactionRollsBackAllFoundationWrites(t *testing.T) {
	store := NewMemoryStore()
	envelope, err := sharedEvent.NewEvent("probe.created.v1", "probe", id.MustPublicID(), "core-test", map[string]string{"ok": "true"})
	if err != nil {
		t.Fatal(err)
	}
	err = store.RunTransaction(context.Background(), func(tx CoreTransaction) error {
		if err := tx.CreateProbeEvent(context.Background(), ProbeEvent{PublicID: id.MustPublicID(), EventType: "probe.created.v1", Payload: json.RawMessage(`{"ok":true}`)}); err != nil {
			return err
		}
		if err := tx.AppendAudit(context.Background(), AuditRecord{ActorType: "machine", ActorID: id.MustPublicID(), Action: "probe.create", ResourceType: "probe", ResourceID: envelope.AggregateID, Result: "success"}); err != nil {
			return err
		}
		if err := tx.AppendOutbox(context.Background(), envelope); err != nil {
			return err
		}
		return errTestRollback
	})
	if err != errTestRollback {
		t.Fatalf("transaction error = %v, want rollback sentinel", err)
	}
	if len(store.ProbeEvents()) != 0 || len(store.AuditRecords()) != 0 || len(store.PendingOutbox()) != 0 {
		t.Fatal("transaction side effects survived rollback")
	}
}

func TestMemoryStoreCommandIsIdempotent(t *testing.T) {
	store := NewMemoryStore()
	command, err := sharedEvent.NewCommand("probe.complete.v1", id.MustPublicID(), id.MustPublicID(), id.MustPublicID(), map[string]string{"action": "complete"})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	execute := func(ctx context.Context, tx CoreTransaction, value sharedEvent.CommandEnvelope) error {
		count++
		return tx.AppendAudit(ctx, AuditRecord{ActorType: "human", ActorID: value.ActorPublicID, Action: "probe.complete", ResourceType: "probe", ResourceID: value.AggregateID, Result: "success"})
	}
	duplicate, err := store.ProcessCommand(context.Background(), command, execute)
	if err != nil || duplicate {
		t.Fatalf("first command result duplicate=%v err=%v", duplicate, err)
	}
	duplicate, err = store.ProcessCommand(context.Background(), command, execute)
	if err != nil || !duplicate {
		t.Fatalf("second command result duplicate=%v err=%v", duplicate, err)
	}
	if count != 1 || len(store.AuditRecords()) != 1 {
		t.Fatalf("command executed %d times with %d audits", count, len(store.AuditRecords()))
	}
}

func TestMemoryStoreRetriesFailedCommand(t *testing.T) {
	store := NewMemoryStore()
	command, err := sharedEvent.NewCommand("probe.complete.v1", id.MustPublicID(), id.MustPublicID(), id.MustPublicID(), map[string]string{"action": "retry"})
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	execute := func(ctx context.Context, tx CoreTransaction, value sharedEvent.CommandEnvelope) error {
		attempts++
		if attempts == 1 {
			return errTestRollback
		}
		return tx.AppendAudit(ctx, AuditRecord{ActorType: "human", ActorID: value.ActorPublicID, Action: "probe.retry", ResourceType: "probe", ResourceID: value.AggregateID, Result: "success"})
	}
	if _, err := store.ProcessCommand(context.Background(), command, execute); err != errTestRollback {
		t.Fatalf("first command error = %v, want retry sentinel", err)
	}
	duplicate, err := store.ProcessCommand(context.Background(), command, execute)
	if err != nil || duplicate {
		t.Fatalf("retry command duplicate=%v err=%v", duplicate, err)
	}
	if attempts != 2 || len(store.AuditRecords()) != 1 {
		t.Fatalf("command attempts=%d audits=%d", attempts, len(store.AuditRecords()))
	}
}

func TestMemoryStoreRejectsCommandIDPayloadConflict(t *testing.T) {
	store := NewMemoryStore()
	command, err := sharedEvent.NewCommand("probe.complete.v1", id.MustPublicID(), id.MustPublicID(), id.MustPublicID(), map[string]string{"action": "one"})
	if err != nil {
		t.Fatal(err)
	}
	execute := func(context.Context, CoreTransaction, sharedEvent.CommandEnvelope) error { return nil }
	if _, err := store.ProcessCommand(context.Background(), command, execute); err != nil {
		t.Fatal(err)
	}
	conflicting := command
	conflicting.Payload = json.RawMessage(`{"action":"two"}`)
	if !errors.Is(mustProcessCommand(store, conflicting, execute), ErrCommandIDConflict) {
		t.Fatal("expected command id conflict")
	}
}

func mustProcessCommand(store *MemoryStore, command sharedEvent.CommandEnvelope, execute CommandExecution) error {
	_, err := store.ProcessCommand(context.Background(), command, execute)
	return err
}

var errTestRollback = &rollbackError{}

type rollbackError struct{}

func (*rollbackError) Error() string { return "test rollback" }

func TestMemoryStoreReportsOutboxStats(t *testing.T) {
	store := NewMemoryStore()
	event, err := sharedEvent.NewEvent("probe.queue.v1", "probe", id.MustPublicID(), "test", map[string]string{"value": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RunTransaction(context.Background(), func(tx CoreTransaction) error { return tx.AppendOutbox(context.Background(), event) }); err != nil {
		t.Fatal(err)
	}

	stats, err := store.OutboxStats(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if stats.PendingCount != 1 || stats.FailedCount != 0 {
		t.Fatalf("unexpected outbox stats: %#v", stats)
	}
}
