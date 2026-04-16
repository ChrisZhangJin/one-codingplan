---
phase: 02-upstream-pool-health-monitor
plan: 01
subsystem: api
tags: [go, pool, round-robin, health, classifier, sqlite, gorm, concurrency]

requires:
  - phase: 01-project-skeleton
    provides: internal/models/models.go (Upstream struct, DecryptAPIKey), internal/crypto/crypto.go (AES-GCM), internal/database/database.go (Open, Migrate)

provides:
  - internal/pool/pool.go: Pool struct with New, Select, Mark, Stop
  - internal/pool/classifier.go: Classify function with per-provider keyword map
  - internal/pool/pool_test.go: 7 unit tests for pool (round-robin, availability, concurrent access)
  - internal/pool/classifier_test.go: 9 table-driven + 5 named classifier fixture tests

affects: [03-relay, proxy-routing, upstream-failover]

tech-stack:
  added: []
  patterns:
    - "sync.RWMutex wrapping mutable slice + index; Select takes write lock (mutates idx)"
    - "Body keyword check before HTTP status code in classifier (GLM/Qwen 429 credits-exhaustion pitfall)"
    - "providerCreditsKeywords map overrides defaultCreditsKeywords per provider slug"
    - "GORM boolean false insert: Create then Model.Update('enabled', false) due to default:true tag"

key-files:
  created:
    - internal/pool/pool.go
    - internal/pool/classifier.go
    - internal/pool/pool_test.go
    - internal/pool/classifier_test.go
  modified: []

key-decisions:
  - "Select takes full write lock (not RLock) because it mutates p.idx — consistent with RESEARCH.md recommendation"
  - "Config.RateLimitBackoff stored as time.Duration in Pool.cfg for Plan 02 probe loop use"
  - "StopCh exposed via stopCh field; Stop() idempotent via sync.Once"
  - "providerCreditsKeywords for glm and minimax use narrow overrides (1113/1008) to prevent false positives"
  - "Body keyword scan precedes 429 rule — critical for GLM (1113) and Qwen (insufficient_quota) correct classification"

patterns-established:
  - "Pool injection: construct pool before server.New(), pass as constructor arg"
  - "Classifier: Classify(provider, status, body) pure function, no state"

requirements-completed: [UPST-02, ROUT-01, ROUT-02, ROUT-03]

duration: 6min
completed: 2026-04-16
---

# Phase 02 Plan 01: Upstream Pool and Per-Provider Error Classifier Summary

**In-memory upstream pool with round-robin Select/Mark/Stop, backed by SQLite, and a body-first per-provider error classifier distinguishing credits-exhausted from rate-limited from transient**

## Performance

- **Duration:** 6 min
- **Started:** 2026-04-16T04:43:34Z
- **Completed:** 2026-04-16T04:48:49Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Pool loads enabled upstreams from SQLite at startup via GORM, decrypts API keys once with AES-GCM
- Pool.Select returns upstreams round-robin skipping unavailable entries; race-safe under 100-goroutine concurrent load
- Classifier correctly distinguishes credits-exhausted for all 5 target providers using body-first keyword strategy (precedes 429 rule)

## Task Commits

Each task was committed atomically:

1. **Task 1: Pool struct with Select, Mark, Stop and round-robin tests** - `d3d8be1` (feat)
2. **Task 2: Per-provider error classifier with fixture tests** - `f4c53a2` (feat)

## Files Created/Modified

- `internal/pool/pool.go` - Pool struct, New (loads from DB), Select (round-robin), Mark (availability toggle), Stop (idempotent shutdown)
- `internal/pool/classifier.go` - Classify function, ErrorClass type, defaultCreditsKeywords, providerCreditsKeywords overrides for glm/minimax
- `internal/pool/pool_test.go` - 7 unit tests: RoundRobin, SkipsUnavailable, NoUpstreams, Mark_Available, Concurrent, LoadsFromDB, DecryptsKeys
- `internal/pool/classifier_test.go` - 9 table-driven + 5 named fixture tests covering all 5 providers

## Decisions Made

- Body keyword check precedes 429 status rule in Classify — required because GLM and Qwen use HTTP 429 for credits exhaustion (RESEARCH.md Pitfalls 2 and 4)
- providerCreditsKeywords for "glm" uses narrow override `{"1113", "insufficient balance"}` so GLM pure rate limits (code 1301) correctly return ClassRateLimited
- Select uses full write lock (not RLock) since it mutates p.idx — simpler and correct per RESEARCH.md Pattern 1 note
- Stop() uses sync.Once for idempotent channel close rather than select-default pattern

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed GORM boolean false insert for disabled upstream in TestNew_LoadsFromDB**
- **Found during:** Task 1 (TestNew_LoadsFromDB verification)
- **Issue:** GORM ignores `false` for a bool column with `gorm:"default:true"` tag — `Create(&Upstream{Enabled: false})` inserted `true` because false is the zero value. Test asserted z (disabled) was absent from pool but it appeared.
- **Fix:** Changed test to `db.Create(&zUp)` then `db.Model(&zUp).Update("enabled", false)` to force the column to false after insert. This is idiomatic for GORM when you need to write a zero bool value.
- **Files modified:** internal/pool/pool_test.go
- **Verification:** TestNew_LoadsFromDB passes; disabled upstream z no longer appears in pool
- **Committed in:** d3d8be1 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Fix was necessary for test correctness. No scope creep.

## Issues Encountered

GORM `default:true` tag on `Upstream.Enabled` causes `Create` to ignore boolean false (zero value). Workaround: Create then Update. This pattern should be documented as a convention for future plans inserting disabled upstreams in tests.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `pool.Pool` is ready for integration in Plan 02 (probe loop) and Phase 3 (relay)
- `pool.Classify` is ready for consumption by Phase 3 relay on upstream error responses
- Phase 3 relay calls: `pool.Select(keyID)` to get next upstream, `pool.Mark(id, false)` on credits-exhausted classification
- Caller (Phase 3 relay) must use `io.LimitReader(resp.Body, 64*1024)` before passing body to Classify (T-02-01 mitigation — documented in RESEARCH.md, enforced in Phase 3)

## Self-Check: PASSED

- FOUND: internal/pool/pool.go
- FOUND: internal/pool/classifier.go
- FOUND: internal/pool/pool_test.go
- FOUND: internal/pool/classifier_test.go
- FOUND: .planning/phases/02-upstream-pool-health-monitor/02-01-SUMMARY.md
- FOUND commit: d3d8be1
- FOUND commit: f4c53a2
- `go test -race -count=1 ./internal/pool/...` exits 0

---
*Phase: 02-upstream-pool-health-monitor*
*Completed: 2026-04-16*
