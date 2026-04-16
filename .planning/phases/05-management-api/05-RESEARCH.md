# Phase 5: Management API - Research

**Researched:** 2026-04-16
**Domain:** Go REST API — CRUD endpoint layer over existing GORM/Gin/SQLite stack
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**D-01:** Management `/api/*` endpoints use a dedicated `adminMiddleware` that compares the Bearer token against `cfg.Server.AdminKey` (already in `config.yaml` as `server.admin_key`). No DB lookup — config comparison only.

**D-02:** `/api/*` endpoints are completely separate from `/v1/*` proxy endpoints. The proxy `authMiddleware` (DB-based) and admin `adminMiddleware` (config-based) never mix.

**D-03:** Extend `models.AccessKey` with these new fields (GORM AutoMigrate handles the migration):
- `TokenBudget int64` — cumulative token limit; `0` = unlimited
- `AllowedUpstreams string` (JSON-encoded `[]string`) — upstream names this key can use; empty string `""` = unrestricted
- `ExpiresAt *time.Time` — nullable; `nil` = no expiry
- `RateLimitPerMinute int` — max requests/minute; `0` = unlimited
- `RateLimitPerDay int` — max requests/day; `0` = unlimited

**D-04:** Store `AllowedUpstreams` as a JSON string (`TEXT` column) on `AccessKey`. Empty string means unrestricted.

**D-05:** Enforcement at `Pool.Select(keyID)` time: reads the key's allowed upstream list from DB (via context set by `authMiddleware`) and filters round-robin pool accordingly.

**D-06:** Rate limit counters are in-memory — a `sync.Map` keyed by `keyID`, reset on restart.

**D-07:** Per-minute windowing uses fixed minute boundary: count resets at `:00` each minute. Track `(count int, minuteOf int)` per key.

**D-08:** Per-day windowing uses the same fixed boundary: count + day-of-year. Resets at midnight UTC.

**D-09:** Rate limit and token budget checks happen in `authMiddleware` (or a new `limitMiddleware` called from the same chain) — BEFORE forwarding the request to any upstream. Return `429` on exceeded limits.

**D-10:** `GET /api/keys` aggregates cumulative token usage by running `SELECT SUM(input_tokens), SUM(output_tokens) FROM usage_records WHERE key_id = ?` per key. No denormalized totals.

**D-11:** No pagination — `GET /api/keys` returns all keys in a single flat JSON array.

**D-12:** `POST /api/keys` accepts all limits in a single request body:
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

**D-13:** `POST /api/keys` response includes the full raw token — this is the ONLY time the token is returned in plaintext. Token format: `ocp-{uuid}`.

**D-14:** `GET /api/keys` and `GET /api/keys/:id` return a masked token — e.g., `"ocp-abc***xyz"` showing first 7 and last 3 chars.

**D-15:** `PATCH /api/keys/:id` supports partial updates — only fields present in the request body are modified.

**D-16:** Complete `/api/*` route set:
- `POST   /api/keys`                — create key (KEY-01)
- `GET    /api/keys`                — list all keys with usage (KEY-02)
- `GET    /api/keys/:id`            — get single key detail
- `PATCH  /api/keys/:id`            — update key limits (KEY-04, KEY-05, KEY-06, KEY-07)
- `POST   /api/keys/:id/block`      — disable key (KEY-03)
- `POST   /api/keys/:id/unblock`    — re-enable key (KEY-03)
- `DELETE /api/keys/:id`            — delete key
- `POST   /api/upstreams/rotate`    — force pool advance (ROUT-04)
- `GET    /api/upstreams`           — list upstreams with health state

**D-17:** Management API uses a simple flat JSON error envelope: `{"error": "message here"}`.

