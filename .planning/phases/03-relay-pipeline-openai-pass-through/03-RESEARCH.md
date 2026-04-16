# Phase 3: Relay Pipeline (OpenAI Pass-Through) - Research

**Researched:** 2026-04-16
**Domain:** Go HTTP reverse-proxy, SSE streaming, Gin middleware, GORM async writes
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Auth: DB lookup `SELECT * FROM access_keys WHERE token = ?` on every request. No cache.
- **D-02:** Auth checks: token exists AND `Enabled = true`. Nothing else in Phase 3.
- **D-03:** Missing/invalid token → `401 Unauthorized`. Do not forward to any upstream.
- **D-04:** Retry ceiling: try ALL available upstreams once per request (one pass through the pool).
- **D-05:** Per-error behavior:
  - Credits-exhausted → `pool.Mark(id, false)`, rotate to next upstream
  - Rate-limited → sleep `pool.Backoff()` duration (5s default), retry same upstream
  - Transient (5xx, timeout) → rotate to next upstream without marking unavailable
- **D-06:** All upstreams fail → return 503 with body `{"error":{"message":"no upstream available","type":"upstream_error","code":"no_upstream","param":null}}`
- **D-07:** Detect streaming by `"stream": true` in JSON request body. Not via `Accept` header.
- **D-08:** Non-streaming: buffer upstream response, forward as single JSON body.
- **D-09:** Streaming: pass SSE frames through via `io.Copy` + `http.Flusher`. Set `X-Accel-Buffering: no`.
- **D-10:** Mid-stream failure (upstream drops after HTTP 200 sent): close client connection. No retry.
- **D-11:** Log every authenticated request — always write a `usage_record`, even on failure.
- **D-12:** On failure: `InputTokens=0`, `OutputTokens=0`, `Success=false`.
- **D-13:** Token counts from `usage.prompt_tokens` / `usage.completion_tokens`; for streaming from final `data: [DONE]` chunk usage field if present, else 0.
- **D-14:** Logging is async fire-and-forget goroutine.

### Claude's Discretion

- Whether to use `httputil.ReverseProxy` or manual `http.NewRequest` + copy for forwarding (manual preferred for failover since body must be re-readable across retry attempts)
- Per-upstream HTTP client timeout value (suggest 30s)
- Exact Gin middleware vs handler structure
- How request body is buffered for retry (e.g., `io.ReadAll` into `[]byte` before first attempt)

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PRXY-01 | Client can send OpenAI-format chat completion requests to `/v1/chat/completions` and receive valid OpenAI-format responses | Auth middleware + relay handler + failover loop sections below |
| PRXY-03 | Proxy streams SSE responses token-by-token without buffering (both OpenAI and Anthropic formats) | SSE streaming pattern, `http.Flusher`, heartbeat goroutine sections below |
| USGR-01 | Every proxied request is logged with: key ID, upstream used, input tokens, output tokens, timestamp, latency, and success/error status | Async usage logging section below |
</phase_requirements>

---

## Summary

Phase 3 wires three concerns together inside the existing `server.Server` struct: (1) a Gin auth middleware that validates bearer tokens against the `access_keys` table, (2) a relay handler that implements the failover loop using the Phase 2 `pool.Pool`, and (3) async usage logging into `usage_records`. All three building blocks — `pool.Select`, `pool.Mark`, `pool.Classify`, `models.UsageRecord` — exist and are tested. Phase 3 adds no new packages; it assembles them.

The key implementation choice left to Claude's discretion is **manual forwarding** (`http.NewRequest` + `io.Copy`) rather than `httputil.ReverseProxy`. The reason is body re-readability across retry attempts: once an `io.Reader` request body is consumed by `ReverseProxy`, it cannot be replayed for a second upstream. Reading the entire body into `[]byte` at entry and rebuilding `bytes.NewReader` for each attempt solves this cleanly.

