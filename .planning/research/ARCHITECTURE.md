# Architecture Research — one-codingplan (ocp)

**Researched:** 2026-04-16
**Confidence:** HIGH (core patterns verified against one-api source analysis, litellm docs, Go stdlib docs)

---

## Component Overview (ASCII diagram)

```
┌─────────────────────────────────────────────────────────────────┐
│  ocp process (single binary)                                    │
│                                                                 │
│  ┌────────────────────────────────────────────────────────┐    │
│  │  HTTP listener :8080  (proxy + management + portal)    │    │
│  │                                                        │    │
│  │  /v1/chat/completions   ──► Relay pipeline             │    │
│  │  /v1/messages           ──► Relay pipeline             │    │
│  │  /api/*                 ──► Management API             │    │
│  │  /*                     ──► Embedded portal (go:embed) │    │
│  └───────────────────────────────────────────────────────-┘    │
│                                                                 │
│  ┌─────────────────┐   ┌──────────────────────────────────┐   │
│  │  Relay Pipeline │   │  Upstream Pool (in-memory)        │   │
│  │                 │   │                                   │   │
│  │  1. Auth MW     │   │  []Upstream {                     │   │
│  │  2. Rate limit  │◄──│    id, name, type                 │   │
│  │  3. Key lookup  │   │    baseURL, apiKey                │   │
│  │  4. Pool select │   │    status (active/cooldown/dead)  │   │
│  │  5. Format xfm  │   │    cooldownUntil time.Time        │   │
│  │  6. HTTP proxy  │   │    consecutiveErrors int          │   │
│  │  7. Usage log   │   │    lastBalance float64            │   │
│  └─────────────────┘   │    lastChecked time.Time          │   │
│                         │  }                               │   │
│  ┌─────────────────┐   │                                   │   │
│  │  Health Monitor │   │  roundRobinIndex uint64 (atomic)  │   │
│  │  (goroutine)    │──►│                                   │   │
│  │                 │   └──────────────────────────────────-┘   │
│  │  tick → balance │                                           │
│  │  tick → test    │   ┌──────────────────────────────────┐   │
│  │  error hook     │   │  SQLite DB (persistent state)    │   │
│  └─────────────────┘   │                                   │   │
│                         │  tables: upstreams, access_keys, │   │
│  ┌─────────────────┐   │          usage_records           │   │
│  │  ocp CLI        │   └──────────────────────────────────-┘   │
│  │  (same binary,  │                                           │
│  │   subcommand)   │──► Management API (:8080/api/*)           │
│  └─────────────────┘                                           │
└─────────────────────────────────────────────────────────────────┘

External:
  Client (Claude Code / Codex / curl)
    │  OpenAI or Anthropic format request
    ▼
  ocp proxy endpoint
    │  translated + forwarded
    ▼
  Upstream (Minimax / Kimi / GLM / Qwen / …)
    │  response (streaming SSE or JSON)
    ▼
  ocp proxy endpoint
    │  forwarded as-is (or re-formatted)
    ▼
  Client
```

**Single process, single binary.** The proxy data path, management API, web portal, and CLI all live in one Go binary. The portal is embedded via `go:embed`. The CLI is a subcommand of the same binary (`ocp server` vs `ocp status`).

---

## Request Data Flow

### Step-by-step: a streaming chat completion request