### Claude's Discretion
- Exact Go struct field names for request/response types
- Whether `adminMiddleware` and `limitMiddleware` are separate functions or combined
- Whether `AllowedUpstreams` JSON serialization uses a custom GORM serializer or manual Marshal/Unmarshal in the handler
- Exact token masking algorithm (character counts for prefix/suffix reveal)
- HTTP method for `DELETE /api/keys/:id` (prefer standard DELETE)

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| KEY-01 | Admin can issue access keys (ocp-prefixed bearer tokens) via management API | `POST /api/keys` handler; `google/uuid` already in go.mod; token format `ocp-{uuid}` |
| KEY-02 | Admin can list all keys with their status, limits, and usage summary | `GET /api/keys` with aggregated `SUM(input_tokens)` query; D-10 specifies query pattern |
| KEY-03 | Admin can block and unblock an access key (blocked keys receive 401) | `POST /api/keys/:id/block` and `/unblock`; existing `authMiddleware` checks `enabled=true` |
| KEY-04 | Admin can set a token budget on a key; requests exceeding budget receive 429 | `TokenBudget int64` field on `AccessKey`; limit check in auth/limit middleware before relay |
| KEY-05 | Admin can restrict a key to a subset of upstream providers | `AllowedUpstreams string` (JSON) on `AccessKey`; enforcement at `Pool.Select(keyID)` |
| KEY-06 | Admin can set an expiry date on a key; expired keys receive 401 | `ExpiresAt *time.Time` on `AccessKey`; expiry check in `authMiddleware` |
| ROUT-04 | Admin can force-rotate the active upstream via ocp next CLI command | `POST /api/upstreams/rotate`; needs new `Pool.ForceRotate()` method |
</phase_requirements>

---

## Summary

Phase 5 adds 9 HTTP endpoints under `/api/*` protected by a config-based admin token. The codebase already has the full stack in place: Gin router (`server.Engine()`), GORM + SQLite, `authMiddleware` pattern, async usage logging, and `google/uuid`. This phase is predominantly an additive layer over existing infrastructure.

The main technical concerns are: (1) extending `models.AccessKey` with 5 new fields and ensuring AutoMigrate handles the SQLite `ALTER TABLE ADD COLUMN` path correctly, (2) implementing the `AllowedUpstreams` filtering in `Pool.Select(keyID)` — the stub for this was planned in Phase 2 (D-17) and the interface already accepts `keyID`, (3) adding a `limitMiddleware` to enforce token budget, expiry, and rate limits before relay — this middleware needs the full `AccessKey` row, which must be carried in Gin context from `authMiddleware`, (4) adding `Pool.ForceRotate()` for ROUT-04, and (5) aggregating usage totals per key in `GET /api/keys`.

The phase does NOT touch streaming logic, Anthropic translation, or the probe loop. All new code lives in `internal/server/` (handlers) and `internal/models/` (schema extension).

**Primary recommendation:** Implement in two plans — Plan 1: schema extension + admin middleware + key CRUD handlers; Plan 2: limit enforcement middleware + upstream filter wiring into Pool + rotate endpoint + upstream list endpoint.

---

## Standard Stack

All libraries are already in `go.mod` — this phase adds zero new dependencies.
[VERIFIED: go.mod in codebase]

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/gin-gonic/gin` | v1.10.1 | Route group `/api`, `ShouldBindJSON`, `AbortWithStatusJSON` | Already used throughout; `c.ShouldBindJSON` for partial PATCH decoding |
| `gorm.io/gorm` | v1.31.1 | `AutoMigrate` for new fields, GORM queries for key CRUD + usage aggregation | Already used; `AutoMigrate` is additive for SQLite |
| `github.com/glebarez/sqlite` | v1.11.0 | SQLite driver (pure Go, no CGo) | Already used; `ALTER TABLE ADD COLUMN` is handled by GORM AutoMigrate |
| `github.com/google/uuid` | v1.6.0 | Token ID generation for `ocp-{uuid}` format | Already in go.mod as indirect; used by existing code |
| `sync` (stdlib) | — | `sync.Map` for in-memory rate limit counters | D-06: no external rate-limit library needed |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `encoding/json` (stdlib) | — | Marshal/Unmarshal `AllowedUpstreams []string` to/from JSON string | Handler-level serialization per D-04 |
| `time` (stdlib) | — | Expiry check (`ExpiresAt`), rate-limit window boundaries | D-07/D-08: fixed minute/day boundary logic |
| `strings` (stdlib) | — | Token masking (`ocp-abc***xyz` format) | D-14: simple string slicing |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Manual JSON for `AllowedUpstreams` | GORM custom serializer (`serializer:"json"` tag) | GORM serializer is cleaner but requires GORM v2's `Serializer` interface; manual is simpler and explicit for a single field |
| `sync.Map` for rate counters | Redis / per-request DB read | D-06 locked: in-memory is sufficient for single-instance personal proxy |
| `google/uuid` for key generation | `crypto/rand` hex | Both are fine; `google/uuid` is already present and produces a canonical UUID string |

**Installation:** No new packages required. `google/uuid` is already an indirect dependency — if needed, run:
```bash
go get github.com/google/uuid
```
to promote it to a direct dependency in go.mod.

---

## Architecture Patterns

### Recommended Project Structure

No new packages needed. All new files land in existing packages:

```
internal/
├── models/
│   └── models.go               # Extend AccessKey with 5 new fields
├── server/
│   ├── server.go               # Add /api group + adminMiddleware registration
│   ├── admin.go                # New: all /api/* handler funcs
│   ├── limit.go                # New: limitMiddleware (token budget, expiry, rate limits)
│   └── relay.go                # Extend authMiddleware to load full AccessKey into context
└── pool/
    └── pool.go                 # Add ForceRotate() method; extend Select(keyID) for upstream filtering
