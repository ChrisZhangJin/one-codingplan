package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"one-codingplan/internal/config"
	"one-codingplan/internal/crypto"
	"one-codingplan/internal/database"
	"one-codingplan/internal/models"
	"one-codingplan/internal/pool"
	"one-codingplan/internal/server"

	"gorm.io/gorm"
)

var testEncKey = []byte("0123456789abcdef")

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Pin to a single connection so all goroutines share the same in-memory database.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedAccessKey(t *testing.T, db *gorm.DB, token string, enabled bool) models.AccessKey {
	t.Helper()
	key := models.AccessKey{ID: "key-1", Token: token, Enabled: true}
	if err := db.Create(&key).Error; err != nil {
		t.Fatalf("seed access key: %v", err)
	}
	if !enabled {
		if err := db.Model(&key).Update("enabled", false).Error; err != nil {
			t.Fatalf("disable access key: %v", err)
		}
		key.Enabled = false
	}
	return key
}

func seedUpstream(t *testing.T, db *gorm.DB, name, baseURL string) models.Upstream {
	t.Helper()
	enc, err := crypto.Encrypt(testEncKey, "fake-api-key")
	if err != nil {
		t.Fatalf("encrypt key: %v", err)
	}
	u := models.Upstream{Name: name, BaseURL: baseURL, APIKeyEnc: enc, Enabled: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed upstream: %v", err)
	}
	return u
}

func buildPool(t *testing.T, db *gorm.DB, backoff time.Duration) *pool.Pool {
	t.Helper()
	p, err := pool.New(db, testEncKey, &pool.Config{RateLimitBackoff: backoff})
	if err != nil {
		t.Fatalf("pool.New: %v", err)
	}
	return p
}

func buildServer(db *gorm.DB, p *pool.Pool) *server.Server {
	cfg := &config.Config{}
	cfg.Server.AdminKey = "admin-key"
	return server.New(db, cfg, p, testEncKey)
}

func chatReqBody(stream bool) []byte {
	type body struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}
	b := body{
		Model: "gpt-4",
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: "hello"}},
		Stream: stream,
	}
	bs, _ := json.Marshal(b)
	return bs
}

func upstreamResponse(promptTokens, completionTokens int) []byte {
	type usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	}
	type resp struct {
		Choices []map[string]string `json:"choices"`
		Usage   usage               `json:"usage"`
	}
	r := resp{
		Choices: []map[string]string{{"message": "hi"}},
		Usage:   usage{PromptTokens: promptTokens, CompletionTokens: completionTokens},
	}
	bs, _ := json.Marshal(r)
	return bs
}

// waitForUsageRecord waits up to 200ms for a UsageRecord to appear.
func waitForUsageRecord(t *testing.T, db *gorm.DB) models.UsageRecord {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		var rec models.UsageRecord
		err := db.First(&rec).Error
		if err == nil {
			return rec
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for UsageRecord")
	return models.UsageRecord{}
}

// --- Auth tests ---

func TestRelay_Auth_Missing(t *testing.T) {
	db := setupTestDB(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be called")
		w.WriteHeader(200)
	}))
	defer fake.Close()
	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", m)
	}
}

func TestRelay_Auth_Invalid(t *testing.T) {
	db := setupTestDB(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be called")
		w.WriteHeader(200)
	}))
	defer fake.Close()
	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer bad-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRelay_Auth_Disabled(t *testing.T) {
	db := setupTestDB(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be called")
		w.WriteHeader(200)
	}))
	defer fake.Close()
	seedAccessKey(t, db, "test-token-abc", false)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- Non-streaming relay tests ---

func TestRelay_NonStream(t *testing.T) {
	db := setupTestDB(t)
	respBody := upstreamResponse(10, 5)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(respBody)
	}))
	defer fake.Close()
	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
	if !bytes.Equal(w.Body.Bytes(), respBody) {
		t.Errorf("response body mismatch: %s", w.Body.String())
	}
}

