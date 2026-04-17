package database

import (
	"one-codingplan/internal/config"
	"one-codingplan/internal/crypto"
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

func SyncUpstreams(db *gorm.DB, cfgUpstreams []config.UpstreamConfig, encKey []byte) error {
	if len(cfgUpstreams) == 0 {
		return nil
	}
	upstreams := make([]models.Upstream, len(cfgUpstreams))
	for i, u := range cfgUpstreams {
		enc, err := crypto.Encrypt(encKey, u.APIKey)
		if err != nil {
			return err
		}
		upstreams[i] = models.Upstream{
			Name:          u.Name,
			BaseURL:       u.BaseURL,
			APIKeyEnc:     enc,
			Enabled:       u.Enabled,
			ModelOverride: u.ModelOverride,
		}
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"base_url", "api_key_enc", "model_override", "updated_at"}),
	}).Create(&upstreams).Error; err != nil {
		return err
	}

	activeNames := make([]string, len(cfgUpstreams))
	for i, u := range cfgUpstreams {
		activeNames[i] = u.Name
	}
	return db.Model(&models.Upstream{}).
		Where("name NOT IN ?", activeNames).
		Update("enabled", false).Error
}
