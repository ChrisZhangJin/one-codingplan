package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Pool     PoolConfig     `mapstructure:"pool"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

type LoggingConfig struct {
	Level      string `mapstructure:"level"`        // debug, info, warn, error (default: info)
	File       string `mapstructure:"file"`          // path to log file; empty = stdout only
	MaxSizeMB  int    `mapstructure:"max_size_mb"`   // MB before rotation (default: 100)
	MaxBackups int    `mapstructure:"max_backups"`   // rotated files to keep (default: 3)
	MaxAgeDays int    `mapstructure:"max_age_days"`  // days to retain rotated files (default: 28)
	Compress   bool   `mapstructure:"compress"`      // gzip rotated files
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

	cfg.Server.Port = v.GetInt("server.port")
	cfg.Server.AdminKey = v.GetString("server.admin_key")
	cfg.Database.Path = v.GetString("database.path")
	cfg.Pool.RateLimitBackoff = v.GetString("pool.rate_limit_backoff")

	if cfg.Server.AdminKey == "" || cfg.Server.AdminKey == "change-me" {
		return nil, fmt.Errorf("server.admin_key must be set to a non-default value")
	}

	return &cfg, nil
}
