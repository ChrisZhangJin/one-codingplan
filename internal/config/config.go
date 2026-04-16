package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig     `mapstructure:"server"`
	Database  DatabaseConfig   `mapstructure:"database"`
	Upstreams []UpstreamConfig `mapstructure:"upstreams"`
}

type ServerConfig struct {
	Port     int    `mapstructure:"port"`
	AdminKey string `mapstructure:"admin_key"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type UpstreamConfig struct {
	Name    string `mapstructure:"name"`
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
	Enabled bool   `mapstructure:"enabled"`
}

func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetDefault("server.port", 8080)
	v.SetDefault("database.path", "./ocp.db")

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

	// Explicitly override critical fields to handle Viper AutomaticEnv + Unmarshal edge case
	cfg.Server.Port = v.GetInt("server.port")
	cfg.Server.AdminKey = v.GetString("server.admin_key")
	cfg.Database.Path = v.GetString("database.path")

	if cfg.Server.AdminKey == "" || cfg.Server.AdminKey == "change-me" {
		return nil, fmt.Errorf("server.admin_key must be set to a non-default value")
	}

	return &cfg, nil
}
