---
phase: 09-rate-limit-backend
plan: 01
subsystem: server/limit
tags: [rate-limit, testing, bug-fix, openai-format]
dependency_graph:
  requires: []
  provides: [openai-compatible-429, per-day-test-coverage, rate-limit-field-fix]
  affects: [internal/server/limit.go, internal/server/admin_test.go, internal/server/e2e_test.go]
tech_stack:
  added: []
  patterns: [openai-error-format, exported-test-reset-functions]
key_files:
  created: []
  modified:
    - internal/server/limit.go
    - internal/server/admin_test.go
    - internal/server/e2e_test.go
decisions:
  - "OpenAI nested error format: gin.H{error: gin.H{message, type, code}} for all three 429 paths"
  - "No Retry-After header added per D-02"
  - "ResetPerDayCounters exported as test utility (not reachable via HTTP)"
metrics:
  duration: 8m
  completed: "2026-04-18"
  tasks: 2
  files: 3
---

# Phase 9 Plan 01: Rate Limit 429 Format, Per-Day Test, and e2e Fix Summary

OpenAI-format 429 responses in all three limitMiddleware paths, exported ResetPerDayCounters, new TestLimitMiddleware_RatePerDay test, and corrected rate_limit_per_minute field name in e2e test.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Update 429 error responses to OpenAI format and fix existing test assertions | 3c3498e | limit.go, admin_test.go |
| 2 | Add ResetPerDayCounters, per-day rate limit test, and fix e2e field name | 8c79b45 | limit.go, admin_test.go, e2e_test.go |

## What Was Built

**Task 1:** Replaced three flat-format `gin.H{"error": "..."}` responses in `limitMiddleware` with OpenAI-nested format:
```go
gin.H{"error": gin.H{"message": "...", "type": "requests", "code": "rate_limit_exceeded"}}
```
Updated `TestLimitMiddleware_TokenBudget` to assert the nested structure. Added body assertions to `TestLimitMiddleware_RatePerMinute` to validate the per-minute 429 path.

**Task 2:** Added `ResetPerDayCounters()` export to `limit.go` mirroring `ResetPerMinuteCounters()`. Updated `makeLimitTestKey` helper to accept `rpd int` parameter and wired `RateLimitPerDay` into the test key. Updated all four existing callers to pass `0` as the new argument. Added `TestLimitMiddleware_RatePerDay` which creates a key with `RateLimitPerDay=2`, sends 3 requests, asserts the 3rd returns 429 with OpenAI error format. Fixed `"rate_limit_per_min"` typo to `"rate_limit_per_minute"` in `TestE2E_Admin_CreateAndListKey`.

## Verification Results

```
go test ./internal/server/ -run "TestLimitMiddleware" -v  → all 5 tests PASS
go test ./internal/server/ -run "TestE2E_Admin_CreateAndListKey" -v  → PASS
grep -c "rate_limit_exceeded" internal/server/limit.go  → 3
grep "rate_limit_per_min\"" internal/server/e2e_test.go  → (no output, typo gone)
```

Pre-existing failures `TestAnthropicPassthrough_NonStream` and `TestE2E_Anthropic_Passthrough_FormatField` are out of scope — confirmed present before these changes via `git stash`.

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None.

## Threat Flags

None. No new network endpoints, auth paths, or schema changes introduced. `ResetPerDayCounters` is not wired to any HTTP route.

## Self-Check: PASSED

- `internal/server/limit.go` — exists, contains `func ResetPerDayCounters()` and 3x `rate_limit_exceeded`
- `internal/server/admin_test.go` — exists, contains `TestLimitMiddleware_RatePerDay` and updated `makeLimitTestKey` signature
- `internal/server/e2e_test.go` — exists, contains `rate_limit_per_minute` (not `rate_limit_per_min`)
- Commit `3c3498e` — exists (Task 1)
- Commit `8c79b45` — exists (Task 2)