The streaming path requires careful sequencing: (a) write response headers and flush before the copy loop, (b) start a heartbeat goroutine that sends SSE comment frames on a 30-second ticker, (c) cancel the heartbeat when the upstream closes or fails, and (d) stop retrying once HTTP 200 has been sent (D-10).

**Primary recommendation:** One new file `internal/server/relay.go` containing the auth middleware and the relay handler method on `*Server`. Register the route in `Engine()`. Tests live in `internal/server/relay_test.go` using `httptest.Server` as the fake upstream.

---

## Standard Stack

### Core (all already in go.mod — no new dependencies)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/gin-gonic/gin` | v1.10.1 | Route group, middleware chain, JSON binding, `http.Flusher` via `c.Writer` | Already in use; `c.Writer` implements `http.Flusher` |
| `net/http` stdlib | Go 1.25 | `http.NewRequest`, `http.Client`, `io.Copy` | Manual forwarding strategy |
| `encoding/json` stdlib | Go 1.25 | Decode request body to detect `"stream": true`; encode 503 error body | |
| `gorm.io/gorm` | v1.31.1 | `s.db.Where(...).First()` for auth lookup; `s.db.Create()` for usage records | Already in use |
| `bytes` / `io` stdlib | Go 1.25 | `io.ReadAll` to buffer body; `bytes.NewReader` to replay per retry | |
| `time` stdlib | Go 1.25 | Latency measurement (`time.Since`), rate-limit backoff sleep | |
| `context` stdlib | Go 1.25 | Per-upstream request timeout with `context.WithTimeout` | |

**Version verification:** All packages verified present in `/root/workspace/one-codingplan/go.mod` [VERIFIED: local go.mod]. No `go get` commands needed.

### No New Packages Required

The `gin-contrib/sse` package is already an indirect dependency (via Gin). It is not needed — raw `io.Copy` with `http.Flusher` is simpler and avoids SSE framing decisions that belong to the upstream.

---

## Architecture Patterns

### Recommended Project Structure (additions only)

```
internal/server/
├── server.go          # existing — add route registration in Engine()
├── server_test.go     # existing
├── relay.go           # NEW: authMiddleware + handleRelay methods
└── relay_test.go      # NEW: tests using httptest.Server fake upstream
```

### Pattern 1: Auth Middleware on Gin Route Group

**What:** Extract the `Authorization: Bearer <token>` header, query DB, set key ID in Gin context for downstream use.

**When to use:** Applied to the `/v1` route group so future routes (Phase 4 `/v1/messages`) inherit it automatically.

**Example:**
```go
// Source: established pattern from server.go + Gin docs [ASSUMED: Gin middleware pattern]
func (s *Server) authMiddleware(c *gin.Context) {
    auth := c.GetHeader("Authorization")
    token, ok := strings.CutPrefix(auth, "Bearer ")
    if !ok || token == "" {
        c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
        return
    }
    var key models.AccessKey
    if err := s.db.Where("token = ? AND enabled = true", token).First(&key).Error; err != nil {
        c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
        return
    }
    c.Set("keyID", key.ID)
    c.Next()
}
```

**Key detail:** `c.AbortWithStatusJSON` stops the middleware chain and writes the response. Do not call `c.Next()` on the error path. [VERIFIED: Gin source — `Abort` sets index to `abortIndex`]

### Pattern 2: Manual Forwarding with Retry Loop

**What:** `io.ReadAll` the request body once into `[]byte`, then loop over `pool.Select` attempts, rebuilding `bytes.NewReader(bodyBytes)` for each upstream.

**Why not `httputil.ReverseProxy`:** `ReverseProxy` reads the body from `req.Body` (an `io.Reader`). After the first upstream attempt, `req.Body` is at EOF and cannot be replayed without reassignment. Manual forwarding with a `[]byte` buffer avoids this entirely. [ASSUMED: known Go io.Reader behavior — training knowledge, well-established]

