# Roadmap: one-codingplan (ocp)

**Milestone:** v1.0
**Phases:** 8
**Requirements:** 25 v1 requirements

---

## Phases

- [ ] **Phase 1: Project Skeleton & Data Layer** — Go module, Gin server, SQLite schema, GORM setup
- [ ] **Phase 2: Upstream Pool & Health Monitor** — In-memory pool with round-robin, per-provider error classification, cooldown state machine
- [ ] **Phase 3: Relay Pipeline (OpenAI Pass-Through)** — Auth middleware, relay handler, SSE streaming, failover retry, async usage logging
- [ ] **Phase 4: Anthropic Format Translation** — Bidirectional Anthropic ↔ OpenAI translation, tool_use ordering, streaming equivalents
- [ ] **Phase 5: Management API** — All `/api/*` routes, admin auth, key lifecycle, upstream control, usage queries
- [ ] **Phase 6: Web Portal** — React SPA embedded in binary, upstream status dashboard, key management table
- [ ] **Phase 7: CLI** — `ocp status`, `ocp next`, `ocp keys` Cobra subcommands

---

## Phase Details

### Phase 1: Project Skeleton & Data Layer
**Goal:** The binary starts, serves a health endpoint, and persists upstream credentials and access keys to SQLite.
**Depends on:** Nothing
**Requirements:** UPST-01, USGR-02
**Success Criteria:**
1. `go run ./cmd/ocp` starts without error and responds `200 OK` to `GET /health`
2. Admin can add, edit, and remove upstream provider entries (name, base URL, API key, enabled flag) via config file, and entries survive a binary restart
3. SQLite database file is created on first run; tables for upstreams, access_keys, and usage_records exist with correct schema
**Plans:** 2 plans

Plans:
- [x] 01-01-PLAN.md — Go module + config + models + database layer
- [x] 01-02-PLAN.md — Gin HTTP server + main.go integration

### Phase 2: Upstream Pool & Health Monitor
**Goal:** The proxy can select an upstream from the in-memory pool via round-robin, automatically marks upstreams unhealthy on error, applies per-provider cooldown timers, and returns cooled-down upstreams to the active pool when they recover.
**Depends on:** Phase 1
**Requirements:** UPST-02, UPST-03, ROUT-01, ROUT-02, ROUT-03
**Success Criteria:**
1. With two upstreams configured, successive calls to the pool's `Select()` function return them in alternating order (round-robin confirmed by unit test run with `-race` flag)
2. When an upstream returns a credits-exhausted error response, the pool marks it unhealthy and excludes it from subsequent selections for at least the configured cooldown duration
3. When an upstream returns a rate-limit error, the pool applies a backoff and retries the same upstream rather than rotating away
4. After a cooldown period expires, the pool re-tests the upstream with a probe request and returns it to the active pool on success
5. Per-provider error classification correctly distinguishes credits-exhausted from rate-limit from transient error for all five target providers (verified against captured error response fixtures)
**Plans:** 2 plans

Plans:
- [x] 02-01-PLAN.md — Pool struct + classifier with TDD (Select, Mark, round-robin, per-provider error classification)
- [x] 02-02-PLAN.md — Probe goroutine + config extension + server/main wiring

### Phase 3: Relay Pipeline (OpenAI Pass-Through)
**Goal:** A client can send an OpenAI-format chat completion request (streaming or non-streaming) to ocp, the request is authenticated, forwarded to the selected upstream, and the response or SSE stream is returned to the client — with automatic failover to the next upstream on failure — while every request is logged to SQLite.
**Depends on:** Phase 2
**Requirements:** PRXY-01, PRXY-03, USGR-01
**Success Criteria:**
1. Claude Code (or `curl`) pointed at `http://localhost:8080/v1/chat/completions` with a valid ocp bearer token receives a complete, well-formed OpenAI response from the active upstream
2. SSE streaming responses arrive token-by-token at the client with no buffering delay; the `data: [DONE]` terminator is forwarded and the connection closes cleanly
3. When the active upstream returns an error or times out mid-request, the proxy retries on the next available upstream and the client receives a successful response (failover is transparent)
4. A request with a missing or invalid bearer token receives `401 Unauthorized` and is not forwarded to any upstream
5. After a proxied request completes, a usage record (key ID, upstream, token counts, latency, status) is readable in the SQLite `usage_records` table and survives a proxy restart
**Plans:** 2 plans

Plans:
- [x] 03-01-PLAN.md — Auth middleware + relay handler with failover, non-streaming proxy, usage logging
- [x] 03-02-PLAN.md — SSE streaming passthrough with heartbeat and streaming tests

