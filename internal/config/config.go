package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the top-level application configuration.
type Config struct {
	Proton  ProtonConfig  `mapstructure:"proton"`
	CardDAV CardDAVConfig `mapstructure:"carddav"`
	Sync    SyncConfig    `mapstructure:"sync"`
	Log     LogConfig     `mapstructure:"log"`
}

// ProtonConfig holds ProtonMail credentials.
type ProtonConfig struct {
	Username   string `mapstructure:"username"`
	Password   string `mapstructure:"password"`
	TOTPSecret string `mapstructure:"totp_secret"`
}

// CardDAVConfig holds CardDAV server settings.
type CardDAVConfig struct {
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// SyncConfig controls sync behaviour.
type SyncConfig struct {
	Interval  time.Duration `mapstructure:"interval"`
	Direction string        `mapstructure:"direction"`
	DBPath    string        `mapstructure:"db_path"`
}

// LogConfig controls logging.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load reads and validates the configuration.
func Load() (*Config, error) {
	viper.SetDefault("sync.interval", "1h")
	viper.SetDefault("sync.direction", "both")
	viper.SetDefault("sync.db_path", expandHome("~/.proton-carddav-sync/state.db"))
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	// Expand ~ in db_path
	cfg.Sync.DBPath = expandHome(cfg.Sync.DBPath)
	if err := os.MkdirAll(filepath.Dir(cfg.Sync.DBPath), 0o700); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}
	return &cfg, nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
