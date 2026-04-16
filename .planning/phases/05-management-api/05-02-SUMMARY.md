---
phase: 05-management-api
plan: 02
subsystem: pool/server
tags: [pool, middleware, rate-limit, token-budget, upstream-routing, admin]
dependency_graph:
  requires:
    - "05-01 (AccessKey model with limit fields, authMiddleware storing full key in context)"
    - "03-relay (pool.Select, handleRelay, handleAnthropicRelay)"
  provides:
    - "Pool.Select with per-key upstream filtering (allowedUpstreams []string)"
    - "Pool.ForceRotate() for POST /api/upstreams/rotate"
    - "Pool.List() returning UpstreamInfo without API keys"
    - "limitMiddleware enforcing token budget and per-minute/per-day rate limits"
    - "Fully implemented handleRotateUpstream and handleListUpstreams"
  affects:
    - "internal/pool/pool.go (Select signature changed, ForceRotate/List/NewForTest added)"
    - "internal/server/relay.go (Select call updated to pass allowedUpstreams)"
    - "internal/server/anthropic.go (Select call updated, models import added)"
    - "internal/server/server.go (limitMiddleware wired into v1 group)"
tech_stack:
  added:
    - "sync.Map for in-process per-minute and per-day rate counters"
  patterns:
    - "TDD: RED (failing tests) committed, then GREEN implementation"
    - "Pool.Select filter uses map[string]bool for O(1) upstream name lookup"
    - "limitMiddleware reads full AccessKey from Gin context (set by authMiddleware) to avoid second DB query"
    - "ResetPerMinuteCounters() exported for test isolation of rate limit state"
key_files:
  created:
    - path: "internal/server/limit.go"
      purpose: "limitMiddleware with token budget (DB query), per-minute counter, per-day counter; ResetPerMinuteCounters for tests"
  modified:
    - path: "internal/pool/pool.go"
      change: "Select signature changed to []string, ForceRotate added, UpstreamInfo type + List added, NewForTest added"
    - path: "internal/pool/pool_test.go"
      change: "All Select('') calls updated to Select(nil); TestForceRotate, TestForceRotate_AllUnavailable, TestSelectWithFilter_*, TestList added"
    - path: "internal/pool/probe_test.go"
      change: "All Select('') calls updated to Select(nil)"
    - path: "internal/server/relay.go"
      change: "handleRelay extracts accessKey from context, computes allowedUpstreams, passes to pool.Select"
    - path: "internal/server/anthropic.go"
      change: "handleAnthropicRelay same as relay.go; models import added"
    - path: "internal/server/server.go"
      change: "v1.Use(s.limitMiddleware) added after authMiddleware"
    - path: "internal/server/admin.go"
      change: "handleRotateUpstream and handleListUpstreams replaced 501 stubs with real implementations"
    - path: "internal/server/admin_test.go"
      change: "Added setupAdminTestWithPool helper; TestRotateUpstream, TestRotateUpstream_NoUpstreams, TestListUpstreams, TestLimitMiddleware_* tests"
    - path: "internal/server/relay_test.go"
      change: "One Select('key-1') call updated to Select(nil)"
decisions:
  - "parseAllowedUpstreams reused from admin.go (same package) rather than duplicated in relay.go"
  - "ResetPerMinuteCounters exported to allow test isolation without package-level test setup/teardown complexity"
  - "NewForTest added to pool package to enable server tests without DB for rotate/list handlers"
  - "limitMiddleware token budget uses COALESCE(SUM) raw query matching existing usageTotals pattern"
metrics:
  duration: "~5 minutes"
  completed: "2026-04-16"
  tasks_completed: 2
  files_modified: 9
---

# Phase 05 Plan 02: Limit Enforcement, Pool Extensions, Admin Endpoint Wiring Summary

One-liner: Pool.Select extended for per-key upstream filtering, ForceRotate/List added, limitMiddleware enforcing token budget and rate limits, and upstream admin endpoints fully wired.

## What Was Built

Changed `Pool.Select` signature from `Select(keyID string)` to `Select(allowedUpstreams []string)` — enabling per-key routing restrictions. Added `ForceRotate()` which advances the round-robin cursor to the next available upstream (used by `POST /api/upstreams/rotate`). Added `UpstreamInfo` struct and `List()` which returns all pool entries without API keys (used by `GET /api/upstreams`). Added `NewForTest` constructor for use in tests without a database.

