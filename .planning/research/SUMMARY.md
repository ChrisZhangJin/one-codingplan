# Research Summary — one-codingplan (ocp)

**Synthesized:** 2026-04-16
**Based on:** STACK.md, FEATURES.md, ARCHITECTURE.md, PITFALLS.md

---

## Recommended Stack

Go 1.22+ with Gin v1.10, GORM v2, and the pure-Go SQLite driver (`glebarez/sqlite`) is the correct stack — this is the same combination used by `one-api` and `new-api`, the established reference implementations for this exact problem class, and Go's goroutine model handles thousands of concurrent SSE connections without the GIL ceiling that caps Python/LiteLLM. The web portal is React 18 + Vite + shadcn/ui, built once and embedded in the Go binary via `//go:embed` so the entire system ships as a single Docker image with no external dependencies. CLI is Cobra + Viper; the circuit breaker is a custom 20-line state machine — no library needed.

---

## Table Stakes Features

These must work before ocp is useful for its stated purpose:

- OpenAI-compatible `/v1/chat/completions` endpoint (every AI coding tool uses this)
- Anthropic-compatible `/v1/messages` endpoint (Claude Code native format)
- Bearer token auth on incoming requests (prevents unauthorized credit consumption)
- SSE streaming pass-through without buffering (blocking mode is unusable for coding sessions)
- Upstream key round-robin with automatic failover on 429/5xx/timeout
- Per-provider credit exhaustion detection (each provider uses different error codes — see pitfalls)
- Access key issuance, listing, block/unblock
- `GET /health` status endpoint (used by Claude Code slash commands)
- Persistent config in SQLite (survives restarts)

**Defer to v2:** semantic caching, per-session upstream pinning, SSO, Redis/Postgres, webhook alerts, plugin system.

---

## Architecture Overview

Single Go binary, single process, single port (8080). The HTTP listener serves three surfaces: proxy endpoints (`/v1/*`), management API (`/api/*`), and the embedded React portal (`/*`). The in-memory upstream pool holds runtime health state (cooldown timers, consecutive error counts, last balance); the SQLite DB holds persistent config (upstream credentials, access keys, usage records). A background health monitor goroutine runs two tracks: balance polling (every 5 min, Kimi only — no other provider exposes a balance API) and active test requests (every 10 min for dead upstreams).

The relay pipeline is a sequential middleware chain: auth → rate limit → upstream selection → optional format translation → HTTP proxy → async usage log. Format detection is by inbound path: `/v1/chat/completions` = OpenAI, `/v1/messages` = Anthropic. All target upstreams are OpenAI-compatible, so the only translation needed in v1 is Anthropic-in → OpenAI-out (request) and OpenAI-out → Anthropic-out (response), implemented as pure typed functions in a `translate/` package. The CLI is a subcommand of the same binary that calls the management API over localhost HTTP — no direct DB access, no embedded business logic.

**Build order (strict dependencies):** DB schema → upstream pool → relay pipeline (OpenAI pass-through) → format translation → management API → web portal → CLI. Phases 4 (translation) and 5 (management API) are independent once Phase 3 is done.

---

## Critical Pitfalls to Avoid

**P1 — Nginx proxy buffering kills SSE streaming.** Default `proxy_buffering on` accumulates SSE chunks and delivers them in bursts. Set `proxy_buffering off`, `gzip off`, and `proxy_read_timeout 600s` on all `/v1/` routes. Also set `X-Accel-Buffering: no` in Go response headers so any intermediate proxy knows to flush immediately.

**P2 — 429 = "rate limited" vs 429 = "credits exhausted" are different.** Every Chinese provider uses 429 for both conditions but distinguishes them in the response body. Inspect the error body: Kimi uses `type: exceeded_current_quota_error` for dead-key vs `type: rate_limit_error` for backoff. Minimax uses status codes 1008 vs 1002. Misclassifying this causes permanent failover away from healthy keys or infinite retry against dead ones.

**P3 — Round-robin index race condition.** Use `atomic.Uint64` for the RR counter and `sync.RWMutex` per upstream state. Run `go test -race` before declaring the pool correct. The race is invisible under low load.

**P4 — Anthropic tool_use/tool_result ordering.** Anthropic requires every `tool_use` assistant block to be immediately followed by `tool_result` in the next user message. Naive OpenAI→Anthropic message history mapping breaks multi-turn tool-call sessions with a 400 error on turn 2.

**P5 — MiniMax emits `"role": ""` in streaming delta chunks.** OpenAI SDK clients reject this. The MiniMax stream transformer must strip empty `role` fields before forwarding.

**P6 — SSE flush omission truncates responses.** After each upstream chunk, call `http.Flusher.Flush()`. After the upstream sends `data: [DONE]`, forward it and flush before closing. Also send SSE heartbeat comments (`: \n\n`) every 15–30 seconds to prevent intermediate proxies from killing idle long-running generations.

**P7 — Kimi `reasoning_content` must be preserved in conversation history.** When Kimi's thinking mode is active, assistant messages with tool calls must include `reasoning_content` in subsequent-turn history or the API returns 400. Strip it for non-Kimi upstreams, include it for Kimi — per-upstream message reconstruction, not a single normalization pass.

---

## Open Questions

These are unresolved as of research completion and require investigation with actual API keys in Phase 1/2:

1. **"Xiao" provider identity.** PROJECT.md lists Xiao as a named upstream; no public AI coding plan API matching this name was found. Treat as config-only (base URL + key fields) with no balance polling until identified. Does not block anything — the extensible upstream config handles unknown providers gracefully.

