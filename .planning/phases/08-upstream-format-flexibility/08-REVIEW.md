---
phase: 08-upstream-format-flexibility
reviewed: 2026-04-17T00:00:00Z
depth: standard
files_reviewed: 9
files_reviewed_list:
  - internal/config/config.go
  - internal/pool/pool.go
  - cmd/ocp/main.go
  - internal/server/anthropic.go
  - internal/server/anthropic_test.go
  - internal/pool/pool_test.go
  - internal/pool/classifier.go
  - internal/pool/classifier_test.go
  - internal/server/relay.go
findings:
  critical: 0
  warning: 4
  info: 3
  total: 7
status: issues_found
---

# Phase 08: Code Review Report

**Reviewed:** 2026-04-17T00:00:00Z
**Depth:** standard
**Files Reviewed:** 9
**Status:** issues_found

## Summary

Phase 08 adds `Format` and `ModelOverride` fields to `UpstreamEntry`, introduces a `ClassModelNotSupported` error class in the classifier, and adds the `handleAnthropicRelay` handler with passthrough (`format: anthropic`) and translate (`format: ""`) paths. The overall design is sound. No critical security or data-loss issues were found. Four warnings were identified: two instances of a shared logic bug in the rate-limit retry loop (present in both `handleRelay` and `handleAnthropicRelay`), one silent error discard in the streaming translation path, and one missing response-side header filtering. Three informational items cover a magic string, an unvalidated config value, and config.go's env setup ordering.

## Warnings

### WR-01: Rate-limit retry loop breaks immediately on single-upstream pool

**File:** `internal/server/relay.go:120-139` (same pattern in `internal/server/anthropic.go:68-86`)

**Issue:** When `rateLimitRetry` becomes true, the loop continues and clears the flag, then falls through to call `s.pool.Select(allowedUpstreams)`. `Select` uses round-robin and may return the same upstream again. When it does, `seen[up.ID]` is already true and the loop breaks immediately — the backed-off retry never happens. With a single enabled upstream (a very common case during credit rotation) this causes the request to fail with 503 instead of being retried after the backoff delay.

**Fix:** Remove the upstream from `seen` before sleeping so it is eligible for retry, or track the retry separately from the seen map:

```go
case pool.ClassRateLimited:
    delete(seen, current.ID) // allow retrying after backoff
    time.Sleep(s.pool.Backoff())
    rateLimitRetry = true
    continue
```

The `rateLimitRetry` flag and the `if rateLimitRetry && current != nil` guard at the top of the loop are then redundant and can be removed once `seen` is managed correctly.

---

### WR-02: Translation errors silently drop SSE events in `proxyAnthropicStream`

**File:** `internal/server/anthropic.go:275-281`

**Issue:** When `st.Translate(buf[:n])` returns a non-nil error, the events slice is silently skipped. The client receives a partial stream with no indication that data was lost. Depending on where the parse failure falls, the client may see a stream that ends without `message_stop`, leaving its internal state undefined.

```go
events, translateErr := st.Translate(buf[:n])
if translateErr == nil {
    for _, event := range events {
        writeAndFlush(event)
    }
}
```

**Fix:** Log the error and break out of the read loop so the connection closes cleanly rather than silently continuing:

```go
events, translateErr := st.Translate(buf[:n])
if translateErr != nil {
    log.Printf("[upstream] id=%d stream translate error: %v", upstreamID, translateErr)
    break
}
for _, event := range events {
    writeAndFlush(event)
}
```

---

### WR-03: Upstream response headers forwarded verbatim without hop-by-hop filtering

**File:** `internal/server/relay.go:210-214`

**Issue:** `proxyBuffer` copies all upstream response headers to the client without removing hop-by-hop headers (e.g., `Connection`, `Transfer-Encoding`, `Keep-Alive`). Forwarding `Transfer-Encoding: chunked` from the upstream can confuse the downstream HTTP stack since Gin already handles chunked encoding. `Connection: close` from upstream would be misinterpreted by the client.

```go
for k, vv := range resp.Header {
    for _, v := range vv {
        c.Header(k, v)
    }
}
```

**Fix:** Filter using the same hop-by-hop list already defined in the package:

```go
for k, vv := range resp.Header {
    if isHopByHop(k) {
        continue
    }
    for _, v := range vv {
        c.Header(k, v)
    }
}
```

Where `isHopByHop` checks against the existing `hopByHopHeaders` slice (or convert it to a map for O(1) lookup).

---

### WR-04: `Classify` treats 501 as `ClassModelNotSupported` — may be over-broad

**File:** `internal/pool/classifier.go:63-69`

**Issue:** The guard is `status >= 500`, which includes 501 (Not Implemented) and 502/503/504 gateway errors. A 502 from an upstream proxy containing the string "model not found" in its HTML error page would permanently mark the upstream unavailable rather than treating it as a transient gateway error. The intent per ROUT-05 is to catch provider-level model configuration errors, not gateway-level errors.

**Fix:** Scope the check to `status >= 500 && status < 502` or add an explicit allowlist (500, 501):

```go
if status == 500 || status == 501 {
    for _, kw := range modelNotSupportedKeywords {
        if strings.Contains(bodyStr, kw) {
            return ClassModelNotSupported
        }
    }
}
```

## Info

### IN-01: `Format` field validated only by convention — typos silently fall through

**File:** `internal/server/anthropic.go:91`, `internal/config/config.go:47`

**Issue:** The only accepted non-default value for `format` is the string `"anthropic"`. Any other value (including `"Anthropic"` or `"openai"`) silently selects the translate path. There is no validation at config load time or pool construction.

**Fix:** Add a validation step in `config.Load` or `database.SyncUpstreams`:

```go
if u.Format != "" && u.Format != "anthropic" {
    return nil, fmt.Errorf("upstream %q: unknown format %q (valid values: \"\", \"anthropic\")", u.Name, u.Format)
}
```

---

### IN-02: `SetEnvPrefix`/`AutomaticEnv` called after `ReadInConfig`

**File:** `internal/config/config.go:57-63`

**Issue:** In Viper, `SetEnvPrefix` and `AutomaticEnv` should be called before `ReadInConfig` for env vars to shadow config-file values during unmarshalling. With the current ordering, the explicit re-reads on lines 74-77 compensate correctly for the top-level scalar fields, but the comment acknowledges that slice-of-struct upstreams cannot be env-overridden anyway. The code is functionally correct due to the explicit field overrides; this is a readability/ordering concern.

**Fix:** Move `SetEnvPrefix`, `SetEnvKeyReplacer`, and `AutomaticEnv` to before `ReadInConfig` (lines 51-57) so the intent is clear and future additions to the explicit override list are not silently forgotten.

---

### IN-03: `relayClient` global timeout and per-request context timeout are redundant

**File:** `internal/server/relay.go:19`, `internal/server/relay.go:142`

**Issue:** `relayClient` is created with `Timeout: 30*time.Second` at line 19. Each outgoing request also wraps the client request context with a 30-second `context.WithTimeout`. The effective deadline is the same (the minimum of the two), but the duplication means a future change to one constant will not update the other.

**Fix:** Either remove the client-level timeout (relying on per-request context cancellation) or remove the per-request `context.WithTimeout` and use the client-level timeout. The per-request approach is preferable since it also propagates client disconnect cancellation:

```go
// Remove Timeout from relayClient declaration:
var relayClient = &http.Client{}

// Keep per-request context timeout as-is (also inherits client cancel on disconnect)
ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
```

---

_Reviewed: 2026-04-17T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
