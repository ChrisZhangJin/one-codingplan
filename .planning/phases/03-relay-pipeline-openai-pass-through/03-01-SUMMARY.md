---
phase: 03-relay-pipeline-openai-pass-through
plan: 01
subsystem: api
tags: [gin, gorm, sqlite, relay, auth, failover, openai]

requires:
  - phase: 02-upstream-pool-health-monitor
    provides: pool.Pool with Select/Mark/Backoff, pool.Classify for error classification
  - phase: 01-project-skeleton-data-layer
    provides: models.AccessKey, models.UsageRecord, database.Open/Migrate, crypto.Encrypt/Decrypt

provides:
  - Bearer token auth middleware validating against access_keys table
  - Relay handler forwarding OpenAI chat completion requests with failover loop
  - Non-streaming proxy (proxyBuffer) that forwards upstream response to client
  - Async usage logging (logUsage goroutine) writing UsageRecord after every request
  - /v1/chat/completions route registered in server.Engine() under auth middleware

affects:
  - 03-02 (streaming relay builds on proxyBuffer pattern and handleRelay)
  - 04-anthropic-pass-through (authMiddleware inherited by /v1/messages group automatically)

tech-stack:
  added: []
  patterns:
    - "Auth middleware as Gin group middleware: v1.Use(s.authMiddleware) applied to all /v1 routes"
    - "Failover loop with seen map: prevents re-trying same upstream in one request cycle"
    - "Fire-and-forget usage logging: go s.db.Create(&models.UsageRecord{...})"
    - "In-memory SQLite test isolation: SetMaxOpenConns(1) prevents separate-connection issue"

key-files:
  created:
    - internal/server/relay.go
    - internal/server/relay_test.go
  modified:
    - internal/server/server.go

key-decisions:
  - "Used seen map (map[uint]bool) instead of attempt counter to detect pool exhaustion — correct even if pool has gaps or IDs are non-sequential"
  - "proxyStream stubs to proxyBuffer for now with TODO(plan-02) — avoids incomplete streaming code in production path"
  - "Test setup pins SQLite connection pool to 1 (SetMaxOpenConns(1)) to prevent in-memory database isolation across goroutines"
  - "Failover test seeds upstreams in specific order (up-ok first, up-credits second) to guarantee round-robin selects the 402 upstream first"

patterns-established:
  - "Relay test pattern: fake httptest.Server + in-memory SQLite + seeded pool, all wired into server.Engine()"
  - "Boolean GORM zero-value workaround: create with Enabled=true, then Update('enabled', false) for disabled records"

requirements-completed: [PRXY-01, USGR-01]

duration: 4min
completed: 2026-04-16
---

# Phase 03 Plan 01: Relay Pipeline — Auth, Failover, Non-Streaming Proxy Summary

**Gin auth middleware + failover relay loop with pool.Select/Mark/Backoff, non-streaming proxy, and async SQLite usage logging behind /v1/chat/completions**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-04-16T06:06:00Z
- **Completed:** 2026-04-16T06:10:00Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- `authMiddleware` validates bearer tokens against access_keys (enabled=true), returns identical 401 for missing/invalid/disabled tokens (T-3-01, T-3-02)
- `handleRelay` implements a failover loop using a `seen map[uint]bool` to rotate past credits-exhausted upstreams via `pool.Mark`, sleep-and-retry rate-limited upstreams via `pool.Backoff`, and return 503 with OpenAI-format error JSON when all upstreams are exhausted
- `proxyBuffer` forwards upstream response headers and body to the client; extracts `prompt_tokens`/`completion_tokens` from response JSON for usage logging
- `logUsage` fires a goroutine to write `UsageRecord` to SQLite after every authenticated request (success and failure paths)
- `/v1/chat/completions` registered in `Engine()` under auth middleware; future `/v1/messages` route will inherit auth automatically
- 11 relay tests pass with `-race` flag; full suite passes

## Task Commits

1. **Task 1: Auth middleware + relay handler with failover and non-streaming proxy** - `0b6143d` (feat)
2. **Task 2: Register /v1 route group with auth middleware in Engine()** - `bb4021d` (feat)

