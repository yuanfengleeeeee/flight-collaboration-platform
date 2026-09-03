// Package realtime exposes the Edge employee WebSocket adapter.
//
// WebSocket delivery is intentionally best effort. It owns live connections
// only; task state, recovery and command truth remain in Edge Projection and
// the HTTP APIs.
package realtime

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/websocket"

	edgenotification "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/notification"
	platformobservability "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const (
	Protocol             = "flight.realtime.v1"
	TicketPath           = "/api/v1/realtime/ticket"
	WebSocketPath        = "/api/v1/ws"
	ticketProtocolPrefix = "flight.realtime.ticket."

	defaultHeartbeatInterval = 25 * time.Second
	defaultIdleTimeout       = 75 * time.Second
	defaultWriteTimeout      = 5 * time.Second
	DefaultTicketTTL         = 30 * time.Second
	defaultMaxPayloadBytes   = 4 << 10
)

var (
	ErrRealtimeUnavailable = errors.New("realtime notification service is unavailable")
	ErrInvalidPrincipal    = errors.New("invalid realtime employee principal")
	ErrInvalidTicket       = errors.New("invalid realtime connection ticket")
	ErrProtocolRequired    = errors.New("realtime WebSocket protocol is required")
	ErrOriginRejected      = errors.New("realtime WebSocket origin was rejected")
)

// Config controls connection liveness. Production defaults are intentionally
// conservative so a client must answer a heartbeat before the idle deadline.
type Config struct {
	HeartbeatInterval time.Duration
	IdleTimeout       time.Duration
	WriteTimeout      time.Duration
	TicketTTL         time.Duration
	MaxPayloadBytes   int
}

func (config Config) withDefaults() Config {
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = defaultHeartbeatInterval
	}
	if config.IdleTimeout <= config.HeartbeatInterval {
		config.IdleTimeout = defaultIdleTimeout
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = defaultWriteTimeout
	}
	if config.TicketTTL <= 0 {
		config.TicketTTL = DefaultTicketTTL
	}
	if config.MaxPayloadBytes <= 0 {
		config.MaxPayloadBytes = defaultMaxPayloadBytes
	}
	return config
}

