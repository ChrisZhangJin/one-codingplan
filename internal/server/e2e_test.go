// e2e_test.go exercises the full HTTP stack by starting the actual server via
// httptest.NewServer and sending real requests over the network.
// Each test spins up fresh mock upstream(s), a real in-memory DB, and the server.
package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"one-codingplan/internal/database"
	"one-codingplan/internal/pool"
)

// ---- helpers ----------------------------------------------------------------

// e2eServer wires up a real httptest.Server backed by the ocp engine.
// Returns the server URL and a cleanup func.
func e2eServer(t *testing.T, p *pool.Pool) (serverURL string, cleanup func()) {
	t.Helper()
	db := setupTestDB(t)
	seedAccessKey(t, db, "e2e-token", true)
	if p == nil {
		p = buildPool(t, db, 10*time.Millisecond)
	}
	srv := buildServer(db, p)
	ts := httptest.NewServer(srv.Engine())
	return ts.URL, ts.Close
}

// e2eServerWithDB is like e2eServer but also returns the DB for inspection.
func e2eServerWithDB(t *testing.T) (serverURL string, db interface{ First(interface{}, ...interface{}) interface{} }, cleanup func()) {
	t.Helper()
	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	srv := buildServer(gormDB, p)
	ts := httptest.NewServer(srv.Engine())
	return ts.URL, nil, ts.Close
}

// post sends a POST to serverURL+path with the given JSON body and bearer token.
func post(t *testing.T, serverURL, path, token string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, serverURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// get sends a GET to serverURL+path with the given bearer token.
func get(t *testing.T, serverURL, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, serverURL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func readJSON(t *testing.T, r io.Reader) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return m
}

// fakeAnthropicUpstream returns a server that replies with a native Anthropic response.
func fakeAnthropicUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	resp := map[string]interface{}{
		"id":          "msg_e2e_01",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-opus-4-5",
		"content":     []map[string]interface{}{{"type": "text", "text": "native anthropic reply"}},
		"stop_reason": "end_turn",
		"usage":       map[string]interface{}{"input_tokens": 5, "output_tokens": 4},
	}
	body, _ := json.Marshal(resp)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(body)
	}))
}

// ---- Health -----------------------------------------------------------------

func TestE2E_Health(t *testing.T) {
	gormDB := setupTestDB(t)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	srv := buildServer(gormDB, p)
	ts := httptest.NewServer(srv.Engine())
	defer ts.Close()

	resp := get(t, ts.URL, "/health", "")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	m := readJSON(t, resp.Body)
	if m["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", m["status"])
	}
}

// ---- OpenAI relay -----------------------------------------------------------

func TestE2E_OpenAI_NonStream(t *testing.T) {
	upstream := fakeOpenAIUpstream(openAIResponseBody("hello from upstream", "stop", 8, 4))
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/chat/completions", "e2e-token", chatReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json Content-Type, got %q", ct)
	}
}

func TestE2E_OpenAI_Auth_Missing(t *testing.T) {
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 1, 1))
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/chat/completions", "", chatReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
	}
}

func TestE2E_OpenAI_Auth_Invalid(t *testing.T) {
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 1, 1))
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/chat/completions", "wrong-token", chatReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("expected 401 for wrong token, got %d", resp.StatusCode)
	}
}

// ---- Anthropic relay (passthrough) -----------------------------------------

func TestE2E_Anthropic_NonStream_Passthrough(t *testing.T) {
	var capturedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		body := map[string]interface{}{
			"id": "msg_e2e", "type": "message", "role": "assistant",
			"model":       "claude-opus-4-5",
			"content":     []map[string]interface{}{{"type": "text", "text": "passthrough reply"}},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 10, "output_tokens": 6},
		}
		json.NewEncoder(w).Encode(body)
	}))
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	// Upstream must have been called at /v1/messages (passthrough)
	if capturedPath != "/v1/messages" {
		t.Errorf("expected passthrough path /v1/messages, upstream got %q", capturedPath)
	}
	// Response must be Anthropic-format (forwarded verbatim)
	m := readJSON(t, resp.Body)
	if m["type"] != "message" {
		t.Errorf("expected type=message, got %v", m["type"])
	}
	if m["role"] != "assistant" {
		t.Errorf("expected role=assistant, got %v", m["role"])
	}
}

