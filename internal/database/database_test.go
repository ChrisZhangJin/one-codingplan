package database

import (
	"os"
	"testing"

	"one-codingplan/internal/models"
)

func TestOpen(t *testing.T) {
	f, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	db, err := Open(f.Name())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if db == nil {
		t.Fatal("Open returned nil db")
	}
}

func TestAutoMigrate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var names []string
	db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name IN (?,?,?)",
		"upstreams", "access_keys", "usage_records").Scan(&names)
	if len(names) != 3 {
		t.Errorf("expected 3 tables, got %d: %v", len(names), names)
	}
}

func TestPersistence(t *testing.T) {
	f, err := os.CreateTemp("", "test-persist-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	dbPath := f.Name()
	f.Close()

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Create(&models.Upstream{Name: "kimi", BaseURL: "https://api.moonshot.ai", APIKeyEnc: []byte("enc"), Enabled: true}).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.Close()

	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	if err := Migrate(db2); err != nil {
		t.Fatalf("Migrate db2: %v", err)
	}

	var upstreams []models.Upstream
	if result := db2.Find(&upstreams); result.Error != nil {
		t.Fatalf("Find: %v", result.Error)
	}
	if len(upstreams) != 1 {
		t.Errorf("expected 1 upstream after re-open, got %d", len(upstreams))
	}
}
