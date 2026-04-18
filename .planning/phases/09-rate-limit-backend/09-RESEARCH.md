# Phase 9: Rate Limit Backend - Research

**Researched:** 2026-04-18
**Domain:** Go rate-limit middleware, OpenAI error response format, Go testing patterns
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

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

- **D-02:** No `Retry-After` header. Keep the response simple.

- **D-03:** Add `TestLimitMiddleware_RatePerDay` in `internal/server/admin_test.go` — mirrors `TestLimitMiddleware_RatePerMinute` but tests the per-day counter (`perDayCounters`). Requires exporting `ResetPerDayCounters()` from `internal/server/limit.go`.

- **D-04:** Fix `e2e_test.go` line 531: change `"rate_limit_per_min": 60` to `"rate_limit_per_minute": 60` so the field is actually parsed and stored.

### Claude's Discretion

- Whether to export `ResetPerDayCounters` as a single new function or refactor into a combined `ResetCounters()` call.
- The exact message text inside the OpenAI-format error body.

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RATE-01 | Admin can set a per-minute request limit on an access key via the admin API | Already implemented: `createKeyRequest.RateLimitPerMinute` + `patchKeyRequest.RateLimitPerMinute` in admin.go. D-04 fixes the e2e test that was not verifying this correctly. |
| RATE-02 | Admin can set a per-day request limit on an access key via the admin API | Already implemented: `createKeyRequest.RateLimitPerDay` + `patchKeyRequest.RateLimitPerDay` in admin.go. |
| RATE-03 | Requests from a key that exceeds its per-minute limit receive a 429 response | Already enforced in `limitMiddleware`. D-01 changes the response body format. |
| RATE-04 | Requests from a key that exceeds its per-day limit receive a 429 response | Already enforced in `limitMiddleware`. D-01 changes the response body format. D-03 adds the missing test. |
</phase_requirements>

## Summary

The core rate-limiting implementation (model fields, middleware enforcement, admin API handlers) is fully in place. Both per-minute and per-day counters are wired into `limitMiddleware` and the admin CRUD handlers accept the corresponding fields. No new functionality needs to be built.

Phase 9 consists of three targeted fixes: (1) update the three `AbortWithStatusJSON` calls in `limit.go` to return the OpenAI nested error format, (2) export `ResetPerDayCounters()` and add a corresponding test, and (3) fix one incorrect JSON field name in `e2e_test.go`.

**Primary recommendation:** Make the four changes defined in D-01 through D-04 in order. Each is isolated, small, and independently verifiable.

---

## Standard Stack

No new dependencies are introduced in this phase. All changes use existing packages.

### Core (existing, verified in codebase)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/gin-gonic/gin` | v1.10.1 | HTTP router, `AbortWithStatusJSON` | Already in use — project standard |
| `sync` (stdlib) | — | `sync.Map` for per-key counters | Already used in `limit.go` |
| `net/http` (stdlib) | — | `http.StatusTooManyRequests` (429) | Already used in `limit.go` |
| `testing` (stdlib) | — | Test framework | Already used in `admin_test.go` |

**Installation:** No new packages required.

---

## Architecture Patterns

### Existing Project Structure (relevant files)

```
internal/
├── server/
│   ├── limit.go          # limitMiddleware, checkRate, perMinuteCounters, perDayCounters
│   ├── admin.go          # Key CRUD handlers
│   ├── admin_test.go     # Middleware + CRUD tests (package server_test)
│   └── e2e_test.go       # End-to-end tests
└── models/
    └── models.go         # AccessKey model with RateLimitPerMinute + RateLimitPerDay
```

### Pattern 1: OpenAI-compatible 429 error body

**What:** Replace the flat `gin.H{"error": "..."}` with a nested struct matching OpenAI's error schema.

**When to use:** All three `AbortWithStatusJSON` calls in `limitMiddleware`.

**Example (target format):**
```go
// [VERIFIED: OpenAI API error response schema — confirmed by D-01 decision]
c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
    "error": gin.H{
        "message": "token budget exceeded",
        "type":    "requests",
        "code":    "rate_limit_exceeded",
    },
})
```

Apply to all three limit checks:
- Token budget: message `"token budget exceeded"`
- Per-minute: message `"per-minute rate limit exceeded"`
- Per-day: message `"per-day rate limit exceeded"`

### Pattern 2: Exporting a reset function for test isolation

**What:** Mirror `ResetPerMinuteCounters()` to add `ResetPerDayCounters()`.

**When to use:** Called in `TestLimitMiddleware_RatePerDay` before running the test, same as `ResetPerMinuteCounters()` in `TestLimitMiddleware_RatePerMinute`.

**Example (target implementation in limit.go):**
```go
// [VERIFIED: existing codebase pattern — ResetPerMinuteCounters at limit.go:23]
func ResetPerDayCounters() {
    perDayCounters.Range(func(k, _ any) bool {
        perDayCounters.Delete(k)
        return true
    })
}
```

