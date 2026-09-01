// Package sync defines the transport boundary between Core and Edge.
package sync

import (
	"context"
	"time"

	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

// CommandProcessor owns the Core transaction used to apply a pulled command.
// The default implementation is the Foundation sync store; the business
// MySQL adapter uses the same interface so domain writes share one transaction
// with Core Inbox, Audit and Outbox.
type CommandProcessor interface {
	ProcessCommand(ctx context.Context, command sharedEvent.CommandEnvelope, execute coresync.CommandExecution) (duplicate bool, err error)
}

type EdgeTransport interface {
	PublishEvent(ctx context.Context, envelope sharedEvent.EventEnvelope) error
	PullCommands(ctx context.Context, limit int) ([]edgesync.CommandRecord, error)
	AcknowledgeCommand(ctx context.Context, commandID string, status string, reason string, nextAttempt time.Time) error
}

type CoreCommandHandler interface {
	HandleCommand(ctx context.Context, command sharedEvent.CommandEnvelope) error
}
