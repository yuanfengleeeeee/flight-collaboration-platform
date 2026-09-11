// Package notification contains the Edge-local notification port and the
// best-effort in-memory fan-out used before the WebSocket adapter is added.
package notification

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const TaskChangedType = "task_changed"

var (
	ErrInvalidNotification = errors.New("invalid edge notification")
	ErrInvalidSubscription = errors.New("invalid edge notification subscription")
	ErrDuplicateConnection = errors.New("edge notification connection already exists")
)

// TaskChanged is a best-effort hint that an employee's task projection may
// have changed. AudienceEmployeePublicID is used only for in-process routing
// and is deliberately excluded from the wire representation.
type TaskChanged struct {
	Type                     string    `json:"type"`
	NotificationID           string    `json:"notification_id"`
	AudienceEmployeePublicID string    `json:"-"`
	TaskPublicID             string    `json:"task_public_id"`
	SyncVersion              uint64    `json:"sync_version"`
	Reason                   string    `json:"reason"`
	IssuedAt                 time.Time `json:"issued_at"`
}

func NewTaskChanged(employeePublicID, taskPublicID string, syncVersion uint64, reason string, issuedAt time.Time) (TaskChanged, error) {
	notificationID, err := id.NewPublicID()
	if err != nil {
		return TaskChanged{}, fmt.Errorf("generate notification id: %w", err)
	}
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	notification := TaskChanged{
		Type:                     TaskChangedType,
		NotificationID:           notificationID,
		AudienceEmployeePublicID: employeePublicID,
		TaskPublicID:             taskPublicID,
		SyncVersion:              syncVersion,
		Reason:                   reason,
		IssuedAt:                 issuedAt.UTC(),
	}
	if err := notification.Validate(); err != nil {
		return TaskChanged{}, err
	}
	return notification, nil
}

func (notification TaskChanged) Validate() error {
	if notification.Type != TaskChangedType || notification.NotificationID == "" || notification.AudienceEmployeePublicID == "" || notification.TaskPublicID == "" || notification.SyncVersion == 0 || notification.Reason == "" || notification.IssuedAt.IsZero() {
		return ErrInvalidNotification
	}
	return nil
}

// Sink is implemented by a future WebSocket connection adapter or a test
// sink. It must not mutate business state in response to a notification.
type Sink interface {
	Deliver(context.Context, TaskChanged) error
}

// Subscriber registers live connection adapters for an employee. The
// registry is deliberately separate from Publisher so a future cross-instance
// fan-out implementation can publish without exposing connection ownership.
type Subscriber interface {
	Subscribe(employeePublicID, connectionID string, sink Sink) (func(), error)
}

type SinkFunc func(context.Context, TaskChanged) error

func (function SinkFunc) Deliver(ctx context.Context, notification TaskChanged) error {
	return function(ctx, notification)
}

type Publisher interface {
	Publish(context.Context, TaskChanged) (PublishResult, error)
}

type PublishResult struct {
	SubscriberCount int
	DeliveredCount  int
	FailedCount     int
}

// InMemoryFanout keeps only live connection adapters. It is intentionally not
// a durable queue: a restart, dropped connection or failed delivery is
// recovered by the employee's next full task snapshot pull.
type InMemoryFanout struct {
	mu          sync.RWMutex
	subscribers map[string]map[string]Sink
}

func NewInMemoryFanout() *InMemoryFanout {
	return &InMemoryFanout{subscribers: make(map[string]map[string]Sink)}
}

func (fanout *InMemoryFanout) Subscribe(employeePublicID, connectionID string, sink Sink) (func(), error) {
	if fanout == nil || employeePublicID == "" || connectionID == "" || sink == nil {
		return nil, ErrInvalidSubscription
	}
	fanout.mu.Lock()
	defer fanout.mu.Unlock()
	if fanout.subscribers == nil {
		fanout.subscribers = make(map[string]map[string]Sink)
	}
	connections := fanout.subscribers[employeePublicID]
	if connections == nil {
		connections = make(map[string]Sink)
		fanout.subscribers[employeePublicID] = connections
	}
	if _, exists := connections[connectionID]; exists {
		return nil, ErrDuplicateConnection
	}
	connections[connectionID] = sink

	var once sync.Once
	return func() {
		once.Do(func() {
			fanout.mu.Lock()
			defer fanout.mu.Unlock()
			connections := fanout.subscribers[employeePublicID]
			if connections == nil {
				return
			}
			delete(connections, connectionID)
			if len(connections) == 0 {
				delete(fanout.subscribers, employeePublicID)
			}
		})
	}, nil
}

func (fanout *InMemoryFanout) Publish(ctx context.Context, notification TaskChanged) (PublishResult, error) {
	if err := notification.Validate(); err != nil {
		return PublishResult{}, err
	}
	if fanout == nil {
		return PublishResult{}, ErrInvalidSubscription
	}
	if ctx == nil {
		return PublishResult{}, errors.New("notification publish context is nil")
	}
	fanout.mu.RLock()
	connections := fanout.subscribers[notification.AudienceEmployeePublicID]
	sinks := make([]Sink, 0, len(connections))
	connectionIDs := make([]string, 0, len(connections))
	for connectionID, sink := range connections {
		connectionIDs = append(connectionIDs, connectionID)
		sinks = append(sinks, sink)
	}
	fanout.mu.RUnlock()

	result := PublishResult{SubscriberCount: len(sinks)}
	var deliveryErrors []error
	for index, sink := range sinks {
		if err := sink.Deliver(ctx, notification); err != nil {
			result.FailedCount++
			deliveryErrors = append(deliveryErrors, fmt.Errorf("connection %s: %w", connectionIDs[index], err))
			continue
		}
		result.DeliveredCount++
	}
	if len(deliveryErrors) > 0 {
		return result, errors.Join(deliveryErrors...)
	}
	return result, nil
}
