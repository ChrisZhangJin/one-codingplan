---
phase: 02-upstream-pool-health-monitor
plan: 02
subsystem: api
tags: [go, pool, probe, health-monitor, config, server, main]

requires:
  - phase: 02-upstream-pool-health-monitor
    plan: 01
    provides: internal/pool/pool.go (Pool, New, Select, Mark, Stop), internal/pool/classifier.go (Classify, ClassCreditsExhausted)

provides:
  - internal/pool/probe.go: runProbeLoop (hourly ticker), probeAll, sendProbe with 64KB body cap
  - internal/pool/probe_test.go: 6 httptest-based probe unit tests
  - internal/config/config.go: PoolConfig struct, Pool field in Config, PoolBackoff() method
  - internal/server/server.go: pool *pool.Pool field, updated New() constructor
  - cmd/ocp/main.go: pool construction from DB, StartProbeLoop, defer Stop, injection into server

affects: [03-relay, proxy-routing, upstream-recovery]

tech-stack:
  added: []
  patterns:
    - "Probe loop separated from pool construction: StartProbeLoop called by main.go, not in New()"
    - "io.LimitReader(resp.Body, 64*1024) in sendProbe caps upstream response body (T-02-05)"
    - "PoolConfig.RateLimitBackoff stored as string; PoolBackoff() parses with fallback (Viper duration pitfall)"
    - "probeModels map for per-provider minimal model selection; fallback gpt-3.5-turbo for unknown providers"
    - "ProbeAll() exported method on Pool for deterministic test invocation without ticker wait"

key-files:
  created:
    - internal/pool/probe.go
    - internal/pool/probe_test.go
  modified:
    - internal/pool/pool.go
    - internal/config/config.go
    - internal/server/server.go
    - internal/server/server_test.go
    - cmd/ocp/main.go
    - config.yaml.example

key-decisions:
  - "StartProbeLoop is a separate method (not called in New) so construction and goroutine lifecycle are decoupled — main.go owns the lifecycle"
  - "ProbeAll exported for tests to invoke probe immediately without waiting for hourly ticker"
  - "PoolConfig.RateLimitBackoff stored as string per Viper mapstructure pitfall (cannot unmarshal duration string to time.Duration)"
  - "probeClient is a package-level var with 10s timeout, not http.DefaultClient, to avoid shared-client interference"
  - "Body keyword classification precedes 2xx check in sendProbe — defer to Classify for credits-exhaustion detection"

requirements-completed: [UPST-03, ROUT-02]

duration: 8min
completed: 2026-04-16
---

# Phase 02 Plan 02: Background Probe, Config Extension, Pool Wiring Summary

**Hourly background probe goroutine recovers unavailable upstreams via minimal chat completion, config extended with pool.rate_limit_backoff (5s default), pool injected into Server and constructed in main.go**

## Performance

- **Duration:** ~8 min
- **Completed:** 2026-04-16
- **Tasks:** 2
- **Files modified:** 7 (2 created, 5 modified)

## Accomplishments

- `probe.go` implements hourly `runProbeLoop` ticker, `probeAll` (collects unavailable entries, calls sendProbe), `sendProbe` (POST /v1/chat/completions, max_tokens=1, body capped at 64KB per T-02-05)
- Per-provider model map (kimi/qwen/glm/minimax) ensures minimal token cost for probe requests
- `pool.go` extended with `StartProbeLoop`, `ProbeAll` (exported for tests), `Backoff` methods
- Config extended: `PoolConfig` struct, `Pool` field in `Config`, `PoolBackoff()` with 5s fallback, `SetDefault` for `pool.rate_limit_backoff`
- `server.go` accepts `*pool.Pool` in constructor — integration point for Phase 3 relay
- `main.go` constructs pool from DB, starts probe loop, defers `p.Stop()` for clean shutdown (T-02-06)

## Task Commits

1. **Task 1: Probe goroutine with httptest fixture tests** - `fc3cec2` (feat)
2. **Task 2: Config extension + pool wiring into server and main.go** - `efb9d53` (feat)

## Files Created/Modified

- `internal/pool/probe.go` — runProbeLoop (hourly), probeAll, sendProbe, probeModels map, probeClient
- `internal/pool/probe_test.go` — 6 tests: RecoverOnSuccess, StayUnavailableOnFailure, StayUnavailableOnCreditsExhausted, SkipsAvailable, BodyLimitedTo64KB, SendProbe_RequestFormat
- `internal/pool/pool.go` — added StartProbeLoop, ProbeAll, Backoff methods
- `internal/config/config.go` — added PoolConfig, Pool field, PoolBackoff(), SetDefault for pool.rate_limit_backoff
- `internal/server/server.go` — added pool field, updated New() signature
- `internal/server/server_test.go` — updated New() calls to pass nil for pool
- `cmd/ocp/main.go` — pool construction, StartProbeLoop, defer Stop, server.New with pool
- `config.yaml.example` — added pool.rate_limit_backoff section

## Decisions Made

- StartProbeLoop separated from New() constructor — main.go controls goroutine lifecycle, making Pool constructable in test contexts without background activity
- ProbeAll exported for synchronous test invocation
- PoolConfig.RateLimitBackoff stored as string (not time.Duration) — Viper mapstructure cannot unmarshal duration strings; PoolBackoff() wraps ParseDuration with 5s fallback
- probeClient is a package-level `&http.Client{Timeout: 10*time.Second}` — not DefaultClient, avoiding interference with test or proxy clients

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None — all wired data flows are functional. The pool field on Server is wired but not yet consumed by any handler (Phase 3 relay will add handler logic that calls pool.Select).

## Threat Flags

No new trust boundaries introduced beyond those in the plan's threat model.

## Self-Check: PASSED

- FOUND: internal/pool/probe.go
- FOUND: internal/pool/probe_test.go
- FOUND commit: fc3cec2
- FOUND commit: efb9d53
- `go test -race -count=1 ./...` exits 0 — all 22 pool tests + config + database + server pass
- `go vet ./...` exits 0

---
*Phase: 02-upstream-pool-health-monitor*
*Completed: 2026-04-16*
