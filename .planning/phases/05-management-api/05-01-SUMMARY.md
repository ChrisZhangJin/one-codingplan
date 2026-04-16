---
phase: 05-management-api
plan: 01
subsystem: server/admin
tags: [api, crud, auth, access-keys, admin]
dependency_graph:
  requires:
    - "01-skeleton (models, database, server structure)"
    - "03-relay (authMiddleware pattern, cutPrefix)"
  provides:
    - "adminMiddleware (constant-time admin key comparison)"
    - "/api route group with 9 endpoints"
    - "AccessKey model extended with limit fields"
    - "key CRUD handlers (create/list/get/update/block/unblock/delete)"
  affects:
    - "internal/models/models.go (AccessKey schema extension)"
    - "internal/server/relay.go (expiry check + accessKey context storage)"
tech_stack:
  added:
    - "crypto/subtle for constant-time comparison (T-5-01)"
    - "github.com/google/uuid for key/ID generation"
    - "encoding/json for AllowedUpstreams serialization"
  patterns:
    - "map-based GORM partial updates (never Save()) to preserve zero-value fields"
    - "TDD: RED tests written first, GREEN implementation after"
    - "COALESCE(SUM()) for null-safe usage aggregation"
key_files:
  created:
    - path: "internal/server/admin.go"
      purpose: "adminMiddleware + all key CRUD handlers + maskToken + stub upstream handlers"
    - path: "internal/server/admin_test.go"
      purpose: "14 tests covering adminMiddleware (4) and key CRUD endpoints (10)"
  modified:
    - path: "internal/models/models.go"
      change: "Added Name, TokenBudget, AllowedUpstreams, ExpiresAt, RateLimitPerMinute, RateLimitPerDay to AccessKey"
    - path: "internal/server/relay.go"
      change: "authMiddleware now stores full AccessKey in context (c.Set('accessKey', key)) and checks expiry"
    - path: "internal/server/server.go"
      change: "Added /api route group with adminMiddleware and 9 endpoint registrations"
decisions:
  - "Use crypto/subtle.ConstantTimeCompare for admin key comparison to prevent timing attacks (T-5-01)"
  - "AllowedUpstreams stored as JSON string in SQLite ('' = unrestricted) not a separate table"
  - "PATCH uses map[string]any updates to correctly handle zero-value fields like token_budget=0"
  - "Full AccessKey stored in Gin context for Plan 02 limitMiddleware reuse without second DB query"
  - "Raw token exposed only on POST /api/keys creation; all GETs return masked form"
metrics:
  duration: "~5 minutes"
  completed: "2026-04-16"
  tasks_completed: 2
  files_modified: 5
---

# Phase 05 Plan 01: Management API — AccessKey CRUD Summary

One-liner: Admin HTTP API with constant-time-auth middleware, 7 key CRUD endpoints, and AccessKey model extended with 6 limit fields.

## What Was Built

Extended the `AccessKey` model with `Name`, `TokenBudget`, `AllowedUpstreams`, `ExpiresAt`, `RateLimitPerMinute`, and `RateLimitPerDay` fields. Created `adminMiddleware` using `crypto/subtle.ConstantTimeCompare` for timing-safe admin key validation. Registered the `/api` route group in `server.go` with 9 endpoints (7 key CRUD + 2 upstream stubs). Updated `authMiddleware` in `relay.go` to store the full `AccessKey` struct in Gin context and check key expiry. Implemented all 7 key CRUD handlers with correct partial-update semantics and token masking.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Extend AccessKey model and add adminMiddleware | 0daa4de | models.go, relay.go, admin.go, server.go, admin_test.go |
| 2 | Key CRUD handlers with usage aggregation | 9f8db66 | admin.go, admin_test.go |

## Key Design Decisions

1. **AllowedUpstreams as JSON string**: Stored as `""` (unrestricted) or JSON array string. Avoids a join table for a field that is read-only at relay time. `parseAllowedUpstreams()` decodes on read.

2. **PATCH via `map[string]any`**: GORM's `Updates(map)` only touches explicitly provided keys, enabling `token_budget=0` to correctly set budget to zero (unlimited) without GORM treating it as "unset". `Save()` is never used.

3. **Token exposure**: Raw token returned only on `POST /api/keys` (one-time view). All subsequent GET responses return `maskToken()` output (`first7***last3`).

4. **Full key in auth context**: `relay.go`'s `authMiddleware` now stores the full `AccessKey` struct under `"accessKey"` key in addition to `"keyID"`. Plan 02's `limitMiddleware` can read it without a second DB query.

5. **Expiry check in authMiddleware**: `ExpiresAt != nil && time.Now().UTC().After(key.ExpiresAt.UTC())` returns 401 with `"key expired"` before the handler runs.

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

| File | Function | Reason |
|------|----------|--------|
| internal/server/admin.go | handleRotateUpstream | Implemented in Plan 02 |
| internal/server/admin.go | handleListUpstreams | Implemented in Plan 02 |

These stubs return 501 and are intentional placeholders for Phase 05 Plan 02.

## Threat Mitigations Applied

| Threat ID | Applied |
|-----------|---------|
| T-5-01 | `crypto/subtle.ConstantTimeCompare` in `adminMiddleware` |
| T-5-02 | GORM parameterized queries via `ShouldBindJSON` typed structs + map updates |
| T-5-03 | `maskToken()` on all GET responses; raw token only on POST create |
| T-5-06 | UUID IDs, `[]string` type constraint on AllowedUpstreams |
| T-5-07 | `/api/*` uses `adminMiddleware`; `/v1/*` uses `authMiddleware` — no cross-contamination |

## Self-Check: PASSED

- internal/models/models.go contains `TokenBudget int64`: confirmed
- internal/server/admin.go contains `func (s *Server) adminMiddleware(`: confirmed
- internal/server/admin.go contains `subtle.ConstantTimeCompare`: confirmed
- internal/server/server.go contains `r.Group("/api")`: confirmed
- internal/server/relay.go contains `c.Set("accessKey", key)`: confirmed
- internal/server/relay.go contains `key expired`: confirmed
- Commits 0daa4de and 9f8db66 exist: confirmed
- `go test ./... -count=1` exits 0: confirmed