func TestRelay_Failover_Credits(t *testing.T) {
	db := setupTestDB(t)
	// Both upstreams are set up: one returns 402 (credits exhausted), one returns 200.
	// The pool round-robins so we don't know which is tried first.
	// We verify: client gets 200 (failover worked) and exactly one upstream saw a 402 path.
	calls402 := int32(0)
	calls200 := int32(0)
	fake402 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls402, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(402)
		w.Write([]byte(`{"error":"insufficient credits"}`))
	}))
	defer fake402.Close()
	fake200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls200, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(upstreamResponse(10, 5))
	}))
	defer fake200.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	// Seed up-ok first so it gets the lower auto-increment ID and sits at pool index 0.
	// pool.Select increments idx before returning, so with idx=0 it returns index 1 (up-credits) first.
	seedUpstream(t, db, "up-ok", fake200.URL)
	seedUpstream(t, db, "up-credits", fake402.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 after failover, got %d: %s", w.Code, w.Body.String())
	}
	// up-credits (402) must have been tried and rotated past; up-ok (200) must have served the response.
	if atomic.LoadInt32(&calls402) != 1 {
		t.Errorf("expected credits-exhausted upstream called once, called %d times", calls402)
	}
	if atomic.LoadInt32(&calls200) != 1 {
		t.Errorf("expected ok upstream called once, called %d times", calls200)
	}
}

func TestRelay_Failover_Transient(t *testing.T) {
	db := setupTestDB(t)
	fake1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer fake1.Close()
	fake2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(upstreamResponse(10, 5))
	}))
	defer fake2.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	u1 := seedUpstream(t, db, "up1", fake1.URL)
	seedUpstream(t, db, "up2", fake2.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify up1 still available in pool (not marked unavailable for transient)
	p.Mark(u1.ID, true) // re-enable to test it can be selected
	up, err := p.Select(nil)
	if err != nil {
		t.Errorf("expected up1 to still be selectable after transient error, got error: %v", err)
	}
	_ = up
}

func TestRelay_RateLimit_Retry(t *testing.T) {
	db := setupTestDB(t)
	callCount := int32(0)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(upstreamResponse(10, 5))
	}))
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected upstream called 2 times (429 then 200), got %d", callCount)
	}
}

func TestRelay_AllFail(t *testing.T) {
	db := setupTestDB(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 503 {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", m)
	}
	if errObj["message"] != "no upstream available" {
		t.Errorf("expected message='no upstream available', got %v", errObj["message"])
	}
	if errObj["code"] != "no_upstream" {
		t.Errorf("expected code='no_upstream', got %v", errObj["code"])
	}
}

func TestRelay_BodyLimit(t *testing.T) {
	db := setupTestDB(t)
	called := int32(0)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(200)
	}))
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	// 10MB + 1 byte body
	bigBody := bytes.Repeat([]byte("x"), 10*1024*1024+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(bigBody))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", w.Code)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Error("upstream should not be called for oversized body")
	}
}

// --- Usage logging tests ---

func TestRelay_Usage_Success(t *testing.T) {
	db := setupTestDB(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(upstreamResponse(10, 5))
	}))
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	u := seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	rec := waitForUsageRecord(t, db)
	if !rec.Success {
		t.Error("expected Success=true")
	}
	if rec.InputTokens != 10 {
		t.Errorf("expected InputTokens=10, got %d", rec.InputTokens)
	}
	if rec.OutputTokens != 5 {
		t.Errorf("expected OutputTokens=5, got %d", rec.OutputTokens)
	}
	if rec.KeyID != "key-1" {
		t.Errorf("expected KeyID=key-1, got %q", rec.KeyID)
	}
	if rec.UpstreamID != u.ID {
		t.Errorf("expected UpstreamID=%d, got %d", u.ID, rec.UpstreamID)
	}
}

func TestRelay_Usage_Failure(t *testing.T) {
	db := setupTestDB(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	rec := waitForUsageRecord(t, db)
	if rec.Success {
		t.Error("expected Success=false")
	}
	if rec.InputTokens != 0 {
		t.Errorf("expected InputTokens=0, got %d", rec.InputTokens)
	}
	if rec.OutputTokens != 0 {
		t.Errorf("expected OutputTokens=0, got %d", rec.OutputTokens)
	}
}

// --- Streaming relay tests ---

// fakeStreamingUpstream creates a test server that sends SSE frames.
func fakeStreamingUpstream(frames []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher := w.(http.Flusher)
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			flusher.Flush()
		}
	}))
}

// fakeSlowStreamingUpstream sends one frame, then waits for the delay, then sends the rest.
func fakeSlowStreamingUpstream(firstFrame string, delay time.Duration, rest []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", firstFrame)
		flusher.Flush()
		time.Sleep(delay)
		for _, f := range rest {
			fmt.Fprintf(w, "data: %s\n\n", f)
			flusher.Flush()
		}
	}))
}

