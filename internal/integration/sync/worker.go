package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	platformobservability "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"go.uber.org/zap"
)

type Worker struct {
	core          coresync.Store
	processor     CommandProcessor
	edge          edgesync.Store
	transport     EdgeTransport
	log           *zap.Logger
	policy        RetryPolicy
	batchSize     int
	metrics       *platformobservability.Registry
	workerID      string
	leaseDuration time.Duration
}

func NewWorker(core coresync.Store, edge edgesync.Store, transport EdgeTransport, log *zap.Logger, policy RetryPolicy, batchSize int) *Worker {
	return NewWorkerWithCommandProcessor(core, core, edge, transport, log, policy, batchSize)
}

func NewWorkerWithCommandProcessor(core coresync.Store, processor CommandProcessor, edge edgesync.Store, transport EdgeTransport, log *zap.Logger, policy RetryPolicy, batchSize int) *Worker {
	if log == nil {
		log = zap.NewNop()
	}
	if batchSize <= 0 {
		batchSize = 20
	}
	if processor == nil {
		processor = core
	}
	return &Worker{core: core, processor: processor, edge: edge, transport: transport, log: log, policy: policy, batchSize: batchSize, workerID: "worker", leaseDuration: 30 * time.Second}
}

func (w *Worker) SetWorkerID(workerID string) *Worker {
	if w != nil && strings.TrimSpace(workerID) != "" {
		w.workerID = strings.TrimSpace(workerID)
	}
	return w
}

func (w *Worker) SetLeaseDuration(duration time.Duration) *Worker {
	if w != nil && duration > 0 {
		w.leaseDuration = duration
	}
	return w
}

func (w *Worker) SetMetrics(metrics *platformobservability.Registry) *Worker {
	if w != nil {
		w.metrics = metrics
	}
	return w
}

func (w *Worker) RefreshMetrics(ctx context.Context) {
	if w == nil || w.metrics == nil || w.core == nil {
		return
	}
	provider, ok := w.core.(coresync.OutboxStatsProvider)
	if !ok {
		return
	}
	stats, err := provider.OutboxStats(ctx, time.Now().UTC())
	if err != nil {
		w.recordError("outbox_stats")
		return
	}
	labels := platformobservability.Labels{"component": "worker", "queue": "core_outbox"}
	w.metrics.SetGauge("flight_sync_queue_pending_items", labels, float64(stats.PendingCount))
	w.metrics.SetGauge("flight_sync_queue_failed_items", labels, float64(stats.FailedCount))
	w.metrics.SetGauge("flight_sync_queue_oldest_age_seconds", labels, stats.OldestPendingAge.Seconds())
}

func (w *Worker) DeliverOutbox(ctx context.Context) error {
	started := time.Now()
	defer w.observeCycle("outbox", started)
	// Core 事务只负责把 Event 写入 Outbox；真正的网络投递在这里完成。
	// 投递失败不会回滚原业务，而是把消息安排为 retry 或 failed。
	if w == nil || w.processor == nil || w.transport == nil {
		w.recordError("outbox_configuration")
		return fmt.Errorf("core outbox worker is not configured")
	}
	records, err := w.claimOutbox(ctx)
	if err != nil {
		w.recordError("outbox_claim")
		return err
	}
	w.addCounter("flight_worker_outbox_claimed_total", nil, float64(len(records)))
	for _, record := range records {
		deliveryStarted := time.Now()
		if err := w.transport.PublishEvent(ctx, record.Envelope); err != nil {
			next, retry := w.policy.Next(record.Attempts+1, time.Now().UTC())
			result := "failed"
			if retry {
				result = "retry"
				_ = w.markOutboxRetry(ctx, record.Envelope.EventID, next, err.Error())
			} else {
				_ = w.markOutboxFailed(ctx, record.Envelope.EventID, err.Error())
			}
			w.addCounter("flight_worker_outbox_delivery_total", platformobservability.Labels{"result": result}, 1)
			w.observe("flight_worker_outbox_delivery_duration_seconds", platformobservability.Labels{"result": result}, deliveryStarted)
			w.log.Warn("core outbox delivery failed", zap.String("event_id", record.Envelope.EventID), zap.Error(err))
			continue
		}
		if err := w.markOutboxSent(ctx, record.Envelope.EventID); err != nil {
			w.addCounter("flight_worker_outbox_delivery_total", platformobservability.Labels{"result": "mark_error"}, 1)
			w.observe("flight_worker_outbox_delivery_duration_seconds", platformobservability.Labels{"result": "mark_error"}, deliveryStarted)
			w.recordError("outbox_mark_sent")
			return fmt.Errorf("mark outbox event sent: %w", err)
		}
		w.addCounter("flight_worker_outbox_delivery_total", platformobservability.Labels{"result": "sent"}, 1)
		w.observe("flight_worker_outbox_delivery_duration_seconds", platformobservability.Labels{"result": "sent"}, deliveryStarted)
	}
	w.RefreshMetrics(ctx)
	return nil
}

