package config

import (
	"os"
	"testing"
)

func TestConfigLoad_FromFile(t *testing.T) {
	yaml := `
server:
  port: 9090
  admin_key: "test-admin-key"
database:
  path: "/tmp/test.db"
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Database.Path != "/tmp/test.db" {
		t.Errorf("Database.Path = %q, want /tmp/test.db", cfg.Database.Path)
	}
}

func TestConfigLoad_EnvOverride(t *testing.T) {
	yaml := `
server:
  port: 8080
  admin_key: "test-admin-key"
database:
  path: "./ocp.db"
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	f.Close()

	os.Setenv("OCP_SERVER_PORT", "9999")
	defer os.Unsetenv("OCP_SERVER_PORT")

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("Server.Port = %d, want 9999 (env override)", cfg.Server.Port)
	}
}

func TestConfigLoad_Defaults(t *testing.T) {
	yaml := `
server:
  admin_key: "test-admin-key"
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080 (default)", cfg.Server.Port)
	}
	if cfg.Database.Path != "./ocp.db" {
		t.Errorf("Database.Path = %q, want ./ocp.db (default)", cfg.Database.Path)
	}
}

func TestConfigLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load with missing file should return error, got nil")
	}
}