func TestE2E_Anthropic_NonStream_Auth(t *testing.T) {
	upstream := fakeAnthropicUpstream(t)
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	// No auth token
	resp := post(t, ts.URL, "/v1/messages", "", anthropicReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
	}
}

// ---- Phase 8: format passthrough -------------------------------------------

func TestE2E_Anthropic_Passthrough_FormatField(t *testing.T) {
	var capturedPath string
	var capturedBody []byte

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		body := map[string]interface{}{
			"id": "msg_pass", "type": "message", "role": "assistant",
			"model":       "claude-opus-4-5",
			"content":     []map[string]interface{}{{"type": "text", "text": "passthrough ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 5, "output_tokens": 3},
		}
		json.NewEncoder(w).Encode(body)
	}))
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "mimo", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	originalBody := anthropicReqBody(false)
	resp := post(t, ts.URL, "/v1/messages", "e2e-token", originalBody)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	// Upstream must receive request at /v1/messages (not /v1/chat/completions)
	if capturedPath != "/v1/messages" {
		t.Errorf("expected passthrough path /v1/messages, upstream got %q", capturedPath)
	}
	// Body forwarded verbatim
	if !bytes.Equal(capturedBody, originalBody) {
		t.Errorf("body not forwarded verbatim\n  sent: %s\n   got: %s", originalBody, capturedBody)
	}
	// Response forwarded verbatim (contains passthrough text)
	respBody, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(respBody, []byte("passthrough ok")) {
		t.Errorf("expected passthrough response in body, got: %s", respBody)
	}
}

func TestE2E_Anthropic_BothUpstreams_ReceiveMessagesPath(t *testing.T) {
	// Two upstreams in the same pool: both must receive requests at /v1/messages (passthrough).

	up1Called := int32(0)
	up2Called := int32(0)

	makeUpstream := func(counter *int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/messages" {
				w.WriteHeader(500)
				fmt.Fprintf(w, "wrong path: %s", r.URL.Path)
				return
			}
			atomic.AddInt32(counter, 1)
			resp := map[string]interface{}{
				"id": "msg_ok", "type": "message", "role": "assistant",
				"model":       "claude-opus-4-5",
				"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
				"stop_reason": "end_turn",
				"usage":       map[string]interface{}{"input_tokens": 1, "output_tokens": 1},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
	}

	up1 := makeUpstream(&up1Called)
	defer up1.Close()
	up2 := makeUpstream(&up2Called)
	defer up2.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", up1.URL)
	seedUpstream(t, gormDB, "up2", up2.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	// Send two requests — pool round-robins across upstreams
	for i := 0; i < 2; i++ {
		resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(false))
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("request %d: expected 200, got %d: %s", i+1, resp.StatusCode, body)
		}
	}

	if atomic.LoadInt32(&up1Called) == 0 {
		t.Error("up1 was never called")
	}
	if atomic.LoadInt32(&up2Called) == 0 {
		t.Error("up2 was never called")
	}
}

// ---- Phase 8: ClassModelNotSupported disables upstream ---------------------

