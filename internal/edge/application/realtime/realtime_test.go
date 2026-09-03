package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/websocket"

	edgenotification "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/notification"
	platformobservability "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func TestTicketStoreIsSingleUse(t *testing.T) {
	store := NewTicketStore(time.Minute)
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	principal := platformsecurity.Principal{Type: platformsecurity.HumanPrincipal, PublicID: "staff-1", SessionID: "session-1", Roles: []string{platformsecurity.RoleStaff}}

	ticket, err := store.Issue(principal)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Ticket == "" || ticket.Protocol != Protocol || ticket.TicketProtocol != TicketProtocol(ticket.Ticket) || !ticket.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected ticket response: %#v", ticket)
	}
	consumed, err := store.Consume(ticket.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.PublicID != principal.PublicID || consumed.SessionID != principal.SessionID {
		t.Fatalf("unexpected consumed principal: %#v", consumed)
	}
	if _, err := store.Consume(ticket.Ticket); !errors.Is(err, ErrInvalidTicket) {
		t.Fatalf("expected single-use ticket error, got %v", err)
	}
}

func TestTicketFromProtocolHeaderDoesNotAcceptURLTokens(t *testing.T) {
	ticket := "short-lived-ticket"
	if value, ok := TicketFromProtocolHeader(Protocol + ", " + TicketProtocol(ticket)); !ok || value != ticket {
		t.Fatalf("failed to parse ticket subprotocol: value=%q ok=%v", value, ok)
	}
	if _, ok := TicketFromProtocolHeader(""); ok {
		t.Fatal("empty protocol header should not produce a ticket")
	}
	if strings.Contains(TicketProtocol(ticket), "?") {
		t.Fatal("ticket protocol must not be a URL query parameter")
	}
}

func TestWebSocketDeliversTaskChangedAndRequiresPong(t *testing.T) {
	fanout := edgenotification.NewInMemoryFanout()
	metrics := platformobservability.NewHTTPRegistry()
	endpoint := NewEndpoint(fanout, metrics, zap.NewNop(), Config{HeartbeatInterval: 10 * time.Millisecond, IdleTimeout: 80 * time.Millisecond, WriteTimeout: time.Second})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		endpoint.ServeHTTP(writer, request, "staff-1")
	}))
	defer server.Close()
	defer endpoint.Close()

	connection := dialTestWebSocket(t, server.URL, []string{Protocol})
	defer connection.Close()
	connection.SetReadDeadline(time.Now().Add(time.Second))
	var ready readyMessage
	if err := websocket.JSON.Receive(connection, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != "ready" || ready.Protocol != Protocol {
		t.Fatalf("unexpected ready message: %#v", ready)
	}

	var ping heartbeatMessage
	if err := websocket.JSON.Receive(connection, &ping); err != nil {
		t.Fatal(err)
	}
	if ping.Type != "ping" {
		t.Fatalf("expected heartbeat ping, got %#v", ping)
	}
	if err := websocket.JSON.Send(connection, heartbeatMessage{Type: "pong", IssuedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	notification, err := edgenotification.NewTaskChanged("staff-1", "task-1", 7, "assigned", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	result, err := fanout.Publish(context.Background(), notification)
	if err != nil || result.SubscriberCount != 1 || result.DeliveredCount != 1 || result.FailedCount != 0 {
		t.Fatalf("unexpected task notification result: result=%#v err=%v", result, err)
	}
	var received edgenotification.TaskChanged
	if err := websocket.JSON.Receive(connection, &received); err != nil {
		t.Fatal(err)
	}
	if received.Type != edgenotification.TaskChangedType || received.NotificationID != notification.NotificationID || received.TaskPublicID != notification.TaskPublicID || received.SyncVersion != notification.SyncVersion || received.Reason != notification.Reason || received.AudienceEmployeePublicID != "" {
		t.Fatalf("unexpected WebSocket task notification: %#v", received)
	}
}

func TestWebSocketClosesIdleConnection(t *testing.T) {
	fanout := edgenotification.NewInMemoryFanout()
	endpoint := NewEndpoint(fanout, platformobservability.NewHTTPRegistry(), zap.NewNop(), Config{HeartbeatInterval: 10 * time.Millisecond, IdleTimeout: 35 * time.Millisecond, WriteTimeout: time.Second})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		endpoint.ServeHTTP(writer, request, "staff-1")
	}))
	defer server.Close()
	defer endpoint.Close()

	connection := dialTestWebSocket(t, server.URL, []string{Protocol})
	defer connection.Close()
	connection.SetReadDeadline(time.Now().Add(time.Second))
	var ready readyMessage
	if err := websocket.JSON.Receive(connection, &ready); err != nil {
		t.Fatal(err)
	}
	for {
		var message heartbeatMessage
		if err := websocket.JSON.Receive(connection, &message); err != nil {
			return
		}
		if message.Type != "ping" {
			t.Fatalf("unexpected idle-test message: %#v", message)
		}
	}
}

func dialTestWebSocket(t *testing.T, httpURL string, protocols []string) *websocket.Conn {
	t.Helper()
	parsed, err := url.Parse(httpURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Scheme = "ws"
	parsed.Path = WebSocketPath
	config, err := websocket.NewConfig(parsed.String(), httpURL)
	if err != nil {
		t.Fatal(err)
	}
	config.Protocol = append([]string(nil), protocols...)
	connection, err := websocket.DialConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	return connection
}
