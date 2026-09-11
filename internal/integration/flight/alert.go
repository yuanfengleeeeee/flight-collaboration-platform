package flight

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	AlertSourceSyncFailure          = "flight_source_sync_failure"
	AlertSourceReconciliationFailed = "flight_source_reconciliation_failed"
	AlertSourceMismatch             = "flight_source_reconciliation_mismatch"
)

type Alert struct {
	Type     string         `json:"type"`
	Severity string         `json:"severity"`
	Provider string         `json:"provider"`
	Message  string         `json:"message"`
	At       time.Time      `json:"at"`
	Details  map[string]any `json:"details,omitempty"`
}

type Notifier interface {
	Notify(context.Context, Alert) error
}

// WebhookNotifier is intentionally generic so an organization can point it
// at an incident gateway, an enterprise bot or an internal alert adapter.
// It never logs the URL or request body, because either can contain secrets
// or operational data in a deployment-specific integration.
type WebhookNotifier struct {
	URL    string
	Client *http.Client
}

func NewWebhookNotifier(rawURL string, client *http.Client) (*WebhookNotifier, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("flight source alert webhook URL is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &WebhookNotifier{URL: rawURL, Client: client}, nil
}

func (n *WebhookNotifier) Notify(ctx context.Context, alert Alert) error {
	if n == nil || n.Client == nil || strings.TrimSpace(n.URL) == "" {
		return errors.New("flight source alert notifier is not configured")
	}
	if alert.At.IsZero() {
		alert.At = time.Now().UTC()
	}
	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshal flight source alert: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create flight source alert request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := n.Client.Do(request)
	if err != nil {
		return fmt.Errorf("send flight source alert: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("flight source alert returned HTTP %d", response.StatusCode)
	}
	return nil
}
