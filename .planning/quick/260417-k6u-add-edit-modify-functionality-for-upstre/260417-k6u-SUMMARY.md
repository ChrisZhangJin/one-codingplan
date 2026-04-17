---
id: 260417-k6u
type: quick
phase: quick
plan: 260417-k6u
subsystem: api,portal
tags: [upstream, access-keys, crud, patch, edit-dialog]
completed: "2026-04-17T14:46:55Z"
duration_minutes: 30
tasks_completed: 3
tasks_total: 3
files_modified: 9
files_created: 2
commits:
  - 5a2591b
  - 28a47a8
  - 83ed963
key_decisions:
  - Thread encKey into Server struct to enable upstream API key encryption in handlers
  - Add Format/ModelOverride to Upstream DB model so they persist across restarts
  - Enrich GET /api/upstreams response with masked_key from DB decrypt+mask
  - UpdateEntry in pool updates name/baseURL/modelOverride/format; empty apiKey preserves existing
dependency_graph:
  requires: []
  provides: [PATCH /api/upstreams/:id, EditUpstreamDialog, EditKeyDialog]
  affects: [internal/server, internal/pool, internal/models, internal/database, web portal]
tech_stack:
  added: []
  patterns:
    - PATCH handler with selective updates map (same pattern as handleUpdateKey)
    - Modal dialog with useEffect pre-population from prop data
    - Pool UpdateEntry for in-memory sync after DB write
key_files:
  created:
    - web/src/components/EditUpstreamDialog.tsx
    - web/src/components/EditKeyDialog.tsx
  modified:
    - internal/models/models.go
    - internal/server/server.go
    - internal/server/admin.go
    - internal/pool/pool.go
    - internal/database/database.go
    - cmd/ocp/serve.go
    - web/src/components/UpstreamStatus.tsx
    - web/src/components/KeyTable.tsx
    - internal/server/server_test.go
    - internal/server/relay_test.go
    - internal/server/admin_test.go
---

# Quick Task 260417-k6u: Add edit/modify functionality for upstreams and access keys

**One-liner:** PATCH /api/upstreams/:id with pool sync, Format/ModelOverride DB persistence, and modal edit dialogs for both upstreams and keys.

## What Was Built

### Task 1: Backend PATCH endpoint and model changes

- Added `Format` and `ModelOverride` columns to `models.Upstream` — GORM AutoMigrate adds them on startup
- Threaded `encKey []byte` into `Server` struct; updated `New()` signature and `serve.go` call site
- Added `patchUpstreamRequest` struct and `handleUpdateUpstream` handler:
  - Looks up upstream by ID, applies partial updates
  - Empty `api_key` field preserves existing encrypted key; non-empty encrypts and replaces
  - Syncs pool in-memory state via new `pool.UpdateEntry()`
  - Returns `UpstreamInfo` with `masked_key` field
- Added `maskAPIKey()` helper (shows last 4 chars: `***xxxx`)
- Updated `handleListUpstreams` to enrich pool list with masked keys from DB
- Added `Format` and `MaskedKey` fields to `pool.UpstreamInfo`
- Added `pool.UpdateEntry()` method — updates name, baseURL, modelOverride, format; skips apiKey if empty
- Updated `pool.New()` to populate `ModelOverride` and `Format` from DB model
- Updated `database.SyncUpstreams` to include `format` and `model_override` in upsert columns
- Registered `PATCH /api/upstreams/:id` route
- Fixed `server.New()` call sites in 3 test files

### Task 2: EditUpstreamDialog + UpstreamStatus edit button

- Created `EditUpstreamDialog.tsx` with fields: name, base_url, api_key (masked placeholder), format, model_override
- Exports `UpstreamInfo` interface for reuse
- Pre-populates via `useEffect` on `upstream` prop change
- Sends only changed fields in PATCH body; empty api_key omits the field
- Added `Pencil` edit button to each upstream card in `UpstreamStatus`
- Renders `EditUpstreamDialog` with `fetchUpstreams` as `onUpdated`

### Task 3: EditKeyDialog + KeyTable edit button

- Created `EditKeyDialog.tsx` with fields: name, token_budget, allowed_upstreams, expires_at, rate_limit_per_minute, rate_limit_per_day
- Token shown as read-only masked display (not editable per design decision)
- Pre-populates via `useEffect` on `keyData` prop change
- Sends only changed fields in PATCH body (compares against original values)
- Added `Pencil` edit button before Block/Unblock in KeyTable Actions column
- Renders `EditKeyDialog` with `refetchKeys` as `onUpdated`

## Verification

- `go build ./...` — clean
- `go test ./internal/... -count=1` — all 7 packages pass
- `npx tsc --noEmit` — clean (run twice: after Task 2 and Task 3)

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints beyond the documented PATCH route; no new auth paths or trust boundary changes.

## Self-Check

Commits verified:
- 5a2591b — backend changes
- 28a47a8 — EditUpstreamDialog + UpstreamStatus
- 83ed963 — EditKeyDialog + KeyTable

Files verified:
- `/root/workspace/one-codingplan/web/src/components/EditUpstreamDialog.tsx` — exists
- `/root/workspace/one-codingplan/web/src/components/EditKeyDialog.tsx` — exists

## Self-Check: PASSED
