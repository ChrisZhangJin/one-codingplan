# Milestones

## v1.0 MVP (Shipped: 2026-04-17)

**Phases completed:** 8 phases, 18 plans
**Timeline:** 2026-04-16 → 2026-04-17 (2 days)
**Stats:** 172 files, ~35,300 lines added, ~7,400 Go LOC, ~1,400 TS/TSX LOC, 137 commits

**Key accomplishments:**

- Go module skeleton with Viper+YAML config, GORM+SQLite models, and Gin HTTP server with health endpoint
- Gin server startup wiring: router, middleware chain, health endpoint, bootable single binary
- In-memory upstream pool with round-robin Select/Mark/Stop, backed by SQLite persistence, and body-first per-provider error classifier distinguishing credits-exhausted from rate-limited from transient
- Hourly background probe goroutine recovers unavailable upstreams via minimal chat completion; config extended with `pool.rate_limit_backoff`
- Gin auth middleware + failover relay loop; non-streaming proxy; async SQLite usage logging behind `/v1/chat/completions`
- `proxyStream` with per-chunk flushing, mutex-guarded heartbeat goroutine, and context lifetime fix for SSE streams
- Pure-function `AnthropicToOpenAI` translator: tool use, system prompt, model override, json.RawMessage input passthrough
- OpenAI-to-Anthropic response translator with stateful SSE frame buffering and `StreamTranslator`
- `/v1/messages` handler: failover relay loop, tool_use handling, stream translation integration, integration tests
- Admin API: 9 endpoints with constant-time admin key auth, access key CRUD, token budgets, expiry, allowed-upstream restrictions, rate limits
- Limit enforcement middleware: `Pool.Select` filter, `ForceRotate`, upstream management endpoints
- React+Vite+shadcn/ui web portal scaffold embedded in binary via `go:embed`; Makefile build integration
- Login page, auth context, API client, and dashboard shell with React Router
- Upstream status cards and key management table with create/block/unblock dialogs
- Cobra CLI foundation: `ocp serve` subcommand, root persistent flags, pool `Position` field
- `ocp status`, `ocp next`, `ocp keys` subcommands wired to management API
- Per-upstream `format` flag for direct Anthropic passthrough: raw body forwarded to `/v1/messages` verbatim when `format: anthropic`
- `ClassModelNotSupported` error classifier (5xx + model/config keywords) — both relay handlers mark upstream unavailable; 16-test E2E suite

**Archive:** `.planning/milestones/v1.0-ROADMAP.md` · `.planning/milestones/v1.0-REQUIREMENTS.md`

---