```
1. Client sends POST /v1/chat/completions
   Authorization: Bearer <ocp-access-key>
   Body: { "model": "...", "messages": [...], "stream": true }

2. AuthMiddleware
   - Parse Bearer token from Authorization header
   - Lookup access key in DB (or in-memory cache with TTL)
   - Validate: not expired, not blocked, within daily/minute rate limit
   - Attach key metadata to request context: allowed_upstreams, token_budget

3. RateLimitMiddleware
   - Check req/min and req/day counters (in-memory, keyed by access key ID)
   - Increment counter; reject 429 if over limit

4. RelayHandler
   - Detect inbound format: OpenAI (/v1/chat/completions) or Anthropic (/v1/messages)
   - Select upstream from pool (see Round-Robin section)
   - If upstream requires different format, translate request (see Format Translation)

5. Upstream HTTP call
   - Build outbound request: upstream baseURL + path, upstream API key header
   - Set timeout; use http.Client with optional SOCKS5 proxy for GFW traversal
   - For streaming: read response body as chunked stream

6. Response forwarding
   - Non-streaming: read full body, re-format if needed, write to client
   - Streaming: set response headers (Content-Type: text/event-stream,
     X-Accel-Buffering: no), pipe chunks to client using http.Flusher
     without accumulating in memory

7. Error handling during request
   - 429 / 503 / timeout → mark upstream on cooldown, retry with next upstream
   - Exhausted credits signal (upstream-specific error code) → mark upstream
     dead until health check recovers it
   - Non-retryable (400 Bad Request) → return error to client immediately

8. Usage logging (async, post-response)
   - Write usage_record to DB: key_id, upstream_id, model, prompt_tokens,
     completion_tokens, latency_ms, status_code, timestamp
   - Update key's remaining token budget if budget is configured
```

### Key invariant
Steps 1–4 are synchronous and in-path (must complete before upstream call). Step 8 is fire-and-forget via a buffered channel drained by a writer goroutine. This keeps the hot path latency-clean.

---

## Data Model

### upstreams

```sql
CREATE TABLE upstreams (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,        -- "kimi-pro", "glm-4"
    provider     TEXT NOT NULL,               -- "openai-compat" | "anthropic-compat"
    base_url     TEXT NOT NULL,               -- "https://api.moonshot.cn/v1"
    api_key      TEXT NOT NULL,               -- encrypted at rest recommended
    models       TEXT NOT NULL,               -- JSON array of model names this key covers
    enabled      INTEGER NOT NULL DEFAULT 1,  -- 0 = admin-disabled
    priority     INTEGER NOT NULL DEFAULT 0,  -- for future weighted routing; 0 = equal
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
```

Runtime health state is NOT stored in this table. It lives in the in-memory upstream pool struct and is reconstructed on restart (a 10-second health check catches dead upstreams within one cycle after restart).

### access_keys

```sql
CREATE TABLE access_keys (
    id               INTEGER PRIMARY KEY,
    key_hash         TEXT NOT NULL UNIQUE,   -- SHA-256 of the raw key; never store raw
    name             TEXT,                   -- human label
    allowed_upstreams TEXT,                  -- NULL = all; JSON array of upstream IDs
    token_budget     INTEGER,                -- NULL = unlimited; remaining tokens
    rate_limit_rpm   INTEGER,                -- NULL = unlimited; requests per minute
    rate_limit_rpd   INTEGER,                -- NULL = unlimited; requests per day
    expires_at       INTEGER,                -- NULL = never; unix timestamp
    blocked          INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL,
    last_used_at     INTEGER
);
```

### usage_records

```sql
CREATE TABLE usage_records (
    id               INTEGER PRIMARY KEY,
    key_id           INTEGER NOT NULL REFERENCES access_keys(id),
    upstream_id      INTEGER REFERENCES upstreams(id),  -- NULL if request failed pre-routing
    model            TEXT,
    prompt_tokens    INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms       INTEGER,
    status_code      INTEGER,
    error_type       TEXT,    -- "rate_limit" | "credits_exhausted" | "timeout" | NULL
    created_at       INTEGER NOT NULL
);
CREATE INDEX usage_records_key_id_created ON usage_records(key_id, created_at);
CREATE INDEX usage_records_upstream_id    ON usage_records(upstream_id, created_at);
```

### In-memory upstream pool

```go
type UpstreamStatus int

const (
    StatusActive    UpstreamStatus = iota
    StatusCooldown                 // temporary; retried after CooldownUntil
    StatusDead                     // permanent until health check re-enables
    StatusDisabled                 // admin-disabled; never retried
)

type UpstreamState struct {
    mu               sync.RWMutex
    ID               int64
    Status           UpstreamStatus
    CooldownUntil    time.Time
    ConsecutiveErrs  int
    LastBalance      float64   // -1 = unknown
    LastBalanceCheck time.Time
    LastTestAt       time.Time
}

type Pool struct {
    mu       sync.RWMutex
    states   []*UpstreamState   // indexed by position in slice; matches DB order
    rrIndex  atomic.Uint64      // monotonically incrementing; mod len on read
}
```

