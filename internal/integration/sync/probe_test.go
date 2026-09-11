package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coreapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"go.uber.org/zap"
)

func TestArchitectureProbeBidirectionalAtLeastOnceFlow(t *testing.T) {
	ctx := context.Background()
	coreStore := coresync.NewMemoryStore()
	edgeStore := edgesync.NewMemoryStore()
	transport := NewMemoryTransport(edgeStore, projectProbeEvent(edgeStore))
	worker := NewWorker(coreStore, edgeStore, transport, zap.NewNop(), RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}, 20)

	employeeID := id.MustPublicID()
	firstProjection := probeProjection(employeeID, "TEST-001", 1)
	firstEvent := newProjectionEvent(firstProjection)
	writeCoreProbe(t, coreStore, firstEvent)
	if err := worker.DeliverOutbox(ctx); err != nil {
		t.Fatal(err)
	}
	assertProjection(t, edgeStore, employeeID, "TEST-001")

	// A duplicated HTTP/message delivery is accepted but never projected twice.
	if err := transport.PublishEvent(ctx, firstEvent); err != nil {
		t.Fatal(err)
	}
	if record, ok := edgeStore.InboxRecord(firstEvent.EventID); !ok || record.Status != sharedEvent.StatusApplied {
		t.Fatalf("duplicate event did not retain applied inbox record: %#v %v", record, ok)
	}

	// The projection is readable through Edge API and does not require Core DB.
	edgeServer := application.NewServerWithStore(config.ServiceConfig{Port: 18083, Mode: "test"}, nil, nil, zap.NewNop(), edgeStore)
	edgeHTTP := httptest.NewServer(edgeServer.Handler())
	defer edgeHTTP.Close()
	request, err := http.NewRequest(http.MethodGet, edgeHTTP.URL+"/api/v1/tasks", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Employee-Public-ID", employeeID)
	response, err := edgeHTTP.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("edge projection read status = %d", response.StatusCode)
	}
	var body struct {
		Items []edgesync.TaskProjection `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if len(body.Items) != 1 || body.Items[0].FlightDisplayNo != "TEST-001" {
		t.Fatalf("unexpected edge API projection: %#v", body.Items)
	}

	// Edge -> Core: command is stored first, then pulled and executed exactly once.
	secondProjection := probeProjection(employeeID, "TEST-002", 2)
	command, err := sharedEvent.NewCommand("probe.complete.v1", employeeID, secondProjection.PublicID, id.MustPublicID(), struct {
		Projection edgesync.TaskProjection `json:"projection"`
	}{Projection: secondProjection})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := edgeStore.PutCommand(ctx, command); err != nil || duplicate {
		t.Fatalf("put command duplicate=%v err=%v", duplicate, err)
	}
	if duplicate, err := edgeStore.PutCommand(ctx, command); err != nil || !duplicate {
		t.Fatalf("duplicate command duplicate=%v err=%v", duplicate, err)
	}
	if err := worker.PullCommands(ctx, coreapp.HandleFoundationCommand); err != nil {
		t.Fatal(err)
	}
	if err := worker.DeliverOutbox(ctx); err != nil {
		t.Fatal(err)
	}
	assertProjection(t, edgeStore, employeeID, "TEST-002")
	if record, ok := edgeStore.CommandRecord(command.CommandID); !ok || record.Status != sharedEvent.StatusSent {
		t.Fatalf("command was not acknowledged as sent: %#v %v", record, ok)
	}

	// Edge outage does not roll back the Core transaction; retry succeeds later.
	thirdProjection := probeProjection(employeeID, "TEST-003", 3)
	thirdEvent := newProjectionEvent(thirdProjection)
	writeCoreProbe(t, coreStore, thirdEvent)
	transport.SetAvailable(false)
	if err := worker.DeliverOutbox(ctx); err != nil {
		t.Fatal(err)
	}
	if records := coreStore.PendingOutbox(); !containsOutboxStatus(records, thirdEvent.EventID, sharedEvent.StatusRetry) {
		t.Fatalf("expected retry outbox record after edge outage: %#v", records)
	}
	transport.SetAvailable(true)
	if err := coreStore.MarkOutboxRetry(ctx, thirdEvent.EventID, time.Now().UTC().Add(-time.Second), "test retry now"); err != nil {
		t.Fatal(err)
	}
	if err := worker.DeliverOutbox(ctx); err != nil {
		t.Fatal(err)
	}
	assertProjection(t, edgeStore, employeeID, "TEST-003")

	// Simulate a worker exit after claiming a message. A new worker can claim
	// the processing record because delivery is at-least-once and idempotent.
	fourthProjection := probeProjection(employeeID, "TEST-004", 4)
	fourthEvent := newProjectionEvent(fourthProjection)
	writeCoreProbe(t, coreStore, fourthEvent)
	claimed, err := coreStore.ClaimPendingOutbox(ctx, 1, time.Now().UTC())
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim before simulated worker exit: %#v err=%v", claimed, err)
	}
	restartedWorker := NewWorker(coreStore, edgeStore, transport, zap.NewNop(), RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}, 20)
	if err := restartedWorker.DeliverOutbox(ctx); err != nil {
		t.Fatal(err)
	}
	assertProjection(t, edgeStore, employeeID, "TEST-004")

	// No Redis client was involved; the Core transaction and outbox remain the
	// source of truth when optional infrastructure is absent.
	if len(coreStore.AuditRecords()) < 2 {
		t.Fatalf("expected audit records for probe writes, got %d", len(coreStore.AuditRecords()))
	}
}

func writeCoreProbe(t *testing.T, store *coresync.MemoryStore, envelope sharedEvent.EventEnvelope) {
	t.Helper()
	err := store.RunTransaction(context.Background(), func(tx coresync.CoreTransaction) error {
		if err := tx.CreateProbeEvent(context.Background(), coresync.ProbeEvent{PublicID: envelope.AggregateID, EventType: envelope.EventType, Payload: envelope.Payload}); err != nil {
			return err
		}
		if err := tx.AppendAudit(context.Background(), coresync.AuditRecord{ActorType: "machine", ActorID: id.MustPublicID(), Action: "probe.event.create", ResourceType: "probe", ResourceID: envelope.AggregateID, Result: "success", TraceID: envelope.TraceID, OccurredAt: envelope.OccurredAt}); err != nil {
			return err
		}
		return tx.AppendOutbox(context.Background(), envelope)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func probeProjection(employeeID, flightNo string, version uint64) edgesync.TaskProjection {
	return edgesync.TaskProjection{PublicID: id.MustPublicID(), EmployeePublicID: employeeID, FlightDisplayNo: flightNo, TaskName: "Architecture Probe", AreaName: "Probe Area", PlannedAt: time.Now().UTC(), Status: "pending", Message: "test only", SyncVersion: version}
}

func newProjectionEvent(projection edgesync.TaskProjection) sharedEvent.EventEnvelope {
	event, err := sharedEvent.NewEvent("probe.task_projection.updated.v1", "task", projection.PublicID, "core-probe", struct {
		Projection edgesync.TaskProjection `json:"projection"`
	}{Projection: projection})
	if err != nil {
		panic(err)
	}
	return event
}

func projectProbeEvent(store *edgesync.MemoryStore) edgesync.EventProjection {
	return func(ctx context.Context, envelope sharedEvent.EventEnvelope) error {
		var payload struct {
			Projection edgesync.TaskProjection `json:"projection"`
		}
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return err
		}
		return store.UpsertTaskProjection(ctx, payload.Projection)
	}
}

func assertProjection(t *testing.T, store *edgesync.MemoryStore, employeeID, flightNo string) {
	t.Helper()
	values, err := store.ListTaskProjections(context.Background(), employeeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		if value.FlightDisplayNo == flightNo {
			return
		}
	}
	t.Fatalf("projection %s not found in %#v", flightNo, values)
}

func containsOutboxStatus(records []coresync.OutboxRecord, eventID, status string) bool {
	for _, record := range records {
		if record.Envelope.EventID == eventID && record.Status == status {
			return true
		}
	}
	return false
}
