// Package log provides a thin wrapper around go.uber.org/zap.
package log

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is an alias for the sugared Zap logger.
type Logger = *zap.SugaredLogger

// New constructs a Logger from level ("debug"|"info"|"warn"|"error") and
// format ("text"|"json").
func New(level, format string) (Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}

	var cfg zap.Config
	if strings.ToLower(format) == "json" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)

	base, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return base.Sugar(), nil
}