2. **Balance APIs for Minimax, GLM, Qwen.** Only Kimi has a documented programmatic balance endpoint (`GET /v1/users/me/balance`). Minimax, GLM, and Qwen are web-dashboard-only. This is confirmed with MEDIUM confidence — there may be undocumented endpoints. The design uses error-inference as fallback for all three, which works but is reactive not proactive.

3. **Qwen region key compatibility.** China and international DashScope keys are not interchangeable. The ocp instance is China-hosted, so the domestic endpoint should work. Confirm which key type is in use and which endpoint to configure.

4. **Upstream connectivity from inside Docker.** Domestic providers (DashScope, GLM) should be reachable without a proxy from inside China. Non-domestic providers or GFW-adjacent endpoints may need the SOCKS5 proxy configured per-upstream in the HTTP client. Verify connectivity for each provider from inside the container before declaring the proxy core working.

5. **MiniMax coding plan key type.** Minimax issues separate keys for the "coding plan" product vs pay-as-you-go. Confirm the key type and whether the same error codes apply to both.

---

## Phase Implications

Research supports a 7-phase structure matching the architecture's dependency graph. Phases 1–3 are the critical path and must be sequential. Phases 4 and 5 can run in parallel.

**Phase 1 — Project skeleton and data layer**
Establish: Go module, Gin server, SQLite schema (upstreams, access_keys, usage_records), GORM setup, config loading (YAML/env), binary entrypoint with subcommand dispatch, outbound HTTP client with SOCKS5 support. No business logic yet, but the foundation everything else runs on.
_Pitfalls to address:_ D3 (SOCKS5 proxy for GFW), D4 (HTTP keep-alive), P3 (atomic RR index from day one).

**Phase 2 — Upstream pool and health monitoring**
Establish: in-memory Pool with round-robin Select(), per-provider error classification table (P2), cooldown/dead state machine, health monitor goroutine (balance polling for Kimi; error-inference for others), per-provider adapters for the five target providers including MiniMax stream sanitizer (P5).
_Research flag:_ Requires actual API keys to verify error codes and test connectivity.

**Phase 3 — Relay pipeline (OpenAI pass-through)**
Establish: auth middleware, rate-limit middleware, relay handler wiring upstream pool selection to `httputil.ReverseProxy`, SSE streaming with `FlushInterval: -1`, SSE heartbeat goroutine, async usage logging, failover retry loop with non-retryable error classification (M6).
_Pitfalls to address:_ P1 (Nginx buffering config), M2 (flush), M3 (timeout 600s), M6 (retry logic). Integration test: Claude Code pointed at ocp, round-robins across two live upstreams.

**Phase 4 — Anthropic format translation**
Establish: `AnthropicToOpenAI()` / `OpenAIToAnthropic()` typed pure functions and their streaming equivalents in `translate/` package, message history normalizer for tool_use ordering (P4), Kimi `reasoning_content` preservation (P7).
_Note:_ This phase is independent of Phase 5 and can overlap.

**Phase 5 — Management API**
Establish: all `/api/*` routes, admin bearer auth (static key, not access key system), key lifecycle endpoints, upstream enable/disable/rotate endpoints, usage query endpoint.
_Note:_ Independent of Phase 4. Portal and CLI both depend on this.

**Phase 6 — Web portal**
Establish: React SPA (upstream status dashboard, key management table, usage charts), Vite build pipeline, `go:embed` integration, SPA routing fallback in Gin.
_Depends on:_ Phase 5.

**Phase 7 — CLI and Claude Code integration**
Establish: `ocp status`, `ocp next`, `ocp keys` Cobra subcommands, `.claude/commands/proxy-status.md` and `proxy-next.md` slash commands.
_Depends on:_ Phase 5.

**Phases needing deeper research before implementation:**
- Phase 2 (provider-specific error codes and balance APIs — requires live keys)
- Phase 3 (SOCKS5 connectivity per provider — requires container environment)

**Phases with well-documented patterns (skip additional research):**
- Phase 1 (standard Go/Gin/GORM setup, extensively documented)
- Phase 4 (translation logic fully mapped in FEATURES.md + ARCHITECTURE.md)
- Phase 6 (go:embed + Vite pattern has multiple verified tutorials)
- Phase 7 (Cobra CLI + Claude Code skill pattern fully specified)

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Go + Gin + GORM + SQLite stack | HIGH | Multiple production reference projects; no ambiguity |
| SSE streaming proxy implementation | HIGH | Go stdlib FlushInterval behavior confirmed; patterns verified |
| Round-robin + failover design | HIGH | Architecture is straightforward; race condition mitigation well-documented |
| Anthropic ↔ OpenAI format translation | HIGH | Field mapping fully specified; the streaming translation is the hard part but approach is clear |
| Kimi balance API | HIGH | Documented endpoint with confirmed schema |
| Minimax/GLM/Qwen balance APIs | MEDIUM | No documented endpoint found; error-inference fallback is confirmed viable |
| Provider error code classification | MEDIUM | Research found most codes; needs live-key verification before trusting |
| "Xiao" provider | LOW | Unknown — cannot plan this upstream beyond a config placeholder |
| MiniMax streaming quirks | HIGH | Two bugs documented with confirmed issue references; mitigations clear |
| Docker/GFW connectivity | MEDIUM | Domestic providers should work; per-provider proxy config needs container-level testing |

**Overall: MEDIUM-HIGH.** Core architecture is high confidence. Provider-specific details (especially error codes and "Xiao") need Phase 1/2 validation with real credentials.