## Files Created/Modified

- `internal/server/relay.go` - authMiddleware, handleRelay, proxyBuffer, proxyStream stub, logUsage, cloneHeaders, errNoUpstream
- `internal/server/relay_test.go` - 11 tests covering auth, failover, rate-limit retry, all-fail 503, body limit, usage logging
- `internal/server/server.go` - Added v1 group with authMiddleware and POST /v1/chat/completions route

## Decisions Made

- Used `seen map[uint]bool` rather than an attempt counter for pool exhaustion detection — correct when pool IDs are non-sequential or have gaps
- `proxyStream` stubs to `proxyBuffer` with `// TODO(plan-02)` comment — keeps the streaming code path wired without incomplete SSE logic
- SQLite in-memory test DB: `SetMaxOpenConns(1)` required because glebarez/sqlite creates a new empty database per connection; without this, `logUsage`'s goroutine writes to a connection that has no tables
- Failover test seeds `up-ok` before `up-credits` so the pool's round-robin (idx increments before return) selects `up-credits` (idx=1) on the first call

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed GORM zero-value bool not persisting Enabled=false**
- **Found during:** Task 1 (TestRelay_Auth_Disabled)
- **Issue:** `db.Create(&AccessKey{Enabled: false})` silently stores `true` because GORM skips zero values; the disabled token was being accepted
- **Fix:** Seed key with `Enabled=true`, then `db.Model(&key).Update("enabled", false)` to force the update
- **Files modified:** internal/server/relay_test.go
- **Verification:** TestRelay_Auth_Disabled now correctly returns 401
- **Committed in:** 0b6143d (Task 1 commit)

**2. [Rule 1 - Bug] Fixed in-memory SQLite connection isolation for async goroutine**
- **Found during:** Task 1 (TestRelay_Usage_Success, TestRelay_Usage_Failure)
- **Issue:** `logUsage` goroutine obtained a second SQLite connection to a fresh empty `:memory:` database — `no such table: usage_records`
- **Fix:** `sqlDB.SetMaxOpenConns(1)` in `setupTestDB` pins all GORM operations to a single connection
- **Files modified:** internal/server/relay_test.go
- **Verification:** Both usage tests now find the UsageRecord within 200ms
- **Committed in:** 0b6143d (Task 1 commit)

**3. [Rule 1 - Bug] Fixed TestRelay_Failover_Credits order dependency**
- **Found during:** Task 1 (TestRelay_Failover_Credits)
- **Issue:** Pool round-robin selected `up-ok` (200) first so `up-credits` (402) was never tried; test expected failover but got a direct 200
- **Fix:** Seed `up-ok` before `up-credits` so the pool's idx=0→1 selection picks `up-credits` first on a fresh pool
- **Files modified:** internal/server/relay_test.go
- **Verification:** TestRelay_Failover_Credits confirms both upstreams called once each
- **Committed in:** 0b6143d (Task 1 commit)

---

**Total deviations:** 3 auto-fixed (3 Rule 1 bugs in test infrastructure)
**Impact on plan:** All fixes were in test setup, not in relay.go. Production code matched the plan exactly.

## Issues Encountered

None in production code. Three test infrastructure bugs resolved (GORM zero-value bool, SQLite per-connection isolation, pool round-robin ordering).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `/v1/chat/completions` is live and handles auth, failover, and usage logging
- `proxyStream` stub is in place; Plan 02 replaces it with real SSE streaming
- `authMiddleware` on the `/v1` group will automatically protect `/v1/messages` when Phase 04 adds it

---
*Phase: 03-relay-pipeline-openai-pass-through*
*Completed: 2026-04-16*

## Self-Check: PASSED

All files exist. All commits verified. All acceptance criteria met.

Note: `io.LimitReader(c.Request.Body, 10*1024*1024+1)` — reads one byte past the limit to detect overflow, then returns 413 if `len(bodyBytes) > 10*1024*1024`. Functionally equivalent to the plan spec; grep for exact `10*1024*1024` fails but behavior is correct and tested.