func (w *Worker) PullCommands(ctx context.Context, handler coresync.CommandExecution) error {
	started := time.Now()
	defer w.observeCycle("commands", started)
	// Edge 先持久化 Command，Worker 再主动拉取。Core Store 会用 command_id
	// 做 Inbox 幂等，并在事务中调用真正的 Core 业务处理器。
	if w == nil || w.core == nil || w.transport == nil {
		w.recordError("command_configuration")
		return fmt.Errorf("core command worker is not configured")
	}
	pullStarted := time.Now()
	commands, err := w.pullCommands(ctx)
	if err != nil {
		w.addCounter("flight_worker_command_batches_total", platformobservability.Labels{"result": "error"}, 1)
		w.observe("flight_worker_cycle_duration_seconds", platformobservability.Labels{"operation": "command_pull"}, pullStarted)
		w.recordError("command_pull")
		return err
	}
	w.addCounter("flight_worker_command_batches_total", platformobservability.Labels{"result": "success"}, 1)
	w.addCounter("flight_worker_commands_claimed_total", nil, float64(len(commands)))
	for _, record := range commands {
		command := record.Envelope
		processingStarted := time.Now()
		duplicate, processErr := w.processor.ProcessCommand(ctx, command, handler)
		if processErr != nil {
			next, retry := w.policy.Next(record.Attempts+1, time.Now().UTC())
			status := sharedEvent.StatusFailed
			result := "failed"
			var terminal interface{ Terminal() bool }
			if errors.As(processErr, &terminal) && terminal.Terminal() {
				retry = false
			}
			if retry {
				status = sharedEvent.StatusRetry
				result = "retry"
			}
			w.addCounter("flight_worker_command_processing_total", platformobservability.Labels{"result": result}, 1)
			w.observe("flight_worker_command_processing_duration_seconds", platformobservability.Labels{"result": result}, processingStarted)
			ackStarted := time.Now()
			if err := w.acknowledgeCommand(ctx, command.CommandID, status, processErr.Error(), next); err != nil {
				w.addCounter("flight_worker_command_ack_total", platformobservability.Labels{"result": "error"}, 1)
				w.observe("flight_worker_command_ack_duration_seconds", platformobservability.Labels{"result": "error"}, ackStarted)
				w.recordError("command_ack")
				return fmt.Errorf("acknowledge failed command: %w", err)
			}
			w.addCounter("flight_worker_command_ack_total", platformobservability.Labels{"result": "success"}, 1)
			w.observe("flight_worker_command_ack_duration_seconds", platformobservability.Labels{"result": "success"}, ackStarted)
			continue
		}
		result := "applied"
		status := sharedEvent.StatusApplied
		if duplicate {
			status = sharedEvent.StatusApplied
			result = "duplicate"
		}
		w.addCounter("flight_worker_command_processing_total", platformobservability.Labels{"result": result}, 1)
		w.observe("flight_worker_command_processing_duration_seconds", platformobservability.Labels{"result": result}, processingStarted)
		ackStarted := time.Now()
		if err := w.acknowledgeCommand(ctx, command.CommandID, status, "", time.Time{}); err != nil {
			w.addCounter("flight_worker_command_ack_total", platformobservability.Labels{"result": "error"}, 1)
			w.observe("flight_worker_command_ack_duration_seconds", platformobservability.Labels{"result": "error"}, ackStarted)
			w.recordError("command_ack")
			return fmt.Errorf("acknowledge command: %w", err)
		}
		w.addCounter("flight_worker_command_ack_total", platformobservability.Labels{"result": "success"}, 1)
		w.observe("flight_worker_command_ack_duration_seconds", platformobservability.Labels{"result": "success"}, ackStarted)
	}
	w.RefreshMetrics(ctx)
	return nil
}

