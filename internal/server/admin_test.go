package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"one-codingplan/internal/config"
	"one-codingplan/internal/database"
	"one-codingplan/internal/server"

	"gorm.io/gorm"
)

func setupAdminTest(t *testing.T) (*server.Server, *gorm.DB) {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{}
	cfg.Server.AdminKey = "test-admin-key"
	srv := server.New(db, cfg, nil)
	return srv, db
}

func TestAdminMiddleware_ValidToken(t *testing.T) {
	srv, _ := setupAdminTest(t)
	engine := srv.Engine()

	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	req.Header.Set("Authorization", "Bearer test-admin-key")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Errorf("expected non-401 for valid admin key, got %d", w.Code)
	}
}

func TestAdminMiddleware_MissingToken(t *testing.T) {
	srv, _ := setupAdminTest(t)
	engine := srv.Engine()

	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing token, got %d", w.Code)
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", m)
	}
}

func TestAdminMiddleware_WrongToken(t *testing.T) {
	srv, _ := setupAdminTest(t)
	engine := srv.Engine()

	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong token, got %d", w.Code)
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", m)
	}
}

func TestAdminMiddleware_NoBearer(t *testing.T) {
	srv, _ := setupAdminTest(t)
	engine := srv.Engine()

	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for Basic auth, got %d", w.Code)
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", m)
	}
}
