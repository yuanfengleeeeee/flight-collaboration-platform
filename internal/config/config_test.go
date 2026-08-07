package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigValidateRejectsReleaseDefaultSecret(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{Port: 8080, Mode: "release"},
		MySQL:  MySQLConfig{Host: "127.0.0.1", Port: 3306, Username: "root", Database: "flight"},
		Redis:  RedisConfig{Host: "127.0.0.1", Port: 6379},
		JWT:    JWTConfig{Secret: "change-me-in-production", ExpireHours: 24, Issuer: "flight"},
		Log:    LogConfig{Level: "info", Encoding: "json", Output: "stdout"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected release secret validation error")
	}
}

func TestConfigValidateAcceptsTestConfig(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{Port: 8080, Mode: "test"},
		MySQL:  MySQLConfig{Host: "127.0.0.1", Port: 3306, Username: "root", Database: "flight"},
		Redis:  RedisConfig{Host: "127.0.0.1", Port: 6379},
		JWT:    JWTConfig{Secret: "test-secret", ExpireHours: 1, Issuer: "flight"},
		Log:    LogConfig{Level: "warn", Encoding: "console", Output: "stdout"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestLoadEnvironmentOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `server:
  port: 8080
  mode: test
mysql:
  host: 127.0.0.1
  port: 3306
  username: root
  database: flight
redis:
  host: 127.0.0.1
  port: 6379
jwt:
  secret: test-secret
  expire_hours: 1
  issuer: flight
log:
  level: info
  encoding: json
  output: stdout
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLIGHT_SERVER_PORT", "18081")
	t.Setenv("FLIGHT_LOG_LEVEL", "error")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 18081 || cfg.Log.Level != "error" {
		t.Fatalf("environment override failed: %+v", cfg)
	}
}
