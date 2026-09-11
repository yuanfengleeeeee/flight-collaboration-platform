package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type HTTPClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type RemoteError struct {
	Status  int
	Code    string
	Message string
}

func (e RemoteError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("identity service returned HTTP %d", e.Status)
	}
	return e.Message
}

func NewHTTPClient(baseURL, apiKey string, client *http.Client) (*HTTPClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("identity core base URL is required")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPClient{baseURL: baseURL, apiKey: apiKey, client: client}, nil
}

func (c *HTTPClient) VerifyPassword(ctx context.Context, request PasswordVerifyRequest) (PasswordVerifyResult, error) {
	var result PasswordVerifyResult
	err := c.post(ctx, "/internal/identity/v1/password/verify", request, &result)
	return result, err
}

func (c *HTTPClient) FindStaff(ctx context.Context, publicID string) (Staff, error) {
	var result Staff
	err := c.post(ctx, "/internal/identity/v1/staff/status", StaffStatusRequest{PublicID: publicID}, &result)
	return result, err
}

func (c *HTTPClient) Exchange(ctx context.Context, request ExchangeRequest) (Staff, error) {
	var result Staff
	err := c.post(ctx, "/internal/identity/v1/providers/exchange", request, &result)
	return result, err
}

func (c *HTTPClient) CompleteBinding(ctx context.Context, request BindingRequest) (Staff, error) {
	var result Staff
	err := c.post(ctx, "/internal/identity/v1/bindings/complete", request, &result)
	return result, err
}

func (c *HTTPClient) post(ctx context.Context, path string, body any, result any) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("identity HTTP client is not configured")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal identity request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create identity request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	if c.apiKey != "" {
		request.Header.Set("X-Internal-Identity-Key", c.apiKey)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("call core identity service: %w", err)
	}
	defer response.Body.Close()
	var envelope struct {
		Data    json.RawMessage `json:"data"`
		Code    string          `json:"code"`
		Message string          `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode identity response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		code := envelope.Code
		if code == "" {
			code = "identity_service_error"
		}
		return RemoteError{Status: response.StatusCode, Code: code, Message: envelope.Message}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errors.New("identity response data is missing")
	}
	if err := json.Unmarshal(envelope.Data, result); err != nil {
		return fmt.Errorf("decode identity response data: %w", err)
	}
	return nil
}
