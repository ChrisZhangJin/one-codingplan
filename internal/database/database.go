package database

import (
	"one-codingplan/internal/config"
	"one-codingplan/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

func SyncUpstreams(db *gorm.DB, cfgUpstreams []config.UpstreamConfig) error {
	if len(cfgUpstreams) == 0 {
		return nil
	}
	upstreams := make([]models.Upstream, len(cfgUpstreams))
	for i, u := range cfgUpstreams {
		upstreams[i] = models.Upstream{
			Name:    u.Name,
			BaseURL: u.BaseURL,
			APIKey:  u.APIKey,
			Enabled: u.Enabled,
		}
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"base_url", "api_key", "enabled", "updated_at"}),
	}).Create(&upstreams).Error
}
