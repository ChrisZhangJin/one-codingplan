# one-codingplan (ocp)

## What This Is

one-codingplan (ocp) aggregates multiple AI coding plan credentials (Minimax, Kimi, Xiao, GLM, Qwen and others) behind a single OpenAI-compatible and Anthropic-compatible endpoint. Users point their tools — Claude Code, Codex, or any API client — at ocp using one URL and one key, and ocp handles routing, failover, and credit tracking transparently.

**v1.0 shipped:** Single Go binary with embedded React portal, Cobra CLI, SQLite persistence, round-robin failover across all configured upstreams, and full Anthropic ↔ OpenAI translation.

## Core Value

A single endpoint that never goes down: when one upstream coding plan runs out of credits or hits a rate limit, ocp silently rotates to the next available one.

## Current State

**Version:** v1.0 (shipped 2026-04-17)
**Codebase:** ~7,400 Go LOC, ~1,400 TS/TSX LOC (React portal)
**Tech stack:** Go + Gin + GORM/SQLite + React/Vite/shadcn + Cobra
**Binary:** Single deployable binary with embedded web portal

All 11 phases complete (v1.0 + v1.1). The proxy is operational with:
- OpenAI and Anthropic relay endpoints with transparent translation
- Round-robin failover with per-provider error classification (credits/rate-limit/transient/model-config)
- Per-upstream `format` flag for native Anthropic upstreams (no translation)
- Admin API (access key lifecycle, upstream control, usage queries)
- Web portal at `/` (upstream health, key management)
- CLI (`ocp status`, `ocp next`, `ocp keys`)
- Docker deployment: multi-stage Dockerfile, docker-compose with service + CLI profiles, data persistence

## Current Milestone: v1.2 Codex + Portal UX

**Goal:** Add Codex CLI support via OpenAI Responses API translation, and fill two portal gaps — upstream creation and per-key usage visibility.

**Target features:**
- Codex CLI connects to ocp: accept `/v1/responses` requests, translate to `/v1/chat/completions`, forward to upstreams
- Web portal: add new upstream (name, base URL, API key, model override) without editing config.yaml manually
- Web portal: usage statistics page showing requests and tokens broken down per access key

## Previous Milestone: v1.1 Ops — COMPLETE

**Target features:**
- Per-key rate limits: per-minute and per-day request caps enforced on access keys, configurable via admin API and portal
- Docker deployment: service builds and runs cleanly in container via Dockerfile + docker-compose

## Requirements

### Validated (v1.0)

- ✓ SQLite persistence for upstreams, access keys, and usage records — v1.0
- ✓ Config-file-driven upstream management (YAML, survives restart) — v1.0
- ✓ Single Go binary with embedded HTTP server (Gin) — v1.0
- ✓ OpenAI-compatible proxy endpoint (`/v1/chat/completions`) with auth and failover — v1.0
- ✓ Transparent upstream failover (credits-exhausted, rate-limit retry, transient rotate) — v1.0
- ✓ SSE streaming passthrough with heartbeat — v1.0
- ✓ Usage logging per authenticated request — v1.0
- ✓ Anthropic-compatible proxy endpoint (`/v1/messages`) with Anthropic ↔ OpenAI translation — v1.0
- ✓ Access key lifecycle: issue, list, block/unblock, token budget, allowed upstreams, rate limit, expiry — v1.0
- ✓ Round-robin across allowed upstreams per access key — v1.0
- ✓ Force-rotate upstream via `ocp next` / management API — v1.0
- ✓ Web portal: upstream health dashboard and key management table — v1.0
- ✓ CLI: `ocp status`, `ocp next`, `ocp keys` — v1.0
- ✓ Per-upstream `format` flag — direct Anthropic passthrough for native Anthropic upstreams — v1.0
- ✓ Model/config error classification — `ClassModelNotSupported` permanently disables misconfigured upstream — v1.0

### Validated (v1.1)

- ✓ Per-minute and per-day request limits enforced on access keys — v1.1 (Phase 9-10)
- ✓ Rate limit status visible per key in web portal — v1.1 (Phase 10)
- ✓ Service builds and runs in Docker container via Dockerfile + docker-compose — v1.1 (Phase 11)
- ✓ CLI controller runs on-demand via docker compose run — v1.1 (Phase 11)

### Active (v1.2)

- [ ] OpenAI Responses API endpoint (`/v1/responses`) — accepts Codex CLI requests, translates to `/v1/chat/completions`
- [ ] Streaming support for `/v1/responses` (Codex uses streaming by default)
- [ ] Web portal: add new upstream via form (name, base URL, API key, model override)
- [ ] Web portal: usage statistics page with per-key request and token breakdown
- [ ] Backend API: usage aggregation endpoint for portal to query

### Deferred (v1.3+)

- [ ] Proactive upstream health polling — poll Kimi `/v1/users/me/balance` every 5 min; mark unhealthy when balance is zero
- [ ] Web portal upstream dashboard: real-time health, credits remaining, request counts per provider
- [ ] Usage charts in portal (requests/tokens over time, per-upstream breakdown)
- [ ] Prometheus-compatible `/metrics` endpoint

### Out of Scope

- Reselling or billing — personal/team routing layer, not a commercial gateway
- Non-OpenAI/non-Anthropic upstream API formats — all upstreams expose compatible APIs
- Session pinning to a specific upstream — force-next is sufficient
- OAuth / SSO for portal — static admin key sufficient for personal/team use
- Redis / Postgres backend — SQLite sufficient; multi-instance not a requirement
- Plugin system — configure upstreams via config, not plugins

## Context

- All upstream providers expose OpenAI-compatible or Anthropic-compatible APIs
- ocp runs as a self-hosted service (Docker container, China network context — GFW applies)
- `config.yaml` is gitignored (contains real API keys); upstream entries are config-file-driven
- Known gap: balance polling APIs — only Kimi has a documented programmatic endpoint; Minimax/GLM/Qwen use error-inference as fallback
- E2E test suite: 16 tests in `internal/server/e2e_test.go` covering health, relay, auth, format passthrough, ClassModelNotSupported, failover, admin API, streaming

## Constraints

- **Network**: Deployed in China — upstream API calls may need proxy config for some providers
- **Compatibility**: Must accept requests formatted for both OpenAI and Anthropic APIs
- **Extensibility**: New upstream providers addable without code changes (config-driven)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Round-robin (not priority-list) failover | Simpler UX — user doesn't need to rank plans; keys define allowed set | ✓ Good — no complaints from usage |
| Single force-next command (not pin-to-plan) | Keeps CLI surface minimal; most sessions don't need pinning | ✓ Good |
| Both web portal + CLI | Web for visibility and key management; CLI for quick ops | ✓ Good |
| Per-key allowed upstreams restrict the round-robin pool | Natural model: unrestricted key = all plans, restricted key = subset | ✓ Good |
| Pure-Go SQLite driver (glebarez) | No CGo — clean Alpine Docker builds | ✓ Good |
| Embed React portal in binary via go:embed | Single binary deployment, no separate dist/ needed | ✓ Good |
| Custom circuit breaker (no library) | HTTP-semantic health (429/402/5xx) requires custom logic | ✓ Good — 20-line implementation |
| ClassModelNotSupported scoped to 500/501 only | Gateway errors (502/503/504) should remain ClassTransient | ✓ Good — prevents false positives |
| Reuse proxyStream/proxyBuffer for Anthropic passthrough | No translation needed — verbatim copy functions already exist | ✓ Good — no new functions needed |

---
*Last updated: 2026-04-18 — v1.2 Codex + Portal UX milestone started*
