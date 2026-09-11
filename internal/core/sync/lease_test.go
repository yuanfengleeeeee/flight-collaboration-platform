package sync

import (
	"context"
	"testing"
	"time"

	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

func TestMemoryOutboxLeaseCanBeReclaimedAfterExpiry(t *testing.T) {
	store := NewMemoryStore()
	event, err := sharedEvent.NewEvent("test.lease.v1", "test", id.MustPublicID(), "test", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RunTransaction(context.Background(), func(tx CoreTransaction) error {
		return tx.AppendOutbox(context.Background(), event)
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claimed, err := store.ClaimPendingOutbox(context.Background(), 1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("legacy claim = %#v, err=%v", claimed, err)
	}
	reclaimed, err := store.ClaimPendingOutboxWithLease(context.Background(), 1, time.Now().UTC(), "worker-2", time.Minute)
	if err != nil || len(reclaimed) != 1 {
		t.Fatalf("expired legacy claim was not reclaimed = %#v, err=%v", reclaimed, err)
	}
}