// TicketResponse is returned by the authenticated ticket endpoint. The ticket
// is short-lived and single-use; clients offer TicketProtocol(ticket) during
// the subsequent WebSocket handshake.
type TicketResponse struct {
	Ticket         string    `json:"ticket"`
	Protocol       string    `json:"protocol"`
	TicketProtocol string    `json:"ticket_protocol"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type connectionTicket struct {
	principal platformsecurity.Principal
	expiresAt time.Time
}

// TicketStore keeps only short-lived connection credentials. It is not a
// business store; horizontal deployments use the shared Edge SQL adapter
// while this in-memory implementation remains useful for single-instance
// tests and local fallback.
type TicketStore struct {
	mu     sync.Mutex
	values map[string]connectionTicket
	ttl    time.Duration
	now    func() time.Time
}

func NewTicketStore(ttl time.Duration) *TicketStore {
	if ttl <= 0 {
		ttl = DefaultTicketTTL
	}
	return &TicketStore{values: make(map[string]connectionTicket), ttl: ttl, now: func() time.Time { return time.Now().UTC() }}
}

func (store *TicketStore) Issue(principal platformsecurity.Principal) (TicketResponse, error) {
	if store == nil || principal.Type != platformsecurity.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" {
		return TicketResponse{}, ErrInvalidPrincipal
	}
	ticketID, err := newOpaqueTicket()
	if err != nil {
		return TicketResponse{}, fmt.Errorf("generate realtime ticket: %w", err)
	}
	now := time.Now().UTC()
	if store.now != nil {
		now = store.now().UTC()
	}
	expiresAt := now.Add(store.ttl)
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.values == nil {
		store.values = make(map[string]connectionTicket)
	}
	for value, ticket := range store.values {
		if !ticket.expiresAt.After(now) {
			delete(store.values, value)
		}
	}
	store.values[ticketID] = connectionTicket{principal: clonePrincipal(principal), expiresAt: expiresAt}
	return TicketResponse{Ticket: ticketID, Protocol: Protocol, TicketProtocol: TicketProtocol(ticketID), ExpiresAt: expiresAt}, nil
}

func (store *TicketStore) Consume(ticketID string) (platformsecurity.Principal, error) {
	if store == nil || strings.TrimSpace(ticketID) == "" {
		return platformsecurity.Principal{}, ErrInvalidTicket
	}
	now := time.Now().UTC()
	if store.now != nil {
		now = store.now().UTC()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	ticket, exists := store.values[ticketID]
	delete(store.values, ticketID)
	if !exists || !ticket.expiresAt.After(now) {
		return platformsecurity.Principal{}, ErrInvalidTicket
	}
	return clonePrincipal(ticket.principal), nil
}

func newOpaqueTicket() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func clonePrincipal(principal platformsecurity.Principal) platformsecurity.Principal {
	principal.Roles = append([]string(nil), principal.Roles...)
	principal.Scopes.AreaIDs = append([]uint64(nil), principal.Scopes.AreaIDs...)
	principal.Scopes.TeamIDs = append([]uint64(nil), principal.Scopes.TeamIDs...)
	return principal
}

// TicketProtocol returns the browser-safe subprotocol value for a short-lived
// connection ticket. The ticket is never placed in a URL query parameter.
func TicketProtocol(ticket string) string {
	return ticketProtocolPrefix + strings.TrimSpace(ticket)
}

// TicketFromProtocolHeader extracts a connection ticket from the offered
// subprotocol list without accepting arbitrary protocol values as credentials.
func TicketFromProtocolHeader(header string) (string, bool) {
	for _, offered := range strings.Split(header, ",") {
		offered = strings.TrimSpace(offered)
		if strings.HasPrefix(offered, ticketProtocolPrefix) {
			ticket := strings.TrimPrefix(offered, ticketProtocolPrefix)
			if ticket != "" {
				return ticket, true
			}
		}
	}
	return "", false
}

// Endpoint adapts the notification Subscriber to authenticated WebSocket
// connections and manages liveness for those connections.
type Endpoint struct {
	subscribers edgenotification.Subscriber
	tickets     TicketStorePort
	metrics     *platformobservability.Registry
	log         *zap.Logger
	config      Config

	mu          sync.Mutex
	connections map[string]*connection
	closed      atomic.Bool
}

// TicketStorePort lets each Edge replica use the same short-lived ticket
// authority. The default in-memory implementation is suitable for a single
// replica; production horizontal scaling uses the Edge SQL adapter.
type TicketStorePort interface {
	Issue(platformsecurity.Principal) (TicketResponse, error)
	Consume(string) (platformsecurity.Principal, error)
}

func NewEndpoint(subscribers edgenotification.Subscriber, metrics *platformobservability.Registry, log *zap.Logger, config Config) *Endpoint {
	return NewEndpointWithTicketStore(subscribers, metrics, log, config, nil)
}

func NewEndpointWithTicketStore(subscribers edgenotification.Subscriber, metrics *platformobservability.Registry, log *zap.Logger, config Config, tickets TicketStorePort) *Endpoint {
	if log == nil {
		log = zap.NewNop()
	}
	config = config.withDefaults()
	if tickets == nil {
		tickets = NewTicketStore(config.TicketTTL)
	}
	return &Endpoint{subscribers: subscribers, tickets: tickets, metrics: metrics, log: log, config: config, connections: make(map[string]*connection)}
}

func (endpoint *Endpoint) Ready() bool {
	return endpoint != nil && endpoint.subscribers != nil && !endpoint.closed.Load()
}

func (endpoint *Endpoint) IssueTicket(principal platformsecurity.Principal) (TicketResponse, error) {
	if endpoint == nil || !endpoint.Ready() || endpoint.tickets == nil {
		return TicketResponse{}, ErrRealtimeUnavailable
	}
	return endpoint.tickets.Issue(principal)
}

func (endpoint *Endpoint) ConsumeTicketFromProtocolHeader(header string) (platformsecurity.Principal, error) {
	if endpoint == nil || endpoint.tickets == nil {
		return platformsecurity.Principal{}, ErrInvalidTicket
	}
	ticket, ok := TicketFromProtocolHeader(header)
	if !ok {
		return platformsecurity.Principal{}, ErrInvalidTicket
	}
	return endpoint.tickets.Consume(ticket)
}

// ServeHTTP performs the protocol and same-origin checks after HTTP identity
// middleware has authenticated the employee. It does not accept a principal
// from the URL or from an employee-supplied header.
func (endpoint *Endpoint) ServeHTTP(writer http.ResponseWriter, request *http.Request, employeePublicID string) {
	if !endpoint.Ready() || strings.TrimSpace(employeePublicID) == "" {
		http.Error(writer, ErrRealtimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	server := websocket.Server{
		Handshake: endpoint.handshake,
		Handler: func(socket *websocket.Conn) {
			endpoint.serveConnection(socket, employeePublicID)
		},
	}
	server.ServeHTTP(writer, request)
}

func (endpoint *Endpoint) handshake(config *websocket.Config, request *http.Request) error {
	origin, err := websocket.Origin(config, request)
	if err != nil || origin == nil {
		return ErrOriginRejected
	}
	expectedScheme := "http"
	if request.TLS != nil {
		expectedScheme = "https"
	}
	if !strings.EqualFold(origin.Scheme, expectedScheme) || !strings.EqualFold(origin.Host, request.Host) {
		return ErrOriginRejected
	}
	if !containsProtocol(config.Protocol, Protocol) {
		return ErrProtocolRequired
	}
	// A browser may offer the realtime protocol together with the short-lived
	// ticket protocol. Echo only the negotiated application protocol; never
	// reflect credentials in the handshake response.
	config.Protocol = []string{Protocol}
	return nil
}

func containsProtocol(protocols []string, expected string) bool {
	for _, protocol := range protocols {
		if strings.TrimSpace(protocol) == expected {
			return true
		}
	}
	return false
}

func (endpoint *Endpoint) serveConnection(socket *websocket.Conn, employeePublicID string) {
	if socket == nil {
		return
	}
	socket.PayloadType = websocket.TextFrame
	socket.MaxPayloadBytes = endpoint.config.MaxPayloadBytes
	connectionID, err := id.NewPublicID()
	if err != nil {
		_ = socket.Close()
		return
	}
	connection := &connection{endpoint: endpoint, socket: socket, employeePublicID: employeePublicID, connectionID: connectionID, done: make(chan struct{})}
	connection.lastSeen.Store(time.Now().UTC().UnixNano())
	unsubscribe, err := endpoint.subscribers.Subscribe(employeePublicID, connectionID, connection)
	if err != nil {
		endpoint.log.Warn("register realtime connection failed", zap.Error(err), zap.String("employee_public_id", employeePublicID))
		_ = socket.Close()
		return
	}
	connection.unsubscribe = unsubscribe
	if !endpoint.addConnection(connection) {
		connection.setReason(closeReasonServerShutdown)
		connection.close()
		unsubscribe()
		return
	}
	defer func() {
		connection.close()
		unsubscribe()
		endpoint.removeConnection(connection)
		endpoint.observeDisconnect(connection.reasonValue())
	}()

	if err := connection.send(readyMessage{Type: "ready", Protocol: Protocol, HeartbeatIntervalSeconds: int(endpoint.config.HeartbeatInterval / time.Second), IdleTimeoutSeconds: int(endpoint.config.IdleTimeout / time.Second)}); err != nil {
		connection.setReason(closeReasonWriteError)
		return
	}
	go connection.heartbeat()
	connection.readLoop()
}

func (endpoint *Endpoint) addConnection(value *connection) bool {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	if endpoint.closed.Load() {
		return false
	}
	endpoint.connections[value.connectionID] = value
	endpoint.observeConnectionCountLocked()
	endpoint.metrics.Inc("flight_websocket_connections_total", platformobservability.Labels{"component": "edge", "result": "accepted"})
	return true
}

func (endpoint *Endpoint) removeConnection(value *connection) {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	delete(endpoint.connections, value.connectionID)
	endpoint.observeConnectionCountLocked()
}

func (endpoint *Endpoint) observeConnectionCountLocked() {
	endpoint.metrics.SetGauge("flight_websocket_connections", platformobservability.Labels{"component": "edge"}, float64(len(endpoint.connections)))
}

func (endpoint *Endpoint) observeDisconnect(reason closeReason) {
	endpoint.metrics.Inc("flight_websocket_disconnects_total", platformobservability.Labels{"component": "edge", "reason": reason.String()})
}

func (endpoint *Endpoint) observeHeartbeat(direction string) {
	endpoint.metrics.Inc("flight_websocket_heartbeat_total", platformobservability.Labels{"component": "edge", "direction": direction})
}

func (endpoint *Endpoint) observeNotification(notification edgenotification.TaskChanged) {
	delay := time.Since(notification.IssuedAt).Seconds()
	if delay < 0 {
		delay = 0
	}
	endpoint.metrics.Observe("flight_websocket_notification_delivery_seconds", platformobservability.Labels{"component": "edge"}, delay)
}

// Close terminates all live sockets. It is called by Edge Server.Shutdown;
// clients then reconnect and perform a full task snapshot pull.
func (endpoint *Endpoint) Close() {
	if endpoint == nil || endpoint.closed.Swap(true) {
		return
	}
	endpoint.mu.Lock()
	connections := make([]*connection, 0, len(endpoint.connections))
	for _, value := range endpoint.connections {
		connections = append(connections, value)
	}
	endpoint.mu.Unlock()
	for _, value := range connections {
		value.setReason(closeReasonServerShutdown)
		value.close()
	}
}

type connection struct {
	endpoint         *Endpoint
	socket           *websocket.Conn
	employeePublicID string
	connectionID     string
	unsubscribe      func()

	writeMu   sync.Mutex
	closeOnce sync.Once
	done      chan struct{}
	lastSeen  atomic.Int64
	reason    atomic.Uint32
}

func (value *connection) Deliver(ctx context.Context, notification edgenotification.TaskChanged) error {
	if err := notification.Validate(); err != nil {
		return err
	}
	if ctx == nil {
		return errors.New("realtime notification context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := value.send(notification); err != nil {
		value.setReason(closeReasonWriteError)
		value.close()
		return fmt.Errorf("write task notification: %w", err)
	}
	value.endpoint.observeNotification(notification)
	return nil
}

func (value *connection) send(message any) error {
	value.writeMu.Lock()
	defer value.writeMu.Unlock()
	select {
	case <-value.done:
		return errors.New("realtime connection is closed")
	default:
	}
	if err := value.socket.SetWriteDeadline(time.Now().Add(value.endpoint.config.WriteTimeout)); err != nil {
		return err
	}
	return websocket.JSON.Send(value.socket, message)
}

func (value *connection) heartbeat() {
	ticker := time.NewTicker(value.endpoint.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now().UTC()
			lastSeen := time.Unix(0, value.lastSeen.Load())
			if now.Sub(lastSeen) >= value.endpoint.config.IdleTimeout {
				value.setReason(closeReasonIdleTimeout)
				value.close()
				return
			}
			value.endpoint.observeHeartbeat("server_ping")
			if err := value.send(heartbeatMessage{Type: "ping", IssuedAt: now}); err != nil {
				value.setReason(closeReasonWriteError)
				value.close()
				return
			}
		case <-value.done:
			return
		}
	}
}

func (value *connection) readLoop() {
	for {
		if err := value.socket.SetReadDeadline(time.Now().Add(value.endpoint.config.IdleTimeout)); err != nil {
			value.setReason(closeReasonReadError)
			return
		}
		var message clientMessage
		if err := websocket.JSON.Receive(value.socket, &message); err != nil {
			if value.reasonValue() == closeReasonNone {
				value.setReason(closeReasonReadError)
			}
			return
		}
		value.lastSeen.Store(time.Now().UTC().UnixNano())
		switch message.Type {
		case "pong":
			value.endpoint.observeHeartbeat("client_pong")
		case "ping":
			value.endpoint.observeHeartbeat("client_ping")
			if err := value.send(heartbeatMessage{Type: "pong", IssuedAt: time.Now().UTC()}); err != nil {
				value.setReason(closeReasonWriteError)
				return
			}
		case "":
			// Empty control messages are harmless and do not mutate business state.
		default:
			if err := value.send(errorMessage{Type: "error", Code: "unsupported_message"}); err != nil {
				value.setReason(closeReasonWriteError)
				return
			}
		}
	}
}

func (value *connection) setReason(reason closeReason) {
	if reason == closeReasonNone {
		return
	}
	value.reason.CompareAndSwap(uint32(closeReasonNone), uint32(reason))
}

func (value *connection) reasonValue() closeReason {
	reason := closeReason(value.reason.Load())
	if reason == closeReasonNone {
		return closeReasonReadError
	}
	return reason
}

func (value *connection) close() {
	value.closeOnce.Do(func() {
		close(value.done)
		value.writeMu.Lock()
		defer value.writeMu.Unlock()
		_ = value.socket.Close()
	})
}

type clientMessage struct {
	Type string `json:"type"`
}

type readyMessage struct {
	Type                     string `json:"type"`
	Protocol                 string `json:"protocol"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
	IdleTimeoutSeconds       int    `json:"idle_timeout_seconds"`
}

type heartbeatMessage struct {
	Type     string    `json:"type"`
	IssuedAt time.Time `json:"issued_at"`
}

type errorMessage struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

type closeReason uint32

const (
	closeReasonNone closeReason = iota
	closeReasonReadError
	closeReasonWriteError
	closeReasonIdleTimeout
	closeReasonServerShutdown
)

func (reason closeReason) String() string {
	switch reason {
	case closeReasonWriteError:
		return "write_error"
	case closeReasonIdleTimeout:
		return "idle_timeout"
	case closeReasonServerShutdown:
		return "server_shutdown"
	default:
		return "read_error"
	}
}
