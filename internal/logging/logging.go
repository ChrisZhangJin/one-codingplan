package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"one-codingplan/internal/config"
)

// Setup configures the default slog logger.
// Console and file receive identical output. log.Printf routes through slog at INFO.
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

	h := &ocpHandler{w: w, level: level, mu: &sync.Mutex{}}
	slog.SetDefault(slog.New(h))
}

// ocpHandler is a custom slog.Handler that writes lines in the format:
// 2006/01/02 15:04:05 [LEVEL][file.go:123] message key=value ...
type ocpHandler struct {
	w     io.Writer
	level slog.Level
	mu    *sync.Mutex
	attrs []slog.Attr
}

func (h *ocpHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *ocpHandler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	// Timestamp: 2006/01/02 15:04:05
	buf.WriteString(r.Time.Format("2006/01/02 15:04:05"))
	buf.WriteByte(' ')

	// Level: [INFO], [DEBUG], [WARN], [ERROR]
	buf.WriteByte('[')
	buf.WriteString(r.Level.String())
	buf.WriteByte(']')

	// Source: [file.go:123]
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		buf.WriteString(fmt.Sprintf("[%s:%d]", filepath.Base(f.File), f.Line))
	}

	buf.WriteByte(' ')
	buf.WriteString(r.Message)

	// Pre-set attrs (from WithAttrs)
	for _, a := range h.attrs {
		buf.WriteByte(' ')
		buf.WriteString(a.Key)
		buf.WriteByte('=')
		buf.WriteString(fmt.Sprintf("%v", a.Value.Any()))
	}

	// Record attrs
	r.Attrs(func(a slog.Attr) bool {
		buf.WriteByte(' ')
		buf.WriteString(a.Key)
		buf.WriteByte('=')
		buf.WriteString(fmt.Sprintf("%v", a.Value.Any()))
		return true
	})

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf.Bytes())
	return err
}

func (h *ocpHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &ocpHandler{w: h.w, level: h.level, mu: h.mu, attrs: merged}
}

func (h *ocpHandler) WithGroup(_ string) slog.Handler { return h }

// LevelVerbose is below DEBUG — for high-volume trace logs (e.g. raw stream chunks).
const LevelVerbose = slog.LevelDebug - 4

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "verbose":
		return LevelVerbose
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