### Phase 4: Anthropic Format Translation
**Goal:** A client can send a native Anthropic-format request to `/v1/messages` and receive a valid Anthropic-format response, with ocp transparently translating to and from OpenAI format on the wire to the upstream.
**Depends on:** Phase 3
**Requirements:** PRXY-02, PRXY-04
**Success Criteria:**
1. Claude Code in native Anthropic mode pointed at `http://localhost:8080/v1/messages` receives a valid `200` response with correct Anthropic response schema (including `content` array and `stop_reason`)
2. SSE streaming via `/v1/messages` delivers Anthropic-format event types (`content_block_delta`, `message_delta`, `message_stop`) token-by-token with no buffering
3. A multi-turn conversation with tool use (tool_use / tool_result blocks) completes without a `400` error on any turn, confirming correct message-ordering translation
4. Non-Anthropic clients hitting `/v1/chat/completions` are unaffected by the translation layer (regression: OpenAI pass-through still works)
**Plans:** 3 plans

Plans:
- [x] 04-01-PLAN.md — Types + request translator (AnthropicToOpenAI) + config/pool ModelOverride
- [x] 04-02-PLAN.md — Response translator (OpenAIToAnthropic) + StreamTranslator SSE re-framing
- [x] 04-03-PLAN.md — /v1/messages handler wiring + integration tests + regression

### Phase 5: Management API
**Goal:** All access key lifecycle operations and upstream control actions are available via authenticated HTTP endpoints at `/api/*`, enabling the portal and CLI to be built against a stable contract.
**Depends on:** Phase 3
**Requirements:** KEY-01, KEY-02, KEY-03, KEY-04, KEY-05, KEY-06, ROUT-04
**Success Criteria:**
1. `POST /api/keys` issues a new `ocp-`-prefixed bearer token; the key is immediately active and accepted by the proxy relay
2. `GET /api/keys` returns all keys with their status, configured limits (token budget, expiry, allowed upstreams), and cumulative usage totals
3. `POST /api/keys/:id/block` and `POST /api/keys/:id/unblock` toggle key status; a blocked key's next request receives `401` and an unblocked key's next request succeeds
4. A key with a token budget set receives `429` when its cumulative token usage would exceed the budget; a key restricted to a subset of upstreams only routes to that subset
5. `POST /api/upstreams/rotate` (used by `ocp next`) forces the round-robin index to advance and the next proxied request uses the newly selected upstream
**Plans:** 2 plans

Plans:
- [x] 05-01-PLAN.md — Schema extension + admin middleware + key CRUD handlers
- [x] 05-02-PLAN.md — Limit enforcement + Pool.Select filter + ForceRotate + upstream endpoints

### Phase 6: Web Portal
**Goal:** An admin can open a browser, authenticate, and see the current upstream health status and the full access key table — all served from the single ocp binary with no external web server.
**Depends on:** Phase 5
**Requirements:** PORT-01, PORT-02
**Success Criteria:**
1. Navigating to `http://localhost:8080/` in a browser presents a login prompt; entering the admin key grants access to the portal
2. The upstream status section shows each configured provider with its current health state (healthy / cooldown / dead) and enabled/disabled toggle
3. The key management table lists all access keys with status, limits, and usage; admin can create a new key, block/unblock a key, and view its details without leaving the page
4. The React SPA is served entirely from the compiled Go binary (no separate `dist/` directory required at runtime); the binary size increases by the portal bundle but no external files are needed
**Plans:** 3 plans
**UI hint**: yes

Plans:
- [x] 06-01-PLAN.md — React/Vite scaffold + shadcn init + Go embed + Makefile
- [x] 06-02-PLAN.md — Login page + auth context + API client + dashboard shell
- [ ] 06-03-PLAN.md — Upstream status cards + key management table + dialogs + checkpoint

### Phase 7: CLI
**Goal:** An admin can run `ocp status`, `ocp next`, and `ocp keys` from the terminal against a running ocp instance and get actionable output, using the same binary as the server.
**Depends on:** Phase 5
**Requirements:** CLI-01, CLI-02, CLI-03
**Success Criteria:**
1. `ocp status` prints a table of configured upstreams with their health state and current round-robin position, sourced live from the management API
2. `ocp next` forces upstream rotation and prints confirmation of which upstream is now active; the next proxied request through the relay uses that upstream
3. `ocp keys` prints all access keys with their limits and cumulative usage in a readable table format
4. All three commands fail with a clear error message (not a stack trace) when the ocp server is not reachable
**Plans:** 2 plans

Plans:
- [x] 08-01-PLAN.md — Per-upstream format flag + direct Anthropic passthrough
- [x] 08-02-PLAN.md — Model/config error classification (ClassModelNotSupported)

