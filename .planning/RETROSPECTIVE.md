# Retrospective

## Milestone: v1.0 MVP

**Shipped:** 2026-04-17
**Phases:** 8 | **Plans:** 18 | **Timeline:** 2 days (2026-04-16 → 2026-04-17)

### What Was Built

- Go module skeleton with Viper+YAML config, GORM+SQLite, and Gin HTTP server
- In-memory upstream pool: round-robin, per-provider error classifier, hourly recovery probe
- OpenAI relay with auth middleware, failover loop, SSE streaming with heartbeat, usage logging
- Full Anthropic ↔ OpenAI translation layer: request, response, streaming, tool use
- Management API: 9 endpoints — access key CRUD, token budgets, rate limits, expiry, allowed upstreams
- React+Vite+shadcn web portal embedded in binary via `go:embed`
- Cobra CLI: `ocp status`, `ocp next`, `ocp keys`
- Per-upstream `format` flag for direct Anthropic passthrough
- `ClassModelNotSupported` error classifier to permanently disable misconfigured upstreams
- 16-test E2E suite covering the full stack

### What Worked

- **Phase decomposition was clean.** Each phase had a clear boundary and could be executed without ambiguity. The dependency graph (linear through phases 1–4, then branching to 5/6/7/8) played out cleanly.
- **TDD on the translator layer.** Writing types + pure functions with tests first (Phase 4) meant the handler wiring in Phase 4-03 was straightforward — no debugging needed.
- **Code review + fix workflow.** Running code review after each phase and fixing before moving on caught issues early (WR-01 retry loop, WR-02 silent stream errors, WR-03 hop-by-hop headers, WR-04 classifier scope).
- **Reusing proxyStream/proxyBuffer for Anthropic passthrough.** No new functions needed — the verbatim copy helpers from the OpenAI relay worked perfectly for the format passthrough path.
- **Custom circuit breaker.** 20-line implementation with atomic ops — no external library needed, and HTTP-semantic logic (429/402/5xx handling) required custom behavior anyway.

### What Was Inefficient

- **REQUIREMENTS.md never updated during execution.** The traceability table shows all "Pending" at v1.0 close. This was purely a documentation gap — all requirements were implemented. Should update inline at each phase completion.
- **Some SUMMARY.md one-liners empty.** Several phases produced summaries with empty `one_liner` fields, which degraded the automated MILESTONES.md output. The one-liner field needs to be filled consistently.
- **No discuss-phase context for phases 1–8.** All phases were planned and executed without CONTEXT.md files (except a few later phases). This meant the planner operated from requirements and research only — acceptable for a well-defined technical build, but would have been better with explicit design decisions locked in upfront.

### Patterns Established

- Error classification: body-first keyword matching with per-provider overrides, status code gate for model errors (5xx only)
- Pool mark/recover: `Mark(id, false)` on credits/model errors, cooldown + hourly probe for recovery
- Relay handler structure: read body → parse → failover loop → classify error → success path (stream or buffer)
- Anthropic passthrough: branch on `current.Format == "anthropic"` before translation block; reuse existing proxy helpers
- E2E tests: `httptest.NewServer` for real HTTP stack, in-memory SQLite for database

### Key Lessons

- Phase 8 advisory anti-pattern: `ClassModelNotSupported` was initially matching all 5xx (including 502/503/504). Gateway errors should remain `ClassTransient` — only 500/501 with model/config keywords warrant permanent disable.
- Stream context lifetime: `resp.Body` must remain readable for the full stream duration; canceling the context before the stream is read causes silent truncation.
- `proxyStream` heartbeat goroutine requires a mutex guard — goroutine and main loop both write to the response writer.

### Cost Observations

- Model mix: primarily sonnet (research, planning, checking), opus for planning agents
- Sessions: ~3-4 sessions across 2 days
- Notable: entire v1.0 built in 2 calendar days from blank project to working proxy with portal, CLI, and E2E tests

---

## Cross-Milestone Trends

| Metric | v1.0 |
|--------|------|
| Days to complete | 2 |
| Phases | 8 |
| Plans | 18 |
| Go LOC | ~7,400 |
| TS/TSX LOC | ~1,400 |
| Test coverage | E2E (16 tests) + unit per subsystem |
| Rework sessions | 1 (code review fixes) |
