---
phase: 10-rate-limit-portal
plan: 01
subsystem: api, ui
tags: [go, react, rate-limit, sync.Map, typescript, keytable]

# Dependency graph
requires:
  - phase: 09-rate-limit-backend
    provides: perDayCounters sync.Map, RateLimitPerDay field on AccessKey model, checkRate implementation
provides:
  - currentDayCount() helper reading perDayCounters with windowID staleness check
  - DayUsage int field in keyResponse struct and GET /api/keys JSON response
  - InjectDayCount/InjectDayCountStale test helpers in limit.go
  - Rate/min, Rate/day, Today columns in KeyTable portal UI
  - day_usage field in both TypeScript KeyResponse interfaces
affects: [11-docker-deployment]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "currentDayCount reads sync.Map under mutex with windowID staleness guard returning 0 for stale windows"
    - "Test helpers InjectDayCount/InjectDayCountStale for seeding in-memory counters in tests"
    - "Zero-value rate fields display as Unlimited in table cells following existing token_budget pattern"

key-files:
  created: []
  modified:
    - internal/server/limit.go
    - internal/server/admin.go
    - internal/server/admin_test.go
    - web/src/components/KeyTable.tsx
    - web/src/components/EditKeyDialog.tsx

key-decisions:
  - "Staleness check uses YearDay() not date comparison — simpler, sufficient for day-boundary detection"
  - "InjectDayCount/InjectDayCountStale are exported (not build-tag-gated) since they only write to package-internal sync.Map and have no production call sites"
  - "day_usage added to EditKeyDialog.tsx interface for consistency even though the dialog does not render it"

patterns-established:
  - "Pattern: rate counter read helpers in limit.go follow same mutex+windowID check pattern as checkRate"

requirements-completed: [RATE-05, RATE-06]

# Metrics
duration: 12min
completed: 2026-04-18
---

# Phase 10 Plan 01: Rate Limit Portal - Backend Field + Table Columns Summary

**day_usage field added to GET /api/keys via currentDayCount() helper; KeyTable gains Rate/min, Rate/day, Today columns with Unlimited display for zero-configured keys**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-04-18T06:54:00Z
- **Completed:** 2026-04-18T06:57:00Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Added `currentDayCount()` in `limit.go` reading `perDayCounters` sync.Map with mutex-protected windowID staleness check
- Added `DayUsage int` to `keyResponse` struct and populated it via `currentDayCount(key.ID)` in `toKeyResponse`
- Added three new Go tests (IncludesDayUsage, ActiveCounter, StaleWindow) — all pass with all existing tests
- Added Rate/min, Rate/day, Today columns to `KeyTable` with Unlimited display for zero values
- Added `day_usage: number` to both TypeScript `KeyResponse` interfaces for consistency

## Task Commits

Each task was committed atomically:

1. **Task 1: Add currentDayCount helper and DayUsage field to backend** - `6c29e38` (feat)
2. **Task 2: Add rate limit columns to KeyTable and day_usage to TypeScript interfaces** - `5a62f59` (feat)

## Files Created/Modified
- `internal/server/limit.go` - Added currentDayCount(), InjectDayCount(), InjectDayCountStale()
- `internal/server/admin.go` - Added DayUsage field to keyResponse struct, populated in toKeyResponse
- `internal/server/admin_test.go` - Added TestListKeys_IncludesDayUsage, TestListKeys_DayUsage_ActiveCounter, TestListKeys_DayUsage_StaleWindow
- `web/src/components/KeyTable.tsx` - Added day_usage to interface; added Rate/min, Rate/day, Today columns
- `web/src/components/EditKeyDialog.tsx` - Added day_usage to KeyResponse interface

## Decisions Made
- `InjectDayCount`/`InjectDayCountStale` are exported unconditionally (not behind a build tag) since they only touch the package-internal `perDayCounters` sync.Map and have no production call sites outside tests.
- Staleness check uses `YearDay()` comparison matching the same pattern already used in `checkRate`.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

`TestAnthropicPassthrough_NonStream` was already failing before this plan's changes (pre-existing: upstream path `/anthropic/v1/messages` instead of expected `/v1/messages`). Confirmed by running tests against the base commit. Out of scope for this plan — logged as deferred.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `GET /api/keys` now includes `day_usage` for all keys
- Portal KeyTable shows rate limit configuration and today's usage count at a glance
- Ready for Phase 11 (Docker Deployment) — no schema changes, binary bake-in unaffected

---
*Phase: 10-rate-limit-portal*
*Completed: 2026-04-18*