### Phase 8: Upstream Format Flexibility
**Goal:** ocp can route requests natively to Anthropic-format upstreams without translation, and model/config errors disable the offending upstream rather than retrying indefinitely.
**Depends on:** Phase 4
**Requirements:** PRXY-05, ROUT-05
**Success Criteria:**
1. An upstream with `format: anthropic` in config receives the original Anthropic request body verbatim (no OpenAI translation); the client receives the upstream's Anthropic response directly
2. An upstream with `format: openai` (the default) behaves identically to the current translation path — no regression
3. When an upstream returns a "model not supported" or "invalid model" error (HTTP 5xx with provider-specific error codes), the pool marks it unavailable for the session rather than retrying on every request
4. `go test ./...` passes with tests covering the direct-passthrough path and the new error classification
**Plans:** 2 plans

Plans:
- [x] 08-01-PLAN.md — Per-upstream format flag + direct Anthropic passthrough
- [x] 08-02-PLAN.md — Model/config error classification (ClassModelNotSupported)

---

## Dependency Graph

```
Phase 1: Skeleton & Data Layer
    |
Phase 2: Upstream Pool & Health
    |
Phase 3: Relay Pipeline (OpenAI)
   / \
Ph4   Ph5
Anthro  Mgmt API
  |     /    \
 Ph8  Ph6    Ph7
Fmt  Portal  CLI
```

Phases 4 and 5 are independent of each other once Phase 3 is complete.
Phases 6 and 7 are independent of each other once Phase 5 is complete.
Phase 8 depends on Phase 4 (Anthropic translation) and can run independently of 6 and 7.

---

## Requirement Coverage

| Requirement | Phase | Description |
|-------------|-------|-------------|
| UPST-01 | Phase 1 | Configure upstream providers via config file or management API |
| ~~UPST-04~~ | ~~Phase 1~~ | ~~SOCKS5 outbound HTTP client~~ *(removed — descoped by D-05)* |
| USGR-02 | Phase 1 | Usage persisted to SQLite; survives restarts |
| UPST-02 | Phase 2 | Detect unhealthy upstreams reactively; mark with cooldown |
| UPST-03 | Phase 2 | Re-test cooled-down upstreams; return to pool on recovery |
| ROUT-01 | Phase 2 | Round-robin upstream selection per key's allowed pool |
| ROUT-02 | Phase 2 | Auto-rotate on credits-exhausted / rate-limit / error |
| ROUT-03 | Phase 2 | Per-provider error classification (exhausted vs rate-limit vs transient) |
| PRXY-01 | Phase 3 | OpenAI-format chat completions endpoint |
| PRXY-03 | Phase 3 | SSE streaming without buffering (both formats) |
| USGR-01 | Phase 3 | Per-request usage log (key, upstream, tokens, latency, status) |
| PRXY-02 | Phase 4 | Anthropic-format messages endpoint |
| PRXY-04 | Phase 4 | Anthropic ↔ OpenAI format translation |
| KEY-01 | Phase 5 | Issue ocp-prefixed bearer tokens via management API |
| KEY-02 | Phase 5 | List all keys with status, limits, usage |
| KEY-03 | Phase 5 | Block and unblock access keys |
| KEY-04 | Phase 5 | Token budget enforcement (429 on exceed) |
| KEY-05 | Phase 5 | Restrict key to subset of upstreams |
| KEY-06 | Phase 5 | Expiry date on key (401 when expired) |
| ROUT-04 | Phase 5 | Force-rotate upstream via management API / ocp next |
| PORT-01 | Phase 6 | Web portal at `/` with admin authentication |
| PORT-02 | Phase 6 | Key management table (list, create, block/unblock, limits, usage) |
| CLI-01 | Phase 7 | `ocp status` — upstream health and round-robin position |
| CLI-02 | Phase 7 | `ocp next` — force rotate active upstream |
| CLI-03 | Phase 7 | `ocp keys` — list keys with limits and usage |
| PRXY-05 | Phase 8 | Per-upstream format flag — direct passthrough for Anthropic-native upstreams |
| ROUT-05 | Phase 8 | Model/config error classification — mark upstream unavailable on persistent config errors |

**Coverage: 26/26 v1 requirements mapped.** (UPST-04 removed)

---

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Skeleton & Data Layer | 2/2 | Planned | — |
| 2. Upstream Pool & Health | 0/2 | Planned | — |
| 3. Relay Pipeline (OpenAI) | 0/2 | Planned | — |
| 4. Anthropic Translation | 0/3 | Planned | — |
| 5. Management API | 0/2 | Planned | — |
| 6. Web Portal | 0/? | Not started | — |
| 7. CLI | 0/? | Not started | — |
| 8. Upstream Format Flexibility | 0/? | Not started | — |

---
*Roadmap created: 2026-04-16*
