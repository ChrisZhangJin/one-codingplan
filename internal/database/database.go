package database

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"one-codingplan/internal/models"
)

func Open(dbPath string) (*gorm.DB, error) {
	dsn := dbPath
	if dbPath != ":memory:" {
		dsn = dbPath + "?_journal=WAL&_timeout=5000"
	}
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{})
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&models.Upstream{}, &models.AccessKey{}, &models.UsageRecord{})
}