```

### Pattern 1: Admin Route Group Registration
[VERIFIED: existing server.go route pattern]

Gin route groups with middleware are the established pattern. Mirror the existing `v1` group:

```go
// Source: internal/server/server.go — extends existing Engine() func
api := r.Group("/api")
api.Use(s.adminMiddleware)
api.POST("/keys", s.handleCreateKey)
api.GET("/keys", s.handleListKeys)
api.GET("/keys/:id", s.handleGetKey)
api.PATCH("/keys/:id", s.handleUpdateKey)
api.DELETE("/keys/:id", s.handleDeleteKey)
api.POST("/keys/:id/block", s.handleBlockKey)
api.POST("/keys/:id/unblock", s.handleUnblockKey)
api.POST("/upstreams/rotate", s.handleRotateUpstream)
api.GET("/upstreams", s.handleListUpstreams)
```

### Pattern 2: adminMiddleware
[VERIFIED: existing authMiddleware in relay.go — mirror this pattern]

Config comparison only, no DB lookup:

```go
// Source: mirrors authMiddleware in internal/server/relay.go
func (s *Server) adminMiddleware(c *gin.Context) {
    auth := c.GetHeader("Authorization")
    token, ok := cutPrefix(auth, "Bearer ")
    if !ok || token == "" || token != s.cfg.Server.AdminKey {
        c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
        return
    }
    c.Next()
}
```

### Pattern 3: GORM AutoMigrate for SQLite Schema Extension
[VERIFIED: database.go Migrate() + GORM docs — SQLite supports ADD COLUMN via AutoMigrate]

GORM's `AutoMigrate` issues `ALTER TABLE access_keys ADD COLUMN token_budget INTEGER` etc. for each new field not yet in the table. It is additive — never drops columns.

```go
// Source: internal/database/database.go — extend Migrate() call
func Migrate(db *gorm.DB) error {
    return db.AutoMigrate(
        &models.Upstream{},
        &models.AccessKey{},   // Now with 5 new fields
        &models.UsageRecord{},
    )
}
```

No migration version tracking needed at this scale.

### Pattern 4: PATCH Partial Update with Pointer Fields
[VERIFIED: GORM docs — Updates() only sets non-zero values; pointer fields allow zero-value updates]

For `PATCH /api/keys/:id`, use a separate request struct with pointer fields. GORM's `Model.Updates(map)` handles partial updates cleanly:

```go
// Source: pattern from GORM v2 docs
type patchKeyRequest struct {
    Name               *string    `json:"name"`
    TokenBudget        *int64     `json:"token_budget"`
    AllowedUpstreams   []string   `json:"allowed_upstreams"`
    ExpiresAt          *time.Time `json:"expires_at"`
    RateLimitPerMinute *int       `json:"rate_limit_per_minute"`
    RateLimitPerDay    *int       `json:"rate_limit_per_day"`
}
```

Build a `map[string]any` from non-nil fields, then call `db.Model(&key).Updates(m)`.

### Pattern 5: AllowedUpstreams JSON Serialization
[VERIFIED: D-04 decision; encoding/json stdlib]

Store as JSON string in TEXT column; marshal/unmarshal in handler:

```go
// On write (create/update):
b, _ := json.Marshal(req.AllowedUpstreams)  // []string -> `["kimi","glm"]`
key.AllowedUpstreams = string(b)            // "" means unrestricted

// On read (list/get response):
var allowed []string
if key.AllowedUpstreams != "" {
    json.Unmarshal([]byte(key.AllowedUpstreams), &allowed)
}
```

### Pattern 6: Usage Aggregation Query
[VERIFIED: D-10 decision; GORM raw query pattern confirmed in codebase]

```go
// Source: GORM docs + D-10 decision
type usageTotals struct {
    TotalInput  int64
    TotalOutput int64
}
var totals usageTotals
s.db.Model(&models.UsageRecord{}).
    Select("COALESCE(SUM(input_tokens), 0) AS total_input, COALESCE(SUM(output_tokens), 0) AS total_output").
    Where("key_id = ?", key.ID).
    Scan(&totals)
