package pool_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"one-codingplan/internal/crypto"
	"one-codingplan/internal/database"
	"one-codingplan/internal/models"
	"one-codingplan/internal/pool"
)

// newProbePool creates a pool where each entry's BaseURL points to the given
// httptest server. names maps entry name -> server URL.
func newProbePool(t *testing.T, entries []struct {
	name   string
	apiKey string
	url    string
}) (*pool.Pool, func()) {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	for _, e := range entries {
		enc, err := crypto.Encrypt(testEncKey, e.apiKey)
		if err != nil {
			t.Fatalf("crypto.Encrypt: %v", err)
		}
		u := models.Upstream{
			Name:    e.name,
			BaseURL: e.url,
			APIKeyEnc: enc,
			Enabled: true,
		}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("db.Create: %v", err)
		}
	}
	p, err := pool.New(db, testEncKey, &pool.Config{RateLimitBackoff: 5 * time.Second})
	if err != nil {
		t.Fatalf("pool.New: %v", err)
	}
	cleanup := func() { p.Stop() }
	return p, cleanup
}

// findID selects until it finds an entry with the given name and returns its ID.
func findID(t *testing.T, p *pool.Pool, name string) uint {
	t.Helper()
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		e, err := p.Select(nil)
		if err != nil {
			break
		}
		if e.Name == name {
			return e.ID
		}
		seen[e.Name] = true
	}
	t.Fatalf("could not find upstream %q in pool", name)
	return 0
}

func TestProbe_RecoverOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"choices":[{"message":{"content":"hi"}}]}`)
	}))
	defer srv.Close()

	p, cleanup := newProbePool(t, []struct {
		name   string
		apiKey string
		url    string
	}{{"kimi", "sk-test", srv.URL}})
	defer cleanup()

	id := findID(t, p, "kimi")
	p.Mark(id, false)

	p.ProbeAll()

	// After successful probe the entry should be available again.
	e, err := p.Select(nil)
	if err != nil {
		t.Fatalf("Select after ProbeAll: %v (expected upstream to be available)", err)
	}
	if e.Name != "kimi" {
		t.Errorf("expected kimi, got %s", e.Name)
	}
}

func TestProbe_StayUnavailableOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"error":"internal server error"}`)
	}))
	defer srv.Close()

	p, cleanup := newProbePool(t, []struct {
		name   string
		apiKey string
		url    string
	}{{"kimi", "sk-test", srv.URL}})
	defer cleanup()

	id := findID(t, p, "kimi")
	p.Mark(id, false)

	p.ProbeAll()

	_, err := p.Select(nil)
	if err != pool.ErrNoUpstreams {
		t.Errorf("expected ErrNoUpstreams after failed probe, got %v", err)
	}
}

func TestProbe_StayUnavailableOnCreditsExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprintln(w, `{"error":{"code":"insufficient_quota","message":"quota exceeded"}}`)
	}))
	defer srv.Close()

	p, cleanup := newProbePool(t, []struct {
		name   string
		apiKey string
		url    string
	}{{"qwen", "sk-test", srv.URL}})
	defer cleanup()

	id := findID(t, p, "qwen")
	p.Mark(id, false)

	p.ProbeAll()

	_, err := p.Select(nil)
	if err != pool.ErrNoUpstreams {
		t.Errorf("expected ErrNoUpstreams after credits-exhausted probe, got %v", err)
	}
}

func TestProbe_SkipsAvailable(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"choices":[{"message":{"content":"hi"}}]}`)
	}))
	defer srv.Close()

	p, cleanup := newProbePool(t, []struct {
		name   string
		apiKey string
		url    string
	}{
		{"kimi", "sk-a", srv.URL},
		{"qwen", "sk-b", srv.URL},
	})
	defer cleanup()

	// Mark only qwen unavailable; kimi stays available.
	qwenID := findID(t, p, "qwen")
	p.Mark(qwenID, false)

	p.ProbeAll()

	count := atomic.LoadInt32(&requestCount)
	if count != 1 {
		t.Errorf("expected exactly 1 probe request (for unavailable qwen), got %d", count)
	}
}

func TestProbe_BodyLimitedTo64KB(t *testing.T) {
	bigBody := strings.Repeat("x", 128*1024) // 128 KB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Write a valid JSON prefix followed by lots of garbage.
		fmt.Fprintf(w, `{"choices":[{"message":{"content":"%s"}}]}`, bigBody)
	}))
	defer srv.Close()

	p, cleanup := newProbePool(t, []struct {
		name   string
		apiKey string
		url    string
	}{{"kimi", "sk-test", srv.URL}})
	defer cleanup()

	id := findID(t, p, "kimi")
	p.Mark(id, false)

	// Probe should still succeed (200 status, not credits-exhausted) even with big body.
	p.ProbeAll()

	e, err := p.Select(nil)
	if err != nil {
		t.Fatalf("Select after large-body probe: %v (expected upstream to be available)", err)
	}
	if e.Name != "kimi" {
		t.Errorf("expected kimi, got %s", e.Name)
	}
}

func TestSendProbe_RequestFormat(t *testing.T) {
	type probeRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens int `json:"max_tokens"`
	}

	var capturedMethod string
	var capturedPath string
	var capturedAuth string
	var capturedBody probeRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	p, cleanup := newProbePool(t, []struct {
		name   string
		apiKey string
		url    string
	}{{"kimi", "sk-probe-key", srv.URL}})
	defer cleanup()

	id := findID(t, p, "kimi")
	p.Mark(id, false)

	p.ProbeAll()

	if capturedMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	if !strings.HasSuffix(capturedPath, "/v1/chat/completions") {
		t.Errorf("path = %q, want suffix /v1/chat/completions", capturedPath)
	}
	if capturedAuth != "Bearer sk-probe-key" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer sk-probe-key")
	}
	if capturedBody.MaxTokens != 1 {
		t.Errorf("max_tokens = %d, want 1", capturedBody.MaxTokens)
	}
	if len(capturedBody.Messages) == 0 || capturedBody.Messages[0].Content != "hi" {
		t.Errorf("messages content = %v, want [{role:user content:hi}]", capturedBody.Messages)
	}
}
