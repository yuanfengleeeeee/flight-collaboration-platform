package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"go.uber.org/zap"
)

type Worker struct {
	core      coresync.Store
	processor CommandProcessor
	edge      edgesync.Store
	transport EdgeTransport
	log       *zap.Logger
	policy    RetryPolicy
	batchSize int
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
	return &Worker{core: core, processor: processor, edge: edge, transport: transport, log: log, policy: policy, batchSize: batchSize}
}

func (w *Worker) DeliverOutbox(ctx context.Context) error {
	// Core 事务只负责把 Event 写入 Outbox；真正的网络投递在这里完成。
	// 投递失败不会回滚原业务，而是把消息安排为 retry 或 failed。
	if w == nil || w.processor == nil || w.transport == nil {
		return fmt.Errorf("core outbox worker is not configured")
	}
	records, err := w.core.ClaimPendingOutbox(ctx, w.batchSize, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := w.transport.PublishEvent(ctx, record.Envelope); err != nil {
			next, retry := w.policy.Next(record.Attempts+1, time.Now().UTC())
			if retry {
				_ = w.core.MarkOutboxRetry(ctx, record.Envelope.EventID, next, err.Error())
			} else {
				_ = w.core.MarkOutboxFailed(ctx, record.Envelope.EventID, err.Error())
			}
			w.log.Warn("core outbox delivery failed", zap.String("event_id", record.Envelope.EventID), zap.Error(err))
			continue
		}
		if err := w.core.MarkOutboxSent(ctx, record.Envelope.EventID); err != nil {
			return fmt.Errorf("mark outbox event sent: %w", err)
		}
	}
	return nil
}

func (w *Worker) PullCommands(ctx context.Context, handler coresync.CommandExecution) error {
	// Edge 先持久化 Command，Worker 再主动拉取。Core Store 会用 command_id
	// 做 Inbox 幂等，并在事务中调用真正的 Core 业务处理器。
	if w == nil || w.core == nil || w.transport == nil {
		return fmt.Errorf("core command worker is not configured")
	}
	commands, err := w.transport.PullCommands(ctx, w.batchSize)
	if err != nil {
		return err
	}
	for _, record := range commands {
		command := record.Envelope
		duplicate, processErr := w.processor.ProcessCommand(ctx, command, handler)
		if processErr != nil {
			next, retry := w.policy.Next(record.Attempts+1, time.Now().UTC())
			status := sharedEvent.StatusFailed
			var terminal interface{ Terminal() bool }
			if errors.As(processErr, &terminal) && terminal.Terminal() {
				retry = false
			}
			if retry {
				status = sharedEvent.StatusRetry
			}
			if err := w.transport.AcknowledgeCommand(ctx, command.CommandID, status, processErr.Error(), next); err != nil {
				return fmt.Errorf("acknowledge failed command: %w", err)
			}
			continue
		}
		status := sharedEvent.StatusApplied
		if duplicate {
			status = sharedEvent.StatusApplied
		}
		if err := w.transport.AcknowledgeCommand(ctx, command.CommandID, status, "", time.Time{}); err != nil {
			return fmt.Errorf("acknowledge command: %w", err)
		}
	}
	return nil
}
