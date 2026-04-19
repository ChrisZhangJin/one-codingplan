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
	return db.AutoMigrate(&models.Upstream{}, &models.AccessKey{}, &models.UsageRecord{})
}
