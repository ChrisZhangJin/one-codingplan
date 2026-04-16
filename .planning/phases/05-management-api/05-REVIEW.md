---
phase: 05-management-api
reviewed: 2026-04-16T00:00:00Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - internal/models/models.go
  - internal/pool/pool.go
  - internal/pool/pool_test.go
  - internal/pool/probe_test.go
  - internal/server/admin.go
  - internal/server/admin_test.go
  - internal/server/anthropic.go
  - internal/server/limit.go
  - internal/server/relay.go
  - internal/server/relay_test.go
  - internal/server/server.go
findings:
  critical: 0
  warning: 5
  info: 4
  total: 9
status: issues_found
---

# Phase 05: Code Review Report

**Reviewed:** 2026-04-16
**Depth:** standard
**Files Reviewed:** 11
**Status:** issues_found

## Summary

The management API phase delivers key CRUD endpoints, upstream rotate/list, auth middleware, limit enforcement, and the Anthropic relay handler. The architecture is sound and the test coverage is thorough. No security vulnerabilities were found — admin key comparison uses `crypto/subtle`, tokens are masked in list/get responses, and API keys are never exposed in the upstream list response.

Five warnings are present, all around correctness or reliability: an unchecked DB reload after PATCH, an TOCTOU race in the token-budget limit check, a streaming-path issue where translate errors are silently dropped, a goroutine leak when `http.Flusher` is not available in `proxyAnthropicStream`, and a stale in-memory pool after upstream enable/disable operations. Four informational items follow.

---

## Warnings

### WR-01: Unchecked DB error on post-PATCH reload

**File:** `internal/server/admin.go:231`
**Issue:** After a successful PATCH, the handler reloads the key with `s.db.First(&key, "id = ?", id)` but ignores the returned error. If the reload fails (e.g., connection hiccup), the handler returns stale data to the caller with HTTP 200, which could be misleading.
**Fix:**
```go
if err := s.db.First(&key, "id = ?", id).Error; err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reload key"})
    return
}
```

---

### WR-02: Token-budget check is a TOCTOU race under concurrent requests

**File:** `internal/server/limit.go:50-58`
**Issue:** The budget check reads the sum of usage records and compares to `TokenBudget` in one SQL query, but the subsequent request is not atomically gated. Under concurrent traffic from the same key, multiple goroutines can pass the check simultaneously before any usage is recorded, allowing the budget to be exceeded. The magnitude depends on request parallelism.
**Fix:** The most practical fix for SQLite is a write-ahead-lock check: record a tentative debit at request start and reverse it on failure, or accept a small over-spend and document it. A simpler safeguard is to cap the tolerance explicitly. At minimum, document that the check is best-effort and not a hard guarantee, so operators don't rely on it for strict billing enforcement.

---

### WR-03: Translate errors silently dropped in Anthropic streaming path

**File:** `internal/server/anthropic.go:233-237`
**Issue:** In `proxyAnthropicStream`, when `st.Translate(buf[:n])` returns a non-nil `translateErr`, the chunk is silently discarded and the loop continues. For the non-streaming path (`proxyAnthropicBuffer`) translation errors correctly abort with a 502. In the streaming path, a persistent translation error causes silent data loss — the client receives a truncated or empty stream with HTTP 200 and no indication of failure.
```go
events, translateErr := st.Translate(buf[:n])
if translateErr == nil {          // <-- silently skips on error
    for _, event := range events {
        writeAndFlush(event)
    }
}
```
**Fix:** At minimum, log the translation error. If the translator can recover across chunks, continue and accept partial output. If translation errors are fatal, write an Anthropic-formatted error event to the stream before closing:
```go
events, translateErr := st.Translate(buf[:n])
if translateErr != nil {
    // write a terminal error event so client is informed
    writeAndFlush([]byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"stream translation failed\"}}\n\n"))
    break
}
for _, event := range events {
    writeAndFlush(event)
}
```

---

### WR-04: Goroutine leak when Flusher unavailable in proxyAnthropicStream

**File:** `internal/server/anthropic.go:195-199`
**Issue:** When `c.Writer.(http.Flusher)` fails, the function returns early after logging usage — but `done` has not been closed at that point because `defer close(done)` on line 213 has not yet been registered. The heartbeat goroutine launched later on line 214 would never be started in this path, so there is no goroutine leak from that goroutine. However, comparing to `proxyStream` in `relay.go` (line 248-251), the same early return occurs before the `defer close(done)` is set up. The leak risk is real if the code is ever reordered or the heartbeat goroutine is started before the flusher check.