```

Use `COALESCE(..., 0)` to handle keys with no usage records.

### Pattern 7: Token Masking
[VERIFIED: D-14 specifies first 7 + last 3; string slicing in stdlib]

Token format is `ocp-{uuid}` = 40 chars total (`ocp-` + 36-char UUID).

```go
// maskToken returns "ocp-abc***xyz" for display
func maskToken(token string) string {
    if len(token) <= 10 {
        return "***"
    }
    return token[:7] + "***" + token[len(token)-3:]
}
```

### Pattern 8: Pool.ForceRotate() for ROUT-04
[VERIFIED: existing pool.go idx field + mu lock pattern]

The pool's `idx` field is the round-robin cursor. A `ForceRotate()` method advances it by one under the write lock and returns the newly selected upstream name:

```go
// Source: extends pool.go Select() pattern
func (p *Pool) ForceRotate() (string, error) {
    p.mu.Lock()
    defer p.mu.Unlock()
    n := len(p.entries)
    if n == 0 {
        return "", ErrNoUpstreams
    }
    for i := 0; i < n; i++ {
        p.idx = (p.idx + 1) % n
        if p.entries[p.idx].available {
            return p.entries[p.idx].Name, nil
        }
    }
    return "", ErrNoUpstreams
}
```

### Pattern 9: Pool.Select() Extension for Per-Key Upstream Filtering (D-05)
[VERIFIED: D-17 in Phase 2 CONTEXT.md; Pool.Select(keyID) stub already accepts keyID]

`Pool.Select(keyID)` currently ignores `keyID`. To implement D-05, the handler must resolve the allowed upstream list from the DB (already loaded by `authMiddleware`) and pass it. Two design options:

**Option A (preferred):** Load the full `AccessKey` in `authMiddleware`, set it in context, then extend `Pool.Select` signature to accept `[]string` allowed names:

```go
func (p *Pool) Select(allowedNames []string) (*UpstreamEntry, error)
```

The relay handler extracts the allowed list from `c.MustGet("accessKey")` and passes it to `Select`. An empty slice means unrestricted.

**Option B:** Keep `Select(keyID string)` and have the Pool look up allowed names from a cache. More coupling — Option A is cleaner.

Note: changing `Select`'s signature is a breaking change for existing callers (`handleRelay`, `handleAnthropicRelay`). Both call sites must be updated in the same commit. [VERIFIED: grep shows 2 call sites in relay.go and anthropic.go]

### Pattern 10: In-Memory Rate Limit Counters
[VERIFIED: D-06/D-07/D-08 decisions; sync.Map stdlib]

```go
type rateWindow struct {
    count   int
    windowID int  // minuteOf (0-59) or dayOfYear
}

