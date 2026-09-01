package sync

import (
	"context"
	"testing"
	"time"

	edgeapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application"
	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"go.uber.org/zap"
	"net/http/httptest"
)

func TestHTTPTransportAndEdgeInboxAreIdempotent(t *testing.T) {
	store := edgesync.NewMemoryStore()
	server := edgeapp.NewServerWithStore(config.ServiceConfig{Port: 18084, Mode: "test"}, nil, nil, zap.NewNop(), store)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	transport, err := NewHTTPTransport(httpServer.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	projection := probeProjection(id.MustPublicID(), "HTTP-001", 1)
	event := newProjectionEvent(projection)
	if err := transport.PublishEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := transport.PublishEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	values, err := store.ListTaskProjections(context.Background(), projection.EmployeePublicID)
	if err != nil || len(values) != 1 {
		t.Fatalf("HTTP duplicate event produced %#v err=%v", values, err)
	}

	command, err := sharedEvent.NewCommand("probe.complete.v1", projection.EmployeePublicID, projection.PublicID, id.MustPublicID(), map[string]string{"ok": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	commands, err := transport.PullCommands(context.Background(), 10)
	if err != nil || len(commands) != 1 || commands[0].Envelope.CommandID != command.CommandID {
		t.Fatalf("unexpected pulled commands %#v err=%v", commands, err)
	}
	if err := transport.AcknowledgeCommand(context.Background(), command.CommandID, sharedEvent.StatusApplied, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if record, ok := store.CommandRecord(command.CommandID); !ok || record.Status != sharedEvent.StatusSent {
		t.Fatalf("command acknowledgement not persisted: %#v %v", record, ok)
	}
}
