package application

import (
	"context"
	"fmt"

	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

// HandleFoundationCommand is deliberately limited to the test/dev Probe. It
// does not complete a real Task or mutate any future business aggregate.
func HandleFoundationCommand(ctx context.Context, tx coresync.CoreTransaction, command sharedEvent.CommandEnvelope) error {
	if command.CommandType != "probe.complete.v1" {
		return fmt.Errorf("unsupported foundation command %q", command.CommandType)
	}
	event, err := sharedEvent.NewEvent("probe.task_projection.updated.v1", "task", command.AggregateID, "core-worker", jsonPayload(command.Payload))
	if err != nil {
		return err
	}
	event.TraceID = command.TraceID
	event.CorrelationID = command.CommandID
	if err := tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: "human", ActorID: command.ActorPublicID, Action: "probe.command.process", ResourceType: "probe", ResourceID: command.AggregateID, Result: "success", TraceID: command.TraceID, OccurredAt: command.OccurredAt}); err != nil {
		return fmt.Errorf("append foundation command audit: %w", err)
	}
	if err := tx.AppendOutbox(ctx, event); err != nil {
		return fmt.Errorf("append foundation command outbox: %w", err)
	}
	return nil
}

type jsonPayload []byte

func (p jsonPayload) MarshalJSON() ([]byte, error) {
	if len(p) == 0 {
		return []byte(`{}`), nil
	}
	return p, nil
}