The pool is loaded at startup from the DB and updated on: admin add/remove/enable/disable, health check results, in-path error signals.

---

## Round-Robin + Failover Design

### Selection algorithm

```
func (p *Pool) Select(allowedIDs []int64) (*UpstreamState, error) {
    p.mu.RLock()
    candidates := filter(p.states, func(s) bool {
        return s.Status == StatusActive &&
               (allowedIDs == nil || contains(allowedIDs, s.ID))
    })
    p.mu.RUnlock()

    if len(candidates) == 0 {
        return nil, ErrNoUpstreamAvailable
    }

    idx := p.rrIndex.Add(1) - 1
    return candidates[idx % uint64(len(candidates))], nil
}
```

This is weighted-equal round-robin. The atomic counter is never reset (avoids lock); modulo wraps naturally. Filtering happens before modulo so the round-robin distributes only over live upstreams, not the whole configured set.

### Failover on error

```
attempt := 0
maxAttempts := min(3, len(activePool))

for attempt < maxAttempts {
    upstream = pool.Select(allowedIDs)
    resp, err = doRequest(upstream, req)

    if err == nil && resp.StatusCode < 500 {
        break  // success
    }

    switch classifyError(err, resp) {
    case ErrRateLimit:
        upstream.SetCooldown(60 * time.Second)
    case ErrCreditsExhausted:
        upstream.SetDead()
        notifyHealthMonitor(upstream)
    case ErrTimeout, ErrServerError:
        upstream.SetCooldown(30 * time.Second)
    case ErrBadRequest:
        return errToClient  // client error; don't retry
    }
    attempt++
}
```

### Cooldown vs Dead distinction

| Condition | State | Recovery |
|-----------|-------|----------|
| 429 rate limit | Cooldown (60s default) | Automatic after timer |
| 5xx transient | Cooldown (30s) | Automatic after timer |
| Persistent 5xx (N consecutive) | Dead | Health check goroutine only |
| Credits exhausted (detected) | Dead | Health check detects balance > threshold |
| Admin disabled | Disabled | Manual re-enable only |

### In-memory vs persistent state

Cooldown and consecutive error counts are **in-memory only**. They are intentionally ephemeral: a restart resets counters and begins afresh. The health check goroutine will re-detect genuinely dead upstreams within one poll cycle (default 60s). This avoids stale "dead" state persisting across restarts and keeps the DB write path out of the hot loop.

If ocp is ever run multi-instance, a Redis layer would be needed for shared cooldown state (same pattern as LiteLLM). For the personal/team single-instance use case, in-memory is correct.

---

## Format Translation (Anthropic ↔ OpenAI)

### The problem space

ocp must accept requests in BOTH formats from clients, then forward to upstreams that may also be in either format. In practice for this project:

- All target upstreams (Minimax, Kimi, GLM, Qwen, etc.) expose **OpenAI-compatible** APIs
- Clients may send in either OpenAI or Anthropic format (Claude Code sends Anthropic format)

The translation matrix is therefore:

```
Inbound format    Upstream format    Action
─────────────────────────────────────────────────
OpenAI            OpenAI-compat      Pass-through (no transform)
Anthropic         OpenAI-compat      Translate request + response
OpenAI            Anthropic-compat   Translate request + response (future)
Anthropic         Anthropic-compat   Pass-through (future)
```

For v1, only the first two rows need implementation.

### Key structural differences

```
Field             OpenAI                      Anthropic
────────────────────────────────────────────────────────
Endpoint          /v1/chat/completions        /v1/messages
System prompt     messages[{role:"system"}]   top-level "system" string
Content           message.content (string)    content[] (typed blocks)
Tool calls        tool_calls array            tool_use content blocks
Stop reason       finish_reason               stop_reason
Response type     choices[0].message          content[] + stop_reason
Stream event      data: {...}                 data: {...} (different schema)
Auth header       Authorization: Bearer KEY   x-api-key: KEY
Version header    (none)                      anthropic-version: 2023-06-01
```

