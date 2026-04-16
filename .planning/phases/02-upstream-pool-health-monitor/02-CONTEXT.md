# Phase 2: Upstream Pool & Health Monitor - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

In-memory upstream pool that: loads upstreams from SQLite at startup, selects them via round-robin, tracks each upstream's health as `available` or `unavailable`, marks upstreams unavailable when API responses signal out-of-credits, and runs a background hourly probe to recover unavailable upstreams when they respond normally.

No relay pipeline (Phase 3). No management API (Phase 5). No Anthropic format translation (Phase 4).

</domain>

<decisions>
## Implementation Decisions

### Health State Machine
- **D-01:** Two states only — `available` and `unavailable`. No "cooling", no "dead", no intermediate states.
- **D-02:** Transition `available` → `unavailable`: upstream API response signals out-of-credits, out-of-quota, or no remaining tokens (any phrasing).
- **D-03:** Transition `unavailable` → `available`: hourly background probe receives a normal (non-error) response from the upstream.
- **D-04:** Rate-limit errors and transient errors do NOT change the upstream's state to `unavailable` — only out-of-credits/quota does.

### Rate-Limit Handling
- **D-05:** When an upstream returns a rate-limit response, ocp backs off and retries the same upstream (does not rotate away).
- **D-06:** Backoff duration is configurable in `config.yaml` (e.g., `pool.rate_limit_backoff`), defaulting to 5 seconds.

### Recovery Probe
- **D-07:** Background goroutine probes each `unavailable` upstream every hour.
- **D-08:** Probe request: minimal chat completion — message `"hi"` with `max_tokens=1` sent to the upstream's API.
- **D-09:** If the probe returns a normal (200-class, non-error-body) response, the upstream is marked `available` again.
- **D-10:** If the probe itself fails (network error, still-out-of-credits, etc.), the upstream stays `unavailable`; the next probe fires in another hour.

### Error Classifier
- **D-11:** Per-provider classifier using a provider-keyed map: `map[string]Classifier` where the key is the upstream's name/provider slug.
- **D-12:** Each classifier entry inspects two things: HTTP status code first, then response body substring match for provider-specific keywords.
- **D-13:** Classification categories:
  - **credits-exhausted**: HTTP 402, or body contains keywords like `"insufficient"`, `"quota"`, `"balance"`, `"out of credits"`, `"no credit"`, `"token limit"` → mark `unavailable`
  - **rate-limited**: HTTP 429 or body signals rate limit → backoff + retry same upstream
  - **transient**: 5xx, timeout, or any unrecognized error → rotate to next available upstream
- **D-14:** New providers are added by appending to the provider map — no code changes to routing logic (config-driven extensibility principle from CLAUDE.md).

### Pool Interface
- **D-15:** Pool reads from the SQLite `upstreams` table at startup (not re-reading config). This ensures DB is the single source of truth after Phase 1's config→DB sync.
- **D-16:** Pool lives in a new package: `internal/pool/`.
- **D-17:** Pool exposes a `Select(keyID string) (*Upstream, error)` method that returns the next available upstream for the given key's allowed pool (full pool if unrestricted).

### Claude's Discretion
- Exact struct/interface names for the Classifier and Pool types
- Whether the round-robin index is per-pool (global) or per-key
- Concurrency primitives (sync.Mutex vs sync.RWMutex vs atomic) for pool state
- How the pool is injected into the Server (constructor param or interface)
- Model to use for probe calls (cheapest available for each provider)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Requirements
- `.planning/REQUIREMENTS.md` — UPST-02 (detect unhealthy reactively), UPST-03 (re-test and recover), ROUT-01 (round-robin), ROUT-02 (auto-rotate on error), ROUT-03 (per-provider error classification)
- `.planning/ROADMAP.md` §Phase 2 — Goal, success criteria, dependency on Phase 1

### Project Context
- `.planning/PROJECT.md` — Core value, constraints, extensibility principle
- `CLAUDE.md` (project root) — Tech stack rationale, Go conventions, reference projects

### Phase 1 Output (read before implementing)
- `.planning/phases/01-project-skeleton-data-layer/01-CONTEXT.md` — All Phase 1 decisions (models, config, DB setup)
- `internal/models/models.go` — Upstream struct (ID, Name, BaseURL, APIKeyEnc, Enabled)
- `internal/config/config.go` — Config struct and UpstreamConfig; note that upstream API keys are AES-encrypted at rest (see `internal/crypto/`)
- `internal/server/server.go` — Server struct; pool should be injected here

### No external specs — requirements fully captured in decisions above

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/models.Upstream` — the upstream DB model; pool reads from this
- `internal/crypto` — AES-GCM decrypt for upstream API keys; pool must call `upstream.DecryptAPIKey(encKey)` before sending requests
- `internal/config.Config` — carries the encryption key and `pool.rate_limit_backoff` default

### Established Patterns
- Server receives dependencies via constructor: `server.New(db, cfg)` — pool should follow the same injection pattern
- Config uses Viper with `OCP_` env prefix; new `pool.*` config fields should follow the same `mapstructure` convention

### Integration Points
- `internal/server/server.go` — pool is injected into Server, exposed to relay handler in Phase 3
- `internal/database/database.go` — pool queries `upstreams` table via GORM at startup

</code_context>

<specifics>
## Specific Ideas

- User explicitly prefers a two-state model (available/unavailable) over more complex state machines — keep it simple
- Probe interval is hourly — not configurable by user request (simple is better here)
- The "hi" probe is intentionally cheap; don't use a real user prompt
- Credits-exhausted is the only condition that marks an upstream unavailable; rate-limit and transient errors never change state

</specifics>

<deferred>
## Deferred Ideas

- Balance API polling (Kimi exposes `GET /v1/users/me/balance`) — HLTH-01 in v2 requirements; intentionally deferred, not in Phase 2
- Per-upstream configurable probe interval — kept hourly for simplicity
- Admin-triggered manual re-enable of unavailable upstreams — Phase 5 Management API can add this

</deferred>

---

*Phase: 02-upstream-pool-health-monitor*
*Context gathered: 2026-04-16*
