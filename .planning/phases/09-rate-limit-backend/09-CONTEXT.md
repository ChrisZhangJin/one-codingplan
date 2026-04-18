# Phase 9: Rate Limit Backend - Context

**Gathered:** 2026-04-18
**Status:** Ready for planning

<domain>
## Phase Boundary

Ensure per-minute and per-day request caps are correctly enforced on access keys — admin can configure limits via the admin API, and the middleware rejects over-limit requests with 429 responses.

**Important finding:** The core backend implementation is already in place (model fields, middleware, admin API handlers, CLI display). Phase 9 work is: update the 429 error response to OpenAI format, add missing per-day enforcement test, and fix the e2e test field name mismatch.

</domain>

<decisions>
## Implementation Decisions

### 429 Error Response Format

- **D-01:** Change the 429 error body from the current flat format to OpenAI-compatible format:
  ```json
  {
    "error": {
      "message": "per-minute rate limit exceeded",
      "type": "requests",
      "code": "rate_limit_exceeded"
    }
  }
  ```
  Apply to all three limit types: token budget, per-minute, and per-day.

- **D-02:** No `Retry-After` header. Keep the response simple — no header is needed.

### Test Gaps to Fix

- **D-03:** Add `TestLimitMiddleware_RatePerDay` in `internal/server/admin_test.go` — mirrors `TestLimitMiddleware_RatePerMinute` but tests the per-day counter (`perDayCounters`). Requires exporting `ResetPerDayCounters()` from `internal/server/limit.go`.

- **D-04:** Fix `e2e_test.go` line ~531: change `"rate_limit_per_min": 60` to `"rate_limit_per_minute": 60` so the field is actually parsed and stored.

### Claude's Discretion

- Whether to export `ResetPerDayCounters` as a single new function or refactor into a combined `ResetCounters()` call — implementation detail.
- The message text inside the OpenAI-format error body (exact wording).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements
- `.planning/REQUIREMENTS.md` — RATE-01 through RATE-04 are the acceptance criteria for this phase

### Existing Implementation (read before touching)
- `internal/server/limit.go` — current middleware; per-minute + per-day enforcement logic
- `internal/server/admin.go` — create/update/list key handlers with rate limit fields
- `internal/models/models.go` — AccessKey model with RateLimitPerMinute + RateLimitPerDay fields
- `internal/server/admin_test.go` — existing limit middleware tests (token budget, per-minute)
- `internal/server/e2e_test.go` — e2e test with the field name bug (line ~531)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `checkRate()` in `limit.go` — generic counter function used for both per-minute and per-day
- `ResetPerMinuteCounters()` in `limit.go` — pattern to follow for `ResetPerDayCounters()`
- `makeLimitTestKey()` in `admin_test.go` — test helper for creating keys with limits; needs a `rpd` parameter added

### Established Patterns
- All other error responses in the codebase use flat `gin.H{"error": "..."}` format — D-01 introduces OpenAI nested format specifically for 429 responses
- Test isolation: each middleware test calls `ResetPerMinuteCounters()` before the test — same pattern needed for per-day

### Integration Points
- `limitMiddleware` is already wired at `internal/server/server.go:42` (`v1.Use(s.limitMiddleware)`)
- No schema migration needed — model fields already exist and are in the DB

</code_context>

<specifics>
## Specific Ideas

- The OpenAI error format should match exactly what OpenAI returns for 429s so clients that handle OpenAI errors natively also handle ocp errors.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 09-rate-limit-backend*
*Context gathered: 2026-04-18*