**Example structure:**
```go
// Source: [ASSUMED: derived from Go stdlib patterns + D-04/D-05 decisions]
func (s *Server) handleRelay(c *gin.Context) {
    bodyBytes, err := io.ReadAll(c.Request.Body)
    if err != nil { /* 400 */ return }

    keyID := c.GetString("keyID")
    start := time.Now()

    // detect stream field
    var req struct { Stream bool `json:"stream"` }
    _ = json.Unmarshal(bodyBytes, &req)

    var lastErr error
    for {
        upstream, err := s.pool.Select(keyID)
        if errors.Is(err, pool.ErrNoUpstreams) {
            writeNoUpstreamError(c)
            logUsage(s.db, keyID, 0, false, 0, 0, time.Since(start))
            return
        }
        // build outbound request
        outReq, _ := http.NewRequestWithContext(c.Request.Context(),
            http.MethodPost,
            upstream.BaseURL+"/v1/chat/completions",
            bytes.NewReader(bodyBytes))
        outReq.Header = c.Request.Header.Clone()
        outReq.Header.Set("Authorization", "Bearer "+upstream.APIKey)

        resp, err := relayClient.Do(outReq)
        if err != nil {
            lastErr = err
            continue // transient — rotate
        }
        defer resp.Body.Close()

        if resp.StatusCode >= 400 {
            respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
            class := pool.Classify(upstream.Name, resp.StatusCode, respBody)
            switch class {
            case pool.ClassCreditsExhausted:
                s.pool.Mark(upstream.ID, false)
                continue
            case pool.ClassRateLimited:
                time.Sleep(s.pool.Backoff())
                continue // retry same upstream — loop calls Select again; will pick same if only one available
            default: // transient
                continue
            }
        }

        // success path — stream or buffer
        if req.Stream {
            s.proxyStream(c, resp, keyID, upstream.ID, start)
        } else {
            s.proxyBuffer(c, resp, keyID, upstream.ID, start)
        }
        return
    }
    _ = lastErr
}
```

**Important nuance on rate-limit retry:** D-05 says "retry the same upstream" after backoff. But `pool.Select` is round-robin and will advance the index on the next call. To retry the same upstream, the loop must track the upstream reference and re-use it for the next attempt without calling `pool.Select` again. The implementation must distinguish "rotate to next" (call `pool.Select`) from "retry same" (re-use current upstream reference). [ASSUMED: derived from D-05 logic]

### Pattern 3: SSE Streaming via `io.Copy` + `http.Flusher`

**What:** Copy upstream SSE bytes directly to the client writer, flushing after each write, with a heartbeat goroutine running in parallel.

**Key steps:**
1. Set response headers before any write: `Content-Type: text/event-stream`, `X-Accel-Buffering: no`, `Cache-Control: no-cache`
2. Assert `c.Writer` to `http.Flusher`
3. Start heartbeat goroutine (sends `: heartbeat\n\n` every 30 seconds)
4. `io.Copy(c.Writer, resp.Body)` — blocks until upstream closes or errors
5. Cancel heartbeat goroutine via context or channel when copy returns
6. Flush once more after copy
7. Log usage async after stream ends

**Flusher availability:** Gin's `gin.ResponseWriter` embeds `http.ResponseWriter`. The underlying `net/http` server's `http.response` type implements `http.Flusher`. In tests with `httptest.ResponseRecorder`, `Flusher` is also implemented. [VERIFIED: gin ResponseWriter interface in gin source v1.10.1]

**Mid-stream failure (D-10):** If `io.Copy` returns an error after headers/body have been started, simply return — the client connection will close. Do not attempt retry. The check "have we written anything yet" is implicit: if we are in `proxyStream`, headers are already written.

**Heartbeat pattern:**
```go
// Source: [ASSUMED: standard Go goroutine pattern]
done := make(chan struct{})
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            fmt.Fprint(c.Writer, ": heartbeat\n\n")
            flusher.Flush()
        case <-done:
            return
        }
    }
}()
io.Copy(c.Writer, resp.Body)
close(done)
```

