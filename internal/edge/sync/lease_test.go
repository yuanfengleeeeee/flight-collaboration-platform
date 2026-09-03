package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

func TestMemoryCommandLeasePreventsConcurrentClaimAndRejectsStaleAck(t *testing.T) {
	store := NewMemoryStore()
	command, err := sharedEvent.NewCommand("test.lease.v1", id.MustPublicID(), id.MustPublicID(), id.MustPublicID(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := store.PutCommand(context.Background(), command); err != nil || duplicate {
		t.Fatalf("put command duplicate=%v err=%v", duplicate, err)
	}
	now := time.Now().UTC()
	claimed, err := store.ClaimPendingCommandsWithLease(context.Background(), 1, now, "worker-1", time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("first claim = %#v, err=%v", claimed, err)
	}
	if second, err := store.ClaimPendingCommandsWithLease(context.Background(), 1, now, "worker-2", time.Minute); err != nil || len(second) != 0 {
		t.Fatalf("concurrent claim = %#v, err=%v", second, err)
	}
	if err := store.MarkCommandSentWithLease(context.Background(), command.CommandID, "worker-2"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale acknowledgement error = %v", err)
	}
	reclaimed, err := store.ClaimPendingCommandsWithLease(context.Background(), 1, now.Add(time.Minute+time.Second), "worker-2", time.Minute)
	if err != nil || len(reclaimed) != 1 {
		t.Fatalf("expired claim = %#v, err=%v", reclaimed, err)
	}
}