var perMinuteCounters sync.Map  // keyID -> *rateWindow
var perDayCounters    sync.Map  // keyID -> *rateWindow
```

Use a mutex per entry (or a global RWMutex) when incrementing. On each request:
1. Compute current minute (time.Now().Minute()) and day-of-year.
2. If stored `windowID` != current window, reset count to 0, store new windowID.
3. Check count < limit before incrementing.

The `sync.Map` value should be a pointer to a struct guarded by its own `sync.Mutex` for atomic read-check-increment without TOCTOU.

### Anti-Patterns to Avoid

- **Storing rate-limit state in SQLite:** D-06 locked in-memory. SQLite writes per-request would create a write bottleneck even though this is personal scale.
- **Returning the raw token on GET /api/keys:** D-14 mandates masking. The full token must never appear after the creation response.
- **Using GORM `Save()` for PATCH:** `Save()` updates ALL fields, turning a partial update into a full overwrite. Use `Updates(map)` or `Model(&key).Select(fields).Updates(req)`.
- **Checking expiry at route registration time:** Expiry must be checked on every request in the middleware chain, not cached.
- **Setting `Content-Length` on PATCH/POST responses:** Gin handles this; do not set it manually.
- **Calling `Pool.Select()` with old signature after extending it:** All call sites must be updated atomically. There are exactly 2 callers: `handleRelay` and `handleAnthropicRelay`.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| UUID generation | Custom random string | `github.com/google/uuid` (already in go.mod) | Collision-safe, RFC 4122, already present |
| JSON binding for request bodies | Manual `json.Decoder` | `c.ShouldBindJSON` (Gin) | Already used in all existing handlers |
| DB query building | Raw SQL strings | GORM fluent API | Consistent with entire codebase |
| Rate window reset logic | Custom time math | `time.Now().Minute()` and `time.Now().YearDay()` | stdlib time is sufficient for fixed-boundary windows |
| Token existence validation | Custom regex | GORM `Where("token = ?", token)` uniqueIndex constraint | DB constraint already enforces uniqueness |

**Key insight:** This phase is purely additive — no greenfield design decisions. Every pattern needed already exists in the codebase; the work is applying known patterns to new endpoints.

---

## Common Pitfalls

### Pitfall 1: GORM AutoMigrate SQLite Limitation — No Column Type Changes
**What goes wrong:** AutoMigrate can ADD columns but cannot change existing column types in SQLite (e.g., cannot change `NOT NULL` to nullable on an existing column).
**Why it happens:** SQLite does not support `ALTER TABLE MODIFY COLUMN`. GORM silently skips type changes.
**How to avoid:** Only ADD new nullable columns (with pointer types or `DEFAULT NULL`). The 5 new `AccessKey` fields are all new additions, so this is fine. Do not attempt to modify `Token` or `Enabled` column definitions.
**Warning signs:** AutoMigrate returns nil but schema hasn't changed.

### Pitfall 2: GORM Updates() Skips Zero Values
**What goes wrong:** `db.Updates(struct)` skips fields with zero values — an explicit `0` for `TokenBudget` (meaning "unlimited") would not be written.
**Why it happens:** GORM treats Go zero values as "not set" when using struct input.
**How to avoid:** Use `db.Updates(map[string]any{...})` for PATCH, only including keys that were explicitly provided in the request JSON. Use pointer fields in the request struct to distinguish "not provided" from "set to zero".
**Warning signs:** Clients send `{"token_budget": 0}` to reset the budget but the field is not updated.

### Pitfall 3: sync.Map Race on Rate Window Reset
**What goes wrong:** Two concurrent requests check the rate window simultaneously — both see an expired window, both reset the count to 0, then both increment. The limit is not enforced for one of them.
**Why it happens:** `sync.Map.Load` + check + `sync.Map.Store` is not atomic.
**How to avoid:** Store a pointer to a struct with its own `sync.Mutex`. Lock the struct mutex for the full read-check-reset-increment sequence. Do NOT use `sync.Map.LoadOrStore` for this (it doesn't help with the check+increment atomicity).
**Warning signs:** Rate-limit enforcement works under low concurrency but fails under load.

### Pitfall 4: Token Budget Check After Relay (Off-by-One)
**What goes wrong:** Token budget is checked BEFORE the request (using cumulative past usage), but the current request's tokens are logged AFTER. A key at 999,999/1,000,000 tokens can make one more request; after that request's usage is logged, it's at 1,000,000 but the budget check still passes for the NEXT request because the DB aggregate hasn't been re-queried.
**Why it happens:** The check uses a point-in-time DB query; new usage is logged async (fire-and-forget goroutine, D-14 Phase 3).
**How to avoid:** Accept this behavior as designed — at personal scale this is acceptable. Document that the budget check is "at start of request" not "before each token". Do not add complexity to pre-deduct tokens.
**Warning signs:** Users notice they can slightly exceed their token budget by at most one request's worth.

### Pitfall 5: AllowedUpstreams Empty String vs. Empty JSON Array
**What goes wrong:** An empty `allowed_upstreams` array in PATCH JSON is serialized as `"[]"` not `""`. The enforcement code must treat both `""` and `"[]"` as unrestricted.
**Why it happens:** `json.Marshal([]string{})` returns `"[]"`, not `""`. But the DB stores `""` for "unrestricted" (initial creation with no restriction).
**How to avoid:** On read: check both `key.AllowedUpstreams == ""` and `key.AllowedUpstreams == "[]"` as unrestricted. On write via PATCH: if `allowed_upstreams` is provided as `[]` (empty slice), store `""` (canonical "unrestricted" marker).
**Warning signs:** A key previously restricted to some upstreams cannot be "unrestricted" via PATCH.

### Pitfall 6: Pool.Select() Signature Change Breaks Existing Relay Callers
**What goes wrong:** Extending `Pool.Select` to accept `[]string` breaks `handleRelay` and `handleAnthropicRelay` at compile time.
**Why it happens:** Both handlers call `s.pool.Select(keyID)` with the old signature.
**How to avoid:** Update all three files in the same commit: `pool/pool.go`, `server/relay.go`, `server/anthropic.go`. The compiler will catch missing updates — this is not a runtime risk.
**Warning signs:** `go build ./...` fails with "too many arguments in call to s.pool.Select".

### Pitfall 7: Expiry Check Timezone
**What goes wrong:** Expiry is stored and checked in UTC, but a user sets it in their local timezone and the value in the DB is not converted.
**Why it happens:** `time.Time` JSON unmarshaling in Go respects timezone info in the ISO 8601 string, but if the client sends a naive datetime (no timezone), Go defaults to UTC. The stored DB value is the SQLite TEXT representation.
**How to avoid:** Always call `.UTC()` when storing and comparing `ExpiresAt`. In `authMiddleware`: `if key.ExpiresAt != nil && time.Now().UTC().After(key.ExpiresAt.UTC())`.
**Warning signs:** Keys expire at unexpected times.

---

## Code Examples

### Create Key Handler Structure
```go
// Source: mirrors existing handleRelay pattern in internal/server/relay.go
func (s *Server) handleCreateKey(c *gin.Context) {
    var req createKeyRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    id := uuid.New().String()
    token := "ocp-" + uuid.New().String()
    
    allowedJSON := ""
    if len(req.AllowedUpstreams) > 0 {
        b, _ := json.Marshal(req.AllowedUpstreams)
        allowedJSON = string(b)
    }
    
    key := models.AccessKey{
        ID:                 id,
        Token:              token,
        Enabled:            true,
        TokenBudget:        derefInt64(req.TokenBudget),
        AllowedUpstreams:   allowedJSON,
        ExpiresAt:          req.ExpiresAt,
        RateLimitPerMinute: derefInt(req.RateLimitPerMinute),
        RateLimitPerDay:    derefInt(req.RateLimitPerDay),
    }
    if err := s.db.Create(&key).Error; err != nil {
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to create key"})
        return
    }
    // D-13: raw token returned ONLY on creation
    c.JSON(http.StatusCreated, keyToResponse(key, token))
}
```

### Usage Aggregation in List Handler
```go
// Source: D-10 decision; GORM Scan pattern
type usageSums struct {
    TotalInput  int64 `gorm:"column:total_input"`
    TotalOutput int64 `gorm:"column:total_output"`
}

