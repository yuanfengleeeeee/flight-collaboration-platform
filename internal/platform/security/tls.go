package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
)

// NewServerTLSConfig loads the configured server certificate and, when
// requested, the CA used to verify mTLS client certificates. It never enables
// insecure verification or silently falls back to plaintext.
func NewServerTLSConfig(cfg config.TLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server certificate: %w", err)
	}
	result := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}
	if cfg.CAFile != "" {
		pool, err := loadCertPool(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("load client CA: %w", err)
		}
		result.ClientCAs = pool
	}
	if cfg.RequireClientCert {
		result.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return result, nil
}

// NewClientTLSConfig builds a verifying client configuration for the Worker
// sync transport. Client certificates are optional for ordinary HTTPS and
// required by config validation when mTLS is enabled for the transport.
func NewClientTLSConfig(cfg config.TLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	result := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.ServerName}
	if cfg.CAFile != "" {
		pool, err := loadCertPool(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("load server CA: %w", err)
		}
		result.RootCAs = pool
	}
	if cfg.ClientCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(cfg.ClientCertFile, cfg.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		result.Certificates = []tls.Certificate{certificate}
	}
	return result, nil
}

func loadCertPool(path string) (*x509.CertPool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(contents) {
		return nil, fmt.Errorf("file %s does not contain a valid PEM certificate", path)
	}
	return pool, nil
}
