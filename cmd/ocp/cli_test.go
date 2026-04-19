package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// binaryPath returns the path to a compiled ocp binary for CLI integration tests.
// It builds once per test run into a temp directory.
func binaryPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "ocp")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build ocp binary: %v\n%s", err, out)
	}
	return bin
}

func TestEnvOrDefault(t *testing.T) {
	const key = "OCP_TEST_ENV_VAR_XYZ"
	os.Unsetenv(key)
	if got := envOrDefault(key, "fallback"); got != "fallback" {
		t.Errorf("want fallback, got %q", got)
	}
	os.Setenv(key, "from-env")
	defer os.Unsetenv(key)
	if got := envOrDefault(key, "fallback"); got != "from-env" {
		t.Errorf("want from-env, got %q", got)
	}
}

func TestStatus_Output(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/upstreams" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "name": "kimi", "enabled": true, "available": true, "position": true},
			{"id": 2, "name": "glm", "enabled": true, "available": false, "position": false},
		})
	}))
	defer srv.Close()

	bin := binaryPath(t)
	cmd := exec.Command(bin, "--host", srv.URL, "--admin-key", "test-key", "status")
	cmd.Env = append(os.Environ(), "OCP_HOST="+srv.URL)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ocp status: %v", err)
	}
	output := string(out)
	if !strings.Contains(output, "kimi") {
		t.Errorf("expected kimi in output, got:\n%s", output)
	}
	if !strings.Contains(output, "glm") {
		t.Errorf("expected glm in output, got:\n%s", output)
	}
	if !strings.Contains(output, ">>>") {
		t.Errorf("expected >>> position marker in output, got:\n%s", output)
	}
	if !strings.Contains(output, "no") {
		t.Errorf("expected 'no' for unavailable upstream, got:\n%s", output)
	}
}

func TestNext_Output(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/upstreams/rotate" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"upstream": "glm"})
	}))
	defer srv.Close()

	bin := binaryPath(t)
	cmd := exec.Command(bin, "--host", srv.URL, "--admin-key", "test-key", "next")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ocp next: %v", err)
	}
	if !strings.Contains(string(out), "glm") {
		t.Errorf("expected 'glm' in output, got: %s", out)
	}
}

func TestKeys_Output(t *testing.T) {
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/keys" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id": "abc123", "name": "dev", "token": "ocp-abc",
				"enabled": true, "token_budget": int64(100000),
				"expires_at": expires, "rate_limit_per_minute": 0, "rate_limit_per_day": 0,
				"usage_total_input": int64(500), "usage_total_output": int64(300),
			},
		})
	}))
	defer srv.Close()

	bin := binaryPath(t)
	cmd := exec.Command(bin, "--host", srv.URL, "--admin-key", "test-key", "keys")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ocp keys: %v", err)
	}
	output := string(out)
	if !strings.Contains(output, "dev") {
		t.Errorf("expected key name 'dev' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "100000") {
		t.Errorf("expected budget 100000 in output, got:\n%s", output)
	}
}

func TestStatus_ServerUnreachable(t *testing.T) {
	bin := binaryPath(t)
	cmd := exec.Command(bin, "--host", "http://127.0.0.1:19999", "--admin-key", "x", "status")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when server unreachable")
	}
	if !strings.Contains(string(out), "cannot reach") && !strings.Contains(string(out), "Error") {
		t.Errorf("expected error message, got: %s", out)
	}
}
