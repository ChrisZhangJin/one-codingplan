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
	"one-codingplan/internal/pool"
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
	srv := server.New(db, cfg, nil, nil)
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

// --- Upstream rotate/list tests ---

func setupAdminTestWithPool(t *testing.T, entries []pool.UpstreamEntry) (*server.Server, *gorm.DB) {
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
	p := pool.NewForTest(entries)
	t.Cleanup(func() { p.Stop() })
	srv := server.New(db, cfg, p, nil)
	return srv, db
}

func TestRotateUpstream(t *testing.T) {
	entries := []pool.UpstreamEntry{
		{ID: 1, Name: "kimi", BaseURL: "https://kimi.example.com", APIKey: "sk-a"},
		{ID: 2, Name: "glm", BaseURL: "https://glm.example.com", APIKey: "sk-b"},
	}
	srv, _ := setupAdminTestWithPool(t, entries)
	engine := srv.Engine()

	req := adminReq(t, http.MethodPost, "/api/upstreams/rotate", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["upstream"] == nil {
		t.Error("expected upstream field in response")
	}
	if resp["message"] == nil {
		t.Error("expected message field in response")
	}
}

func TestRotateUpstream_NoUpstreams(t *testing.T) {
	srv, _ := setupAdminTestWithPool(t, nil)
	engine := srv.Engine()

	req := adminReq(t, http.MethodPost, "/api/upstreams/rotate", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["error"] == nil {
		t.Error("expected error field in response")
	}
}

func TestListUpstreams(t *testing.T) {
	entries := []pool.UpstreamEntry{
		{ID: 1, Name: "kimi", BaseURL: "https://kimi.example.com", APIKey: "sk-secret-a"},
		{ID: 2, Name: "glm", BaseURL: "https://glm.example.com", APIKey: "sk-secret-b"},
	}
	srv, _ := setupAdminTestWithPool(t, entries)
	engine := srv.Engine()

	req := adminReq(t, http.MethodGet, "/api/upstreams", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 upstreams, got %d", len(resp))
	}
	for _, u := range resp {
		if u["name"] == nil {
			t.Error("expected name field")
		}
		if u["base_url"] == nil {
			t.Error("expected base_url field")
		}
		if _, ok := u["available"]; !ok {
			t.Error("expected available field")
		}
		if _, ok := u["api_key"]; ok {
			t.Error("api_key must not be exposed in upstream list")
		}
	}
}

// --- Limit middleware tests ---

func makeLimitTestKey(db *gorm.DB, id, token string, budget int64, rpm, rpd int) {
	key := models.AccessKey{
		ID:                 id,
		Token:              token,
		Enabled:            true,
		Name:               id,
		TokenBudget:        budget,
		RateLimitPerMinute: rpm,
		RateLimitPerDay:    rpd,
	}
	db.Create(&key)
}

func TestLimitMiddleware_TokenBudget(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	makeLimitTestKey(db, "budget-k1", "ocp-budget-token-1", 100, 0, 0)
	// Seed usage at exactly the budget
	db.Create(&models.UsageRecord{KeyID: "budget-k1", UpstreamID: 0, InputTokens: 60, OutputTokens: 40, Success: true})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4","messages":[]}`)))
	req.Header.Set("Authorization", "Bearer ocp-budget-token-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for exhausted budget, got %d: %s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested error object, got %v", m["error"])
	}
	if errObj["code"] != "rate_limit_exceeded" {
		t.Errorf("expected code=rate_limit_exceeded, got %v", errObj["code"])
	}
	if errObj["message"] != "token budget exceeded" {
		t.Errorf("expected message='token budget exceeded', got %v", errObj["message"])
	}
	if errObj["type"] != "requests" {
		t.Errorf("expected type=requests, got %v", errObj["type"])
	}
}

func TestLimitMiddleware_TokenBudget_UnderLimit(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	makeLimitTestKey(db, "budget-k2", "ocp-budget-token-2", 100, 0, 0)
	// Seed usage under budget
	db.Create(&models.UsageRecord{KeyID: "budget-k2", UpstreamID: 0, InputTokens: 30, OutputTokens: 20, Success: true})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4","messages":[]}`)))
	req.Header.Set("Authorization", "Bearer ocp-budget-token-2")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Should not be 429 (will be 503 because no pool upstreams, but not budget error)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("expected request to pass limit check, got 429: %s", w.Body.String())
	}
}

func TestLimitMiddleware_NoBudget(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	makeLimitTestKey(db, "budget-k3", "ocp-budget-token-3", 0, 0, 0)
	// Seed large usage — should not trigger any limit since budget=0 means unlimited
	db.Create(&models.UsageRecord{KeyID: "budget-k3", UpstreamID: 0, InputTokens: 999999, OutputTokens: 999999, Success: true})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4","messages":[]}`)))
	req.Header.Set("Authorization", "Bearer ocp-budget-token-3")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Errorf("expected no limit for budget=0, got 429: %s", w.Body.String())
	}
}

