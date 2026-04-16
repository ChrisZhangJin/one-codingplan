# Phase 5: Management API - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 5 delivers all `/api/*` management HTTP endpoints: access key lifecycle (issue, list, update, block/unblock, delete) and upstream control (force-rotate). This is the stable API contract that Phase 6 (web portal) and Phase 7 (CLI) build against.

**In scope:** All KEY-01 through KEY-07 requirements plus ROUT-04.
**Not in scope:** Web UI, CLI commands, upstream health polling (Phase 2), proxy relay endpoints.

</domain>

<decisions>
## Implementation Decisions

### Admin Authentication
- **D-01:** Management `/api/*` endpoints use a dedicated `adminMiddleware` that compares the Bearer token against `cfg.Server.AdminKey` (already in `config.yaml` as `server.admin_key`). No DB lookup — config comparison only.
- **D-02:** `/api/*` endpoints are completely separate from `/v1/*` proxy endpoints. The proxy `authMiddleware` (DB-based) and admin `adminMiddleware` (config-based) never mix.

### AccessKey Schema Extensions
- **D-03:** Extend `models.AccessKey` with these new fields (GORM AutoMigrate handles the migration):
  - `TokenBudget int64` — cumulative token limit; `0` = unlimited
  - `AllowedUpstreams string` (JSON-encoded `[]string`) — upstream names this key can use; empty string `""` = unrestricted (all upstreams)
  - `ExpiresAt *time.Time` — nullable; `nil` = no expiry
  - `RateLimitPerMinute int` — max requests/minute; `0` = unlimited
  - `RateLimitPerDay int` — max requests/day; `0` = unlimited

### AllowedUpstreams Enforcement
- **D-04:** Store `AllowedUpstreams` as a JSON string (`TEXT` column) on `AccessKey`. Empty string means unrestricted.
- **D-05:** Enforcement at `Pool.Select(keyID)` time: the method reads the key's allowed upstream list from DB (via context already set by `authMiddleware`) and filters its round-robin pool accordingly. Phase 2 designed `Pool.Select(keyID)` for this (D-17 in Phase 2 CONTEXT.md).

### Rate Limit Counters
- **D-06:** Rate limit counters are **in-memory** — a `sync.Map` keyed by `keyID`, reset on restart. No DB writes per request. Acceptable for a single-instance personal proxy.
- **D-07:** Per-minute windowing uses **fixed minute boundary**: count resets at `:00` each minute. Track `(count int, minuteOf int)` per key. Burstable at boundary but acceptable at personal scale.
- **D-08:** Per-day windowing uses the same fixed boundary: count + day-of-year. Resets at midnight UTC.
- **D-09:** Rate limit and token budget checks happen in `authMiddleware` (or a new `limitMiddleware` called from the same chain) — BEFORE forwarding the request to any upstream. Return `429` on exceeded limits.

### Usage Totals
- **D-10:** `GET /api/keys` aggregates cumulative token usage by running `SELECT SUM(input_tokens), SUM(output_tokens) FROM usage_records WHERE key_id = ?` per key. No denormalized totals — the `key_id` index (already present) makes this fast enough at personal scale.
- **D-11:** No pagination — `GET /api/keys` returns all keys in a single flat JSON array.

### Key Creation and Update
- **D-12:** `POST /api/keys` accepts all limits in a single request body:
  ```json
  {
    "name": "my-key",
    "token_budget": 1000000,
    "allowed_upstreams": ["kimi", "glm"],
    "expires_at": "2026-12-31T23:59:59Z",
    "rate_limit_per_minute": 30,
    "rate_limit_per_day": 500
  }
  ```
  All limit fields are optional; omitted = unlimited/no expiry.
- **D-13:** `POST /api/keys` response includes the **full raw token** — this is the ONLY time the token is returned in plaintext. The token has format `ocp-{uuid}`.
- **D-14:** `GET /api/keys` and `GET /api/keys/:id` return a **masked token** — e.g., `"ocp-abc***xyz"` showing first 7 and last 3 chars.
- **D-15:** `PATCH /api/keys/:id` supports partial updates — only fields present in the request body are modified. Enables the portal's edit flow without delete+recreate.