func (w *Worker) addCounter(name string, labels platformobservability.Labels, value float64) {
	if w != nil && w.metrics != nil {
		w.metrics.Add(name, labels, value)
	}
}

func (w *Worker) observe(name string, labels platformobservability.Labels, started time.Time) {
	if w != nil && w.metrics != nil {
		w.metrics.Observe(name, labels, time.Since(started).Seconds())
	}
}

func (w *Worker) observeCycle(operation string, started time.Time) {
	if w != nil && w.metrics != nil {
		w.metrics.Observe("flight_worker_cycle_duration_seconds", platformobservability.Labels{"operation": operation}, time.Since(started).Seconds())
	}
}

func (w *Worker) recordError(operation string) {
	w.addCounter("flight_worker_errors_total", platformobservability.Labels{"operation": operation}, 1)
}

func (w *Worker) claimOutbox(ctx context.Context) ([]coresync.OutboxRecord, error) {
	now := time.Now().UTC()
	if leased, ok := w.core.(coresync.LeasedStore); ok {
		return leased.ClaimPendingOutboxWithLease(ctx, w.batchSize, now, w.workerID, w.leaseDuration)
	}
	return w.core.ClaimPendingOutbox(ctx, w.batchSize, now)
}

func (w *Worker) markOutboxSent(ctx context.Context, eventID string) error {
	if leased, ok := w.core.(coresync.LeasedStore); ok {
		return leased.MarkOutboxSentWithLease(ctx, eventID, w.workerID)
	}
	return w.core.MarkOutboxSent(ctx, eventID)
}

func (w *Worker) markOutboxRetry(ctx context.Context, eventID string, next time.Time, reason string) error {
	if leased, ok := w.core.(coresync.LeasedStore); ok {
		return leased.MarkOutboxRetryWithLease(ctx, eventID, w.workerID, next, reason)
	}
	return w.core.MarkOutboxRetry(ctx, eventID, next, reason)
}

func (w *Worker) markOutboxFailed(ctx context.Context, eventID string, reason string) error {
	if leased, ok := w.core.(coresync.LeasedStore); ok {
		return leased.MarkOutboxFailedWithLease(ctx, eventID, w.workerID, reason)
	}
	return w.core.MarkOutboxFailed(ctx, eventID, reason)
}

func (w *Worker) pullCommands(ctx context.Context) ([]edgesync.CommandRecord, error) {
	if leased, ok := w.transport.(LeasedEdgeTransport); ok {
		return leased.PullCommandsWithLease(ctx, w.batchSize, w.workerID, w.leaseDuration)
	}
	return w.transport.PullCommands(ctx, w.batchSize)
}

func (w *Worker) acknowledgeCommand(ctx context.Context, commandID, status, reason string, nextAttempt time.Time) error {
	if leased, ok := w.transport.(LeasedEdgeTransport); ok {
		return leased.AcknowledgeCommandWithLease(ctx, commandID, w.workerID, status, reason, nextAttempt)
	}
	return w.transport.AcknowledgeCommand(ctx, commandID, status, reason, nextAttempt)
}