func (s *Server) keyUsage(keyID string) (in, out int64) {
    var sums usageSums
    s.db.Model(&models.UsageRecord{}).
        Select("COALESCE(SUM(input_tokens),0) AS total_input, COALESCE(SUM(output_tokens),0) AS total_output").
        Where("key_id = ?", keyID).
        Scan(&sums)
    return sums.TotalInput, sums.TotalOutput
}
```

### Limit Middleware (token budget + expiry)
```go
// Source: pattern extends authMiddleware in internal/server/relay.go
func (s *Server) limitMiddleware(c *gin.Context) {
    key := c.MustGet("accessKey").(models.AccessKey)

    // Expiry check (KEY-06)
    if key.ExpiresAt != nil && time.Now().UTC().After(key.ExpiresAt.UTC()) {
        c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "key expired"})
        return
    }

    // Token budget check (KEY-04)
    if key.TokenBudget > 0 {
        in, out := s.keyUsage(key.ID)
        if in+out >= key.TokenBudget {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "token budget exceeded"})
            return
        }
    }

    // Rate limit checks (KEY-07 / v2, but fields present in schema)
    if key.RateLimitPerMinute > 0 {
        if !s.checkMinuteLimit(key.ID, key.RateLimitPerMinute) {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "per-minute rate limit exceeded"})
            return
        }
    }

    c.Next()
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| GORM v1 (jinzhu/gorm) | GORM v2 (gorm.io/gorm) | 2020 | `Updates(map)` behavior differs; codebase is already v2 |
| `mattn/go-sqlite3` (CGo) | `glebarez/sqlite` (pure Go) | Project decision | No CGo; Alpine Docker safe |

**No deprecated patterns relevant to this phase** — all tooling is current.

---

## Runtime State Inventory

> This phase extends the DB schema and adds in-memory state. No rename/refactor.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | `access_keys` table in `ocp.db` — existing rows have no new columns yet | AutoMigrate adds columns with SQLite `NULL` defaults; existing rows become `TokenBudget=0` (unlimited), `AllowedUpstreams=""` (unrestricted), `ExpiresAt=NULL` (no expiry) — correct behavior |
| Live service config | None — server.admin_key already in config.yaml | None |
| OS-registered state | None | None |
| Secrets/env vars | `OCP_SERVER_ADMIN_KEY` env var — already wired | None |
| Build artifacts | None | None |

