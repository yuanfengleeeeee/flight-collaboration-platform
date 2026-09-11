package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 全局配置
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	MySQL  MySQLConfig  `mapstructure:"mysql"`
	Redis  RedisConfig  `mapstructure:"redis"`
	JWT    JWTConfig    `mapstructure:"jwt"`
	Log    LogConfig    `mapstructure:"log"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type MySQLConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	Database        string `mapstructure:"database"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

// DSN 返回 MySQL 连接串
func (c MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.Username, c.Password, c.Host, c.Port, c.Database)
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// Addr 返回 Redis 地址
func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	ExpireHours int    `mapstructure:"expire_hours"`
	Issuer      string `mapstructure:"issuer"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
	Output   string `mapstructure:"output"`
}

// Load 加载配置
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("FLIGHT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	for _, key := range []string{
		"server.port", "server.mode",
		"mysql.host", "mysql.port", "mysql.username", "mysql.password", "mysql.database",
		"mysql.max_idle_conns", "mysql.max_open_conns", "mysql.conn_max_lifetime",
		"redis.host", "redis.port", "redis.password", "redis.db",
		"jwt.secret", "jwt.expire_hours", "jwt.issuer",
		"log.level", "log.encoding", "log.output",
	} {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("绑定环境变量失败: %s: %w", key, err)
		}
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置校验失败: %w", err)
	}

	return &cfg, nil
}

// Validate 校验启动所需的基础配置，不检查外部依赖是否在线。
func (c Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port 必须在 1-65535 之间")
	}
	mode := strings.ToLower(c.Server.Mode)
	if mode != "debug" && mode != "release" && mode != "test" {
		return fmt.Errorf("server.mode 必须是 debug、release 或 test")
	}
	if c.MySQL.Host == "" || c.MySQL.Port < 1 || c.MySQL.Port > 65535 || c.MySQL.Username == "" || c.MySQL.Database == "" {
		return fmt.Errorf("mysql.host、mysql.port、mysql.username、mysql.database 必须有效")
	}
	if c.Redis.Host == "" || c.Redis.Port < 1 || c.Redis.Port > 65535 {
		return fmt.Errorf("redis.host 和 redis.port 必须有效")
	}
	if c.JWT.Secret == "" || c.JWT.Issuer == "" || c.JWT.ExpireHours <= 0 {
		return fmt.Errorf("jwt.secret、jwt.issuer、jwt.expire_hours 必须有效")
	}
	if mode == "release" && (c.JWT.Secret == "change-me-in-production" || len(c.JWT.Secret) < 16) {
		return fmt.Errorf("release 模式必须使用至少 16 位非默认 JWT secret")
	}
	if c.Log.Level != "debug" && c.Log.Level != "info" && c.Log.Level != "warn" && c.Log.Level != "error" {
		return fmt.Errorf("log.level 必须是 debug、info、warn 或 error")
	}
	if c.Log.Encoding != "json" && c.Log.Encoding != "console" {
		return fmt.Errorf("log.encoding 必须是 json 或 console")
	}
	if c.Log.Output == "" {
		return fmt.Errorf("log.output 不能为空")
	}
	return nil
}
