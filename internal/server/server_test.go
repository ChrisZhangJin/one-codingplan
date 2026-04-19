package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"one-codingplan/internal/server"
)

func TestHealthEndpoint_Status200(t *testing.T) {
	engine := server.New(nil, nil, nil, nil).Engine()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHealthEndpoint_JSONBody(t *testing.T) {
	engine := server.New(nil, nil, nil, nil).Engine()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var m map[string]string
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if m["status"] != "ok" {
		t.Errorf("expected status ok, got %q", m["status"])
	}
}

func TestHealthEndpoint_ContentType(t *testing.T) {
	engine := server.New(nil, nil, nil, nil).Engine()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type to contain application/json, got %q", ct)
	}
}
