---
phase: 01-project-skeleton-data-layer
reviewed: 2026-04-16T00:00:00Z
depth: standard
files_reviewed: 12
files_reviewed_list:
  - .gitignore
  - cmd/ocp/main.go
  - config.yaml.example
  - go.mod
  - go.sum
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/database/database.go
  - internal/database/database_test.go
  - internal/models/models.go
  - internal/server/server.go
  - internal/server/server_test.go
findings:
  critical: 2
  warning: 4
  info: 2
  total: 8
status: issues_found
---

# Phase 01: Code Review Report

**Reviewed:** 2026-04-16T00:00:00Z
**Depth:** standard
**Files Reviewed:** 12
**Status:** issues_found

## Summary

This phase establishes the project skeleton: config loading (Viper), SQLite data layer (GORM), model definitions, and a Gin HTTP server stub. The overall structure is sound and follows the chosen technology stack correctly. The pure-Go SQLite driver, WAL mode, and OnConflict upsert are all appropriate choices.

Two critical issues require attention before this skeleton can be safely extended: upstream API keys are stored in plaintext, and the server engine is never wired to the database or config (making future handler work impossible without architectural rework). Four warnings cover a `go.mod` version that does not exist, a dependency wiring gap in `main.go`, the default admin key bypass, and an env-override gap for upstream config. Two info items cover unchecked GORM errors in tests and a removed-upstream retention behavior.

## Critical Issues

### CR-01: Upstream API keys stored in plaintext in SQLite

**File:** `internal/models/models.go:12`
**Issue:** `APIKey string` is stored as a plain text column. Any process or user with read access to `ocp.db` can extract all upstream API keys. This is a particularly high-value target because these are coding plan credentials with billing implications.
**Fix:** At minimum, encrypt the key before writing and decrypt after reading, using a key derived from `admin_key` or a separate `OCP_ENCRYPTION_KEY` env var. A simpler interim step is to store only the last 4 chars for display and require the plaintext only at write time, but that does not help if the record is read for proxying. For proxying use cases the full key is needed at runtime, so envelope encryption (AES-GCM, key from env) is the appropriate pattern.

```go
// Example: store encrypted, expose plaintext via getter
type Upstream struct {
    // ...
    APIKeyEnc []byte `gorm:"column:api_key_enc"` // AES-GCM ciphertext
}

func (u *Upstream) DecryptAPIKey(encKey []byte) (string, error) { ... }
```

### CR-02: Database and config never wired into the server engine

**File:** `cmd/ocp/main.go:35`
**Issue:** `server.NewEngine()` is called with no arguments and returns a bare `*gin.Engine` that has no reference to `db` or `cfg`. Any handler added in subsequent phases will have no way to access the database or configuration without architectural changes. This is not merely a future concern — the current structure will force either global variables or a full refactor when the first real handler is written.
**Fix:** Define a server struct that holds dependencies and expose a constructor accepting them:

```go
// internal/server/server.go
type Server struct {
    db  *gorm.DB
    cfg *config.Config
}

func New(db *gorm.DB, cfg *config.Config) *Server {
    return &Server{db: db, cfg: cfg}
}

func (s *Server) Engine() *gin.Engine {
    r := gin.New()
    r.Use(gin.Logger(), gin.Recovery())
    r.GET("/health", s.handleHealth)
    return r
}
```

```go
// cmd/ocp/main.go
srv := server.New(db, cfg)
r := srv.Engine()
```

## Warnings

### WR-01: `go.mod` specifies non-existent Go version

**File:** `go.mod:3`
**Issue:** `go 1.25.0` is specified but Go 1.25 does not exist (current release line as of April 2026 is Go 1.24.x). A toolchain with a lower version will refuse to build (`go: toolchain go1.25.0 not found`), and `go mod tidy` will fail in CI unless the installed toolchain happens to be newer than what's declared.
**Fix:** Change to the actual Go version in use:

```
go 1.24.0
```

Run `go mod tidy` afterward to confirm the go.sum remains consistent.

### WR-02: Default `admin_key` value accepted at runtime without warning

**File:** `config.yaml.example:3`, `internal/config/config.go:31-56`
**Issue:** The example config ships `admin_key: "change-me"`. There is no runtime check that the operator has changed this value. Any deployment that copies the example file verbatim will expose admin endpoints to anyone who knows the default key.
**Fix:** Add a validation step in `Load` (or in `main.go` before starting the server) that refuses to start if `admin_key` is empty or equals known insecure defaults:

```go
// in Load or in main.go after Load
if cfg.Server.AdminKey == "" || cfg.Server.AdminKey == "change-me" {
    return nil, fmt.Errorf("server.admin_key must be set to a non-default value")
}
```

### WR-03: Env override only covers scalar fields; upstream slice silently ignores env vars

**File:** `internal/config/config.go:41-53`
**Issue:** `AutomaticEnv()` is called after `ReadInConfig()`. The explicit overrides on lines 51-53 rescue `server.port`, `server.admin_key`, and `database.path` from the Viper `AutomaticEnv` + `Unmarshal` edge case, but the `Upstreams` slice is never explicitly re-read. An operator cannot override `OCP_UPSTREAMS_0_API_KEY` via environment — the slice always comes from the file. This creates a security problem: if the intent is to keep API keys out of the config file and inject them via env (a common 12-factor practice), that path is silently broken.
**Fix:** Either document that upstream keys must be in the config file (not ideal for secrets), or add explicit env reads for each upstream field after `Unmarshal`, or switch to a pattern where keys are stored in the DB only and the config file only declares `name`/`base_url`.

### WR-04: Removed upstreams are not disabled during sync

**File:** `internal/database/database.go:24-41`
**Issue:** `SyncUpstreams` performs an upsert for upstreams present in config but never disables or removes upstreams that were removed from config. If an operator removes an upstream from `config.yaml` and restarts the server, the old upstream remains in the database with `enabled=true`. Any future routing logic that reads `Enabled` upstreams from the database will continue routing to a removed/expired upstream.
**Fix:** After the upsert, disable any upstream whose `name` is not in the current config list:

```go
activeNames := make([]string, len(cfgUpstreams))
for i, u := range cfgUpstreams {
    activeNames[i] = u.Name
}
db.Model(&models.Upstream{}).
    Where("name NOT IN ?", activeNames).
    Update("enabled", false)
```

## Info

### IN-01: Unchecked GORM errors in database tests

**File:** `internal/database/database_test.go:65,104,129,172`
**Issue:** Several `db.Find(...)`, `db.Model(...).Count(...)` calls in tests do not check the returned error. A GORM query failure would silently produce zero results and cause a misleading assertion failure rather than a clear error.
**Fix:** Check `db.Error` after each query or use the fluent error return:

```go
result := db.Find(&upstreams)
if result.Error != nil {
    t.Fatalf("Find: %v", result.Error)
}
```

### IN-02: `go.sum` contains two YAML libraries for the same semantic role

**File:** `go.mod:47,55`
**Issue:** Both `go.yaml.in/yaml/v3 v3.0.4` and `gopkg.in/yaml.v3 v3.0.1` appear as indirect dependencies. These are forks of the same library. This is not a bug (Viper pulls in both transitively), but it is worth noting during future dependency audits that both are present.
**Fix:** No action required now. Track in dependency review when upgrading Viper.

---

_Reviewed: 2026-04-16T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