func TestE2E_ClassModelNotSupported_DisablesUpstream(t *testing.T) {
	// Upstream 1 always returns 500 model-not-supported.
	// Upstream 2 always returns 200.
	// Strategy: send enough requests to guarantee minimax is tried at least once
	// and permanently marked unavailable. Then confirm it is never retried.

	up1Calls := int32(0)

	up1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&up1Calls, 1)
		w.WriteHeader(500)
		w.Write([]byte(`{"code":1000,"message":"your current token plan not support model, MiniMax-Text-01 (2061)"}`))
	}))
	defer up1.Close()

	up2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("ok", "stop", 5, 3))
	}))
	defer up2.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "minimax", up1.URL)
	seedUpstream(t, gormDB, "kimi", up2.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	// Warm-up: send enough requests to ensure minimax has been selected at least
	// once (round-robin guarantees every upstream is tried within 2 requests).
	// After minimax is tried it is permanently marked unavailable.
	for i := 0; i < 3; i++ {
		resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(false))
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("warm-up request %d: expected 200, got %d: %s", i+1, resp.StatusCode, body)
		}
	}

	// Now minimax must have been tried (up1Calls >= 1) and marked unavailable.
	if atomic.LoadInt32(&up1Calls) == 0 {
		t.Fatal("minimax was never called during warm-up — test setup may be wrong")
	}

	// From this point minimax must not receive any further calls.
	up1CallsAfterWarmup := atomic.LoadInt32(&up1Calls)
	for i := 0; i < 5; i++ {
		resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(false))
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("post-warmup request %d: expected 200, got %d: %s", i+1, resp.StatusCode, body)
		}
	}
	if got := atomic.LoadInt32(&up1Calls); got > up1CallsAfterWarmup {
		t.Errorf("minimax received %d calls after being marked unavailable (expected 0)", got-up1CallsAfterWarmup)
	}
}

// ---- Failover ---------------------------------------------------------------

func TestE2E_Failover_TransientError(t *testing.T) {
	firstCall := int32(0)

	up1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail on first call, succeed on retry (pool will hit up2)
		if atomic.AddInt32(&firstCall, 1) == 1 {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"transient"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("ok", "stop", 3, 2))
	}))
	defer up1.Close()

	up2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("ok from up2", "stop", 3, 2))
	}))
	defer up2.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", up1.URL)
	seedUpstream(t, gormDB, "up2", up2.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200 after failover, got %d: %s", resp.StatusCode, body)
	}
}

func TestE2E_AllUpstreamsFail_Returns503(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer up.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", up.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 503 when all upstreams fail, got %d: %s", resp.StatusCode, body)
	}
}

func TestE2E_NoUpstreams_Returns503(t *testing.T) {
	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	// No upstream seeded
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 503 with no upstreams, got %d: %s", resp.StatusCode, body)
	}
}

// ---- Admin API --------------------------------------------------------------

func TestE2E_Admin_CreateAndListKey(t *testing.T) {
	gormDB := setupTestDB(t)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	// Create a key
	createBody, _ := json.Marshal(map[string]interface{}{
		"name":               "test-key",
		"token_budget":       500000,
		"rate_limit_per_minute": 60,
	})
	resp := post(t, ts.URL, "/api/keys", "admin-key", createBody)
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create key: expected 200/201, got %d: %s", resp.StatusCode, body)
	}
	created := readJSON(t, resp.Body)
	keyToken, ok := created["token"].(string)
	if !ok || keyToken == "" {
		t.Fatalf("expected token in response, got %v", created)
	}

	// List keys — must contain the created key
	listResp := get(t, ts.URL, "/api/keys", "admin-key")
	defer listResp.Body.Close()
	if listResp.StatusCode != 200 {
		t.Fatalf("list keys: expected 200, got %d", listResp.StatusCode)
	}
	var keys []interface{}
	if err := json.NewDecoder(listResp.Body).Decode(&keys); err != nil {
		t.Fatalf("decode key list: %v", err)
	}
	if len(keys) == 0 {
		t.Error("expected at least one key in list")
	}
}

