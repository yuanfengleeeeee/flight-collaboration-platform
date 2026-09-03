package notification

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestInMemoryFanoutIsolatesEmployeeConnections(t *testing.T) {
	fanout := NewInMemoryFanout()
	var first, second int
	if _, err := fanout.Subscribe("employee-1", "connection-1", SinkFunc(func(context.Context, TaskChanged) error { first++; return nil })); err != nil {
		t.Fatal(err)
	}
	if _, err := fanout.Subscribe("employee-2", "connection-2", SinkFunc(func(context.Context, TaskChanged) error { second++; return nil })); err != nil {
		t.Fatal(err)
	}
	notification, err := NewTaskChanged("employee-1", "task-1", 3, "assigned", time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	result, err := fanout.Publish(context.Background(), notification)
	if err != nil {
		t.Fatal(err)
	}
	if result.SubscriberCount != 1 || result.DeliveredCount != 1 || result.FailedCount != 0 || first != 1 || second != 0 {
		t.Fatalf("unexpected isolated delivery: result=%#v first=%d second=%d", result, first, second)
	}
}

func TestInMemoryFanoutContinuesAfterDeliveryFailure(t *testing.T) {
	fanout := NewInMemoryFanout()
	var delivered int
	if _, err := fanout.Subscribe("employee-1", "failed", SinkFunc(func(context.Context, TaskChanged) error { return errors.New("connection closed") })); err != nil {
		t.Fatal(err)
	}
	if _, err := fanout.Subscribe("employee-1", "healthy", SinkFunc(func(context.Context, TaskChanged) error { delivered++; return nil })); err != nil {
		t.Fatal(err)
	}
	notification, err := NewTaskChanged("employee-1", "task-1", 4, "completed", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	result, err := fanout.Publish(context.Background(), notification)
	if err == nil || result.SubscriberCount != 2 || result.DeliveredCount != 1 || result.FailedCount != 1 || delivered != 1 {
		t.Fatalf("unexpected failure handling: result=%#v err=%v delivered=%d", result, err, delivered)
	}
}

func TestInMemoryFanoutUnsubscribeIsIdempotent(t *testing.T) {
	fanout := NewInMemoryFanout()
	delivered := 0
	unsubscribe, err := fanout.Subscribe("employee-1", "connection-1", SinkFunc(func(context.Context, TaskChanged) error { delivered++; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	unsubscribe()
	unsubscribe()
	notification, err := NewTaskChanged("employee-1", "task-1", 1, "assigned", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	result, err := fanout.Publish(context.Background(), notification)
	if err != nil || result.SubscriberCount != 0 || delivered != 0 {
		t.Fatalf("unexpected delivery after unsubscribe: result=%#v err=%v delivered=%d", result, err, delivered)
	}
}

func TestTaskChangedOmitsAudienceFromWirePayload(t *testing.T) {
	notification, err := NewTaskChanged("employee-1", "task-1", 2, "assigned", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(notification)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || string(encoded) == "{}" || strings.Contains(string(encoded), "employee-1") {
		t.Fatalf("notification wire payload leaked audience: %s", encoded)
	}
}

func TestRedisEnvelopeCarriesRoutingSeparatelyFromNotification(t *testing.T) {
	notification, err := NewTaskChanged("employee-1", "task-1", 2, "assigned", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(redisNotificationEnvelope{Origin: "edge-1", AudienceEmployeePublicID: notification.AudienceEmployeePublicID, Notification: notification})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "employee-1") || !strings.Contains(string(encoded), "audience_employee_public_id") {
		t.Fatalf("redis envelope omitted routing key: %s", encoded)
	}
	var decoded redisNotificationEnvelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.Notification.AudienceEmployeePublicID = decoded.AudienceEmployeePublicID
	if err := decoded.Notification.Validate(); err != nil {
		t.Fatalf("redis envelope could not restore notification audience: %v", err)
	}
}
