---
phase: quick
plan: 260417-jxm
subsystem: pool
tags: [pool, upstream, visibility, portal]
dependency_graph:
  requires: []
  provides: [pool-loads-disabled-upstreams]
  affects: [portal-upstream-list, set-enabled-toggle]
tech_stack:
  added: []
  patterns: []
key_files:
  modified:
    - internal/pool/pool.go
    - internal/pool/pool_test.go
decisions:
  - "Load all upstreams unconditionally in New(); drive available/enabled from DB field rather than filtering at query level"
metrics:
  duration: "~10 minutes"
  completed: "2026-04-17"
  tasks_completed: 1
  tasks_total: 1
  files_modified: 2
---

# Quick Fix 260417-jxm: Fix pool.New() to load all upstreams including disabled

pool.New() now loads all upstreams from DB regardless of enabled state; disabled entries appear in List() with Enabled=false/Available=false and are skipped by Select() until re-enabled via SetEnabled().

## What Changed

**`internal/pool/pool.go`**

- Removed `.Where("enabled = ?", true)` from the GORM query in `New()` — changed to `db.Find(&upstreams)`.
- Entry construction now sets `available: u.Enabled` and `enabled: u.Enabled` instead of hardcoding both to `true`.
- Updated constructor comment from "loads all enabled upstreams" to "loads all upstreams".

**`internal/pool/pool_test.go`**

- `TestNew_LoadsFromDB` rewritten to assert new behavior:
  - `List()` returns 3 entries (2 enabled + 1 disabled)
  - Disabled upstream `z` has `Enabled=false`, `Available=false` in `List()`
  - `Select()` never returns `z`
  - `SetEnabled("z", true)` makes `z` appear in `Select()`

## Deviations from Plan

None — plan executed exactly as written.

## Self-Check

- [x] `internal/pool/pool.go` modified
- [x] `internal/pool/pool_test.go` modified
- [x] Commit 957541f exists
- [x] `go test ./internal/pool/` passes (27/27)
