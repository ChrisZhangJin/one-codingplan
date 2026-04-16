package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"one-codingplan/internal/config"
	"one-codingplan/internal/database"
	"one-codingplan/internal/models"
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

func adminReq(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		bs, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(bs)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Authorization", "Bearer test-admin-key")
	req.Header.Set("Content-Type", "application/json")
	return req
}

// --- Admin middleware tests ---

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

// --- Key CRUD tests ---

func TestCreateKey(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	req := adminReq(t, http.MethodPost, "/api/keys", map[string]interface{}{
		"name": "test-key",
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected token in response, got %v", resp)
	}
	if len(token) < 4 || token[:4] != "ocp-" {
		t.Errorf("expected token to start with 'ocp-', got %q", token)
	}
	if resp["id"] == nil {
		t.Error("expected id in response")
	}
	if resp["name"] != "test-key" {
		t.Errorf("expected name=test-key, got %v", resp["name"])
	}
	if resp["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", resp["enabled"])
	}

	// Verify key exists in DB
	var count int64
	db.Model(&models.AccessKey{}).Where("token = ?", token).Count(&count)
	if count != 1 {
		t.Errorf("expected key in DB, count=%d", count)
	}
}

func TestCreateKey_Response(t *testing.T) {
	srv, _ := setupAdminTest(t)
	engine := srv.Engine()

	req := adminReq(t, http.MethodPost, "/api/keys", map[string]interface{}{
		"name": "my-key",
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	token, _ := resp["token"].(string)
	if len(token) < 4 || token[:4] != "ocp-" {
		t.Errorf("token must start with ocp-, got %q", token)
	}
	if resp["id"] == nil {
		t.Error("id field missing")
	}
	if resp["name"] != "my-key" {
		t.Errorf("name mismatch: %v", resp["name"])
	}
	if resp["enabled"] != true {
		t.Errorf("enabled should be true: %v", resp["enabled"])
	}
}

func TestListKeys(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	// Seed 2 keys
	key1 := models.AccessKey{ID: "list-k1", Token: "ocp-token-aaa", Enabled: true, Name: "key-one"}
	key2 := models.AccessKey{ID: "list-k2", Token: "ocp-token-bbb", Enabled: true, Name: "key-two"}
	db.Create(&key1)
	db.Create(&key2)

	// Seed usage records for key1
	db.Create(&models.UsageRecord{KeyID: "list-k1", UpstreamID: 1, InputTokens: 100, OutputTokens: 50, Success: true})
	db.Create(&models.UsageRecord{KeyID: "list-k1", UpstreamID: 1, InputTokens: 200, OutputTokens: 80, Success: true})

	req := adminReq(t, http.MethodGet, "/api/keys", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(resp))
	}

	// Find key1 in response
	var k1resp map[string]interface{}
	for _, k := range resp {
		if k["id"] == "list-k1" {
			k1resp = k
			break
		}
	}
	if k1resp == nil {
		t.Fatal("key1 not found in response")
	}

	// Token must be masked
	tok := k1resp["token"].(string)
	if !containsMask(tok) {
		t.Errorf("expected masked token, got %q", tok)
	}

	// Usage totals
	if k1resp["usage_total_input"] != float64(300) {
		t.Errorf("expected usage_total_input=300, got %v", k1resp["usage_total_input"])
	}
	if k1resp["usage_total_output"] != float64(130) {
		t.Errorf("expected usage_total_output=130, got %v", k1resp["usage_total_output"])
	}
}

func containsMask(s string) bool {
	for i := 0; i < len(s)-2; i++ {
		if s[i] == '*' && s[i+1] == '*' && s[i+2] == '*' {
			return true
		}
	}
	return false
}

func TestGetKey(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "get-k1", Token: "ocp-token-get", Enabled: true, Name: "get-key"}
	db.Create(&key)
	db.Create(&models.UsageRecord{KeyID: "get-k1", UpstreamID: 1, InputTokens: 50, OutputTokens: 25, Success: true})

	req := adminReq(t, http.MethodGet, "/api/keys/get-k1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["id"] != "get-k1" {
		t.Errorf("expected id=get-k1, got %v", resp["id"])
	}
	tok := resp["token"].(string)
	if !containsMask(tok) {
		t.Errorf("expected masked token, got %q", tok)
	}
	if resp["usage_total_input"] != float64(50) {
		t.Errorf("expected usage_total_input=50, got %v", resp["usage_total_input"])
	}
	if resp["usage_total_output"] != float64(25) {
		t.Errorf("expected usage_total_output=25, got %v", resp["usage_total_output"])
	}
}

func TestGetKey_NotFound(t *testing.T) {
	srv, _ := setupAdminTest(t)
	engine := srv.Engine()

	req := adminReq(t, http.MethodGet, "/api/keys/nonexistent", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateKey(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "upd-k1", Token: "ocp-token-upd", Enabled: true, Name: "upd-key", TokenBudget: 1000}
	db.Create(&key)

	req := adminReq(t, http.MethodPatch, "/api/keys/upd-k1", map[string]interface{}{
		"token_budget": 5000,
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.AccessKey
	db.First(&updated, "id = ?", "upd-k1")
	if updated.TokenBudget != 5000 {
		t.Errorf("expected TokenBudget=5000, got %d", updated.TokenBudget)
	}
	// Name unchanged
	if updated.Name != "upd-key" {
		t.Errorf("expected Name=upd-key unchanged, got %q", updated.Name)
	}
}

func TestUpdateKey_ZeroBudget(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "upd-k2", Token: "ocp-token-upd2", Enabled: true, Name: "upd-key2", TokenBudget: 5000}
	db.Create(&key)

	req := adminReq(t, http.MethodPatch, "/api/keys/upd-k2", map[string]interface{}{
		"token_budget": 0,
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.AccessKey
	db.First(&updated, "id = ?", "upd-k2")
	if updated.TokenBudget != 0 {
		t.Errorf("expected TokenBudget=0 after patch, got %d", updated.TokenBudget)
	}
}

func TestBlockKey(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "blk-k1", Token: "ocp-token-blk", Enabled: true}
	db.Create(&key)

	req := adminReq(t, http.MethodPost, "/api/keys/blk-k1/block", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.AccessKey
	db.First(&updated, "id = ?", "blk-k1")
	if updated.Enabled {
		t.Error("expected key to be disabled after block")
	}
}

func TestUnblockKey(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "ublk-k1", Token: "ocp-token-ublk", Enabled: false}
	db.Create(&key)

	req := adminReq(t, http.MethodPost, "/api/keys/ublk-k1/unblock", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.AccessKey
	db.First(&updated, "id = ?", "ublk-k1")
	if !updated.Enabled {
		t.Error("expected key to be enabled after unblock")
	}
}

func TestDeleteKey(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "del-k1", Token: "ocp-token-del", Enabled: true}
	db.Create(&key)

	req := adminReq(t, http.MethodDelete, "/api/keys/del-k1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.AccessKey{}).Where("id = ?", "del-k1").Count(&count)
	if count != 0 {
		t.Errorf("expected key deleted from DB, count=%d", count)
	}
}

// Ensure time import used
var _ = time.Now
