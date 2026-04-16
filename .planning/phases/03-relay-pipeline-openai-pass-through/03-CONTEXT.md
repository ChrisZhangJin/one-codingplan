# Phase 3: Relay Pipeline (OpenAI Pass-Through) - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Accept OpenAI-format chat completion requests at `/v1/chat/completions` → authenticate bearer token → select upstream from pool → forward request → stream or buffer response → failover on error → log usage to SQLite.

No Anthropic format translation (Phase 4). No management API (Phase 5). No per-key rate limiting or token budgets (Phase 5).

</domain>

<decisions>
## Implementation Decisions

### Auth Middleware
- **D-01:** Token validation: DB lookup (`SELECT * FROM access_keys WHERE token = ?`) on every request. Simple, always current, no cache invalidation problem. SQLite WAL reads are fast enough at single-instance scale.
- **D-02:** Checks: token exists AND `Enabled = true`. Nothing else in Phase 3 — no rate limiting, no token budget, no per-key upstream restrictions. Those come in Phase 5.
- **D-03:** On missing or invalid token: return `401 Unauthorized`. Do not forward the request to any upstream.

### Failover Retry Logic
- **D-04:** Retry ceiling: try ALL available upstreams once per request (one pass through the pool). If pool has 3 upstreams and 2 fail, the 3rd gets the request. No hard cap beyond pool size.
- **D-05:** Per-error behavior (from Phase 2 decisions, now enforced in relay):
  - Credits-exhausted → call `pool.Mark(id, false)` to mark unavailable, rotate to next upstream
  - Rate-limited → sleep `pool.Backoff()` duration (5s default), retry the same upstream
  - Transient (5xx, timeout) → rotate to next upstream without marking unavailable
- **D-06:** When all upstreams fail or pool returns `ErrNoUpstreams`: return **503** with OpenAI error JSON body:
  `{"error":{"message":"no upstream available","type":"upstream_error","code":"no_upstream","param":null}}`
  Rationale: matches what OpenAI itself returns for "no healthy upstream"; Claude Code auto-retries 503 with backoff.

### Streaming
- **D-07:** Detect streaming by checking `"stream": true` in the JSON request body (OpenAI spec field). Do not rely on `Accept: text/event-stream` header.
- **D-08:** Non-streaming: buffer upstream response, forward to client as single JSON body.
- **D-09:** Streaming: pass SSE frames through to client as they arrive (`io.Copy` + `http.Flusher`). Set `X-Accel-Buffering: no` to prevent nginx buffering.
- **D-10:** Mid-stream failure (upstream drops after HTTP 200 sent): close the client connection. No retry. Retrying mid-stream would corrupt SSE framing already received by client.

### Usage Logging
- **D-11:** Log every authenticated request — always write a `usage_record`, even on failure.
- **D-12:** On failure (no upstream response): `InputTokens=0`, `OutputTokens=0`, `Success=false`.
- **D-13:** Token counts from upstream response body: `usage.prompt_tokens` and `usage.completion_tokens` fields. For streaming, accumulate from the final `data: [DONE]` chunk's usage field if present; otherwise log 0.
- **D-14:** Logging is **async fire-and-forget** — spawn goroutine to write `usage_record` after response is sent. Acceptable data loss risk (crash between response and write). Keeps p99 latency clean.

### Claude's Discretion
- Whether to use `httputil.ReverseProxy` or manual `http.NewRequest` + copy for forwarding (manual preferred for failover since body must be re-readable across retry attempts)
- Per-upstream HTTP client timeout value (suggest 30s)
- Exact Gin middleware vs handler structure
- How request body is buffered for retry (e.g., `io.ReadAll` into `[]byte` before first attempt)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Requirements
- `.planning/REQUIREMENTS.md` — PRXY-01 (OpenAI chat completions), PRXY-03 (SSE streaming), USGR-01 (usage logging), ROUT-01 (round-robin), ROUT-02 (auto-rotate on error)
- `.planning/ROADMAP.md` §Phase 3 — Goal, success criteria, dependency on Phase 2

### Project Context
- `.planning/PROJECT.md` — Core value, constraints, extensibility principle
- `CLAUDE.md` (project root) — Tech stack rationale, streaming approach (`httputil.ReverseProxy` + `FlushInterval: -1`), SSE heartbeat note

### Phase 1 & 2 Output (read before implementing)
- `.planning/phases/02-upstream-pool-health-monitor/02-CONTEXT.md` — All Phase 2 decisions (pool interface, error classification, rate-limit backoff)
- `internal/pool/pool.go` — Pool struct: `Select(keyID string)`, `Mark(id uint, available bool)`, `Stop()`, `Backoff() time.Duration`, `ErrNoUpstreams`
- `internal/pool/classifier.go` — `Classify(provider string, status int, body []byte) ErrorClass`; `ClassTransient`, `ClassRateLimited`, `ClassCreditsExhausted`
- `internal/models/models.go` — `AccessKey{ID, Token, Enabled}`, `UsageRecord{KeyID, UpstreamID, InputTokens, OutputTokens, LatencyMs, Success}`
- `internal/server/server.go` — Server struct with `db`, `cfg`, `pool` fields; relay handler registers here
- `internal/config/config.go` — Config struct; `cfg.Server.AdminKey` for admin auth

### OpenAI Error Format (researched)
- Standard error shape: `{"error":{"message":"...","type":"...","code":"...","param":null}}`
- 503 = correct status for "no healthy upstream" (matches OpenAI's own behavior; Claude Code retries 503 automatically)

### No external specs — requirements fully captured in decisions above

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pool.Select(keyID string) (*UpstreamEntry, error)` — returns next available upstream, round-robin
- `pool.Mark(id uint, available bool)` — marks upstream available/unavailable
- `pool.Backoff() time.Duration` — returns configured rate-limit backoff duration
- `Classify(provider, status, body) ErrorClass` — classifies upstream error response
- `models.AccessKey`, `models.UsageRecord` — GORM models, tables already migrated

### Established Patterns
- Gin handlers on `Server` struct (see `handleHealth`)
- GORM queries via `s.db`
- Pool injected as `s.pool` on Server struct

### Integration Points
- New route registered in `Server.Engine()`: `POST /v1/chat/completions`
- Auth middleware wraps the relay route group
- Usage records written to `s.db` (async goroutine)
- `pool.Mark(id, false)` called when `Classify` returns `ClassCreditsExhausted` on upstream response

</code_context>

<specifics>
## Specific Ideas

- Phase 3 closes the two gaps from Phase 2 verification: (1) relay calls `pool.Mark(id, false)` on credits-exhausted response; (2) relay applies `pool.Backoff()` sleep on rate-limit response before retrying same upstream.
- SSE heartbeat comment (`: heartbeat\n\n`) on 30s ticker to prevent proxy/client timeouts — mentioned in CLAUDE.md tech stack notes.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 03-relay-pipeline-openai-pass-through*
*Context gathered: 2026-04-16*