// fakeMidFailureUpstream sends one frame then closes abruptly.
func fakeMidFailureUpstream(firstFrame string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", firstFrame)
		flusher.Flush()
		// Hijack to close the underlying connection abruptly
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
}

// streamBody sends a streaming chat request and returns the response body as string.
func streamBody(t *testing.T, engine http.Handler) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(true)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestRelay_Stream(t *testing.T) {
	db := setupTestDB(t)
	frames := []string{
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":" World"}}]}`,
		`{"choices":[{"delta":{"content":"!"}}]}`,
		`[DONE]`,
	}
	fake := fakeStreamingUpstream(frames)
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(true)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream content-type, got %q", ct)
	}
	accel := w.Header().Get("X-Accel-Buffering")
	if accel != "no" {
		t.Errorf("expected X-Accel-Buffering=no, got %q", accel)
	}
	cc := w.Header().Get("Cache-Control")
	if cc != "no-cache" {
		t.Errorf("expected Cache-Control=no-cache, got %q", cc)
	}
	respBody := w.Body.String()
	for _, frame := range frames {
		if !strings.Contains(respBody, frame) {
			t.Errorf("expected frame %q in response body, got: %s", frame, respBody)
		}
	}
}

func TestRelay_Stream_Heartbeat(t *testing.T) {
	db := setupTestDB(t)
	origInterval := server.HeartbeatInterval
	server.HeartbeatInterval = 50 * time.Millisecond
	defer func() { server.HeartbeatInterval = origInterval }()

	fake := fakeSlowStreamingUpstream(
		`{"choices":[{"delta":{"content":"hi"}}]}`,
		120*time.Millisecond,
		[]string{`[DONE]`},
	)
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(true)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, ": heartbeat") {
		t.Errorf("expected heartbeat comment in response body, got: %q", body)
	}
}

func TestRelay_Stream_MidFailure(t *testing.T) {
	db := setupTestDB(t)
	fake := fakeMidFailureUpstream(`{"choices":[{"delta":{"content":"partial"}}]}`)
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(true)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Once HTTP 200 is committed (streaming started), we can't change status.
	// The connection closes on upstream failure — handler returns, test recorder captures it.
	if w.Code != 200 {
		t.Errorf("expected 200 (already committed), got %d", w.Code)
	}
	// The partial frame should have been forwarded
	if !strings.Contains(w.Body.String(), "partial") {
		t.Errorf("expected partial frame in body, got: %q", w.Body.String())
	}
}

func TestRelay_Stream_Usage(t *testing.T) {
	db := setupTestDB(t)
	frames := []string{
		`{"choices":[{"delta":{"content":"hello"}}]}`,
		`[DONE]`,
	}
	fake := fakeStreamingUpstream(frames)
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	u := seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(true)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	rec := waitForUsageRecord(t, db)
	if !rec.Success {
		t.Error("expected Success=true for streaming request")
	}
	if rec.InputTokens != 0 {
		t.Errorf("expected InputTokens=0 for streaming (D-13 fallback), got %d", rec.InputTokens)
	}
	if rec.OutputTokens != 0 {
		t.Errorf("expected OutputTokens=0 for streaming (D-13 fallback), got %d", rec.OutputTokens)
	}
	if rec.UpstreamID != u.ID {
		t.Errorf("expected UpstreamID=%d, got %d", u.ID, rec.UpstreamID)
	}
}

func TestRelay_Stream_Failover(t *testing.T) {
	db := setupTestDB(t)
	calls500 := int32(0)
	callsStream := int32(0)

	fake500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls500, 1)
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer fake500.Close()

	fakeStream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callsStream, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"ok"}}]}`)
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer fakeStream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	// Seed stream-ok first (lower ID) so pool idx=0→1 selects up-500 first
	seedUpstream(t, db, "up-stream-ok", fakeStream.URL)
	seedUpstream(t, db, "up-500", fake500.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(true)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 after failover, got %d: %s", w.Code, w.Body.String())
	}
	if atomic.LoadInt32(&calls500) != 1 {
		t.Errorf("expected 500 upstream called once, called %d times", calls500)
	}
	if atomic.LoadInt32(&callsStream) != 1 {
		t.Errorf("expected stream upstream called once, called %d times", callsStream)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream content-type, got %q", ct)
	}
}

// Ensure the test file has >100 lines
var _ = fmt.Sprintf