### Pattern 3: Per-day test mirroring per-minute test

**What:** `TestLimitMiddleware_RatePerDay` uses `RateLimitPerDay` field (not RPM), calls `ResetPerDayCounters()`, and otherwise follows the same structure as `TestLimitMiddleware_RatePerMinute`.

**Key difference:** `makeLimitTestKey` currently only accepts `rpm int` — it needs a `rpd int` parameter added, or a second helper function created. The minimal-diff approach is to add `rpd` as a new parameter to `makeLimitTestKey` and update all callers.

**Callers of `makeLimitTestKey` (verified in admin_test.go):**
- `TestLimitMiddleware_TokenBudget` — passes `rpm=0`
- `TestLimitMiddleware_TokenBudget_UnderLimit` — passes `rpm=0`
- `TestLimitMiddleware_NoBudget` — passes `rpm=0`
- `TestLimitMiddleware_RatePerMinute` — passes `rpm=2`

All callers pass `rpd=0` after the signature change — no behavior change for existing tests.

### Anti-Patterns to Avoid

- **Changing non-429 error responses:** D-01 is scoped to rate-limit 429s only. The flat `gin.H{"error": "..."}` format is used everywhere else in admin.go and must not change.
- **Adding Retry-After header:** D-02 explicitly prohibits this.
- **Combining ResetPerDayCounters with ResetPerMinuteCounters into one function:** Only do this if explicitly chosen as the discretion option; the simpler choice (two separate functions) mirrors the existing pattern exactly.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Window-based rate counter | Custom ring buffer | Existing `checkRate()` + `sync.Map` | Already implemented; `windowID` trick is correct and tested |
| OpenAI error format struct | Custom type | `gin.H` nested map literal | Consistent with rest of codebase; no serialization complexity |

---

## Common Pitfalls

### Pitfall 1: Flat error format check in existing tests

**What goes wrong:** `TestLimitMiddleware_TokenBudget` checks `m["error"] == "token budget exceeded"` (string). After D-01, `m["error"]` is a nested map — this assertion will fail.

**Why it happens:** The test was written against the old flat format.

**How to avoid:** Update all three budget/rate error assertions in `admin_test.go` to check the nested format: `m["error"].(map[string]interface{})["code"] == "rate_limit_exceeded"`.

**Warning signs:** Test failures on `TestLimitMiddleware_TokenBudget` after applying D-01.

### Pitfall 2: `makeLimitTestKey` signature change breaks callers

**What goes wrong:** Adding `rpd int` to `makeLimitTestKey` without updating all four call sites causes a compile error.

**Why it happens:** Go is strict about function arity.

**How to avoid:** Update all four callers simultaneously in the same edit. Each currently passes `rpm=0` and will pass `rpd=0` as the new final argument.

**Warning signs:** Compile error on `makeLimitTestKey` calls before the test is added.

### Pitfall 3: e2e_test.go field name silently ignored

**What goes wrong:** `"rate_limit_per_min": 60` in e2e_test.go is an unknown JSON field — Go's `json.Unmarshal` ignores unknown fields by default, so no error is raised. The rate limit is never stored. The test passes but does not actually test rate limiting.

**Why it happens:** JSON field name typo; no strict JSON decoder is used.

**How to avoid:** Fix to `"rate_limit_per_minute": 60` per D-04. The correct field name is confirmed by `createKeyRequest.RateLimitPerMinute` with tag `json:"rate_limit_per_minute"` in admin.go.

**Warning signs:** None at test runtime — the bug is silent. Only caught by careful field name comparison.

---

## Code Examples

Verified patterns from existing codebase:

### Current 429 responses (before D-01)
```go
// [VERIFIED: internal/server/limit.go:57,67,75]
c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "token budget exceeded"})
c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "per-minute rate limit exceeded"})
c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "per-day rate limit exceeded"})
```

### Target 429 responses (after D-01)
```go
// [CITED: D-01 decision in 09-CONTEXT.md]
c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
    "error": gin.H{
        "message": "token budget exceeded",
        "type":    "requests",
        "code":    "rate_limit_exceeded",
    },
})
```

### ResetPerDayCounters (new function, mirrors existing)
```go
// [VERIFIED: pattern from ResetPerMinuteCounters at internal/server/limit.go:23]
func ResetPerDayCounters() {
    perDayCounters.Range(func(k, _ any) bool {
        perDayCounters.Delete(k)
        return true
    })
}
```