**Nothing found requiring data migration** — AutoMigrate handles schema evolution with safe defaults.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| Go toolchain | Build | ✓ | go1.25.0 | — |
| SQLite (via glebarez pure-Go) | DB layer | ✓ | 1.11.0 | — |
| `google/uuid` | Token generation | ✓ | v1.6.0 (indirect) | — |
| `go test ./...` | Test validation | ✓ | All 6 packages pass | — |

[VERIFIED: go.mod + `go test ./...` output showing all packages pass]

No missing dependencies. All required packages are already in the module graph.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package |
| Config file | none (no external config needed for `_test` package) |
| Quick run command | `go test ./internal/server/... -count=1` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| KEY-01 | POST /api/keys creates key; response contains `ocp-` token | unit (httptest) | `go test ./internal/server/... -run TestCreateKey -v` | ❌ Wave 0 |
| KEY-02 | GET /api/keys returns keys with usage sums | unit (httptest + in-memory DB) | `go test ./internal/server/... -run TestListKeys -v` | ❌ Wave 0 |
| KEY-03 | POST /api/keys/:id/block blocks key; next relay returns 401 | unit (httptest) | `go test ./internal/server/... -run TestBlockUnblockKey -v` | ❌ Wave 0 |
| KEY-04 | Token budget enforced; 429 returned on exceed | unit (limitMiddleware) | `go test ./internal/server/... -run TestTokenBudget -v` | ❌ Wave 0 |
| KEY-05 | Allowed upstreams filter applied at Pool.Select | unit (pool + server) | `go test ./internal/pool/... -run TestSelectWithFilter -v` | ❌ Wave 0 |
| KEY-06 | Expired key receives 401 | unit (limitMiddleware) | `go test ./internal/server/... -run TestKeyExpiry -v` | ❌ Wave 0 |
| ROUT-04 | POST /api/upstreams/rotate advances pool idx; next relay uses new upstream | unit (pool.ForceRotate + httptest) | `go test ./internal/server/... -run TestRotateUpstream -v` | ❌ Wave 0 |
| adminMiddleware | Wrong/missing token → 401; correct token → passes through | unit (httptest) | `go test ./internal/server/... -run TestAdminMiddleware -v` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/server/... -count=1`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/server/admin_test.go` — covers KEY-01 through KEY-06, ROUT-04, adminMiddleware
- [ ] `internal/pool/pool_test.go` — extend to cover `TestSelectWithFilter` and `TestForceRotate` (file exists, add new test functions)

*(No new framework or fixture infrastructure needed — `setupTestDB`, `seedAccessKey`, `seedUpstream`, `buildPool` helpers in `relay_test.go` are reusable)*

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | `adminMiddleware` — config-based bearer token comparison; constant-time compare recommended (`subtle.ConstantTimeCompare`) |
| V3 Session Management | no | No sessions; stateless bearer token per request |
| V4 Access Control | yes | Single admin role; no authorization needed beyond "is admin" |
| V5 Input Validation | yes | `c.ShouldBindJSON` + explicit field validation (name length, budget > 0 check) |
| V6 Cryptography | no | Keys are not encrypted at rest (access key tokens are bearer tokens, not secrets requiring AES) |

### Known Threat Patterns for Management REST API

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Timing attack on admin key comparison | Information Disclosure | Use `crypto/subtle.ConstantTimeCompare` instead of `==` for token comparison in `adminMiddleware` |
| Mass enumeration of key IDs | Tampering | UUID-format IDs are not guessable; no additional mitigation needed |
| Token leakage via GET response | Information Disclosure | D-14: mask token on all GET responses; only expose full token on POST /api/keys response |
| AllowedUpstreams injection | Tampering | JSON unmarshal into `[]string` type limits injection to string values; upstream names are matched against pool entries (no eval, no exec) |
| SQL injection via key name | Tampering | GORM parameterized queries; never interpolate user input into raw SQL |

**Timing attack note:** The current `authMiddleware` in `relay.go` uses `token != s.cfg.Server.AdminKey` (string equality). For `adminMiddleware`, use `subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.Server.AdminKey)) == 1` to prevent timing attacks on the admin key. This is a LOW-severity concern for a single-admin personal proxy but is trivial to implement correctly.

---

## Open Questions (RESOLVED)