func TestLimitMiddleware_RatePerMinute(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	makeLimitTestKey(db, "rate-k1", "ocp-rate-token-1", 0, 2, 0)

	server.ResetPerMinuteCounters()

	doReq := func() int {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4","messages":[]}`)))
		req.Header.Set("Authorization", "Bearer ocp-rate-token-1")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}

	// First two requests should pass the rate limit check
	for i := 1; i <= 2; i++ {
		code := doReq()
		if code == http.StatusTooManyRequests {
			t.Errorf("request %d should pass rate limit, got 429", i)
		}
	}
	// Third request should be rate limited
	code := doReq()
	if code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on third request (limit=2), got %d", code)
	}

	// Verify OpenAI error format on the 429 response
	req4 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4","messages":[]}`)))
	req4.Header.Set("Authorization", "Bearer ocp-rate-token-1")
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	engine.ServeHTTP(w4, req4)
	var m map[string]interface{}
	json.NewDecoder(w4.Body).Decode(&m)
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested error object, got %v", m["error"])
	}
	if errObj["code"] != "rate_limit_exceeded" {
		t.Errorf("expected code=rate_limit_exceeded, got %v", errObj["code"])
	}
	if errObj["message"] != "per-minute rate limit exceeded" {
		t.Errorf("expected message='per-minute rate limit exceeded', got %v", errObj["message"])
	}
}

func TestLimitMiddleware_RatePerDay(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	makeLimitTestKey(db, "rate-k2", "ocp-rate-token-2", 0, 0, 2)

	server.ResetPerDayCounters()

	doReq := func() (int, map[string]interface{}) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4","messages":[]}`)))
		req.Header.Set("Authorization", "Bearer ocp-rate-token-2")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		var m map[string]interface{}
		json.NewDecoder(w.Body).Decode(&m)
		return w.Code, m
	}

	// First two requests should pass the rate limit check
	for i := 1; i <= 2; i++ {
		code, _ := doReq()
		if code == http.StatusTooManyRequests {
			t.Errorf("request %d should pass rate limit, got 429", i)
		}
	}
	// Third request should be rate limited
	code, m := doReq()
	if code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on third request (limit=2), got %d", code)
	}
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested error object, got %v", m["error"])
	}
	if errObj["code"] != "rate_limit_exceeded" {
		t.Errorf("expected code=rate_limit_exceeded, got %v", errObj["code"])
	}
	if errObj["message"] != "per-day rate limit exceeded" {
		t.Errorf("expected message='per-day rate limit exceeded', got %v", errObj["message"])
	}
}

func TestListKeys_IncludesDayUsage(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "day-k1", Token: "ocp-day-token-1", Enabled: true, Name: "day-key-1"}
	db.Create(&key)
	server.ResetPerDayCounters()

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
	var k map[string]interface{}
	for _, item := range resp {
		if item["id"] == "day-k1" {
			k = item
			break
		}
	}
	if k == nil {
		t.Fatal("key day-k1 not found in response")
	}
	if k["day_usage"] != float64(0) {
		t.Errorf("expected day_usage=0, got %v", k["day_usage"])
	}
}

func TestListKeys_DayUsage_ActiveCounter(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "day-k2", Token: "ocp-day-token-2", Enabled: true, Name: "day-key-2"}
	db.Create(&key)
	server.InjectDayCount("day-k2", 42)

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
	var k map[string]interface{}
	for _, item := range resp {
		if item["id"] == "day-k2" {
			k = item
			break
		}
	}
	if k == nil {
		t.Fatal("key day-k2 not found in response")
	}
	if k["day_usage"] != float64(42) {
		t.Errorf("expected day_usage=42, got %v", k["day_usage"])
	}
}

func TestListKeys_DayUsage_StaleWindow(t *testing.T) {
	srv, db := setupAdminTest(t)
	engine := srv.Engine()

	key := models.AccessKey{ID: "day-k3", Token: "ocp-day-token-3", Enabled: true, Name: "day-key-3"}
	db.Create(&key)
	server.InjectDayCountStale("day-k3")

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
	var k map[string]interface{}
	for _, item := range resp {
		if item["id"] == "day-k3" {
			k = item
			break
		}
	}
	if k == nil {
		t.Fatal("key day-k3 not found in response")
	}
	if k["day_usage"] != float64(0) {
		t.Errorf("expected day_usage=0 for stale window, got %v", k["day_usage"])
	}
}

// Ensure time import used
var _ = time.Now
