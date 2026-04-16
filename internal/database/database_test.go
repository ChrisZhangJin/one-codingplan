package database

import (
	"os"
	"testing"

	"one-codingplan/internal/config"
	"one-codingplan/internal/models"
)

// testEncKey is a 32-byte key used only in tests.
var testEncKey = []byte("test-encryption-key-32-bytes-ok!")

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

func TestSyncUpstreams_Create(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfgUpstreams := []config.UpstreamConfig{
		{Name: "kimi", BaseURL: "https://api.moonshot.ai", APIKey: "sk-kimi", Enabled: true},
		{Name: "qwen", BaseURL: "https://dashscope.aliyuncs.com", APIKey: "sk-qwen", Enabled: true},
	}

	if err := SyncUpstreams(db, cfgUpstreams, testEncKey); err != nil {
		t.Fatalf("SyncUpstreams: %v", err)
	}

	var upstreams []models.Upstream
	if result := db.Find(&upstreams); result.Error != nil {
		t.Fatalf("Find: %v", result.Error)
	}
	if len(upstreams) != 2 {
		t.Errorf("expected 2 upstreams, got %d", len(upstreams))
	}

	names := map[string]bool{}
	for _, u := range upstreams {
		names[u.Name] = true
	}
	if !names["kimi"] || !names["qwen"] {
		t.Errorf("unexpected upstream names: %v", names)
	}
}

func TestSyncUpstreams_Update(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	first := []config.UpstreamConfig{
		{Name: "kimi", BaseURL: "https://old.url", APIKey: "sk-kimi", Enabled: true},
	}
	if err := SyncUpstreams(db, first, testEncKey); err != nil {
		t.Fatalf("SyncUpstreams first: %v", err)
	}

	second := []config.UpstreamConfig{
		{Name: "kimi", BaseURL: "https://new.url", APIKey: "sk-kimi", Enabled: true},
	}
	if err := SyncUpstreams(db, second, testEncKey); err != nil {
		t.Fatalf("SyncUpstreams second: %v", err)
	}

	var upstreams []models.Upstream
	if result := db.Find(&upstreams); result.Error != nil {
		t.Fatalf("Find: %v", result.Error)
	}
	if len(upstreams) != 1 {
		t.Errorf("expected 1 upstream, got %d", len(upstreams))
	}
	if upstreams[0].BaseURL != "https://new.url" {
		t.Errorf("BaseURL = %q, want https://new.url", upstreams[0].BaseURL)
	}
}

func TestSyncUpstreams_Empty(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if err := SyncUpstreams(db, nil, testEncKey); err != nil {
		t.Fatalf("SyncUpstreams with nil: %v", err)
	}
	if err := SyncUpstreams(db, []config.UpstreamConfig{}, testEncKey); err != nil {
		t.Fatalf("SyncUpstreams with empty slice: %v", err)
	}

	var count int64
	if result := db.Model(&models.Upstream{}).Count(&count); result.Error != nil {
		t.Fatalf("Count: %v", result.Error)
	}
	if count != 0 {
		t.Errorf("expected 0 records, got %d", count)
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
	if err := SyncUpstreams(db, []config.UpstreamConfig{
		{Name: "kimi", BaseURL: "https://api.moonshot.ai", APIKey: "sk-kimi", Enabled: true},
	}, testEncKey); err != nil {
		t.Fatalf("SyncUpstreams: %v", err)
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
