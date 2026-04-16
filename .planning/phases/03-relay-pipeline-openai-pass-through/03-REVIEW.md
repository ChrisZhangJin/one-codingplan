---
phase: 03-relay-pipeline-openai-pass-through
reviewed: 2026-04-16T00:00:00Z
depth: standard
files_reviewed: 3
files_reviewed_list:
  - internal/server/relay.go
  - internal/server/relay_test.go
  - internal/server/server.go
findings:
  critical: 0
  warning: 3
  info: 2
  total: 5
status: issues_found
---

# Phase 03: Code Review Report

**Reviewed:** 2026-04-16
**Depth:** standard
**Files Reviewed:** 3
**Status:** issues_found

## Summary

Three files reviewed: the relay pipeline (`relay.go`), its test suite (`relay_test.go`), and the server wiring (`server.go`). The implementation is generally well-structured with correct auth enforcement, body-size capping, failover logic, streaming with heartbeats, and async usage logging. No security vulnerabilities or data loss risks were found.

Three warnings were identified: an unbounded retry loop when an upstream persistently rate-limits, a blocking `time.Sleep` on the handler goroutine during rate-limit backoff, and multi-value response headers being silently clobbered in the non-streaming path. Two informational items: a test helper that ignores a parameter value, and a magic `+1` in the body-limit check.

---

## Warnings

### WR-01: Unbounded retry loop on persistent 429 response

**File:** `internal/server/relay.go:113-168`

**Issue:** When an upstream returns 429, `rateLimitRetry` is set to `true` and the loop `continue`s. On re-entry the rate-limit branch resets `rateLimitRetry = false` and falls through to create a new request context — but if that same upstream returns 429 a second time, `rateLimitRetry` is set `true` again and the loop continues indefinitely. The `seen` map guards against rotating to already-tried upstreams, but it is only consulted in the `else` branch (lines 118-132), which is bypassed entirely when `rateLimitRetry == true`. There is no iteration counter or maximum-retry guard on this path.

**Scenario:** One upstream in the pool, upstream returns 429 on every call → handler loops forever, goroutine and client connection are both held indefinitely.

**Fix:** Add a retry counter and cap it (e.g. one retry):

```go
rateLimitRetries := 0
const maxRateLimitRetries = 1

// Inside the loop, replace the ClassRateLimited case:
case pool.ClassRateLimited:
    if rateLimitRetries >= maxRateLimitRetries {
        continue // give up on this upstream, rotate
    }
    time.Sleep(s.pool.Backoff())
    rateLimitRetries++
    rateLimitRetry = true
    continue
```

Reset `rateLimitRetries = 0` when a new upstream is selected (move the reset into the `else` branch after `seen[up.ID] = true`).

---

### WR-02: `time.Sleep` blocks the handler goroutine during rate-limit backoff

**File:** `internal/server/relay.go:163`

**Issue:** `time.Sleep(s.pool.Backoff())` is called directly on the Gin handler goroutine. For the production backoff value (likely seconds), this holds the HTTP connection open, blocks the goroutine, and means every concurrent rate-limited request occupies a goroutine for the full sleep duration. Under load this can exhaust the goroutine pool.

**Fix:** Use a context-aware sleep so the goroutine is released if the client disconnects:

```go
case pool.ClassRateLimited:
    timer := time.NewTimer(s.pool.Backoff())
    select {
    case <-timer.C:
        // backoff elapsed, retry
    case <-c.Request.Context().Done():
        timer.Stop()
        cancel() // already called above
        return
    }
    rateLimitRetry = true
    continue
```

---

### WR-03: Multi-value response headers clobbered in `proxyBuffer`

**File:** `internal/server/relay.go:199-202`

**Issue:** `c.Header(k, v)` calls `w.Header().Set(k, v)`, which replaces any existing value for key `k`. The inner loop iterates over all values of each header key (`for _, v := range vv`), so for headers with multiple values (e.g. `Set-Cookie`, `Vary`, `Link`) every value except the last is silently discarded.

```go
// Current — overwrites on each iteration:
for k, vv := range resp.Header {
    for _, v := range vv {
        c.Header(k, v) // Set(), not Add()
    }
}
```

**Fix:** Write directly to the underlying `ResponseWriter` header map, which allows multiple values, or use `Add`:

```go
for k, vv := range resp.Header {
    for _, v := range vv {
        c.Writer.Header().Add(k, v)
    }
}
```

---

## Info

### IN-01: `seedAccessKey` ignores the `enabled` parameter for the initial `Create`

**File:** `internal/server/relay_test.go:44-57`

**Issue:** The helper always creates the key with `Enabled: true` (line 46) regardless of the `enabled` argument, then conditionally issues a separate `Update` call. This is not a functional bug (the update corrects it), but it means the helper always performs two database operations when `enabled=false`, and the initial `Create` fires with incorrect state, which could matter if a trigger or hook observed it. The `enabled` parameter name gives no hint that it is applied post-creation.

**Fix:** Pass the `enabled` value directly into the struct literal at creation:

```go
key := models.AccessKey{ID: "key-1", Token: token, Enabled: enabled}
```

Remove the conditional update block that follows.

---

### IN-02: Magic `+1` in body-limit check

**File:** `internal/server/relay.go:94`

**Issue:** `io.LimitReader(c.Request.Body, 10*1024*1024+1)` uses `+1` to detect whether the body exceeds the limit without reading one byte past it and discarding it. The intent is correct but the constant is undocumented and looks like an off-by-one at a glance.

**Fix:** Extract a named constant and add a brief inline comment:

```go
const bodyLimit = 10 * 1024 * 1024
// Read one extra byte; if we get it, the body exceeds the limit.
bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, bodyLimit+1))
```

---

_Reviewed: 2026-04-16_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
