package sync

import (
	"context"
	"fmt"
	"time"

	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

// MemoryTransport is a deterministic transport for the Architecture Probe.
// It can be made unavailable to exercise Outbox retry without Redis or a
// network connection.
type MemoryTransport struct {
	store     edgesync.Store
	project   edgesync.EventProjection
	available bool
}

func NewMemoryTransport(store edgesync.Store, project edgesync.EventProjection) *MemoryTransport {
	return &MemoryTransport{store: store, project: project, available: true}
}

func (t *MemoryTransport) SetAvailable(available bool) { t.available = available }

func (t *MemoryTransport) PublishEvent(ctx context.Context, envelope sharedEvent.EventEnvelope) error {
	if !t.available {
		return fmt.Errorf("edge transport unavailable")
	}
	if t.store == nil {
		return fmt.Errorf("edge store is not configured")
	}
	_, err := t.store.ApplyEvent(ctx, envelope, t.project)
	return err
}

func (t *MemoryTransport) PullCommands(ctx context.Context, limit int) ([]edgesync.CommandRecord, error) {
	if !t.available {
		return nil, fmt.Errorf("edge transport unavailable")
	}
	if t.store == nil {
		return nil, fmt.Errorf("edge store is not configured")
	}
	values, err := t.store.ClaimPendingCommands(ctx, limit, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return values, nil
}

func (t *MemoryTransport) AcknowledgeCommand(ctx context.Context, commandID string, status string, reason string, nextAttempt time.Time) error {
	if !t.available {
		return fmt.Errorf("edge transport unavailable")
	}
	if t.store == nil {
		return fmt.Errorf("edge store is not configured")
	}
	switch status {
	case sharedEvent.StatusApplied, sharedEvent.StatusSent:
		return t.store.MarkCommandSent(ctx, commandID)
	case sharedEvent.StatusRetry:
		if nextAttempt.IsZero() {
			nextAttempt = time.Now().UTC().Add(time.Second)
		}
		return t.store.MarkCommandRetry(ctx, commandID, nextAttempt, reason)
	case sharedEvent.StatusFailed, sharedEvent.StatusRejected:
		return t.store.MarkCommandFailed(ctx, commandID, reason)
	default:
		return fmt.Errorf("unsupported command status %q", status)
	}
}
