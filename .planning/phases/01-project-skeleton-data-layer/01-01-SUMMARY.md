---
phase: 01-project-skeleton-data-layer
plan: "01"
subsystem: foundation
tags: [go, viper, gorm, sqlite, config, database]
dependency_graph:
  requires: []
  provides:
    - go-module-one-codingplan
    - config-package
    - models-package
    - database-package
  affects:
    - all-subsequent-phases
tech_stack:
  added:
    - github.com/gin-gonic/gin@v1.10.1
    - gorm.io/gorm@v1.31.1
    - github.com/glebarez/sqlite@v1.11.0
    - github.com/spf13/viper@v1.21.0
    - github.com/google/uuid@v1.6.0
  patterns:
    - Viper config loading with explicit post-Unmarshal env override for nested keys
    - GORM AutoMigrate for schema creation on first run
    - clause.OnConflict upsert for config-driven upstream sync
    - SQLite WAL mode via DSN query params at Open time
key_files:
  created:
    - go.mod
    - go.sum
    - .gitignore
    - config.yaml.example
    - internal/config/config.go
    - internal/config/config_test.go
    - internal/models/models.go
    - internal/database/database.go
    - internal/database/database_test.go
    - cmd/ocp/main.go
  modified: []
decisions:
  - "Gin pinned at v1.10.1 (not @latest=v1.12.0) because v1.12 requires Go 1.25; project targets Go 1.24"
  - "Post-Unmarshal explicit field override pattern for Viper env vars (cfg.Server.Port = v.GetInt) to bypass AutomaticEnv + mapstructure nested key edge case"
  - "cmd/ocp/main.go created in this plan (not deferred to later) to make gin a direct go.mod dependency and establish startup sequence"
metrics:
  duration_minutes: 5
  completed_date: "2026-04-16"
  tasks_completed: 2
  files_created: 10
  files_modified: 0
---

# Phase 01 Plan 01: Go Module, Config, Models, and Database Layer Summary

**One-liner:** Go module scaffolded with Viper YAML config (env override), GORM model structs for all three tables, and SQLite database package with WAL mode open, AutoMigrate, and upsert-based upstream sync.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Go module + config package + model structs | 35952ff | go.mod, go.sum, .gitignore, config.yaml.example, internal/config/config.go, internal/config/config_test.go, internal/models/models.go |
| 2 | Database package — open, migrate, sync upstreams | 698bb54 | internal/database/database.go, internal/database/database_test.go |
| 3 | main.go stub + gin direct dependency | d10ec9e | cmd/ocp/main.go, go.mod, go.sum |

## Test Results

- `go test ./internal/config/... -v -count=1` — 4/4 PASS
- `go test ./internal/database/... -v -count=1` — 6/6 PASS
- `go test ./internal/... -race -count=1` — all PASS, no races

## Decisions Made

1. **Gin v1.10.1 pinned explicitly.** `@latest` resolves to v1.12.0 which requires Go 1.25; the project targets Go 1.24.13. Pinned with `go get github.com/gin-gonic/gin@v1.10.1` and verified go.mod locks the version.

2. **Post-Unmarshal explicit field override for Viper env vars.** After `v.Unmarshal(&cfg)`, explicitly set `cfg.Server.Port = v.GetInt("server.port")` etc. This bypasses the Viper `AutomaticEnv` + mapstructure nested key edge case documented in RESEARCH.md Pitfall 2. Tested and confirmed working.

3. **main.go created in this plan.** The plan listed `cmd/ocp/main.go` as a file to be modified and acceptance criteria required gin in go.mod. Creating a minimal startup-sequence main.go establishes the foundation pattern and satisfies the direct dependency requirement without deviating from plan intent.

4. **WAL mode DSN conditional.** `:memory:` paths do not accept `?_journal=WAL` query params; the `Open` function applies WAL params only for non-memory paths.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] gin not in go.mod as direct dependency**
- **Found during:** Task 2 verification
- **Issue:** After `go mod tidy`, gin was removed from go.mod because no source file imported it yet. The plan's acceptance criteria requires `github.com/gin-gonic/gin v1.10.1` in go.mod.
- **Fix:** Created `cmd/ocp/main.go` (also listed as a plan artifact) which imports gin directly. This satisfies both the dependency requirement and the startup sequence pattern described in RESEARCH.md.
- **Files modified:** cmd/ocp/main.go, go.mod, go.sum
- **Commit:** d10ec9e

**2. [Rule 1 - Bug] WAL mode DSN fails for in-memory SQLite**
- **Found during:** Task 2 database tests
- **Issue:** `glebarez/sqlite` driver rejects `?_journal=WAL&_timeout=5000` appended to `:memory:` path.
- **Fix:** Added conditional in `Open()` to only append DSN params for non-memory paths. In-memory DB (used exclusively in tests) opens without params.
- **Files modified:** internal/database/database.go
- **Commit:** 698bb54

## Known Stubs

None. All code is wired. `cmd/ocp/main.go` is a functional startup sequence, not a placeholder.

## Threat Flags

None beyond those covered in the plan's threat model (T-01-01 through T-01-04):
- `.gitignore` includes `config.yaml` and `ocp.db` (T-01-01, T-01-02 mitigated)
- `config.yaml.example` contains only placeholder values (T-01-01 mitigated)

## Self-Check: PASSED

All 11 created files confirmed on disk. All 3 task commits confirmed in git history (35952ff, 698bb54, d10ec9e).