### Translation approach: thin adapter layer

Do NOT use a general-purpose library for this. The project's upstream set is small and their APIs are OpenAI-compatible. The translation needed is Anthropic-in → OpenAI-out (for the relay path) and OpenAI-response-out → Anthropic-response-out (for the response path).

Implement as two pure functions:

```go
// AnthropicToOpenAI converts an Anthropic /v1/messages request body
// into an OpenAI /v1/chat/completions request body.
func AnthropicToOpenAI(req anthropic.MessagesRequest) openai.ChatRequest

// OpenAIToAnthropic converts an OpenAI chat completion response
// into an Anthropic messages response.
func OpenAIToAnthropic(resp openai.ChatResponse) anthropic.MessagesResponse

// Same pair for streaming:
func AnthropicStreamToOpenAIStream(chunk anthropic.StreamEvent) openai.StreamChunk
func OpenAIStreamToAnthropicStream(chunk openai.StreamChunk) anthropic.StreamEvent
```

These are table-driven struct mappings — no dynamic dispatch, no reflection. Keep them in a `translate/` package. Each function has a clear input/output type so they are trivially unit-testable.

### Streaming translation

SSE streaming events have different schemas between providers. The safest approach:

1. Read each `data: {...}\n\n` chunk from the upstream as raw bytes
2. Unmarshal into the upstream's event type
3. Marshal into the client's expected event type
4. Write `data: <marshalled>\n\n` and flush

This adds a marshal/unmarshal per chunk but keeps the code correct. For the token rates involved (tens of tokens/second), this overhead is negligible.

**Do not** attempt byte-level string replacement on SSE events — it breaks when field names or values contain special characters.

---

## Health Monitoring Design

### Two-track monitoring

```
Track 1: Balance polling (where available)
  - Goroutine wakes on ticker (default: every 5 minutes)
  - For each enabled upstream, call provider's balance/quota API if known
  - Update upstream.LastBalance + LastBalanceCheck
  - If balance < threshold (e.g., ¥1.00 equivalent), mark Dead

Track 2: Reactive error detection (in-path)
  - RelayHandler signals health monitor via channel on fatal errors
  - Health monitor updates upstream state immediately
  - No polling needed for "credits exhausted" errors that surface in-path

Track 3: Active test requests (fallback for providers with no balance API)
  - Goroutine wakes on slower ticker (default: every 10 minutes) or when
    upstream transitions from Cooldown → Active
  - Send minimal test request (cheapest model, 1-token prompt)
  - Success → confirm Active; failure → re-evaluate status
```

### Balance API availability (research note)

The upstream providers targeted (Minimax, Kimi/Moonshot, Xiao, GLM/Zhipu, Qwen/Alibaba) each expose some form of account/balance API, but the specific endpoints are undocumented or require verification. **Each provider adapter must implement a `CheckBalance() (float64, error)` interface method**, with a no-op fallback returning `(-1, nil)` for providers where no balance API exists. Discovery of these endpoints is a per-provider research task, flagged as a build-time concern for the upstream adapter phase.

### Goroutine structure

```go
func (m *HealthMonitor) Run(ctx context.Context) {
    balanceTicker := time.NewTicker(5 * time.Minute)
    testTicker    := time.NewTicker(10 * time.Minute)

    for {
        select {
        case <-ctx.Done():
            return
        case <-balanceTicker.C:
            m.pollAllBalances()
        case <-testTicker.C:
            m.testDeadUpstreams()
        case upstream := <-m.errorSignalCh:  // buffered channel, capacity 64
            m.handleErrorSignal(upstream)
        }
    }
}
```

Single goroutine, event-driven. No need for a worker pool at this scale (< 10 upstreams). The error signal channel is buffered to avoid blocking the hot request path.

