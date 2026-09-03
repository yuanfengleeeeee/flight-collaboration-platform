package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	coreapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"gorm.io/gorm"
)

// TestArchitectureProbeAgainstRunningServices is opt-in because it writes
// test-only records to the configured Core and Edge databases. It expects the
// real core-api, edge-api and worker processes to be running.
func TestArchitectureProbeAgainstRunningServices(t *testing.T) {
	if os.Getenv("FLIGHT_RUN_DB_PROBE") != "1" {
		t.Skip("set FLIGHT_RUN_DB_PROBE=1 to run the SQL/HTTP Architecture Probe")
	}

	configPath, err := findProbeConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	coreDB, err := platformmysql.Open(cfg.Core.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer platformmysql.Close(coreDB)
	edgeDB, err := platformmysql.Open(cfg.Edge.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer platformmysql.Close(edgeDB)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := platformmysql.Ping(ctx, coreDB); err != nil {
		t.Fatal(err)
	}
	if err := platformmysql.Ping(ctx, edgeDB); err != nil {
		t.Fatal(err)
	}

	coreStore := coresync.NewSQLStore(coreDB)
	edgeStore := edgesync.NewSQLStore(edgeDB)
	transport, err := NewHTTPTransport(cfg.Sync.EdgeBaseURL, time.Duration(cfg.Sync.RequestTimeoutMS)*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	employeeID := id.MustPublicID()
	firstProjection := sqlProbeProjection(employeeID, "SQL-PROBE-001", 1)
	firstEvent := sqlProbeEvent(firstProjection)
	if err := writeSQLProbe(ctx, coreStore, firstProjection, firstEvent); err != nil {
		t.Fatal(err)
	}
	if err := waitForProjection(ctx, edgeStore, employeeID, "SQL-PROBE-001"); err != nil {
		t.Fatal(err)
	}

	assertSQLCount(t, ctx, coreDB, "SELECT COUNT(*) FROM architecture_probe_event WHERE public_id = ?", 1, firstProjection.PublicID)
	assertSQLCount(t, ctx, coreDB, "SELECT COUNT(*) FROM outbox_event WHERE event_id = ?", 1, firstEvent.EventID)
	assertSQLCount(t, ctx, coreDB, "SELECT COUNT(*) FROM audit_log WHERE resource_id = ? AND action = ?", 1, firstProjection.PublicID, "probe.event.create")

	// Delivering the same event twice exercises the real Edge Inbox unique key.
	if err := transport.PublishEvent(ctx, firstEvent); err != nil {
		t.Fatal(err)
	}
	if err := transport.PublishEvent(ctx, firstEvent); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, ctx, edgeDB, "SELECT COUNT(*) FROM sync_inbox WHERE event_id = ?", 1, firstEvent.EventID)

	// A projection failure is durably recorded and the same event_id can be
	// retried after the payload is corrected.
	recoveryProjection := sqlProbeProjection(employeeID, "SQL-PROBE-RECOVERED-EVENT", 3)
	failedEvent := sqlProbeEvent(recoveryProjection)
	failedEvent.Payload = json.RawMessage(`{"invalid_projection":true}`)
	if err := transport.PublishEvent(ctx, failedEvent); err == nil {
		t.Fatal("invalid projection unexpectedly succeeded")
	}
	assertSQLCount(t, ctx, edgeDB, "SELECT COUNT(*) FROM sync_inbox WHERE event_id = ? AND status = ?", 1, failedEvent.EventID, sharedEvent.StatusFailed)
	if err := transport.PublishEvent(ctx, sqlProbeEventWithID(recoveryProjection, failedEvent.EventID)); err != nil {
		t.Fatal(err)
	}
	if err := waitForProjection(ctx, edgeStore, employeeID, "SQL-PROBE-RECOVERED-EVENT"); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, ctx, edgeDB, "SELECT COUNT(*) FROM sync_inbox WHERE event_id = ? AND status = ?", 1, failedEvent.EventID, sharedEvent.StatusApplied)

	secondProjection := sqlProbeProjection(employeeID, "SQL-PROBE-002", 2)
	command, err := sharedEvent.NewCommand("probe.complete.v1", employeeID, secondProjection.PublicID, id.MustPublicID(), struct {
		Projection edgesync.TaskProjection `json:"projection"`
	}{Projection: secondProjection})
	if err != nil {
		t.Fatal(err)
	}
	firstDuplicate, err := postSQLProbeCommand(ctx, cfg.Sync.EdgeBaseURL, command)
	if err != nil {
		t.Fatal(err)
	}
	secondDuplicate, err := postSQLProbeCommand(ctx, cfg.Sync.EdgeBaseURL, command)
	if err != nil {
		t.Fatal(err)
	}
	if firstDuplicate || !secondDuplicate {
		t.Fatalf("command idempotency duplicate flags = %v, %v", firstDuplicate, secondDuplicate)
	}
	if err := waitForProjection(ctx, edgeStore, employeeID, "SQL-PROBE-002"); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, ctx, edgeDB, "SELECT COUNT(*) FROM mobile_command WHERE command_id = ?", 1, command.CommandID)
	assertSQLCount(t, ctx, coreDB, "SELECT COUNT(*) FROM core_inbox WHERE command_id = ? AND status = ?", 1, command.CommandID, sharedEvent.StatusApplied)
	assertSQLCount(t, ctx, coreDB, "SELECT COUNT(*) FROM outbox_event WHERE correlation_id = ?", 1, command.CommandID)

	items, err := readSQLProbeTasks(ctx, cfg.Sync.EdgeBaseURL, employeeID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSQLProbeTask(items, "SQL-PROBE-001") || !hasSQLProbeTask(items, "SQL-PROBE-002") {
		t.Fatalf("Edge API did not expose both SQL projections: %#v", items)
	}

	// Core Inbox failures are durable too and the same command_id can be
	// retried in the Core application transaction.
	coreRecoveryProjection := sqlProbeProjection(employeeID, "SQL-PROBE-RECOVERED-COMMAND", 4)
	failedCommand, err := sharedEvent.NewCommand("probe.complete.v1", employeeID, coreRecoveryProjection.PublicID, id.MustPublicID(), struct {
		Projection edgesync.TaskProjection `json:"projection"`
	}{Projection: coreRecoveryProjection})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coreStore.ProcessCommand(ctx, failedCommand, func(context.Context, coresync.CoreTransaction, sharedEvent.CommandEnvelope) error {
		return errors.New("temporary core probe failure")
	}); err == nil {
		t.Fatal("failed Core command unexpectedly succeeded")
	}
	assertSQLCount(t, ctx, coreDB, "SELECT COUNT(*) FROM core_inbox WHERE command_id = ? AND status = ?", 1, failedCommand.CommandID, sharedEvent.StatusFailed)
	duplicate, err := coreStore.ProcessCommand(ctx, failedCommand, coreapp.HandleFoundationCommand)
	if err != nil || duplicate {
		t.Fatalf("Core command retry duplicate=%v err=%v", duplicate, err)
	}
	if err := waitForProjection(ctx, edgeStore, employeeID, "SQL-PROBE-RECOVERED-COMMAND"); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, ctx, coreDB, "SELECT COUNT(*) FROM core_inbox WHERE command_id = ? AND status = ?", 1, failedCommand.CommandID, sharedEvent.StatusApplied)
}

