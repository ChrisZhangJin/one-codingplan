---
phase: 08-upstream-format-flexibility
fixed_at: 2026-04-17T00:00:00Z
review_path: .planning/phases/08-upstream-format-flexibility/08-REVIEW.md
iteration: 1
findings_in_scope: 4
fixed: 4
skipped: 0
status: all_fixed
---

# Phase 08: Code Review Fix Report

**Fixed at:** 2026-04-17T00:00:00Z
**Source review:** .planning/phases/08-upstream-format-flexibility/08-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 4
- Fixed: 4
- Skipped: 0

## Fixed Issues

### WR-01: Rate-limit retry loop breaks immediately on single-upstream pool

**Files modified:** `internal/server/relay.go`, `internal/server/anthropic.go`
**Commit:** acde2a5
**Applied fix:** Removed the `rateLimitRetry` flag and its conditional branch at the top of the loop in both `handleRelay` and `handleAnthropicRelay`. The loop now always calls `s.pool.Select` to pick an upstream. In the `ClassRateLimited` case, `delete(seen, current.ID)` is called before sleeping so that after the backoff the same upstream is eligible to be selected again rather than causing an immediate `seen` break.

---

### WR-02: Translation errors silently drop SSE events in `proxyAnthropicStream`

**Files modified:** `internal/server/anthropic.go`
**Commit:** ebc5203
**Applied fix:** Changed the translation error guard in `proxyAnthropicStream` from a no-op silence to logging the error with `log.Printf` and breaking out of the read loop, so the connection closes cleanly instead of silently omitting events.

---

### WR-03: Upstream response headers forwarded verbatim without hop-by-hop filtering

**Files modified:** `internal/server/relay.go`
**Commit:** 535b422
**Applied fix:** Added an `isHopByHop` helper that checks a header name against the existing `hopByHopHeaders` slice. Updated `proxyBuffer` to skip hop-by-hop headers (e.g., `Transfer-Encoding`, `Connection`, `Keep-Alive`) when copying upstream response headers to the client.

---

### WR-04: `Classify` treats 501 as `ClassModelNotSupported` — may be over-broad

**Files modified:** `internal/pool/classifier.go`
**Commit:** cade953
**Applied fix:** Changed the guard from `status >= 500` to `status == 500 || status == 501`, scoping the model-not-supported keyword check to provider-originated errors only. 502/503/504 gateway errors now fall through to `ClassTransient` regardless of body content.

---

_Fixed: 2026-04-17T00:00:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
