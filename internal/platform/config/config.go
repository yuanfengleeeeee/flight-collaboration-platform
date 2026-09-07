// Package config loads the Architecture v2 process configuration.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Core     ServiceConfig  `mapstructure:"core"`
	Edge     ServiceConfig  `mapstructure:"edge"`
	Sync     SyncConfig     `mapstructure:"sync"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Identity IdentityConfig `mapstructure:"identity"`
	Log      LogConfig      `mapstructure:"log"`
}

type ServiceConfig struct {
	Port                 int            `mapstructure:"port"`
	Mode                 string         `mapstructure:"mode"`
	AllowDevActorHeaders bool           `mapstructure:"allow_dev_actor_headers"`
	FlightSourceAPIKey   string         `mapstructure:"flight_source_api_key"`
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
	EdgeBaseURL       string    `mapstructure:"edge_base_url"`
	PollIntervalMS    int       `mapstructure:"poll_interval_ms"`
	BatchSize         int       `mapstructure:"batch_size"`
	MaxAttempts       int       `mapstructure:"max_attempts"`
	RequestTimeoutMS  int       `mapstructure:"request_timeout_ms"`
	ClaimLeaseSeconds int       `mapstructure:"claim_lease_seconds"`
	AssignmentReceiptTimeoutSeconds int `mapstructure:"assignment_receipt_timeout_seconds"`
	TLS               TLSConfig `mapstructure:"tls"`
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

type IdentityConfig struct {
	CoreBaseURL                 string `mapstructure:"core_base_url"`
	InternalAPIKey              string `mapstructure:"internal_api_key"`
	RealProvidersEnabled        bool   `mapstructure:"real_providers_enabled"`
	PersonalWeChatAppID         string `mapstructure:"personal_wechat_app_id"`
	PersonalWeChatSecret        string `mapstructure:"personal_wechat_secret"`
	WeComCorpID                 string `mapstructure:"wecom_corp_id"`
	WeComAgentID                string `mapstructure:"wecom_agent_id"`
	WeComSecret                 string `mapstructure:"wecom_secret"`
	WeComMiniappCode2SessionURL string `mapstructure:"wecom_miniapp_code2session_url"`
	AccessTokenTTLSeconds       int    `mapstructure:"access_token_ttl_seconds"`
	SessionTTLHours             int    `mapstructure:"session_ttl_hours"`
	SessionAbsoluteTTLHours     int    `mapstructure:"session_absolute_ttl_hours"`
	MaxLoginAttempts            int    `mapstructure:"max_login_attempts"`
	LockoutSeconds              int    `mapstructure:"lockout_seconds"`
	BindingTicketSeconds        int    `mapstructure:"binding_ticket_seconds"`
	AdminSSOEnabled             bool   `mapstructure:"admin_sso_enabled"`
	AdminSSOProvider            string `mapstructure:"admin_sso_provider"`
	AdminSSOAuthorizeURL        string `mapstructure:"admin_sso_authorize_url"`
	AdminSSOTokenURL            string `mapstructure:"admin_sso_token_url"`
	AdminSSOUserInfoURL         string `mapstructure:"admin_sso_userinfo_url"`
	AdminSSOClientID            string `mapstructure:"admin_sso_client_id"`
	AdminSSOClientSecret        string `mapstructure:"admin_sso_client_secret"`
	AdminSSOAllowedRedirect     string `mapstructure:"admin_sso_allowed_redirect_uris"`
	AdminSSOStateTTLSeconds     int    `mapstructure:"admin_sso_state_ttl_seconds"`
	AdminSSODevSubject          string `mapstructure:"admin_sso_dev_subject"`
}

func (c IdentityConfig) AdminSSOStateTTL() time.Duration {
	if c.AdminSSOStateTTLSeconds <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(c.AdminSSOStateTTLSeconds) * time.Second
}

func (c IdentityConfig) AccessTokenTTL() time.Duration {
	if c.AccessTokenTTLSeconds <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(c.AccessTokenTTLSeconds) * time.Second
}

func (c IdentityConfig) SessionTTL() time.Duration {
	if c.SessionTTLHours <= 0 {
		return 30 * 24 * time.Hour
	}
	return time.Duration(c.SessionTTLHours) * time.Hour
}

func (c IdentityConfig) SessionAbsoluteTTL() time.Duration {
	if c.SessionAbsoluteTTLHours <= 0 {
		return 90 * 24 * time.Hour
	}
	return time.Duration(c.SessionAbsoluteTTLHours) * time.Hour
}

func (c IdentityConfig) LockoutDuration() time.Duration {
	if c.LockoutSeconds <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(c.LockoutSeconds) * time.Second
}

func (c IdentityConfig) BindingTicketTTL() time.Duration {
	if c.BindingTicketSeconds <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(c.BindingTicketSeconds) * time.Second
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
		"core.port", "core.mode", "core.allow_dev_actor_headers", "core.flight_source_api_key", "core.db.host", "core.db.port", "core.db.username", "core.db.password", "core.db.database", "core.db.max_idle_conns", "core.db.max_open_conns", "core.db.conn_max_lifetime", "core.redis.enabled", "core.redis.host", "core.redis.port", "core.redis.password", "core.redis.db", "core.tls.enabled", "core.tls.cert_file", "core.tls.key_file", "core.tls.ca_file", "core.tls.client_cert_file", "core.tls.client_key_file", "core.tls.require_client_cert", "core.tls.server_name",
		"edge.port", "edge.mode", "edge.allow_dev_actor_headers", "edge.db.host", "edge.db.port", "edge.db.username", "edge.db.password", "edge.db.database", "edge.db.max_idle_conns", "edge.db.max_open_conns", "edge.db.conn_max_lifetime", "edge.redis.enabled", "edge.redis.host", "edge.redis.port", "edge.redis.password", "edge.redis.db", "edge.tls.enabled", "edge.tls.cert_file", "edge.tls.key_file", "edge.tls.ca_file", "edge.tls.client_cert_file", "edge.tls.client_key_file", "edge.tls.require_client_cert", "edge.tls.server_name",
		"sync.edge_base_url", "sync.poll_interval_ms", "sync.batch_size", "sync.max_attempts", "sync.request_timeout_ms", "sync.claim_lease_seconds", "sync.assignment_receipt_timeout_seconds", "sync.tls.enabled", "sync.tls.cert_file", "sync.tls.key_file", "sync.tls.ca_file", "sync.tls.client_cert_file", "sync.tls.client_key_file", "sync.tls.require_client_cert", "sync.tls.server_name",
		"jwt.secret", "jwt.issuer", "jwt.audience", "identity.core_base_url", "identity.internal_api_key", "identity.real_providers_enabled", "identity.personal_wechat_app_id", "identity.personal_wechat_secret", "identity.wecom_corp_id", "identity.wecom_agent_id", "identity.wecom_secret", "identity.wecom_miniapp_code2session_url", "identity.access_token_ttl_seconds", "identity.session_ttl_hours", "identity.session_absolute_ttl_hours", "identity.max_login_attempts", "identity.lockout_seconds", "identity.binding_ticket_seconds", "identity.admin_sso_enabled", "identity.admin_sso_provider", "identity.admin_sso_authorize_url", "identity.admin_sso_token_url", "identity.admin_sso_userinfo_url", "identity.admin_sso_client_id", "identity.admin_sso_client_secret", "identity.admin_sso_allowed_redirect_uris", "identity.admin_sso_state_ttl_seconds", "identity.admin_sso_dev_subject", "log.level", "log.encoding", "log.output",
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
	if c.Core.Mode == "release" && strings.TrimSpace(c.Core.FlightSourceAPIKey) == "" {
		return fmt.Errorf("core.flight_source_api_key is required in release mode")
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
	if (c.Core.Mode == "release" || c.Edge.Mode == "release") && c.Identity.InternalAPIKey == "" {
		return fmt.Errorf("identity.internal_api_key is required in release mode")
	}
	if c.Edge.Mode == "release" && strings.TrimSpace(c.Identity.CoreBaseURL) == "" {
		return fmt.Errorf("identity.core_base_url is required for edge in release mode")
	}
	if c.Identity.RealProvidersEnabled && (strings.TrimSpace(c.Identity.PersonalWeChatAppID) == "" || strings.TrimSpace(c.Identity.PersonalWeChatSecret) == "" || strings.TrimSpace(c.Identity.WeComCorpID) == "" || strings.TrimSpace(c.Identity.WeComAgentID) == "" || strings.TrimSpace(c.Identity.WeComSecret) == "") {
		return fmt.Errorf("identity real providers require personal WeChat and WeCom credentials")
	}
	if c.Identity.AdminSSOEnabled {
		provider := strings.ToLower(strings.TrimSpace(c.Identity.AdminSSOProvider))
		if provider == "" {
			return fmt.Errorf("identity.admin_sso_provider is required when admin SSO is enabled")
		}
		if c.Core.Mode == "release" && provider == "development" {
			return fmt.Errorf("development admin SSO provider is not allowed in release mode")
		}
		if provider != "oidc" && provider != "wecom" && provider != "development" {
			return fmt.Errorf("identity.admin_sso_provider must be oidc, wecom or development")
		}
		if strings.TrimSpace(c.Identity.AdminSSOAllowedRedirect) == "" {
			return fmt.Errorf("identity.admin_sso_allowed_redirect_uris is required when admin SSO is enabled")
		}
		if provider == "development" && strings.TrimSpace(c.Identity.AdminSSODevSubject) == "" {
			return fmt.Errorf("identity.admin_sso_dev_subject is required for development admin SSO")
		}
		if provider == "oidc" && (strings.TrimSpace(c.Identity.AdminSSOAuthorizeURL) == "" || strings.TrimSpace(c.Identity.AdminSSOTokenURL) == "" || strings.TrimSpace(c.Identity.AdminSSOUserInfoURL) == "" || strings.TrimSpace(c.Identity.AdminSSOClientID) == "" || strings.TrimSpace(c.Identity.AdminSSOClientSecret) == "") {
			return fmt.Errorf("oidc admin SSO requires authorize, token, userinfo, client ID and client secret")
		}
		if provider == "wecom" && (strings.TrimSpace(c.Identity.WeComCorpID) == "" || strings.TrimSpace(c.Identity.WeComAgentID) == "" || strings.TrimSpace(c.Identity.WeComSecret) == "") {
			return fmt.Errorf("wecom admin SSO requires wecom corp ID, agent ID and secret")
		}
	}
	if c.Identity.AdminSSOStateTTLSeconds < 0 {
		return fmt.Errorf("identity.admin_sso_state_ttl_seconds cannot be negative")
	}
	if c.Identity.AccessTokenTTLSeconds < 0 || c.Identity.SessionTTLHours < 0 || c.Identity.SessionAbsoluteTTLHours < 0 || c.Identity.MaxLoginAttempts < 0 || c.Identity.LockoutSeconds < 0 || c.Identity.BindingTicketSeconds < 0 {
		return fmt.Errorf("identity duration and retry settings cannot be negative")
	}
	if c.Sync.ClaimLeaseSeconds < 0 {
		return fmt.Errorf("sync.claim_lease_seconds cannot be negative")
	}
	if c.Sync.AssignmentReceiptTimeoutSeconds < 0 {
		return fmt.Errorf("sync.assignment_receipt_timeout_seconds cannot be negative")
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