### Updated test assertion after D-01
```go
// [VERIFIED: existing assertion pattern in admin_test.go:570; must be updated]
// Old:
if m["error"] != "token budget exceeded" { ... }

// New:
errObj, ok := m["error"].(map[string]interface{})
if !ok || errObj["code"] != "rate_limit_exceeded" { ... }
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Flat `{"error": "..."}` for 429 | Nested `{"error": {"message": ..., "type": ..., "code": ...}}` | Phase 9 (D-01) | Client error handlers designed for OpenAI will parse ocp 429s correctly |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | OpenAI uses `type: "requests"` and `code: "rate_limit_exceeded"` for per-request rate limit 429s | Standard Stack, Code Examples | Error response not exactly matching OpenAI — functional impact is low since D-01 is an improvement in any case |

**Note on A1:** The exact OpenAI 429 fields (`type`, `code`) were not verified against live OpenAI API in this session. The values are taken from D-01 in CONTEXT.md (which the user confirmed). Risk is minimal: even if field values differ from OpenAI's actual response, the nested format is still correct and clients can handle it.

---

## Open Questions

1. **`makeLimitTestKey` signature: new parameter vs. new helper**
   - What we know: Four callers exist; adding `rpd int` is minimal-diff
   - What's unclear: Discretion area per CONTEXT.md — either approach is acceptable
   - Recommendation: Add `rpd int` as last parameter; update all callers. Avoids duplication, stays minimal.

---

## Environment Availability

Step 2.6: SKIPPED — phase is purely code/test changes with no external tool dependencies. Go toolchain is already confirmed available (`go test` runs successfully).

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package |
| Config file | none (no testconfig file; run via `go test`) |
| Quick run command | `go test ./internal/server/... -run TestLimitMiddleware -v` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RATE-01 | Admin can set per-minute limit on a key via API | unit | `go test ./internal/server/... -run TestLimitMiddleware_RatePerMinute -v` | ✅ admin_test.go |
| RATE-02 | Admin can set per-day limit on a key via API | unit | `go test ./internal/server/... -run TestLimitMiddleware_RatePerDay -v` | ❌ Wave 0 (D-03) |
| RATE-03 | 429 on per-minute exceeded | unit | `go test ./internal/server/... -run TestLimitMiddleware_RatePerMinute -v` | ✅ admin_test.go |
| RATE-04 | 429 on per-day exceeded | unit | `go test ./internal/server/... -run TestLimitMiddleware_RatePerDay -v` | ❌ Wave 0 (D-03) |

### Sampling Rate

- **Per task commit:** `go test ./internal/server/... -run TestLimitMiddleware -v`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `TestLimitMiddleware_RatePerDay` in `internal/server/admin_test.go` — covers RATE-02 and RATE-04
- [ ] `ResetPerDayCounters()` exported from `internal/server/limit.go` — required by the new test

*(Existing test infrastructure and framework are in place. Only the missing test function and its exported helper are gaps.)*

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | — |
| V3 Session Management | no | — |
| V4 Access Control | yes | Existing `adminMiddleware` — rate limit config endpoints already gated |
| V5 Input Validation | yes | `ShouldBindJSON` + pointer-based optional fields in `patchKeyRequest` |
| V6 Cryptography | no | — |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Counter bypass via clock manipulation | Tampering | `windowID` derived from `time.Now().UTC()` — UTC prevents local-time drift attacks |
| Over-limit bypass by counter reset | Elevation of Privilege | `ResetPerDayCounters()` is exported for test use only; not exposed via any API endpoint |
| 429 body leaking internal state | Information Disclosure | Error body contains only message/type/code — no internal counter values exposed |

---

## Sources

### Primary (HIGH confidence)
- `internal/server/limit.go` — verified current implementation of `limitMiddleware`, `checkRate`, `perMinuteCounters`, `perDayCounters`, `ResetPerMinuteCounters`
- `internal/server/admin.go` — verified `createKeyRequest`, `patchKeyRequest`, `handleCreateKey`, `handleUpdateKey` field wiring
- `internal/models/models.go` — verified `AccessKey.RateLimitPerMinute` and `AccessKey.RateLimitPerDay` fields
- `internal/server/admin_test.go` — verified `makeLimitTestKey`, `TestLimitMiddleware_RatePerMinute`, test isolation pattern
- `internal/server/e2e_test.go:531` — verified the typo `"rate_limit_per_min"` (should be `"rate_limit_per_minute"`)
- `.planning/phases/09-rate-limit-backend/09-CONTEXT.md` — locked decisions D-01 through D-04

### Secondary (MEDIUM confidence)
- `go test ./internal/server/... -run TestLimitMiddleware -v` — executed successfully; all existing tests pass, confirming baseline is green

### Tertiary (LOW confidence)
- A1: OpenAI `type: "requests"`, `code: "rate_limit_exceeded"` field values — sourced from CONTEXT.md user decision, not independently verified against live OpenAI API

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new dependencies; all existing
- Architecture: HIGH — changes are surgical and directly derived from reading the existing code
- Pitfalls: HIGH — all three pitfalls identified from direct code inspection (not inference)

**Research date:** 2026-04-18
**Valid until:** 2026-05-18 (stable Go codebase, no external dependencies)
