package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	edgesync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpclient"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

type HTTPTransport struct {
	baseURL string
	client  *httpclient.Client
}

func NewHTTPTransport(baseURL string, timeout time.Duration) (*HTTPTransport, error) {
	return NewHTTPTransportWithTLS(baseURL, timeout, config.TLSConfig{})
}

func NewHTTPTransportWithTLS(baseURL string, timeout time.Duration, tlsConfig config.TLSConfig) (*HTTPTransport, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("edge sync base URL is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("parse edge sync base URL: %w", err)
	}
	clientTLS, err := security.NewClientTLSConfig(tlsConfig)
	if err != nil {
		return nil, err
	}
	return &HTTPTransport{baseURL: baseURL, client: httpclient.NewWithTLS(timeout, clientTLS)}, nil
}

func (t *HTTPTransport) PublishEvent(ctx context.Context, envelope sharedEvent.EventEnvelope) error {
	return t.doJSON(ctx, http.MethodPost, "/internal/sync/v1/events", envelope, nil)
}

func (t *HTTPTransport) PullCommands(ctx context.Context, limit int) ([]edgesync.CommandRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/internal/sync/v1/commands/pending?limit="+strconv.Itoa(limit), nil)
	if err != nil {
		return nil, err
	}
	response, err := t.client.Do(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("pull edge commands: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return nil, fmt.Errorf("pull edge commands: HTTP %d", response.StatusCode)
	}
	var body struct {
		Items []edgesync.CommandRecord `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode edge commands: %w", err)
	}
	return body.Items, nil
}

func (t *HTTPTransport) AcknowledgeCommand(ctx context.Context, commandID string, status string, reason string, nextAttempt time.Time) error {
	path := "/internal/sync/v1/commands/" + url.PathEscape(commandID) + "/ack"
	body := map[string]any{"status": status, "reason": reason}
	if !nextAttempt.IsZero() {
		body["next_attempt_at"] = nextAttempt.UTC()
	}
	return t.doJSON(ctx, http.MethodPost, path, body, nil)
}

func (t *HTTPTransport) doJSON(ctx context.Context, method string, path string, body any, decodeInto any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, t.baseURL+path, strings.NewReader(string(encoded)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := t.client.Do(ctx, request)
	if err != nil {
		return fmt.Errorf("sync request %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("sync request %s %s: HTTP %d", method, path, response.StatusCode)
	}
	if decodeInto != nil && response.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(decodeInto); err != nil {
			return fmt.Errorf("decode sync response: %w", err)
		}
	}
	return nil
}
