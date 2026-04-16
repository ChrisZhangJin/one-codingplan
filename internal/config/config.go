package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig     `mapstructure:"server"`
	Database  DatabaseConfig   `mapstructure:"database"`
	Pool      PoolConfig       `mapstructure:"pool"`
	Upstreams []UpstreamConfig `mapstructure:"upstreams"`
}

type PoolConfig struct {
	RateLimitBackoff string `mapstructure:"rate_limit_backoff"`
}

// PoolBackoff parses Pool.RateLimitBackoff as a duration.
// Falls back to 5s if the value is missing or unparseable.
func (c *Config) PoolBackoff() time.Duration {
	d, err := time.ParseDuration(c.Pool.RateLimitBackoff)
	if err != nil {
		return 5 * time.Second
	}
	return d
}

type ServerConfig struct {
	Port     int    `mapstructure:"port"`
	AdminKey string `mapstructure:"admin_key"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type UpstreamConfig struct {
	Name          string `mapstructure:"name"`
	BaseURL       string `mapstructure:"base_url"`
	APIKey        string `mapstructure:"api_key"`
	Enabled       bool   `mapstructure:"enabled"`
	ModelOverride string `mapstructure:"model_override"`
}

func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetDefault("server.port", 8080)
	v.SetDefault("database.path", "./ocp.db")
	v.SetDefault("pool.rate_limit_backoff", "5s")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	v.SetEnvPrefix("OCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Explicitly override critical fields to handle Viper AutomaticEnv + Unmarshal edge case.
	// Note: the Upstreams slice is not env-overridable via OCP_UPSTREAMS_* because Viper does
	// not support slice-of-struct env injection. Upstream API keys must be supplied via the
	// config file or stored directly in the database.
	cfg.Server.Port = v.GetInt("server.port")
	cfg.Server.AdminKey = v.GetString("server.admin_key")
	cfg.Database.Path = v.GetString("database.path")
	cfg.Pool.RateLimitBackoff = v.GetString("pool.rate_limit_backoff")

	if cfg.Server.AdminKey == "" || cfg.Server.AdminKey == "change-me" {
		return nil, fmt.Errorf("server.admin_key must be set to a non-default value")
	}

	return &cfg, nil
}
