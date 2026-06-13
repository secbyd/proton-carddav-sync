package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all runtime configuration.
type Config struct {
	Proton struct {
		Username string `mapstructure:"username"`
		TOTP     string `mapstructure:"totp"`
	} `mapstructure:"proton"`

	CardDAV struct {
		URL      string `mapstructure:"url"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	} `mapstructure:"carddav"`

	Sync struct {
		Direction     string `mapstructure:"direction"`
		MergeStrategy string `mapstructure:"merge_strategy"`
		Interval      string `mapstructure:"interval"`
	} `mapstructure:"sync"`

	DB struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"db"`

	Log struct {
		Level  string `mapstructure:"level"`
		Format string `mapstructure:"format"`
	} `mapstructure:"log"`
}

// Load reads configuration from viper and applies defaults.
func Load() (*Config, error) {
	setDefaults()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.DB.Path = expandPath(cfg.DB.Path)

	// Ensure DB directory exists.
	if err := os.MkdirAll(filepath.Dir(cfg.DB.Path), 0o700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	return &cfg, nil
}

func setDefaults() {
	home, _ := os.UserHomeDir()
	viper.SetDefault("sync.direction", "both")
	viper.SetDefault("sync.merge_strategy", "prefer-newer")
	viper.SetDefault("sync.interval", "15m")
	viper.SetDefault("db.path", filepath.Join(home, ".local", "share", "proton-carddav-sync", "sync.db"))
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