### Pattern 4: Async Usage Logging

**What:** Spawn goroutine after response to write `models.UsageRecord`. Fire-and-forget.

**D-13 implementation:** For non-streaming responses, parse `usage.prompt_tokens` and `usage.completion_tokens` from the buffered JSON body before forwarding. For streaming, the final SSE frame before `data: [DONE]` may include a `usage` field — scan for it during copy or log 0.

**Simplest streaming token count approach:** Do not attempt to parse SSE frames during `io.Copy`. If `data: [DONE]` precedes a `usage` frame, the upstream sends it as a distinct frame. Parsing requires buffering the stream, which defeats D-09. Decision: log 0 tokens for streaming responses unless the upstream sends a final JSON object before close (not `data: [DONE]`). This is acceptable per D-13 ("otherwise log 0"). [ASSUMED: simplest compliant interpretation of D-13]

```go
// Source: [ASSUMED: derived from D-14]
func logUsage(db *gorm.DB, keyID string, upstreamID uint, success bool, in, out int, latency time.Duration) {
    go db.Create(&models.UsageRecord{
        KeyID:        keyID,
        UpstreamID:   upstreamID,
        InputTokens:  in,
        OutputTokens: out,
        LatencyMs:    latency.Milliseconds(),
        Success:      success,
    })
}
```

### Anti-Patterns to Avoid

- **`httputil.ReverseProxy` for failover:** Body is consumed after first attempt; cannot replay. Use manual `http.NewRequest`.
- **Calling `pool.Select` for rate-limit retry:** `Select` advances the round-robin index. For "retry same upstream", hold the `*UpstreamEntry` reference and re-use it.
- **Writing response headers before checking upstream status:** Once `c.Status(200)` and any body bytes are written, `c.Writer.Written()` returns true and the response is committed. Check upstream HTTP status before committing.
- **Blocking the handler goroutine with DB writes:** Violates D-14. Always spawn a goroutine for `db.Create(&usageRecord)`.
- **`c.Next()` on auth error path:** Always use `c.AbortWithStatusJSON` on failure, not `c.JSON` + `return`, because `return` alone does not stop Gin's middleware chain.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SSE framing | Custom SSE encoder | Raw `io.Copy` of upstream bytes | Upstream already produces valid SSE; re-encoding risks corruption |
| JSON binding | Manual string parsing | `encoding/json.Unmarshal` into struct | Edge cases in number parsing, whitespace |
| HTTP client with timeout | Manual deadline logic | `context.WithTimeout` + `http.NewRequestWithContext` | Context cancellation propagates to all I/O automatically |
| Error body classification | Custom HTTP status rules | `pool.Classify` (already exists) | Per-provider keyword matching already implemented |

---

## Common Pitfalls

### Pitfall 1: Rate-limit retry calls `pool.Select` again (rotates instead of retrying)

**What goes wrong:** The loop unconditionally calls `pool.Select` at the top, so after a rate-limit sleep, `Select` returns the next upstream instead of the rate-limited one.

**Why it happens:** Conflating "get an upstream" with "start a new attempt". Rate-limit retry should re-use the current upstream reference.

**How to avoid:** Structure the loop so rate-limit sleep is followed by a `continue` that re-uses the current `upstream` variable, not a call to `Select`. Separate the "get upstream" step from the "retry same" step via a flag or inner loop.

**Warning signs:** Test where pool has one upstream and rate-limit response is returned — if the handler returns 503 immediately, `Select` is being called incorrectly after backoff.

### Pitfall 2: Committing response before upstream status is known

**What goes wrong:** Writing `c.Writer.WriteHeader(200)` or any body bytes commits the response. After that, you cannot change the status to 503.

**Why it happens:** Naive approach reads upstream response status and body simultaneously with forwarding.