---

## Portal + CLI Architecture

### Same process, same binary

Both the web portal and the CLI (`ocp`) are in the same Go binary as the proxy. This is the standard pattern for self-hosted tools (same approach as one-api, Caddy, Traefik).

```
cmd/
  ocp/
    main.go          -- entry point; dispatches on os.Args[1]

subcommands:
  server             -- starts the proxy+portal+management API
  status             -- calls /api/status, prints table
  next               -- calls /api/upstream/next, prints result
  keys               -- subcommands: list, create, block, unblock
  version            -- prints version
```

### Web portal

Built as a React SPA (or similar lightweight framework), compiled to static files, embedded in the binary via `go:embed`:

```go
//go:embed web/dist
var portalFS embed.FS

// Gin route: serve portal for all non-API routes
router.NoRoute(func(c *gin.Context) {
    if strings.HasPrefix(c.Request.URL.Path, "/api/") {
        c.JSON(404, gin.H{"error": "not found"})
        return
    }
    // serve from embedded FS; fall back to index.html for SPA routing
    serveEmbedded(c, portalFS)
})
```

The portal communicates with the management API over the same HTTP port. No separate service, no CORS complexity.

### CLI ↔ proxy communication

The CLI communicates with the running proxy via the **local HTTP management API** at `http://localhost:8080/api/`. This is the correct pattern for this use case:

- No Unix socket needed: the proxy is always local and the port is known
- Simple `curl`-equivalent using Go's `net/http` client
- Same API surface used by the web portal (no duplication)
- Claude Code slash commands can also `curl` this API directly without the CLI

The CLI reads the base URL from `~/.config/ocp/config.json` (or `$OCP_URL` env var), defaulting to `http://localhost:8080`. An auth token for the management API is also stored there so the CLI can authenticate against the proxy's admin API.

### Management API routes (sketch)

```
GET  /api/status              -- overall health, active upstream, pool state
GET  /api/upstreams           -- list all upstreams with health state
POST /api/upstreams/:id/next  -- force-rotate away from current upstream
POST /api/upstreams/:id/disable
POST /api/upstreams/:id/enable
GET  /api/keys                -- list access keys
POST /api/keys                -- create key
POST /api/keys/:id/block
POST /api/keys/:id/unblock
GET  /api/usage               -- usage stats (accepts ?key_id=&upstream_id=&from=&to=)
```

---

## Suggested Build Order

Dependencies flow upward. Each layer must be complete before the layer above it.

```
Phase 1: Core data layer + process skeleton
  ├── SQLite schema (upstreams, access_keys, usage_records)
  ├── DB access layer (CRUD, no business logic)
  ├── Config loading (YAML/env → startup config)
  └── Binary entrypoint with subcommand dispatch

Phase 2: Upstream pool + health monitor
  ├── In-memory Pool struct with round-robin Select()
  ├── Error classification and cooldown/dead state transitions
  ├── Health monitor goroutine (balance polling skeleton)
  └── Per-provider balance API adapters (Minimax, Kimi, GLM, Qwen, Xiao)
        [research required per provider; no-op fallback until confirmed]

Phase 3: Relay pipeline (OpenAI pass-through only)
  ├── Auth middleware (key lookup, rate limit)
  ├── OpenAI → upstream OpenAI-compat pass-through (no translation)
  ├── SSE streaming proxy (FlushInterval: -1 or manual http.Flusher)
  ├── Usage logging (async writer goroutine)
  └── Integration test: Claude Code pointed at ocp, round-robins across two upstreams

Phase 4: Anthropic format translation
  ├── AnthropicToOpenAI() request transformer
  ├── OpenAIToAnthropic() response transformer
  ├── Streaming event translation
  └── Integration test: Claude Code (Anthropic SDK) through ocp to OpenAI-compat upstream

Phase 5: Management API
  ├── All /api/* routes
  ├── Admin auth (separate static bearer token, not access key system)
  └── Integration tests for key lifecycle and upstream control

Phase 6: Web portal
  ├── React SPA (dashboard: upstream status, key table, usage charts)
  ├── go:embed integration
  └── Portal served from same process

Phase 7: CLI
  ├── `ocp status`, `ocp next`, `ocp keys` subcommands
  └── Claude Code slash command wrappers calling management API
```

