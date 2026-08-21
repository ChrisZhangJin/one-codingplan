package database

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"one-codingplan/internal/models"
)

func Open(dbPath string) (*gorm.DB, error) {
	dsn := dbPath
	if dbPath != ":memory:" {
		dsn = dbPath + "?_journal=WAL&_timeout=5000&_sync=NORMAL"
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.New(
			gormSlogWriter{},
			logger.Config{
				SlowThreshold:             2 * time.Second,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
			},
		),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	return db, nil
}

type gormSlogWriter struct{}

func (gormSlogWriter) Printf(format string, args ...any) {
	slog.Warn("gorm: " + fmt.Sprintf(format, args...))
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.Upstream{}, &models.AccessKey{}, &models.UsageRecord{}); err != nil {
		return err
	}
	// One-shot protocol fix-up: AutoMigrate adds the protocol column with
	// default 'both', but the known providers below are OpenAI-only — without
	// this, /v1/messages would waste a failover roundtrip on each before
	// finding a real Anthropic-compat upstream. Idempotent: only flips rows
	// that are still on the migration default.
	if err := db.Exec(
		"UPDATE upstreams SET protocol = ? WHERE protocol = ? AND name IN ?",
		models.ProtocolOpenAI, models.ProtocolBoth,
		// GLM is deliberately absent: it serves a native Anthropic endpoint
		// (see pool.GLMAdapter), so pinning it to OpenAI would force
		// /v1/messages through translation for no reason.
		[]string{"qwen", "deepseek"},
	).Error; err != nil {
		return fmt.Errorf("migrate: fixup protocol: %w", err)
	}
	return nil
}