**How to avoid:** For non-streaming, buffer the entire upstream response body first, check status, classify, then either forward or retry. For streaming, only commit headers after receiving upstream HTTP 200.

**Warning signs:** Client receives 200 with an error body, or truncated response on failover.

### Pitfall 3: Missing `X-Accel-Buffering: no` causes nginx to buffer the SSE stream

**What goes wrong:** When ocp sits behind nginx, nginx buffers the response until its proxy buffer fills, breaking token-by-token streaming.

**Why it happens:** nginx defaults to buffering proxy responses.

**How to avoid:** Set `X-Accel-Buffering: no` on every streaming response before writing any body bytes. [CITED: nginx docs — ngx_http_proxy_module proxy_buffering directive]

**Warning signs:** Stream arrives in large chunks rather than token-by-token; works locally but not behind a reverse proxy.

### Pitfall 4: Auth middleware uses `c.JSON` + `return` instead of `c.AbortWithStatusJSON`

**What goes wrong:** The handler still executes because `c.JSON` does not stop Gin's chain; only `return` stops the current function, not downstream handlers.

**Why it happens:** Developers coming from frameworks where returning early from a handler stops processing.

**How to avoid:** Always use `c.AbortWithStatusJSON` (or `c.Abort()` + `c.JSON`) in middleware error paths.

### Pitfall 5: Heartbeat goroutine leaks if relay handler panics or returns early

**What goes wrong:** `done` channel is never closed; heartbeat goroutine runs forever, writing to a closed connection.

**Why it happens:** Forgetting to `close(done)` on all return paths.

**How to avoid:** `defer close(done)` immediately after `make(chan struct{})`. The goroutine reads from the channel, so multiple closes are a panic — use `sync.Once` if needed, or ensure `close(done)` is called exactly once via `defer`.

### Pitfall 6: `io.ReadAll` on request body without a size limit

**What goes wrong:** Malicious client sends an unbounded body, exhausting memory.

**Why it happens:** `io.ReadAll(c.Request.Body)` has no cap.

**How to avoid:** Use `io.LimitReader(c.Request.Body, 10*1024*1024)` (10 MB is reasonable for chat completions). Return 413 if the limit is hit.

---

## Code Examples

### Registering the route group in `Engine()`
```go
// Source: [ASSUMED: Gin route group pattern]
func (s *Server) Engine() *gin.Engine {
    r := gin.New()
    r.Use(gin.Logger())
    r.Use(gin.Recovery())
    r.GET("/health", s.handleHealth)

    v1 := r.Group("/v1")
    v1.Use(s.authMiddleware)
    v1.POST("/chat/completions", s.handleRelay)

    return r
}
```

### 503 error response body (D-06)
```go
// Source: D-06 decision (verbatim format)
var errNoUpstream = gin.H{
    "error": gin.H{
        "message": "no upstream available",
        "type":    "upstream_error",
        "code":    "no_upstream",
        "param":   nil,
    },
}
// Usage: c.AbortWithStatusJSON(http.StatusServiceUnavailable, errNoUpstream)
```

### Per-upstream HTTP client (discretion: 30s timeout)
```go
// Source: [ASSUMED: standard Go http.Client pattern]
var relayClient = &http.Client{
    Timeout: 30 * time.Second,
}
```

Place as a package-level variable in `relay.go`. Reusing a single `http.Client` is important for connection pooling. [ASSUMED: Go net/http transport connection pooling behavior]

