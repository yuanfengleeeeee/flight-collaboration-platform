// Package config loads the Architecture v2 process configuration.
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Core ServiceConfig `mapstructure:"core"`
	Edge ServiceConfig `mapstructure:"edge"`
	Sync SyncConfig    `mapstructure:"sync"`
	JWT  JWTConfig     `mapstructure:"jwt"`
	Log  LogConfig     `mapstructure:"log"`
}

type ServiceConfig struct {
	Port                 int            `mapstructure:"port"`
	Mode                 string         `mapstructure:"mode"`
	AllowDevActorHeaders bool           `mapstructure:"allow_dev_actor_headers"`
	DB                   DatabaseConfig `mapstructure:"db"`
	Redis                RedisConfig    `mapstructure:"redis"`
	TLS                  TLSConfig      `mapstructure:"tls"`
}

type DatabaseConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	Database        string `mapstructure:"database"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

type SyncConfig struct {
	EdgeBaseURL      string    `mapstructure:"edge_base_url"`
	PollIntervalMS   int       `mapstructure:"poll_interval_ms"`
	BatchSize        int       `mapstructure:"batch_size"`
	MaxAttempts      int       `mapstructure:"max_attempts"`
	RequestTimeoutMS int       `mapstructure:"request_timeout_ms"`
	TLS              TLSConfig `mapstructure:"tls"`
}

// TLSConfig contains both server-side TLS settings and optional client
// certificate settings for the Worker sync transport. A single shape keeps
// environment/config-file wiring consistent while validation is contextual.
type TLSConfig struct {
	Enabled           bool   `mapstructure:"enabled"`
	CertFile          string `mapstructure:"cert_file"`
	KeyFile           string `mapstructure:"key_file"`
	CAFile            string `mapstructure:"ca_file"`
	ClientCertFile    string `mapstructure:"client_cert_file"`
	ClientKeyFile     string `mapstructure:"client_key_file"`
	RequireClientCert bool   `mapstructure:"require_client_cert"`
	ServerName        string `mapstructure:"server_name"`
}

type JWTConfig struct {
	Secret   string `mapstructure:"secret"`
	Issuer   string `mapstructure:"issuer"`
	Audience string `mapstructure:"audience"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
	Output   string `mapstructure:"output"`
}

type RedisConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=UTC&time_zone=%%27%%2B00:00%%27",
		c.Username, c.Password, c.Host, c.Port, c.Database)
}

func (c DatabaseConfig) Validate(name string) error {
	if c.Host == "" || c.Port < 1 || c.Port > 65535 || c.Username == "" || c.Database == "" {
		return fmt.Errorf("%s db host, port, username and database are required", name)
	}
	return nil
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("FLIGHT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	keys := []string{
		"core.port", "core.mode", "core.allow_dev_actor_headers", "core.db.host", "core.db.port", "core.db.username", "core.db.password", "core.db.database", "core.db.max_idle_conns", "core.db.max_open_conns", "core.db.conn_max_lifetime", "core.redis.enabled", "core.redis.host", "core.redis.port", "core.redis.password", "core.redis.db", "core.tls.enabled", "core.tls.cert_file", "core.tls.key_file", "core.tls.ca_file", "core.tls.client_cert_file", "core.tls.client_key_file", "core.tls.require_client_cert", "core.tls.server_name",
		"edge.port", "edge.mode", "edge.allow_dev_actor_headers", "edge.db.host", "edge.db.port", "edge.db.username", "edge.db.password", "edge.db.database", "edge.db.max_idle_conns", "edge.db.max_open_conns", "edge.db.conn_max_lifetime", "edge.redis.enabled", "edge.redis.host", "edge.redis.port", "edge.redis.password", "edge.redis.db", "edge.tls.enabled", "edge.tls.cert_file", "edge.tls.key_file", "edge.tls.ca_file", "edge.tls.client_cert_file", "edge.tls.client_key_file", "edge.tls.require_client_cert", "edge.tls.server_name",
		"sync.edge_base_url", "sync.poll_interval_ms", "sync.batch_size", "sync.max_attempts", "sync.request_timeout_ms", "sync.tls.enabled", "sync.tls.cert_file", "sync.tls.key_file", "sync.tls.ca_file", "sync.tls.client_cert_file", "sync.tls.client_key_file", "sync.tls.require_client_cert", "sync.tls.server_name",
		"jwt.secret", "jwt.issuer", "jwt.audience", "log.level", "log.encoding", "log.output",
	}
	for _, key := range keys {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("bind environment variable %s: %w", key, err)
		}
	}
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

func (c Config) Validate() error {
	if c.Core.Port < 1 || c.Core.Port > 65535 || c.Edge.Port < 1 || c.Edge.Port > 65535 {
		return fmt.Errorf("core.port and edge.port must be between 1 and 65535")
	}
	if err := c.Core.DB.Validate("core"); err != nil {
		return err
	}
	if err := c.Edge.DB.Validate("edge"); err != nil {
		return err
	}
	for name, mode := range map[string]string{"core": c.Core.Mode, "edge": c.Edge.Mode} {
		if mode != "debug" && mode != "release" && mode != "test" {
			return fmt.Errorf("%s.mode must be debug, release or test", name)
		}
	}
	if c.Core.Mode == "release" && c.Core.AllowDevActorHeaders || c.Edge.Mode == "release" && c.Edge.AllowDevActorHeaders {
		return fmt.Errorf("allow_dev_actor_headers is not allowed in release mode")
	}
	if err := c.Core.TLS.ValidateServer("core"); err != nil {
		return err
	}
	if err := c.Edge.TLS.ValidateServer("edge"); err != nil {
		return err
	}
	if err := c.Sync.TLS.ValidateClient("sync"); err != nil {
		return err
	}
	if c.JWT.Secret == "" || c.JWT.Issuer == "" || c.JWT.Audience == "" {
		return fmt.Errorf("jwt.secret, jwt.issuer and jwt.audience are required")
	}
	if c.Log.Level != "debug" && c.Log.Level != "info" && c.Log.Level != "warn" && c.Log.Level != "error" {
		return fmt.Errorf("log.level must be debug, info, warn or error")
	}
	if c.Log.Encoding != "json" && c.Log.Encoding != "console" {
		return fmt.Errorf("log.encoding must be json or console")
	}
	if c.Log.Output == "" {
		return fmt.Errorf("log.output is required")
	}
	return nil
}

func (c TLSConfig) ValidateServer(name string) error {
	if !c.Enabled {
		return nil
	}
	if c.CertFile == "" || c.KeyFile == "" {
		return fmt.Errorf("%s tls cert_file and key_file are required when enabled", name)
	}
	if c.RequireClientCert && c.CAFile == "" {
		return fmt.Errorf("%s tls ca_file is required when client certificates are required", name)
	}
	return nil
}

func (c TLSConfig) ValidateClient(name string) error {
	if !c.Enabled {
		return nil
	}
	if (c.ClientCertFile == "") != (c.ClientKeyFile == "") {
		return fmt.Errorf("%s tls client_cert_file and client_key_file must be provided together", name)
	}
	if c.RequireClientCert && c.ClientCertFile == "" {
		return fmt.Errorf("%s tls client certificate and key are required for mTLS", name)
	}
	return nil
}
