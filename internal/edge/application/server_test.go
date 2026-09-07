package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	edgenotification "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/notification"
	edgerealtime "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/realtime"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"go.uber.org/zap"
)

func TestHealthEndpointsWithoutMySQL(t *testing.T) {
	server := NewServer(config.ServiceConfig{Port: 18082, Mode: "test"}, nil, nil, zap.NewNop())
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	request, err := http.NewRequest(http.MethodGet, ts.URL+"/health/live", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Request-ID", "edge-test-request")
	response, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Request-ID") != "edge-test-request" || response.Header.Get("X-Trace-ID") == "" {
		t.Fatalf("unexpected liveness response: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

func TestEmployeeCommandIdempotencyAndStatus(t *testing.T) {
	store := edgesync.NewMemoryStore()
	server := NewServerWithStoreAndAuth(config.ServiceConfig{Port: 18083, Mode: "test", AllowDevActorHeaders: true}, nil, nil, zap.NewNop(), store, nil)
	handler := server.Handler()

	payload := []byte(`{"command_id":"command-1","assignment_public_id":"assignment-1","expected_sync_version":1,"note":"accept"}`)
	first := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/tasks/task-1/accept", "staff-1", payload)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first command status=%d body=%s", first.Code, first.Body.String())
	}
	var firstBody struct {
		CommandID string `json:"command_id"`
		Status    string `json:"status"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatal(err)
	}
	if firstBody.CommandID != "command-1" || firstBody.Status != "pending" || firstBody.Duplicate {
		t.Fatalf("unexpected first command response: %#v", firstBody)
	}

	second := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/tasks/task-1/accept", "staff-1", payload)
	if second.Code != http.StatusAccepted {
		t.Fatalf("duplicate command status=%d body=%s", second.Code, second.Body.String())
	}
	var secondBody struct {
		CommandID string `json:"command_id"`
		Status    string `json:"status"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondBody); err != nil {
		t.Fatal(err)
	}
	if secondBody.CommandID != "command-1" || secondBody.Status != "pending" || !secondBody.Duplicate {
		t.Fatalf("unexpected duplicate command response: %#v", secondBody)
	}

	status := executeEmployeeRequest(handler, http.MethodGet, "/api/v1/commands/command-1", "staff-1", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("command status=%d body=%s", status.Code, status.Body.String())
	}
	var statusBody struct {
		Data struct {
			CommandID string `json:"command_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &statusBody); err != nil {
		t.Fatal(err)
	}
	if statusBody.Data.CommandID != "command-1" || statusBody.Data.Status != "pending" {
		t.Fatalf("unexpected command status response: %#v", statusBody)
	}

	forbidden := executeEmployeeRequest(handler, http.MethodGet, "/api/v1/commands/command-1", "staff-2", nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("foreign command status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
	notFound := executeEmployeeRequest(handler, http.MethodGet, "/api/v1/commands/missing", "staff-1", nil)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("missing command status=%d body=%s", notFound.Code, notFound.Body.String())
	}

	conflict := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/tasks/task-1/accept", "staff-1", []byte(`{"command_id":"command-1","assignment_public_id":"assignment-1","expected_sync_version":2}`))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflicting command status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestInternalCommandLeaseHTTPFlow(t *testing.T) {
	store := edgesync.NewMemoryStore()
	server := NewServerWithStore(config.ServiceConfig{Port: 18087, Mode: "test"}, nil, nil, zap.NewNop(), store)
	command, err := sharedEvent.NewCommand("test.command.lease.v1", id.MustPublicID(), id.MustPublicID(), id.MustPublicID(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := store.PutCommand(context.Background(), command); err != nil || duplicate {
		t.Fatalf("put command duplicate=%v err=%v", duplicate, err)
	}

	pendingRequest := httptest.NewRequest(http.MethodGet, "/internal/sync/v1/commands/pending?worker_id=worker-a&lease_seconds=30", nil)
	pendingResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(pendingResponse, pendingRequest)
	if pendingResponse.Code != http.StatusOK {
		t.Fatalf("pending status=%d body=%s", pendingResponse.Code, pendingResponse.Body.String())
	}
	var pendingBody struct {
		Items []edgesync.CommandRecord `json:"items"`
	}
	if err := json.Unmarshal(pendingResponse.Body.Bytes(), &pendingBody); err != nil {
		t.Fatal(err)
	}
	if len(pendingBody.Items) != 1 || pendingBody.Items[0].Envelope.CommandID != command.CommandID {
		t.Fatalf("unexpected pending commands: %#v", pendingBody.Items)
	}
	if second, err := store.ClaimPendingCommandsWithLease(context.Background(), 1, time.Now().UTC(), "worker-b", time.Minute); err != nil || len(second) != 0 {
		t.Fatalf("leased command was concurrently reclaimed: %#v err=%v", second, err)
	}

	ackBody := []byte(`{"status":"sent","lease_owner":"worker-b"}`)
	wrongAck := httptest.NewRequest(http.MethodPost, "/internal/sync/v1/commands/"+command.CommandID+"/ack", bytes.NewReader(ackBody))
	wrongAck.Header.Set("Content-Type", "application/json")
	wrongResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(wrongResponse, wrongAck)
	if wrongResponse.Code != http.StatusInternalServerError {
		t.Fatalf("wrong owner ack status=%d body=%s", wrongResponse.Code, wrongResponse.Body.String())
	}

	ackBody = []byte(`{"status":"sent","lease_owner":"worker-a"}`)
	rightAck := httptest.NewRequest(http.MethodPost, "/internal/sync/v1/commands/"+command.CommandID+"/ack", bytes.NewReader(ackBody))
	rightAck.Header.Set("Content-Type", "application/json")
	rightResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(rightResponse, rightAck)
	if rightResponse.Code != http.StatusNoContent {
		t.Fatalf("right owner ack status=%d body=%s", rightResponse.Code, rightResponse.Body.String())
	}
	record, ok := store.CommandRecord(command.CommandID)
	if !ok || record.Status != sharedEvent.StatusSent || record.LeaseOwner != "" {
		t.Fatalf("unexpected acknowledged command: %#v exists=%v", record, ok)
	}
}

func TestEmployeeTaskSnapshotReturnsRecoveryMetadata(t *testing.T) {
	store := edgesync.NewMemoryStore()
	projection := edgesync.TaskProjection{
		PublicID:         "task-http-1",
		EmployeePublicID: "staff-snapshot-1",
		FlightDisplayNo:  "CA5678",
		TaskName:         "Snapshot task",
		AreaName:         "Gate A",
		Status:           "assigned",
		BusinessStatus:   "assigned",
		Message:          "ready",
		SyncVersion:      1,
		UpdatedAt:        time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
		t.Fatal(err)
	}
	server := NewServerWithStoreAndAuth(config.ServiceConfig{Port: 18084, Mode: "test", AllowDevActorHeaders: true}, nil, nil, zap.NewNop(), store, nil)
	response := executeEmployeeRequest(server.Handler(), http.MethodGet, "/api/v1/tasks", projection.EmployeePublicID, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("task snapshot status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Items                []edgesync.TaskProjection `json:"items"`
		Source               string                    `json:"source"`
		SyncMode             string                    `json:"sync_mode"`
		SnapshotAt           string                    `json:"snapshot_at"`
		ProjectionRevision   uint64                    `json:"projection_revision"`
		ProjectionLagSeconds float64                   `json:"projection_lag_seconds"`
		ProjectionLagState   string                    `json:"projection_lag_state"`
		NextCursor           *string                   `json:"next_cursor"`
		ResetRequired        bool                      `json:"reset_required"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].PublicID != projection.PublicID {
		t.Fatalf("unexpected task snapshot items: %#v", body.Items)
	}
	if body.Source != "edge_projection" || body.SyncMode != "full_snapshot" || body.SnapshotAt == "" {
		t.Fatalf("unexpected snapshot identity: %#v", body)
	}
	if body.ProjectionRevision != 1 || body.ProjectionLagState != "unknown" || body.ProjectionLagSeconds != 0 {
		t.Fatalf("unexpected snapshot recovery metadata: %#v", body)
	}
	if body.NextCursor != nil || body.ResetRequired {
		t.Fatalf("unexpected cursor metadata: %#v", body)
	}

	empty := executeEmployeeRequest(server.Handler(), http.MethodGet, "/api/v1/tasks", "staff-without-tasks", nil)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty task snapshot status=%d body=%s", empty.Code, empty.Body.String())
	}
	var emptyBody struct {
		Items []edgesync.TaskProjection `json:"items"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatal(err)
	}
	if emptyBody.Items == nil || len(emptyBody.Items) != 0 {
		t.Fatalf("empty snapshot should return an empty array: %#v", emptyBody.Items)
	}
}

func TestEmployeeHistoryAndNotificationRoutesAreScopedAndIdempotent(t *testing.T) {
	store := edgesync.NewMemoryStore()
	now := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	for _, projection := range []edgesync.TaskProjection{
		{PublicID: "history-completed", EmployeePublicID: "staff-history-1", Status: "completed", BusinessStatus: "completed", SyncVersion: 1, UpdatedAt: now},
		{PublicID: "history-active", EmployeePublicID: "staff-history-1", Status: "assigned", BusinessStatus: "assigned", SyncVersion: 1, UpdatedAt: now.Add(time.Minute)},
	} {
		if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertNotificationProjection(context.Background(), edgesync.NotificationProjection{PublicID: "notification-1", EmployeePublicID: "staff-history-1", Title: "task update", Message: "task completed", Status: "unread", SyncVersion: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	server := NewServerWithStoreAndAuth(config.ServiceConfig{Port: 18089, Mode: "test", AllowDevActorHeaders: true}, nil, nil, zap.NewNop(), store, nil)
	handler := server.Handler()
	history := executeEmployeeRequest(handler, http.MethodGet, "/api/v1/history", "staff-history-1", nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), "history-completed") || strings.Contains(history.Body.String(), "history-active") {
		t.Fatalf("unexpected employee history response: status=%d body=%s", history.Code, history.Body.String())
	}
	notifications := executeEmployeeRequest(handler, http.MethodGet, "/api/v1/notifications?status=unread", "staff-history-1", nil)
	if notifications.Code != http.StatusOK || !strings.Contains(notifications.Body.String(), "notification-1") {
		t.Fatalf("unexpected notification response: status=%d body=%s", notifications.Code, notifications.Body.String())
	}
	read := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/notifications/notification-1/read", "staff-history-1", nil)
	if read.Code != http.StatusOK {
		t.Fatalf("mark notification read status=%d body=%s", read.Code, read.Body.String())
	}
	readAgain := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/notifications/notification-1/read", "staff-history-1", nil)
	if readAgain.Code != http.StatusOK {
		t.Fatalf("mark notification read should be idempotent: status=%d body=%s", readAgain.Code, readAgain.Body.String())
	}
	foreign := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/notifications/notification-1/read", "staff-history-2", nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign notification should be hidden: status=%d body=%s", foreign.Code, foreign.Body.String())
	}
}

func TestEmployeeExceptionReportQueuesDurableCommand(t *testing.T) {
	store := edgesync.NewMemoryStore()
	server := NewServerWithStoreAndAuth(config.ServiceConfig{Port: 18090, Mode: "test", AllowDevActorHeaders: true}, nil, nil, zap.NewNop(), store, nil)
	handler := server.Handler()
	payload := []byte(`{"command_id":"exception-command-1","assignment_public_id":"assignment-1","expected_sync_version":7,"category":"equipment","severity":"high","description":"Belt loader unavailable"}`)
	first := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/tasks/task-exception-1/exceptions", "staff-exception-1", payload)
	if first.Code != http.StatusAccepted || strings.Contains(first.Body.String(), "invalid_exception") {
		t.Fatalf("exception report status=%d body=%s", first.Code, first.Body.String())
	}
	record, ok := store.CommandRecord("exception-command-1")
	if !ok || record.Envelope.CommandType != "employee_report_task_exception.v1" || record.Envelope.AggregateID != "task-exception-1" {
		t.Fatalf("unexpected exception command: %#v exists=%v", record, ok)
	}
	duplicate := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/tasks/task-exception-1/exceptions", "staff-exception-1", payload)
	if duplicate.Code != http.StatusAccepted || !strings.Contains(duplicate.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate exception report status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	invalid := executeEmployeeRequest(handler, http.MethodPost, "/api/v1/tasks/task-exception-1/exceptions", "staff-exception-1", []byte(`{"command_id":"exception-command-2","assignment_public_id":"assignment-1","expected_sync_version":7,"category":"equipment","severity":"unknown","description":"Belt loader unavailable"}`))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid exception report status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestProjectionNotificationRunsAfterCommitAndIsBestEffort(t *testing.T) {
	store := edgesync.NewMemoryStore()
	fanout := edgenotification.NewInMemoryFanout()
	var received []edgenotification.TaskChanged
	if _, err := fanout.Subscribe("staff-notify", "healthy", edgenotification.SinkFunc(func(ctx context.Context, notification edgenotification.TaskChanged) error {
		values, err := store.ListTaskProjections(ctx, notification.AudienceEmployeePublicID)
		if err != nil {
			return err
		}
		projectionVisible := false
		for _, value := range values {
			if value.PublicID == notification.TaskPublicID {
				projectionVisible = true
				break
			}
		}
		if !projectionVisible {
			return errors.New("notification arrived before projection commit")
		}
		received = append(received, notification)
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := fanout.Subscribe("staff-notify", "failed", edgenotification.SinkFunc(func(context.Context, edgenotification.TaskChanged) error {
		return errors.New("temporary connection failure")
	})); err != nil {
		t.Fatal(err)
	}
	server := NewServerWithStoreAndAuthAndIdentityAndNotifications(config.ServiceConfig{Port: 18085, Mode: "test"}, nil, nil, zap.NewNop(), store, nil, nil, fanout)
	handler := server.Handler()

	envelope, err := sharedEvent.NewEvent("task.assigned.v1", "task", "task-notify-1", "core-test", taskProjectionEventPayload{
		TaskPublicID: "task-notify-1", AssignmentPublicID: "assignment-notify-1", EmployeePublicID: "staff-notify",
		FlightDisplayNo: "CA1001", TaskName: "Notify task", AreaName: "A1", PlannedAt: time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC),
		BusinessStatus: "assigned", Message: "assigned", SyncVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	requestBody, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	first := executeInternalEventRequest(handler, requestBody)
	if first.Code != http.StatusAccepted {
		t.Fatalf("event status=%d body=%s", first.Code, first.Body.String())
	}
	if len(received) != 1 || received[0].Type != edgenotification.TaskChangedType || received[0].TaskPublicID != "task-notify-1" || received[0].Reason != "assigned" {
		t.Fatalf("unexpected notification: %#v", received)
	}
	values, err := store.ListTaskProjections(context.Background(), "staff-notify")
	if err != nil || len(values) != 1 {
		t.Fatalf("projection was not committed: values=%#v err=%v", values, err)
	}

	duplicate := executeInternalEventRequest(handler, requestBody)
	if duplicate.Code != http.StatusAccepted {
		t.Fatalf("duplicate event status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	var duplicateBody struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(duplicate.Body.Bytes(), &duplicateBody); err != nil {
		t.Fatal(err)
	}
	if !duplicateBody.Duplicate || len(received) != 1 {
		t.Fatalf("duplicate event should not fan out again: body=%s received=%d", duplicate.Body.String(), len(received))
	}

	envelope.EventID = "event-notify-2"
	envelope.AggregateID = "task-notify-2"
	envelope.Payload, err = json.Marshal(taskProjectionEventPayload{
		TaskPublicID: "task-notify-2", AssignmentPublicID: "assignment-notify-2", EmployeePublicID: "staff-notify",
		FlightDisplayNo: "CA1002", TaskName: "Notify task 2", AreaName: "A1", PlannedAt: time.Date(2026, 9, 2, 8, 5, 0, 0, time.UTC),
		BusinessStatus: "assigned", Message: "assigned", SyncVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedDelivery := executeInternalEventRequest(handler, mustMarshal(t, envelope))
	if failedDelivery.Code != http.StatusAccepted {
		t.Fatalf("best-effort notification failure changed event status=%d body=%s", failedDelivery.Code, failedDelivery.Body.String())
	}
	values, err = store.ListTaskProjections(context.Background(), "staff-notify")
	if err != nil || len(values) != 2 || len(received) != 2 {
		t.Fatalf("projection/healthy delivery did not survive failed subscriber: values=%#v received=%d err=%v", values, len(received), err)
	}
	metricsRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, metricsRequest)
	if metricsResponse.Code != http.StatusOK || !strings.Contains(metricsResponse.Body.String(), `flight_notification_delivery_total{component="edge",result="failed"} 2`) {
		t.Fatalf("notification failure metric was not recorded: status=%d body=%s", metricsResponse.Code, metricsResponse.Body.String())
	}
}

func TestEmployeeWebSocketUsesJWTAndSingleUseTicket(t *testing.T) {
	jwtAuthenticator, err := platformsecurity.NewJWTAuthenticator(config.JWTConfig{Secret: "websocket-test-secret-123", Issuer: "edge-test", Audience: "flight-platform-edge"})
	if err != nil {
		t.Fatal(err)
	}
	principal := platformsecurity.Principal{Type: platformsecurity.HumanPrincipal, PublicID: "staff-websocket", SessionID: "session-websocket", Roles: []string{platformsecurity.RoleStaff}}
	accessToken, err := jwtAuthenticator.IssueForSession(principal, principal.SessionID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	fanout := edgenotification.NewInMemoryFanout()
	edgeServer := NewServerWithStoreAndAuthAndIdentityAndNotifications(config.ServiceConfig{Port: 18086, Mode: "test"}, nil, nil, zap.NewNop(), edgesync.NewMemoryStore(), jwtAuthenticator, nil, fanout)
	httpServer := httptest.NewServer(edgeServer.Handler())
	defer httpServer.Close()
	defer edgeServer.Shutdown(context.Background())

	ticketRequest, err := http.NewRequest(http.MethodPost, httpServer.URL+edgerealtime.TicketPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	ticketRequest.Header.Set("Authorization", "Bearer "+accessToken)
	ticketResponse, err := http.DefaultClient.Do(ticketRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer ticketResponse.Body.Close()
	if ticketResponse.StatusCode != http.StatusOK {
		t.Fatalf("ticket status=%d", ticketResponse.StatusCode)
	}
	var ticketBody struct {
		Data edgerealtime.TicketResponse `json:"data"`
	}
	if err := json.NewDecoder(ticketResponse.Body).Decode(&ticketBody); err != nil {
		t.Fatal(err)
	}
	if ticketBody.Data.Ticket == "" || ticketBody.Data.Protocol != edgerealtime.Protocol {
		t.Fatalf("unexpected ticket response: %#v", ticketBody.Data)
	}

	wsURL := strings.Replace(httpServer.URL, "http://", "ws://", 1) + edgerealtime.WebSocketPath
	wsConfig, err := websocket.NewConfig(wsURL, httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	wsConfig.Protocol = []string{edgerealtime.Protocol, edgerealtime.TicketProtocol(ticketBody.Data.Ticket)}
	connection, err := websocket.DialConfig(wsConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if len(connection.Config().Protocol) != 1 || connection.Config().Protocol[0] != edgerealtime.Protocol {
		t.Fatalf("credential subprotocol was reflected: %#v", connection.Config().Protocol)
	}
	connection.SetReadDeadline(time.Now().Add(time.Second))
	var ready struct {
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
	}
	if err := websocket.JSON.Receive(connection, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != "ready" || ready.Protocol != edgerealtime.Protocol {
		t.Fatalf("unexpected WebSocket ready message: %#v", ready)
	}

	notification, err := edgenotification.NewTaskChanged(principal.PublicID, "task-websocket", 2, "assigned", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fanout.Publish(context.Background(), notification); err != nil {
		t.Fatal(err)
	}
	var received edgenotification.TaskChanged
	if err := websocket.JSON.Receive(connection, &received); err != nil {
		t.Fatal(err)
	}
	if received.TaskPublicID != notification.TaskPublicID || received.SyncVersion != notification.SyncVersion || received.AudienceEmployeePublicID != "" {
		t.Fatalf("unexpected employee WebSocket notification: %#v", received)
	}

	replayedConfig, err := websocket.NewConfig(wsURL, httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	replayedConfig.Protocol = []string{edgerealtime.Protocol, edgerealtime.TicketProtocol(ticketBody.Data.Ticket)}
	if _, err := websocket.DialConfig(replayedConfig); err == nil {
		t.Fatal("replayed realtime ticket should not open a second connection")
	}
}

func executeInternalEventRequest(handler http.Handler, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/internal/sync/v1/events", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func executeEmployeeRequest(handler http.Handler, method, path, employeePublicID string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("X-Employee-Public-ID", employeePublicID)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