### Extracting token counts from non-streaming response
```go
// Source: [ASSUMED: OpenAI response schema, well-established]
type usageField struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
}
type chatResponse struct {
    Usage usageField `json:"usage"`
}
var cr chatResponse
_ = json.Unmarshal(respBody, &cr)
in, out := cr.Usage.PromptTokens, cr.Usage.CompletionTokens
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `httputil.ReverseProxy` for AI proxy | Manual `http.NewRequest` + copy | When failover with body replay was needed | ReverseProxy cannot replay request body; manual forwarding is required |
| SSE via `gin-contrib/sse` | Raw `io.Copy` passthrough | N/A for transparent proxy | Re-encoding upstream SSE adds latency and risk of frame corruption |
| Synchronous usage logging | Async fire-and-forget goroutine | Standard pattern for proxy logging | Eliminates DB write from request critical path |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `httputil.ReverseProxy` cannot replay request body across retries without manual reassignment | Architecture Patterns, Pattern 2 | Low — well-established Go io.Reader behavior; if wrong, could use ReverseProxy with body reassignment |
| A2 | Rate-limit retry must hold upstream reference rather than call `pool.Select` again | Pattern 2, Pitfall 1 | Medium — if Select were idempotent (same upstream), this wouldn't matter; but round-robin guarantees rotation |
| A3 | Streaming token count set to 0 when SSE frame parsing not performed | Pattern 4 | Low — D-13 explicitly allows 0 fallback for streaming |
| A4 | 10 MB body size limit is reasonable for chat completions | Pitfall 6 | Low — easily adjustable; no upstream enforces a specific limit |
| A5 | `defer close(done)` is safe for single-writer heartbeat goroutine | Pattern 3 | Low — goroutine reads, handler writes; no double-close risk if deferred correctly |

---

## Open Questions

1. **Rate-limit retry loop structure**
   - What we know: D-05 says "sleep Backoff(), retry same upstream"; D-04 says "try ALL upstreams once"
   - What's unclear: Does "retry same upstream" count against the "one pass" ceiling? If pool has 1 upstream and it rate-limits, do we retry it or return 503?
   - Recommendation: Retry the same upstream after backoff without counting it against the "one pass" ceiling. The "one pass" limit applies to rotating across different upstreams, not to rate-limit retries on the same one. Planner should confirm or document.

2. **`Content-Length` header on forwarded non-streaming response**
   - What we know: The upstream may send `Content-Length` on non-streaming responses; Gin does not strip it automatically
   - What's unclear: If we buffer the body and the upstream's `Content-Length` matches the actual body, forwarding it is safe. But if we modify the body (e.g., on error), we must strip or recalculate it.
   - Recommendation: For the success non-streaming path (body passed through unmodified), forward `Content-Length` as-is. For error paths (503 JSON from ocp), Gin sets `Content-Length` automatically.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go 1.25 | All Go source | ✓ | go1.25.0 linux/amd64 | — |
| Gin v1.10.1 | HTTP routing | ✓ | in go.mod | — |
| gorm.io/gorm v1.31.1 | DB queries | ✓ | in go.mod | — |
| `net/http` stdlib | HTTP client | ✓ | stdlib | — |
| `encoding/json` stdlib | JSON decode | ✓ | stdlib | — |

[VERIFIED: `go version` and `go.mod` contents checked locally]

**No missing dependencies.** Phase 3 requires no new packages.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `net/http/httptest` |
| Config file | none (no pytest.ini / jest.config equivalent needed) |
| Quick run command | `go test ./internal/server/... -run TestRelay -v` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PRXY-01 | Valid token + non-streaming request → upstream response forwarded | unit | `go test ./internal/server/... -run TestRelay_NonStream` | ❌ Wave 0 |
| PRXY-01 | Missing token → 401, no upstream call | unit | `go test ./internal/server/... -run TestRelay_Auth_Missing` | ❌ Wave 0 |
| PRXY-01 | Invalid token → 401, no upstream call | unit | `go test ./internal/server/... -run TestRelay_Auth_Invalid` | ❌ Wave 0 |
| PRXY-01 | All upstreams fail → 503 with correct error body | unit | `go test ./internal/server/... -run TestRelay_AllFail` | ❌ Wave 0 |
| PRXY-01 | First upstream credits-exhausted, second succeeds → client gets success | unit | `go test ./internal/server/... -run TestRelay_Failover_Credits` | ❌ Wave 0 |
| PRXY-03 | Streaming request → SSE frames arrive without buffering | unit | `go test ./internal/server/... -run TestRelay_Stream` | ❌ Wave 0 |
| PRXY-03 | Heartbeat comment sent on idle stream | unit | `go test ./internal/server/... -run TestRelay_Stream_Heartbeat` | ❌ Wave 0 |
| USGR-01 | Usage record written to DB after successful non-streaming request | unit | `go test ./internal/server/... -run TestRelay_Usage_Success` | ❌ Wave 0 |
| USGR-01 | Usage record written with Success=false when all upstreams fail | unit | `go test ./internal/server/... -run TestRelay_Usage_Failure` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/server/... -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/server/relay.go` — implementation (all relay tests depend on this)
- [ ] `internal/server/relay_test.go` — covers PRXY-01, PRXY-03, USGR-01

