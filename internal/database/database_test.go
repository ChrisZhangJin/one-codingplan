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

func TestMigrate_ProtocolFixup(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Seed rows on the migration default 'both' for known OpenAI-only names.
	seeds := []models.Upstream{
		{Name: "deepseek", BaseURL: "https://api.deepseek.com", Protocol: models.ProtocolBoth},
		{Name: "qwen", BaseURL: "https://dashscope.aliyuncs.com", Protocol: models.ProtocolBoth},
		{Name: "glm", BaseURL: "https://open.bigmodel.cn", Protocol: models.ProtocolBoth},
		{Name: "kimi", BaseURL: "https://api.kimi.com/coding", Protocol: models.ProtocolBoth},
		{Name: "my-claude", BaseURL: "http://proxy", Protocol: models.ProtocolBoth},
	}
	for _, u := range seeds {
		u := u
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("seed %s: %v", u.Name, err)
		}
	}

	// Re-running Migrate should flip the OpenAI-only names and leave the rest
	// alone. GLM stays on "both": it has a native Anthropic endpoint.
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (rerun): %v", err)
	}

	wantProtocol := map[string]string{
		"deepseek":  models.ProtocolOpenAI,
		"qwen":      models.ProtocolOpenAI,
		"glm":       models.ProtocolBoth,
		"kimi":      models.ProtocolBoth,
		"my-claude": models.ProtocolBoth,
	}
	for name, want := range wantProtocol {
		var got models.Upstream
		if err := db.First(&got, "name = ?", name).Error; err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		if got.Protocol != want {
			t.Errorf("%s: protocol = %q, want %q", name, got.Protocol, want)
		}
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