### API Endpoint Inventory
- **D-16:** Complete `/api/*` route set:
  - `POST   /api/keys`           — create key (KEY-01)
  - `GET    /api/keys`           — list all keys with usage (KEY-02)
  - `GET    /api/keys/:id`       — get single key detail
  - `PATCH  /api/keys/:id`       — update key limits (KEY-04, KEY-05, KEY-06, KEY-07)
  - `POST   /api/keys/:id/block`   — disable key (KEY-03)
  - `POST   /api/keys/:id/unblock` — re-enable key (KEY-03)
  - `DELETE /api/keys/:id`       — delete key
  - `POST   /api/upstreams/rotate` — force pool advance (ROUT-04)
  - `GET    /api/upstreams`      — list upstreams with health state (used by portal + CLI)

### Error Format
- **D-17:** Management API uses a simple flat JSON error envelope consistent with existing relay error format:
  `{"error": "message here"}` — no nested type/code envelope needed for a single-admin API.

### Claude's Discretion
- Exact Go struct field names for the request/response types
- Whether `adminMiddleware` and `limitMiddleware` are separate functions or combined
- Whether `AllowedUpstreams` JSON serialization uses a custom GORM serializer or manual Marshal/Unmarshal in the handler
- Exact token masking algorithm (character counts for prefix/suffix reveal)
- HTTP method for `DELETE /api/keys/:id` vs `POST /api/keys/:id/delete` (prefer standard DELETE)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Requirements
- `.planning/REQUIREMENTS.md` — KEY-01 through KEY-07, ROUT-04 (all in scope for this phase)

### Prior Phase Decisions
- `.planning/phases/01-project-skeleton-data-layer/01-CONTEXT.md` — D-07 (admin_key in config), D-11/D-13 (GORM AutoMigrate, DB schema)
- `.planning/phases/02-upstream-pool-health-monitor/02-CONTEXT.md` — D-15 (pool reads from DB), D-17 (Pool.Select(keyID) for per-key filtering)
- `.planning/phases/03-relay-pipeline-openai-pass-through/03-CONTEXT.md` — D-01 (auth middleware pattern), D-02 (token + enabled check), D-14 (async usage logging)

### Existing Code to Read
- `internal/config/config.go` — `ServerConfig.AdminKey` field already present
- `internal/models/models.go` — current `AccessKey` struct to extend
- `internal/server/server.go` — route registration pattern
- `internal/server/relay.go` — `authMiddleware` implementation to mirror for `adminMiddleware`
- `internal/pool/pool.go` — `Select(keyID)` interface to extend for upstream filtering

No external specs — requirements fully captured in decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `adminMiddleware` pattern: mirror `authMiddleware` in `internal/server/relay.go` — same Bearer token extraction, but compare against `s.cfg.Server.AdminKey` instead of DB lookup
- `crypto` package: AES-GCM already available for any future key value encryption needs
- GORM AutoMigrate: `database.Migrate()` in `internal/database/database.go` — add new `AccessKey` fields and AutoMigrate handles the schema change
- `google/uuid` already in go.mod (used by `models.AccessKey.ID`) — reuse for token generation

### Established Patterns
- Route registration: `v1.POST("/chat/completions", s.handleRelay)` pattern — add `api := r.Group("/api")` with `adminMiddleware`
- Handler pattern: `func (s *Server) handleXxx(c *gin.Context)` receiving from `c.ShouldBindJSON`
- Error response: `c.AbortWithStatusJSON(http.StatusXxx, gin.H{"error": "..."})`
- Async logging: fire-and-forget goroutine after response sent (D-14, Phase 3)

### Integration Points
- `Pool.Select(keyID string)` in `internal/pool/pool.go` — extend to accept/use allowed upstream list from DB
- `authMiddleware` in `internal/server/relay.go` — extend to check rate limits + token budget after token validation
- `database.Migrate()` in `internal/database/database.go` — add `AccessKey` fields to the AutoMigrate call

</code_context>

<specifics>
## Specific Ideas

- Token format: `ocp-{uuid}` (already used in existing Phase 3 implementation)
- Token masking in list/get responses: show first 7 chars + `***` + last 3 chars
- `POST /api/upstreams/rotate` response: `{"upstream": "kimi", "message": "rotated to kimi"}` showing the newly selected upstream name

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 05-management-api*
*Context gathered: 2026-04-16*
