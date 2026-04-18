---
phase: 13-portal-ux
plan: "01"
subsystem: portal-ux
tags: [backend, frontend, upstream-management, admin-api]
dependency_graph:
  requires: []
  provides: [POST /api/upstreams, pool.AddEntry, AddUpstreamDialog, UpstreamStatus add button]
  affects: [internal/pool/pool.go, internal/server/admin.go, internal/server/server.go, web/src/components/AddUpstreamDialog.tsx, web/src/components/UpstreamStatus.tsx]
tech_stack:
  added: []
  patterns: [gin-handler-create-pattern, pool-add-entry, react-dialog-form]
key_files:
  created:
    - web/src/components/AddUpstreamDialog.tsx
  modified:
    - internal/pool/pool.go
    - internal/server/admin.go
    - internal/server/server.go
    - web/src/components/UpstreamStatus.tsx
decisions:
  - "Return the pool List() entry for the new upstream in the 201 response (same pattern as handleUpdateUpstream) to give the caller current availability state"
  - "DialogFooter placed outside the gap-4 flex container to match shadcn Dialog layout convention"
metrics:
  duration: "~12 minutes"
  completed: "2026-04-18"
  tasks_completed: 2
  tasks_total: 2
  files_changed: 5
---

# Phase 13 Plan 01: Add Upstream Portal Flow Summary

**One-liner:** POST /api/upstreams with AES key encryption, pool.AddEntry for live registration, and AddUpstreamDialog React component with 4-field form wired to UpstreamStatus header button.

## Tasks Completed

| # | Name | Commit | Files |
|---|------|--------|-------|
| 1 | Backend — POST /api/upstreams + pool.AddEntry | 4c67780 | internal/pool/pool.go, internal/server/admin.go, internal/server/server.go |
| 2 | Frontend — AddUpstreamDialog + UpstreamStatus integration | f8c6881 | web/src/components/AddUpstreamDialog.tsx, web/src/components/UpstreamStatus.tsx |

## What Was Built

**Backend (Task 1):**
- `pool.AddEntry(id, name, baseURL, apiKey, modelOverride)` — appends a new enabled+available entry to the in-memory pool under write lock
- `createUpstreamRequest` struct with `binding:"required"` on name, base_url, api_key
- `handleCreateUpstream` — binds JSON, encrypts API key with `crypto.Encrypt`, creates DB record via GORM, calls `pool.AddEntry`, returns 201 with pool `UpstreamInfo` + masked key
- Route: `api.POST("/upstreams", s.handleCreateUpstream)` registered before the existing `/upstreams/rotate` route

**Frontend (Task 2):**
- `AddUpstreamDialog` component: 4 fields (name, base URL, API key as `type="password"`, model override), client-side validation with `toast.error`, POST to `/api/upstreams`, success toast + `onCreated()` + close on success
- `UpstreamStatus` updated: imports `AddUpstreamDialog`, adds `addOpen` state, wraps h2 sibling buttons in `<div className="flex items-center gap-2">`, mounts `<AddUpstreamDialog>` after `EditUpstreamDialog`

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None — all data paths are wired end-to-end.

## Threat Flags

No new security surface beyond what the plan's threat model covers. The new `POST /api/upstreams` route is inside the `api` group which applies `adminMiddleware` (Bearer token check) — T-13-01 mitigated. API key is encrypted before DB storage — T-13-02 mitigated. Response uses `maskAPIKey()` — T-13-03 mitigated.

## Self-Check: PASSED

- `internal/pool/pool.go` — AddEntry at line 189: FOUND
- `internal/server/admin.go` — handleCreateUpstream at line 287: FOUND
- `internal/server/server.go` — POST /upstreams route: FOUND
- `web/src/components/AddUpstreamDialog.tsx` — created: FOUND
- `web/src/components/UpstreamStatus.tsx` — AddUpstreamDialog references: FOUND (2 matches)
- Commit 4c67780: FOUND
- Commit f8c6881: FOUND
- `go build ./...` — PASSED
- `tsc --noEmit` — PASSED
