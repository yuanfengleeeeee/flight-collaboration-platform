package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"crypto/tls"
)

type Client struct{ HTTP *http.Client }

func New(timeout time.Duration) *Client {
	return NewWithTLS(timeout, nil)
}

func NewWithTLS(timeout time.Duration, tlsConfig *tls.Config) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &Client{HTTP: &http.Client{Timeout: timeout, Transport: transport}}
}

func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if c == nil || c.HTTP == nil {
		return nil, fmt.Errorf("http client is not configured")
	}
	return c.HTTP.Do(req.WithContext(ctx))
}
