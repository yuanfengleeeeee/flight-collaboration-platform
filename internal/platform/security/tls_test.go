package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
)

func TestTLSConfigsLoadServerAndClientCertificates(t *testing.T) {
	certificateDir := t.TempDir()
	caCert, caKey := newCertificate(t, nil, nil, true, "bvs206-ca")
	serverCert, serverKey := newCertificate(t, caCert, caKey, false, "edge.local")
	clientCert, clientKey := newCertificate(t, caCert, caKey, false, "worker")

	caPath := writePEM(t, certificateDir, "ca.pem", "CERTIFICATE", caCert.Raw)
	serverCertPath := writePEM(t, certificateDir, "server.pem", "CERTIFICATE", serverCert.Raw)
	serverKeyPath := writePEM(t, certificateDir, "server-key.pem", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverKey))
	clientCertPath := writePEM(t, certificateDir, "client.pem", "CERTIFICATE", clientCert.Raw)
	clientKeyPath := writePEM(t, certificateDir, "client-key.pem", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(clientKey))

	serverTLS, err := NewServerTLSConfig(config.TLSConfig{Enabled: true, CertFile: serverCertPath, KeyFile: serverKeyPath, CAFile: caPath, RequireClientCert: true})
	if err != nil {
		t.Fatal(err)
	}
	if serverTLS.MinVersion != tls.VersionTLS12 || serverTLS.ClientAuth != tls.RequireAndVerifyClientCert || serverTLS.ClientCAs == nil {
		t.Fatalf("unexpected server TLS config: %#v", serverTLS)
	}

	clientTLS, err := NewClientTLSConfig(config.TLSConfig{Enabled: true, CAFile: caPath, ClientCertFile: clientCertPath, ClientKeyFile: clientKeyPath, ServerName: "edge.local", RequireClientCert: true})
	if err != nil {
		t.Fatal(err)
	}
	if clientTLS.MinVersion != tls.VersionTLS12 || clientTLS.RootCAs == nil || len(clientTLS.Certificates) != 1 || clientTLS.InsecureSkipVerify {
		t.Fatalf("unexpected client TLS config: %#v", clientTLS)
	}
}

func TestTLSDisabledDoesNotReadCertificateFiles(t *testing.T) {
	serverTLS, err := NewServerTLSConfig(config.TLSConfig{CertFile: filepath.Join(t.TempDir(), "missing")})
	if err != nil || serverTLS != nil {
		t.Fatalf("disabled server TLS = %#v, err=%v", serverTLS, err)
	}
	clientTLS, err := NewClientTLSConfig(config.TLSConfig{ClientCertFile: filepath.Join(t.TempDir(), "missing")})
	if err != nil || clientTLS != nil {
		t.Fatalf("disabled client TLS = %#v, err=%v", clientTLS, err)
	}
}

func newCertificate(t *testing.T, parent *x509.Certificate, parentKey *rsa.PrivateKey, isCA bool, commonName string) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: commonName}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), BasicConstraintsValid: true, IsCA: isCA, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
	}
	if !isCA {
		template.DNSNames = []string{commonName}
	}
	if parent == nil {
		parent = template
		parentKey = key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, key
}

func writePEM(t *testing.T, dir, name, blockType string, contents []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: contents}), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
