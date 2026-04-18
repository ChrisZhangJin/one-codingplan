package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"

	"one-codingplan/internal/config"
)

// Setup configures the default slog logger with the given config.
// After this call, log.Printf routes through slog at INFO level automatically.
func Setup(cfg config.LoggingConfig) {
	level := parseLevel(cfg.Level)

	var w io.Writer = os.Stdout
	if cfg.File != "" {
		maxSize := cfg.MaxSizeMB
		if maxSize <= 0 {
			maxSize = 100
		}
		maxBackups := cfg.MaxBackups
		if maxBackups <= 0 {
			maxBackups = 3
		}
		maxAge := cfg.MaxAgeDays
		if maxAge <= 0 {
			maxAge = 28
		}
		lj := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
			Compress:   cfg.Compress,
		}
		w = io.MultiWriter(os.Stdout, lj)
	}

	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
