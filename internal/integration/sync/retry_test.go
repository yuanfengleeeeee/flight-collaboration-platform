package sync

import (
	"context"
	"testing"
	"time"

	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"go.uber.org/zap"
)

func TestOutboxMovesToFailedAfterRetryBudget(t *testing.T) {
	coreStore := coresync.NewMemoryStore()
	edgeStore := edgesync.NewMemoryStore()
	transport := NewMemoryTransport(edgeStore, nil)
	transport.SetAvailable(false)
	event, err := sharedEvent.NewEvent("probe.failed.v1", "probe", id.MustPublicID(), "test", map[string]string{"failure": "expected"})
	if err != nil {
		t.Fatal(err)
	}
	if err := coreStore.RunTransaction(context.Background(), func(tx coresync.CoreTransaction) error { return tx.AppendOutbox(context.Background(), event) }); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(coreStore, edgeStore, transport, zap.NewNop(), RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond}, 1)
	if err := worker.DeliverOutbox(context.Background()); err != nil {
		t.Fatal(err)
	}
	records := coreStore.PendingOutbox()
	if !containsOutboxStatus(records, event.EventID, sharedEvent.StatusFailed) {
		t.Fatalf("expected failed outbox record, got %#v", records)
	}
}