More concretely: `defer close(done)` is on line 213, and the heartbeat goroutine is launched on line 214. If the flusher check at line 195 fails and returns early, neither the defer nor the goroutine exists yet — no leak. But the same pattern in `relay.go:proxyStream` (lines 248-280) has the flusher check at line 249, `done` channel on line 266, `defer close(done)` on line 267, and goroutine on line 268. Again no leak in the current code. **The actual issue is** that the `Flusher` check in `proxyAnthropicStream` returns without writing any response to the client — not even an error. The connection simply closes with a partially-written HTTP 200 header (already committed by `WriteHeader` on line 193), leaving the client with an empty 200 streaming response and no explanation.
**Fix:** Log an error and, if possible, write a terminal SSE error event before returning:
```go
if !ok {
    writeAndFlush([]byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"streaming not supported\"}}\n\n"))
    s.logUsage(keyID, upstreamID, false, 0, 0, time.Since(start))
    return
}
```

---

### WR-05: pool.List() hardcodes Enabled=true, masking disabled state

**File:** `internal/pool/pool.go:155`
**Issue:** `List()` always sets `Enabled: true` in the returned `UpstreamInfo` records, because the pool only loads enabled upstreams from the DB (by design). However, the `UpstreamInfo.Enabled` field is exported in the JSON API response (`GET /api/upstreams`) and consumers of that API will always see `"enabled": true` even for entries that were disabled in the database after pool construction. There is currently no mechanism to remove a runtime-disabled upstream from the pool without a restart, and the API gives no signal that the pool may be stale. This is a correctness gap for the management API consumers.
**Fix:** Either remove the `Enabled` field from `UpstreamInfo` (since it is always `true` and misleads), or document clearly in the API that the pool reflects state at startup and requires restart to pick up enable/disable changes. If dynamic enable/disable is planned, this gap should be tracked.

---

## Info

### IN-01: patchKeyRequest cannot clear ExpiresAt (null vs. absent distinction)

**File:** `internal/server/admin.go:38-43` and `213-215`
**Issue:** `patchKeyRequest.ExpiresAt` is `*time.Time`. A JSON `null` and a missing field both result in a `nil` pointer in Go — the handler cannot distinguish "client explicitly set expiry to null (clear it)" from "client did not include expires_at". An operator cannot clear an expiry date via PATCH once set.
**Fix:** Use a sentinel wrapper type or accept a dedicated `clear_expires_at: bool` field in the request. This is a known pattern limitation in Go JSON partial-update APIs.

---

### IN-02: `usageTotals` ignores Row().Scan() errors

**File:** `internal/server/admin.go:70-77`
**Issue:** The raw SQL query's `Row().Scan(&input, &output)` return value is discarded. If the query fails, `input` and `output` remain 0, which silently returns 0 usage totals rather than surfacing a DB error.
**Fix:** Capture and log the scan error:
```go
if err := s.db.Raw(...).Row().Scan(&input, &output); err != nil {
    // log but don't abort — usage totals are informational
}
```

---

### IN-03: `limitMiddleware` token-budget query also ignores Row().Scan() errors

**File:** `internal/server/limit.go:52-55`
**Issue:** Same pattern as IN-02. If the DB query errors, both totals stay 0 and the budget check passes, potentially allowing a key to bypass enforcement.
**Fix:** Same as IN-02 — capture the error. For the limit middleware, a DB error should be treated conservatively (reject the request or at least log with a warn).

---

### IN-04: `relay_test.go` and `pool_test.go` declare conflicting `testEncKey`

**File:** `internal/server/relay_test.go:24` and `internal/pool/pool_test.go:15`
**Issue:** Both test files declare a `testEncKey` package-level variable. They are in different packages (`server_test` vs `pool_test`) so this does not cause a compile error, but the values differ: `pool_test` uses a 32-byte key (`"test-encryption-key-32bytes!!XXX"`) while `server/relay_test.go` uses a 16-byte key (`"0123456789abcdef"`). If the crypto layer ever rejects non-32-byte keys, the relay tests will break silently. This is minor since both are test-only, but the inconsistency is a latent risk.
**Fix:** Standardize on a 32-byte test key in both test packages or extract a shared test helper.

---

_Reviewed: 2026-04-16_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