### Critical path

Phases 1 → 2 → 3 are strictly sequential (each is a prerequisite). Phases 4 and 5 are independent of each other and can proceed in parallel after Phase 3. Phase 6 requires Phase 5 (it calls the management API). Phase 7 requires Phase 5 (same dependency).

### What must exist before what

| To build | Requires |
|----------|----------|
| Upstream selection | DB schema + Pool struct |
| Relay handler | Upstream selection + auth middleware |
| Usage logging | DB schema + relay handler (provides data) |
| Health monitor | Upstream pool + provider adapters |
| Format translation | Relay handler (plugs into it) |
| Management API | DB layer + Pool (reads/writes both) |
| Web portal | Management API (all portal calls go through it) |
| CLI | Management API |

---

## Key Architecture Decisions and Rationale

### SQLite, not Postgres

For a personal/team tool running on a single host, SQLite is appropriate. It removes the operational burden of a separate DB process, fits in a Docker container without a compose file, and is fast enough for the write rates involved (a few usage records per second at most). Migrate to Postgres if multi-instance deployment is ever needed.

### No Redis

LiteLLM uses Redis for multi-instance cooldown coordination. ocp is single-instance by design. In-memory state for the upstream pool is correct and simpler. If multi-instance is ever added, extract the pool state to Redis at that point.

### gin as the HTTP framework

One-api, new-api, and most Go proxy projects in this space use gin. It has efficient routing, good middleware support, and straightforward SSE support. The alternative (standard `net/http` with `httputil.ReverseProxy`) is usable but requires more boilerplate for middleware chaining. gin is the pragmatic choice.

### No external message queue

Usage records are written via a buffered in-process channel to a single writer goroutine. This is sufficient for the write rates involved and avoids introducing a queue dependency. If the process crashes, at most a few seconds of usage records may be lost — acceptable for a personal routing tool.

### Format detection by inbound path

Inbound format (OpenAI vs Anthropic) is detected by the request path:
- `/v1/chat/completions` → OpenAI format
- `/v1/messages` → Anthropic format

This is unambiguous and requires no content inspection. The relay handler dispatches to the appropriate code path before any upstream selection occurs.

---

## Sources

- [one-api architecture overview (DeepWiki)](https://deepwiki.com/songquanpeng/one-api/1-overview)
- [one-api GitHub repository](https://github.com/songquanpeng/one-api)
- [LiteLLM routing documentation](https://docs.litellm.ai/docs/routing)
- [LiteLLM load balancing documentation](https://docs.litellm.ai/docs/proxy/load_balancing)
- [LiteLLM Anthropic provider docs](https://docs.litellm.ai/docs/providers/anthropic)
- [Go httputil.ReverseProxy SSE issue #27816](https://github.com/golang/go/issues/27816)
- [Go httputil.ReverseProxy FlushInterval issue #41642](https://github.com/golang/go/issues/41642)
- [Building an SSE proxy in Go (Medium)](https://medium.com/@sercan.celenk/building-an-sse-proxy-in-go-streaming-and-forwarding-server-sent-events-1c951d3acd70)
- [sony/gobreaker circuit breaker](https://github.com/sony/gobreaker)
- [LiteLLM management CLI](https://docs.litellm.ai/docs/proxy/management_cli)
- [anthropic-proxy: Anthropic → OpenAI translation](https://github.com/maxnowack/anthropic-proxy)
- [Envoy AI Gateway control plane / Unix socket pattern](https://aigateway.envoyproxy.io/docs/concepts/architecture/control-plane/)
- [Round-robin load balancer in Go (DEV)](https://dev.to/vivekalhat/building-a-simple-load-balancer-in-go-70d)
- [Embedding frontend in Go binary with go:embed](https://leapcell.io/blog/embedding-frontend-assets-in-go-binaries-with-embed-package)