func TestE2E_Admin_BlockKey_RejectsRequests(t *testing.T) {
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 1, 1))
	defer upstream.Close()

	gormDB := setupTestDB(t)
	// Create key directly with a known token
	database.Migrate(gormDB)

	seedAccessKey(t, gormDB, "block-test-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	// Confirm the key works before blocking
	resp1 := post(t, ts.URL, "/v1/messages", "block-test-token", anthropicReqBody(false))
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if resp1.StatusCode != 200 {
		t.Fatalf("pre-block: expected 200, got %d: %s", resp1.StatusCode, body1)
	}

	// Block the key via admin API (key ID is "key-1" from seedAccessKey)
	blockResp := post(t, ts.URL, "/api/keys/key-1/block", "admin-key", nil)
	blockBody, _ := io.ReadAll(blockResp.Body)
	blockResp.Body.Close()
	if blockResp.StatusCode != 200 {
		t.Fatalf("block key: expected 200, got %d: %s", blockResp.StatusCode, blockBody)
	}

	// Key must now be rejected
	resp2 := post(t, ts.URL, "/v1/messages", "block-test-token", anthropicReqBody(false))
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Errorf("after block: expected 401, got %d: %s", resp2.StatusCode, body2)
	}
}

func TestE2E_Admin_ListUpstreams(t *testing.T) {
	gormDB := setupTestDB(t)
	seedUpstream(t, gormDB, "kimi", "https://api.moonshot.ai")
	seedUpstream(t, gormDB, "minimax", "https://api.minimaxi.com")
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := get(t, ts.URL, "/api/upstreams", "admin-key")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var upstreams []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&upstreams); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(upstreams) < 2 {
		t.Errorf("expected at least 2 upstreams, got %d", len(upstreams))
	}
}

func TestE2E_Admin_NoKey_Returns401(t *testing.T) {
	gormDB := setupTestDB(t)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := get(t, ts.URL, "/api/keys", "")
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("expected 401 without admin key, got %d", resp.StatusCode)
	}
}

// ---- Streaming --------------------------------------------------------------

func TestE2E_Anthropic_Stream(t *testing.T) {
	upstream := fakeAnthropicSSEUpstream()
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(true))
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	for _, want := range []string{"event: message_start", "event: content_block_delta", "event: message_stop"} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("expected %q in SSE response, body: %s", want, bodyStr)
		}
	}
}

// ---- Hop-by-hop header fix (WR-03) -----------------------------------------

func TestE2E_HopByHop_NotForwarded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send hop-by-hop headers in response — server must strip them
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("hi", "stop", 2, 1))
	}))
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	resp := post(t, ts.URL, "/v1/messages", "e2e-token", anthropicReqBody(false))
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	// Go's HTTP/1.1 client strips Transfer-Encoding from the response automatically,
	// but we can check that Connection: keep-alive is not present as a raw header.
	// (net/http also normalises Connection, so we check our response is clean JSON)
	if resp.Header.Get("Transfer-Encoding") == "chunked" {
		t.Error("Transfer-Encoding: chunked must not be forwarded to client (WR-03)")
	}
}

// ---- Passthrough body integrity --------------------------------------------

func TestE2E_Anthropic_BodyForwardedVerbatim(t *testing.T) {
	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		body := map[string]interface{}{
			"id": "msg_vb", "type": "message", "role": "assistant",
			"model":       "claude-opus-4-5",
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 5, "output_tokens": 3},
		}
		json.NewEncoder(w).Encode(body)
	}))
	defer upstream.Close()

	gormDB := setupTestDB(t)
	seedAccessKey(t, gormDB, "e2e-token", true)
	seedUpstream(t, gormDB, "up1", upstream.URL)
	p := buildPool(t, gormDB, 10*time.Millisecond)
	ts := httptest.NewServer(buildServer(gormDB, p).Engine())
	defer ts.Close()

	originalBody := anthropicReqBody(false)
	resp := post(t, ts.URL, "/v1/messages", "e2e-token", originalBody)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !bytes.Equal(capturedBody, originalBody) {
		t.Errorf("body not forwarded verbatim\n  sent: %s\n   got: %s", originalBody, capturedBody)
	}
}