The existing test infrastructure (`testing` + `httptest`) covers all phase needs. No new framework install required.

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Bearer token DB lookup — `WHERE token = ? AND enabled = true` |
| V3 Session Management | no | Stateless per-request auth; no sessions |
| V4 Access Control | no | Phase 3 has no per-key restrictions (Phase 5) |
| V5 Input Validation | yes | `io.LimitReader` on request body (10 MB cap); JSON unmarshal into typed struct |
| V6 Cryptography | no | No new crypto operations; upstream API keys already encrypted by Phase 1 |

### Known Threat Patterns for Go HTTP Proxy

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Unbounded request body | DoS | `io.LimitReader(c.Request.Body, 10*1024*1024)` — return 413 if exceeded |
| Token enumeration via timing | Information Disclosure | GORM query returns same error for "not found" and "disabled" — return identical 401 for both; do not leak reason |
| Upstream credential leakage in error body | Information Disclosure | Never forward upstream error bodies that may contain the upstream API key in response headers or body |
| SSRF via upstream BaseURL | Elevation of Privilege | Upstreams are admin-configured, not user-supplied — acceptable risk at this phase; no user-controlled URL |
| Goroutine leak via heartbeat | DoS | `defer close(done)` ensures goroutine exits; context cancellation from client disconnect propagates via `c.Request.Context()` |

---

## Sources

### Primary (HIGH confidence)

- Local codebase: `internal/pool/pool.go`, `internal/pool/classifier.go`, `internal/models/models.go`, `internal/server/server.go`, `internal/config/config.go`, `go.mod` — [VERIFIED: Read tool]
- `03-CONTEXT.md` — locked decisions D-01 through D-14 [VERIFIED: Read tool]
- Go stdlib `net/http`, `io`, `bytes`, `encoding/json` — standard library behavior [ASSUMED for precise behavior; HIGH confidence based on well-documented stdlib]

### Secondary (MEDIUM confidence)

- Gin v1.10.1 `AbortWithStatusJSON`, `ResponseWriter` flusher interface — [ASSUMED: Gin documentation pattern; consistent with existing server.go usage]
- nginx `X-Accel-Buffering` header — [CITED: nginx ngx_http_proxy_module documentation]

### Tertiary (LOW confidence)

- OpenAI SSE `data: [DONE]` + `usage` field in final frame — provider-dependent; some providers do not send usage in streaming mode [ASSUMED: based on OpenAI API docs knowledge, not verified against each Chinese provider in this session]

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all packages verified in go.mod; no new dependencies
- Architecture: HIGH — patterns derived from locked decisions D-01 through D-14 and existing codebase
- Pitfalls: HIGH for Go-specific ones (io.Reader exhaustion, Gin Abort); MEDIUM for provider-specific ones (SSE usage field)
- Test map: HIGH — Go stdlib testing is established; test file gaps are known (Wave 0)

**Research date:** 2026-04-16
**Valid until:** 2026-05-16 (stable Go stdlib + Gin; providers' SSE behavior may change)
