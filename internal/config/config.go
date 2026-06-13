// Package config loads and validates the application configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Sentinel errors for configuration validation.
var (
	// ErrMissingProtonUsername is returned when the Proton username is empty.
	ErrMissingProtonUsername = errors.New("proton username is required")
	// ErrMissingCardDAVURL is returned when the CardDAV server URL is empty.
	ErrMissingCardDAVURL = errors.New("carddav server URL is required")
	// ErrMissingCardDAVUsername is returned when the CardDAV username is empty.
	ErrMissingCardDAVUsername = errors.New("carddav username is required")
)

// Config holds all application configuration.
// Fields are ordered so that structs whose trailing field is a non-pointer
// (SyncConfig ends in an int) come last, keeping the GC pointer-scan prefix
// minimal (satisfies govet fieldalignment).
type Config struct {
	Proton   ProtonConfig   `mapstructure:"proton"`
	CardDAV  CardDAVConfig  `mapstructure:"carddav"`
	Database DatabaseConfig `mapstructure:"database"`
	Log      LogConfig      `mapstructure:"log"`
	Sync     SyncConfig     `mapstructure:"sync"`
}

// ProtonConfig holds Proton Mail connection settings.
type ProtonConfig struct {
	Username string `mapstructure:"username"`
}

// CardDAVConfig holds CardDAV server connection settings.
type CardDAVConfig struct {
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
}

// SyncConfig holds synchronisation behaviour settings.
// string before int for optimal field alignment.
type SyncConfig struct {
	Direction       string `mapstructure:"direction"`
	IntervalSeconds int    `mapstructure:"interval_seconds"`
}

// DatabaseConfig holds SQLite storage settings.
type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load reads configuration from the given file path.
// It returns a validated *Config or a descriptive error.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	if cfgFile != "" {
		v.SetConfigFile(expandHome(cfgFile))
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("$HOME/.config/proton-carddav-sync")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Ensure the database directory exists.
	dbDir := filepath.Dir(expandHome(cfg.Database.Path))
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory %q: %w", dbDir, err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("sync.interval_seconds", 300)
	v.SetDefault("sync.direction", "both")
	v.SetDefault("database.path", "~/.local/share/proton-carddav-sync/sync.db")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")
}

func validate(cfg *Config) error {
	if strings.TrimSpace(cfg.Proton.Username) == "" {
		return ErrMissingProtonUsername
	}
	if strings.TrimSpace(cfg.CardDAV.URL) == "" {
		return ErrMissingCardDAVURL
	}
	if strings.TrimSpace(cfg.CardDAV.Username) == "" {
		return ErrMissingCardDAVUsername
	}
	return nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
