package sync

import (
	"context"
	"strings"
	"testing"
	"time"

	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	platformobservability "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"go.uber.org/zap"
)

func TestWorkerRecordsOutboxAndCommandMetrics(t *testing.T) {
	coreStore := coresync.NewMemoryStore()
	edgeStore := edgesync.NewMemoryStore()
	transport := NewMemoryTransport(edgeStore, nil)
	event, err := sharedEvent.NewEvent("probe.metrics.v1", "probe", id.MustPublicID(), "test", map[string]string{"value": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := coreStore.RunTransaction(context.Background(), func(tx coresync.CoreTransaction) error { return tx.AppendOutbox(context.Background(), event) }); err != nil {
		t.Fatal(err)
	}
	command, err := sharedEvent.NewCommand("probe.metrics.command.v1", id.MustPublicID(), id.MustPublicID(), "trace-metrics", map[string]string{"value": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edgeStore.PutCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}

	metrics := platformobservability.NewWorkerRegistry()
	worker := NewWorker(coreStore, edgeStore, transport, zap.NewNop(), RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond}, 10).SetMetrics(metrics)
	if err := worker.DeliverOutbox(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := worker.PullCommands(context.Background(), func(context.Context, coresync.CoreTransaction, sharedEvent.CommandEnvelope) error { return nil }); err != nil {
		t.Fatal(err)
	}

	output := metrics.Render()
	for _, expected := range []string{
		`flight_worker_outbox_delivery_total{result="sent"} 1`,
		`flight_worker_command_batches_total{result="success"} 1`,
		`flight_worker_commands_claimed_total 1`,
		`flight_worker_command_processing_total{result="applied"} 1`,
		`flight_worker_command_ack_total{result="success"} 1`,
		`flight_sync_queue_pending_items{component="worker",queue="core_outbox"} 0`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", expected, output)
		}
	}
}
