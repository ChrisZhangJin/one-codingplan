---
phase: 06-web-portal
plan: 03
subsystem: web-portal
tags: [react, typescript, shadcn, upstream-toggle, key-management, dashboard]
dependency_graph:
  requires: [06-01, 06-02]
  provides: [upstream-status-cards, key-table, create-key-dialog, block-key-dialog, upstream-toggle-endpoint]
  affects:
    - internal/pool/pool.go
    - internal/server/admin.go
    - internal/server/server.go
    - web/src/pages/DashboardPage.tsx
tech_stack:
  added:
    - shadcn/ui table
    - shadcn/ui badge
    - shadcn/ui dialog
    - shadcn/ui switch
    - shadcn/ui skeleton
  patterns:
    - optimistic UI with error rollback for upstream toggle
    - one-time token reveal after key creation
    - confirmation dialog for destructive key block action
    - direct unblock without confirmation
key_files:
  created:
    - web/src/components/UpstreamStatus.tsx
    - web/src/components/KeyTable.tsx
    - web/src/components/CreateKeyDialog.tsx
    - web/src/components/BlockKeyDialog.tsx
  modified:
    - internal/pool/pool.go
    - internal/server/admin.go
    - internal/server/server.go
    - web/src/pages/DashboardPage.tsx
decisions:
  - "Used SetEnabled(name, enabled) on pool rather than full Reload — simpler and sufficient for running instances; disabled entries marked available=false and skip Select"
  - "Fixed pool.List() Enabled field which was hardcoded true — now reflects actual entry.enabled state"
  - "Details icon button logs key ID to console — future plan wires to detail route per plan spec"
metrics:
  duration: "~15 minutes"
  completed: "2026-04-17"
  tasks_completed: 2
  tasks_total: 3
  files_created: 4
  files_modified: 4
---

# Phase 06 Plan 03: Upstream Status Dashboard + Key Management Table — Summary

**One-liner:** Complete interactive dashboard with upstream health cards (4-state badges + optimistic toggle), access key table (create/block/unblock/details), and POST /api/upstreams/:id/toggle backend endpoint.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Add upstream toggle endpoint + install shadcn components | 32931ee | internal/pool/pool.go, internal/server/admin.go, internal/server/server.go, web/src/components/ui/{table,badge,dialog,switch,skeleton}.tsx |
| 2 | Upstream status cards + key management table + dialogs | 125a1f6 | web/src/components/UpstreamStatus.tsx, KeyTable.tsx, CreateKeyDialog.tsx, BlockKeyDialog.tsx, web/src/pages/DashboardPage.tsx |

## Task 3: Awaiting Human Verification

Task 3 is a `checkpoint:human-verify` gate — the binary is built and ready for browser verification.

## Verification Results

- `go build ./...` exits 0
- `cd web && npm run build` exits 0
- `make build` exits 0, produces `./ocp` binary with embedded web dist

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] pool.List() returned hardcoded Enabled: true for all entries**
- **Found during:** Task 1 (code review before writing toggle handler)
- **Issue:** `pool.List()` always returned `Enabled: true` regardless of actual state; the `entry` struct had no `enabled` field. This meant the UpstreamStatus component would always show upstreams as enabled even after toggling.
- **Fix:** Added `enabled bool` field to `entry` struct; set it to `true` in `New()` constructor; updated `List()` to return `e.enabled` instead of hardcoded `true`. Added `SetEnabled(name, enabled)` method that updates both `enabled` and `available` fields under mutex.
- **Files modified:** `internal/pool/pool.go`
- **Commit:** 32931ee

## Known Stubs

- Details icon button in KeyTable.tsx uses `console.log('detail', key.id)` — stub click handler per plan spec; a future plan will wire this to a detail panel or route. Button is present with correct `aria-label="View key details"` and ghost variant.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: new-admin-mutation | internal/server/admin.go | POST /api/upstreams/:id/toggle — new mutation endpoint at admin trust boundary; protected by adminMiddleware (constant-time comparison). Covered by T-6-08 and T-6-12 in plan threat model. |

## Self-Check: PASSED

All created files exist on disk:
- `/root/workspace/one-codingplan/web/src/components/UpstreamStatus.tsx` — FOUND
- `/root/workspace/one-codingplan/web/src/components/KeyTable.tsx` — FOUND
- `/root/workspace/one-codingplan/web/src/components/CreateKeyDialog.tsx` — FOUND
- `/root/workspace/one-codingplan/web/src/components/BlockKeyDialog.tsx` — FOUND

Both task commits verified in git log:
- 32931ee — FOUND
- 125a1f6 — FOUND

`go build ./...` exits 0. `make build` exits 0.
