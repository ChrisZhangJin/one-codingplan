---
phase: 07-cli
plan: "02"
subsystem: cli
tags: [cobra, cli, tabwriter, http-client]
dependency_graph:
  requires: [cobra-root-command, pool-position-field]
  provides: [ocp-status-cmd, ocp-next-cmd, ocp-keys-cmd]
  affects: [cmd/ocp/api.go, cmd/ocp/status.go, cmd/ocp/next.go, cmd/ocp/keys.go]
tech_stack:
  added: []
  patterns: [thin-http-client-subcommand, tabwriter-table-output, shared-api-helpers]
key_files:
  created: [cmd/ocp/api.go, cmd/ocp/status.go, cmd/ocp/next.go, cmd/ocp/keys.go]
  modified: []
decisions:
  - Shared apiGet/apiPost helpers extracted to api.go rather than duplicated across files
  - http.NoBody used for POST body (no payload needed for rotate endpoint)
  - Single httpClient package-level var with 10s timeout shared across helpers
metrics:
  duration_seconds: 117
  completed_date: "2026-04-17"
  tasks_completed: 1
  tasks_total: 1
  files_changed: 4
---

# Phase 07 Plan 02: CLI Subcommands Summary

**One-liner:** Three CLI subcommands (status/next/keys) as thin HTTP clients rendering tabwriter tables, with shared apiGet/apiPost helpers handling auth, timeouts, and error formatting.

## What Was Built

Three Cobra subcommands and a shared helpers file:

- `cmd/ocp/api.go`: `apiGet` and `apiPost` helpers using a 10-second `http.Client`. Both attach `Authorization: Bearer <flagAdminKey>`, print `"Error: cannot reach ocp server at <host> -- is it running?"` on connection failure (exit 1), and decode `{"error":"..."}` from non-200 responses (exit 1).
- `cmd/ocp/status.go`: `ocp status` — GET `/api/upstreams`, renders NAME / HEALTHY / POSITION / ENABLED table via `text/tabwriter`. Position shows `>>>` for the current round-robin target.
- `cmd/ocp/next.go`: `ocp next` — POST `/api/upstreams/rotate`, prints `"Rotated to: <name>"`.
- `cmd/ocp/keys.go`: `ocp keys` — GET `/api/keys`, renders ID (8-char prefix) / NAME / TOKEN / ENABLED / BUDGET / RPM / RPD / EXPIRES / IN TOKENS / OUT TOKENS table. Zero-value limits display as `-`.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Implement ocp status, ocp next, and ocp keys subcommands | 2736d17 |

## Verification Results

- `go build ./cmd/ocp` — passes
- `go vet ./cmd/ocp` — no issues
- `ocp status --help` — shows "Show upstream health and round-robin position"
- `ocp next --help` — shows "Force rotate to the next available upstream"
- `ocp keys --help` — shows "List access keys with limits and usage"

## Deviations from Plan

**1. [Rule 2 - Missing Critical] Extracted shared helpers to api.go**
- **Found during:** Task 1
- **Issue:** Plan suggested placing apiGet in status.go and apiPost in next.go, which would have required copying the identical connection-error and non-200 error handling into two files. The plan explicitly noted this as a DRY consideration and invited use of a helpers file.
- **Fix:** Created `cmd/ocp/api.go` with both helpers centralized. All three command files use the shared helpers.
- **Files modified:** cmd/ocp/api.go (new)
- **Commit:** 2736d17

## Known Stubs

None.

## Threat Flags

None — no new network endpoints or auth paths introduced. T-07-05 (DoS via hanging requests) is mitigated by the 10-second client timeout as specified.

## Self-Check: PASSED

- cmd/ocp/api.go exists: confirmed
- cmd/ocp/status.go exists: confirmed
- cmd/ocp/next.go exists: confirmed
- cmd/ocp/keys.go exists: confirmed
- Commit 2736d17: confirmed in git log
