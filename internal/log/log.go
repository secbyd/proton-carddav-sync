// Package log provides a thin slog-based logger constructor.
//
// Follows go-logging skill: use log/slog for new Go code; structured
// key-value pairs; snake_case keys; no third-party library unless profiling
// shows slog is a bottleneck.
package log

import (
	"log/slog"
	"os"
)

// New returns a *slog.Logger writing to stderr.
//
// format must be "json" or "text" (anything else defaults to text).
// level is the minimum level string: "debug", "info", "warn", "error".
func New(format, levelStr string) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}
