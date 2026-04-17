---
phase: 07-cli
plan: "01"
subsystem: cli
tags: [cobra, cli, pool, serve]
dependency_graph:
  requires: []
  provides: [cobra-root-command, serve-subcommand, pool-position-field]
  affects: [cmd/ocp/main.go, cmd/ocp/root.go, cmd/ocp/serve.go, internal/pool/pool.go]
tech_stack:
  added: [github.com/spf13/cobra v1.10.2, github.com/inconshreveable/mousetrap v1.1.0]
  patterns: [cobra-subcommand-tree, persistent-flags-with-env-fallback]
key_files:
  created: [cmd/ocp/root.go, cmd/ocp/serve.go]
  modified: [cmd/ocp/main.go, internal/pool/pool.go, go.mod, go.sum]
decisions:
  - Cobra persistent flags on root command (--host, --admin-key) available to all subcommands
  - OCP_HOST and OCP_ADMIN_KEY env vars as defaults via os.Getenv in flag default value
  - serve subcommand holds all server startup logic; main.go is 4-line shim
  - Position field as bool (true = current round-robin target) rather than index integer
metrics:
  duration_seconds: 109
  completed_date: "2026-04-17"
  tasks_completed: 3
  tasks_total: 3
  files_changed: 6
---

# Phase 07 Plan 01: Cobra CLI Foundation Summary

**One-liner:** Cobra root command with persistent --host/--admin-key flags, ocp serve subcommand wrapping all server logic, and Position bool field added to pool.List() output.

## What Was Built

Restructured the ocp binary from a flat `flag`-based `main()` to a Cobra command tree:

- `cmd/ocp/root.go`: Root command defining `--host` (default `http://localhost:8080`, env `OCP_HOST`) and `--admin-key` (env `OCP_ADMIN_KEY`) as persistent flags available to all subcommands.
- `cmd/ocp/serve.go`: `ocp serve` subcommand with `--config` flag containing all server startup logic previously in `main()`. Uses `RunE` so errors propagate through Cobra's exit handling.
- `cmd/ocp/main.go`: Reduced to 4-line `rootCmd.Execute()` entry point with no business logic.
- `internal/pool/pool.go`: `UpstreamInfo` struct gains `Position bool` field; `List()` populates it as `i == p.idx` to identify the current round-robin target.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Add Cobra dependency and create root command | b0e1506 |
| 2 | Create serve subcommand and rewrite main.go | 6f2174b |
| 3 | Add Position field to UpstreamInfo | 2cc44ea |

## Verification Results

- `go build ./cmd/ocp` — passes
- `ocp --help` — shows `serve` command and `--host`, `--admin-key` flags
- `ocp serve --help` — shows `--config` flag and global flags
- `go test ./internal/pool/ -race -count=1` — all 14 tests pass

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints or auth paths introduced. The threat model item T-07-01 (admin-key visible in process list) is accepted behavior; `OCP_ADMIN_KEY` env var is the documented preferred approach.

## Self-Check: PASSED

- cmd/ocp/root.go exists: confirmed
- cmd/ocp/serve.go exists: confirmed
- cmd/ocp/main.go rewritten: confirmed (no flag.String or flag.Parse)
- internal/pool/pool.go Position field: confirmed
- Commits b0e1506, 6f2174b, 2cc44ea: confirmed in git log