1. **Pool.Select() signature change scope**
   - What we know: 2 call sites exist (`handleRelay`, `handleAnthropicRelay` in relay.go); both must be updated.
   - What's unclear: Whether the allowed upstream list should be read from the DB in `authMiddleware` (one DB round-trip per request) or cached in memory (adds invalidation complexity).
   - Recommendation: Read from DB in `authMiddleware` — it's already doing a DB lookup for the key row; add `AllowedUpstreams` to the existing query. Store the full `AccessKey` struct in Gin context (not just `keyID`) to avoid a second DB round-trip in `limitMiddleware`.
   - RESOLVED: Store full `AccessKey` struct in Gin context via `authMiddleware`; both relay.go and anthropic.go call sites updated to pass `accessKey.AllowedUpstreams`.

2. **PATCH /api/keys/:id — handling `allowed_upstreams: []` (reset to unrestricted)**
   - What we know: Empty JSON array `[]` marshals to `"[]"` not `""`. The DB canonical form for "unrestricted" is `""`.
   - What's unclear: Should PATCH with `"allowed_upstreams": []` reset to unrestricted or be ignored as "no change"?
   - Recommendation: Treat explicit `[]` in PATCH as "reset to unrestricted" — store `""`. Use a presence-check (JSON decoder with `json.Number` or a custom type) to distinguish "field absent" from "field present but empty".
   - RESOLVED: Explicit `[]` in PATCH stores `""` (unrestricted). `patchKeyRequest` uses pointer fields (`*string`) to distinguish absent vs. present-but-empty.

3. **`GET /api/upstreams` — what data to return**
   - What we know: D-16 includes this endpoint; portal and CLI use it.
   - What's unclear: What health state fields to expose — pool availability, cooldown timer remaining, last error?
   - Recommendation: Return `{name, base_url, enabled, available, model_override}` from pool entries. Do not expose API keys. Add `available bool` from pool's internal state. The pool needs a `List()` method that returns non-sensitive entries.
   - RESOLVED: `pool.List()` returns `[]UpstreamInfo{Name, BaseURL, Enabled, Available}` (no API keys). `handleListUpstreams` calls `pool.List()` and returns the array.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | GORM AutoMigrate adds new nullable columns to SQLite without data loss | Standard Stack / Pitfalls | Low — GORM + SQLite ADD COLUMN is well-tested behavior; all new fields have zero-value defaults |
| A2 | `google/uuid` (indirect dep v1.6.0) is importable without adding to go.mod as direct dep | Standard Stack | Low — it's already in the module graph; `go get` will promote if needed |
| A3 | `crypto/subtle.ConstantTimeCompare` is adequate for admin key comparison | Security Domain | Low — stdlib function; appropriate for byte-slice comparison |

**All three assumptions are LOW risk and based on well-established stdlib/GORM behavior.**

---

## Sources

### Primary (HIGH confidence)
- `go.mod` — verified all dependencies present [VERIFIED: codebase]
- `internal/server/relay.go` — `authMiddleware` pattern, `cutPrefix`, `AbortWithStatusJSON`, handler structure [VERIFIED: codebase]
- `internal/server/server.go` — route group registration pattern [VERIFIED: codebase]
- `internal/pool/pool.go` — `Select(keyID)`, `Mark()`, `idx` field, `mu` locking pattern [VERIFIED: codebase]
- `internal/models/models.go` — current `AccessKey` struct to extend [VERIFIED: codebase]
- `internal/database/database.go` — `Migrate()` call to extend [VERIFIED: codebase]
- `internal/config/config.go` — `ServerConfig.AdminKey` confirmed present [VERIFIED: codebase]
- `go test ./...` — all 6 packages pass, race detector clean [VERIFIED: runtime]
- `.planning/phases/05-management-api/05-CONTEXT.md` — all D-xx decisions [VERIFIED: codebase]

### Secondary (MEDIUM confidence)
- GORM v2 `Updates(map)` partial update behavior — documented in GORM v2 docs; consistent with codebase usage patterns [ASSUMED based on GORM v2 docs knowledge — behavior is stable and well-documented]
- SQLite `ALTER TABLE ADD COLUMN` via GORM AutoMigrate — known limitation (no modify, only add) [ASSUMED based on SQLite and GORM documentation knowledge]

### Tertiary (LOW confidence)
- None — all claims are either verified against codebase or well-established GORM/stdlib behavior.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all deps verified in go.mod; all patterns verified in existing codebase files
- Architecture: HIGH — every pattern is an extension of existing, verified patterns in the codebase
- Pitfalls: HIGH — derived directly from codebase inspection and Go/GORM/SQLite known behaviors
- Security: MEDIUM — ASVS categories are standard; timing attack recommendation is standard practice

**Research date:** 2026-04-16
**Valid until:** 2026-07-16 (stable Go/GORM/SQLite stack; 90 days)
