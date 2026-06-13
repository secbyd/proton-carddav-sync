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

// DefaultProtonAppVersion is the app version sent to the Proton API. The
// upstream default ("go-proton-api") is rejected by Proton with
// "Platform `go` is not valid", so a web-client-like value is required. This
// can be overridden per-deployment via the proton.app_version config key or the
// PCS_PROTON_APP_VERSION environment variable, since Proton occasionally tightens
// which versions it accepts.
const DefaultProtonAppVersion = "web-mail@5.0.999.0"

// Sentinel errors for configuration validation.
var (
	// ErrMissingProtonUsername is returned when the Proton username is empty.
	ErrMissingProtonUsername = errors.New("proton username is required")
	// ErrMissingCardDAVURL is returned when the CardDAV server URL is empty.
	ErrMissingCardDAVURL = errors.New("carddav server URL is required")
	// ErrMissingCardDAVUsername is returned when the CardDAV username is empty.
	ErrMissingCardDAVUsername = errors.New("carddav username is required")
	// ErrConfigNotFound is returned by Load when no config file exists at the
	// searched locations. Callers (e.g. `init`) can detect this to offer
	// interactive setup.
	ErrConfigNotFound = errors.New("config file not found")
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
	// AppVersion is the x-pm-appversion header value sent to the Proton API.
	AppVersion string `mapstructure:"app_version"`
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

	const defaultConfigPath = "$HOME/.config/proton-carddav-sync/config.yaml"

	if cfgFile != "" {
		v.SetConfigFile(expandHome(cfgFile))
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("$HOME/.config/proton-carddav-sync")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || errors.Is(err, os.ErrNotExist) {
			where := defaultConfigPath + " or ./config.yaml"
			if cfgFile != "" {
				where = cfgFile
			}
			return nil, fmt.Errorf(
				"no config file found at %s — run 'proton-carddav-sync init' to create it "+
					"(or pass --config <path>): %w", where, ErrConfigNotFound)
		}
		return nil, fmt.Errorf("read config %q: %w", v.ConfigFileUsed(), err)
	}

	// PCS_PROTON_APP_VERSION overrides proton.app_version for deployments that
	// must track Proton's accepted client versions without editing the file.
	if env := os.Getenv("PCS_PROTON_APP_VERSION"); env != "" {
		v.Set("proton.app_version", env)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Expand ~ once so every consumer (db.Open, the MkdirAll below) sees the
	// same absolute path — SQLite does not expand ~ itself.
	cfg.Database.Path = expandHome(cfg.Database.Path)

	// Ensure the database directory exists.
	dbDir := filepath.Dir(cfg.Database.Path)
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory %q: %w", dbDir, err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("proton.app_version", DefaultProtonAppVersion)
	v.SetDefault("sync.interval_seconds", 300)
	v.SetDefault("sync.direction", "both")
	v.SetDefault("database.path", "~/.local/share/proton-carddav-sync/sync.db")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")
}

// DefaultConfigPath returns the default config file location with $HOME
// expanded.
func DefaultConfigPath() string {
	return expandHome("~/.config/proton-carddav-sync/config.yaml")
}

// ResolvePath returns the path Load would read for the given --config value:
// cfgFile when set, otherwise the default location.
func ResolvePath(cfgFile string) string {
	if cfgFile != "" {
		return expandHome(cfgFile)
	}
	return DefaultConfigPath()
}

// Save writes cfg to path as YAML, creating parent directories (0700). It is
// used by `init` to persist an interactively-gathered configuration. Secrets
// are never written here — they live encrypted in the database.
func Save(cfg *Config, path string) error {
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	v := viper.New()
	v.Set("proton.username", cfg.Proton.Username)
	v.Set("proton.app_version", cfg.Proton.AppVersion)
	v.Set("carddav.url", cfg.CardDAV.URL)
	v.Set("carddav.username", cfg.CardDAV.Username)
	v.Set("sync.direction", cfg.Sync.Direction)
	v.Set("sync.interval_seconds", cfg.Sync.IntervalSeconds)
	v.Set("database.path", cfg.Database.Path)
	v.Set("log.level", cfg.Log.Level)
	v.Set("log.format", cfg.Log.Format)

	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	return nil
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