Created `internal/server/limit.go` with `limitMiddleware` that enforces three constraints before allowing relay: (1) token budget — queries cumulative `input_tokens + output_tokens` from `usage_records` for the key, rejects with 429 if at or above budget; (2) per-minute rate limit — in-process `sync.Map` counter keyed by `keyID + minuteWindow`; (3) per-day rate limit — same pattern with `YearDay()` window. The middleware reads the full `AccessKey` from Gin context (set by `authMiddleware` in Plan 01) without a second DB query.

Updated `relay.go` and `anthropic.go` to extract `allowedUpstreams` from the context key and pass to `pool.Select`. Replaced the 501 stub handlers for `handleRotateUpstream` and `handleListUpstreams` with real implementations. All call sites for `pool.Select("")` across pool tests, probe tests, and relay tests updated to `pool.Select(nil)`.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Pool extensions + limitMiddleware | 04caabd | pool.go, pool_test.go, probe_test.go, relay.go, anthropic.go, server.go, limit.go, admin_test.go, relay_test.go |
| 2 | Wire rotate and upstream-list handlers | 61561cf | pool.go, admin.go, admin_test.go |

## Key Design Decisions

1. **parseAllowedUpstreams reuse**: The function already existed in `admin.go` (same `server` package). Rather than duplicating it in `relay.go` as the plan suggested, `relay.go` and `anthropic.go` call it directly. No code change needed to admin.go.

2. **ResetPerMinuteCounters exported**: Rate limit counters are package-level `sync.Map` variables. To prevent test interference between `TestLimitMiddleware_RatePerMinute` runs (which can bleed across parallel test invocations), `ResetPerMinuteCounters()` is exported. Tests call it before asserting rate-limit behavior.

3. **NewForTest without DB**: The `TestRotateUpstream` and `TestListUpstreams` tests need a pool with known entries. Adding `NewForTest(entries []UpstreamEntry) *Pool` to the pool package avoids DB setup in those tests and documents the intended test pattern.

4. **limitMiddleware token budget query**: Uses the same `COALESCE(SUM(...))` pattern as `usageTotals()` in `admin.go`. Threat T-5-08 (DB query per request) is accepted at personal scale — `key_id` is indexed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] probe_test.go and relay_test.go used old Select signature**
- **Found during:** Task 1 GREEN phase
- **Issue:** `probe_test.go` had 5 `Select("")` calls and `relay_test.go` had 1 — both failed to compile after the signature change
- **Fix:** Updated all occurrences to `Select(nil)` using replace_all for probe_test.go
- **Files modified:** internal/pool/probe_test.go, internal/server/relay_test.go
- **Commit:** 04caabd

**2. [Rule 2 - Missing import] anthropic.go missing models import**
- **Found during:** Task 1, updating anthropic.go to use `models.AccessKey`
- **Fix:** Added `one-codingplan/internal/models` import to anthropic.go imports block
- **Files modified:** internal/server/anthropic.go
- **Commit:** 04caabd

## Threat Mitigations Applied

| Threat ID | Applied |
|-----------|---------|
| T-5-09 | Pool.Select filter applied against DB-stored JSON (not user input); exact string match only |
| T-5-10 | Pool.List() returns UpstreamInfo — no APIKey field; verified by TestListUpstreams assertion |
| T-5-11 | handleRotateUpstream protected by adminMiddleware; ForceRotate uses modulo bounds |
| T-5-12 | Documented: in-memory counters reset on restart; acceptable for personal proxy |

## Known Stubs

None — all stubs from Plan 01 resolved.

## Self-Check: PASSED

- internal/pool/pool.go contains `func (p *Pool) Select(allowedUpstreams []string)`: confirmed
- internal/pool/pool.go contains `func (p *Pool) ForceRotate()`: confirmed
- internal/pool/pool.go contains `func (p *Pool) List() []UpstreamInfo`: confirmed
- internal/pool/pool.go contains `type UpstreamInfo struct`: confirmed
- internal/server/limit.go contains `func (s *Server) limitMiddleware(`: confirmed
- internal/server/limit.go contains `token budget exceeded`: confirmed
- internal/server/limit.go contains `per-minute rate limit exceeded`: confirmed
- internal/server/server.go contains `v1.Use(s.limitMiddleware)`: confirmed
- internal/server/relay.go contains `s.pool.Select(allowedUpstreams)`: confirmed
- internal/server/anthropic.go contains `s.pool.Select(allowedUpstreams)`: confirmed
- internal/server/admin.go contains `s.pool.ForceRotate()`: confirmed
- internal/server/admin.go contains `s.pool.List()`: confirmed
- Commits 04caabd and 61561cf exist: confirmed
- `go test ./... -race -count=1` exits 0: confirmed