func findProbeConfig() (string, error) {
	candidates := []string{
		"configs/config.v2.yaml",
		filepath.Join("..", "..", "..", "configs", "config.v2.yaml"),
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absolute); err == nil {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("architecture probe config not found from %s", mustWorkingDirectory())
}

func mustWorkingDirectory() string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return workingDirectory
}

func writeSQLProbe(ctx context.Context, store *coresync.SQLStore, projection edgesync.TaskProjection, envelope sharedEvent.EventEnvelope) error {
	return store.RunTransaction(ctx, func(tx coresync.CoreTransaction) error {
		if err := tx.CreateProbeEvent(ctx, coresync.ProbeEvent{PublicID: projection.PublicID, EventType: envelope.EventType, Payload: envelope.Payload}); err != nil {
			return err
		}
		if err := tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: "machine", ActorID: id.MustPublicID(), Action: "probe.event.create", ResourceType: "probe", ResourceID: projection.PublicID, Result: "success", RequestID: id.MustPublicID(), TraceID: envelope.TraceID, SourceIP: "127.0.0.1", OccurredAt: envelope.OccurredAt}); err != nil {
			return err
		}
		return tx.AppendOutbox(ctx, envelope)
	})
}

func sqlProbeProjection(employeeID, flightNo string, version uint64) edgesync.TaskProjection {
	return edgesync.TaskProjection{PublicID: id.MustPublicID(), EmployeePublicID: employeeID, FlightDisplayNo: flightNo, TaskName: "Architecture SQL Probe", AreaName: "Probe Area", PlannedAt: time.Now().UTC(), Status: "pending", Message: "test only", SyncVersion: version}
}

func sqlProbeEvent(projection edgesync.TaskProjection) sharedEvent.EventEnvelope {
	event, err := sharedEvent.NewEvent("probe.task_projection.updated.v1", "task", projection.PublicID, "core-sql-probe", struct {
		Projection edgesync.TaskProjection `json:"projection"`
	}{Projection: projection})
	if err != nil {
		panic(err)
	}
	return event
}

func sqlProbeEventWithID(projection edgesync.TaskProjection, eventID string) sharedEvent.EventEnvelope {
	event := sqlProbeEvent(projection)
	event.EventID = eventID
	return event
}

func waitForProjection(ctx context.Context, store *edgesync.SQLStore, employeeID, flightNo string) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		values, err := store.ListTaskProjections(ctx, employeeID)
		if err != nil {
			return err
		}
		if hasSQLProbeTask(values, flightNo) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Edge projection %s: %w", flightNo, ctx.Err())
		case <-ticker.C:
		}
	}
}

func hasSQLProbeTask(values []edgesync.TaskProjection, flightNo string) bool {
	for _, value := range values {
		if value.FlightDisplayNo == flightNo {
			return true
		}
	}
	return false
}

func postSQLProbeCommand(ctx context.Context, baseURL string, command sharedEvent.CommandEnvelope) (bool, error) {
	payload, err := json.Marshal(command)
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/commands", bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	// The Compose probe opts into the development actor adapter explicitly;
	// production Edge routes derive this identity from JWT instead.
	request.Header.Set("X-Employee-Public-ID", command.ActorPublicID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return false, fmt.Errorf("post probe command: HTTP %d", response.StatusCode)
	}
	var body struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return false, err
	}
	return body.Duplicate, nil
}

func readSQLProbeTasks(ctx context.Context, baseURL, employeeID string) ([]edgesync.TaskProjection, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/tasks", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Employee-Public-ID", employeeID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read Edge probe tasks: HTTP %d", response.StatusCode)
	}
	var body struct {
		Items []edgesync.TaskProjection `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Items, nil
}

func assertSQLCount(t *testing.T, ctx context.Context, db *gorm.DB, query string, want int64, args ...any) {
	t.Helper()
	var count int64
	if err := db.WithContext(ctx).Raw(query, args...).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("SQL assertion returned count %d, want %d for %s", count, want, query)
	}
}
