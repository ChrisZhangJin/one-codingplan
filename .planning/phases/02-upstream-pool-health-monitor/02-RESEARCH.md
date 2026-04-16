# Phase 2: Upstream Pool & Health Monitor - Research

**Researched:** 2026-04-16
**Domain:** Go in-memory pool, concurrency primitives, HTTP-based health classification
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Two states only — `available` and `unavailable`. No "cooling", no "dead", no intermediate states.
- **D-02:** Transition `available` → `unavailable`: upstream API response signals out-of-credits, out-of-quota, or no remaining tokens (any phrasing).
- **D-03:** Transition `unavailable` → `available`: hourly background probe receives a normal (non-error) response from the upstream.
- **D-04:** Rate-limit errors and transient errors do NOT change the upstream's state to `unavailable` — only out-of-credits/quota does.
- **D-05:** When an upstream returns a rate-limit response, ocp backs off and retries the same upstream (does not rotate away).
- **D-06:** Backoff duration is configurable in `config.yaml` (e.g., `pool.rate_limit_backoff`), defaulting to 5 seconds.
- **D-07:** Background goroutine probes each `unavailable` upstream every hour.
- **D-08:** Probe request: minimal chat completion — message `"hi"` with `max_tokens=1` sent to the upstream's API.
- **D-09:** If the probe returns a normal (200-class, non-error-body) response, the upstream is marked `available` again.
- **D-10:** If the probe itself fails (network error, still-out-of-credits, etc.), the upstream stays `unavailable`; the next probe fires in another hour.
- **D-11:** Per-provider classifier using a provider-keyed map: `map[string]Classifier` where the key is the upstream's name/provider slug.
- **D-12:** Each classifier entry inspects two things: HTTP status code first, then response body substring match for provider-specific keywords.
- **D-13:** Classification categories:
  - **credits-exhausted**: HTTP 402, or body contains keywords like `"insufficient"`, `"quota"`, `"balance"`, `"out of credits"`, `"no credit"`, `"token limit"` → mark `unavailable`
  - **rate-limited**: HTTP 429 or body signals rate limit → backoff + retry same upstream
  - **transient**: 5xx, timeout, or any unrecognized error → rotate to next available upstream
- **D-14:** New providers are added by appending to the provider map — no code changes to routing logic.
- **D-15:** Pool reads from the SQLite `upstreams` table at startup (not re-reading config).
- **D-16:** Pool lives in a new package: `internal/pool/`.
- **D-17:** Pool exposes a `Select(keyID string) (*Upstream, error)` method that returns the next available upstream for the given key's allowed pool.

### Claude's Discretion

- Exact struct/interface names for the Classifier and Pool types
- Whether the round-robin index is per-pool (global) or per-key
- Concurrency primitives (sync.Mutex vs sync.RWMutex vs atomic) for pool state
- How the pool is injected into the Server (constructor param or interface)
- Model to use for probe calls (cheapest available for each provider)

### Deferred Ideas (OUT OF SCOPE)

- Balance API polling (Kimi `GET /v1/users/me/balance`) — HLTH-01 in v2 requirements
- Per-upstream configurable probe interval — kept hourly for simplicity
- Admin-triggered manual re-enable of unavailable upstreams — Phase 5 Management API
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| UPST-02 | Proxy detects unhealthy upstreams reactively via error-inference and marks them with a cooldown period | Error classifier per-provider keywords; two-state model; `Mark(id, unavailable)` call from relay |
| UPST-03 | Proxy re-tests cooled-down upstreams and returns them to the active pool when healthy | Hourly background goroutine with `time.Ticker`; probe HTTP call; `Mark(id, available)` on success |
| ROUT-01 | Proxy selects upstreams via round-robin across the key's allowed upstream pool | Atomic or mutex-guarded index per pool; `Select(keyID)` interface |
| ROUT-02 | Proxy automatically rotates to next available upstream when current upstream returns credits-exhausted, rate-limit, or error/timeout response | Relay loop (Phase 3) consumes `ErrorClass` from classifier; pool `Select` skips unavailable |
| ROUT-03 | Proxy classifies upstream error responses per-provider to distinguish credits-exhausted from rate-limited from transient error | `Classify(providerName, httpStatus, body) ErrorClass` function driven by provider keyword map |
</phase_requirements>

## Summary

Phase 2 introduces `internal/pool/` — an in-memory upstream pool backed by SQLite state loaded at startup. The pool supports two operations used by the relay (Phase 3): `Select(keyID)` returns the next available upstream via round-robin, and `Mark(upstreamID, status)` updates health state. A background goroutine probes each `unavailable` upstream hourly with a minimal chat completion and restores it to `available` on success. An error classifier translates raw HTTP status + body text into one of three categories (`CreditsExhausted`, `RateLimited`, `Transient`) using a per-provider keyword map.

The implementation is pure Go stdlib + GORM — no new library dependencies required. Concurrency correctness is the main engineering risk: the pool must handle concurrent relay goroutines reading the round-robin index while the probe goroutine writes health state. A single `sync.RWMutex` is the right tool; `sync/atomic` is sufficient only for the counter but adds complexity when combined with slice access.

**Primary recommendation:** Use `sync.RWMutex` to protect both the upstream slice and the round-robin counter. Keep the classifier as a package-level `map[string][]string` of keywords (not an interface hierarchy) — the two-state model and per-provider keyword approach are simple enough that an interface abstraction adds no value at this scale.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `sync` (stdlib) | Go 1.25 | `sync.RWMutex` for pool state | Zero-dep, correct semantics for many-readers/one-writer |
| `net/http` (stdlib) | Go 1.25 | Probe HTTP client | Already used by server; no extra dep |
| `time` (stdlib) | Go 1.25 | `time.Ticker` for hourly probe loop | No external scheduler needed |
| `gorm.io/gorm` | v1.31.1 | GORM query to load upstreams at startup | Already in go.mod |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `encoding/json` (stdlib) | Go 1.25 | Decode error body from upstream response | Needed for body-based classification |
| `strings` (stdlib) | Go 1.25 | `strings.Contains` for keyword matching | Classifier body inspection |
| `context` (stdlib) | Go 1.25 | Probe request deadline, pool shutdown | Needed for graceful stop of background goroutine |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `sync.RWMutex` | `sync/atomic` on index + separate mutex for slice | Atomic index is fine for the counter alone but the slice of upstream states also needs protection; two separate locks creates a race window |
| Package-level keyword map | Interface `Classifier` hierarchy | Overcomplicated for 5 providers with simple substring rules; add interface only if classifier logic diverges across providers in a future phase |
| `time.Ticker` in probe goroutine | `time.AfterFunc` | `AfterFunc` reschedules after each probe completes (avoids probe drift) but adds code complexity; hourly ticker is acceptable for this use case |

**Installation:** No new packages — all dependencies already in `go.mod`.

## Architecture Patterns

### Recommended Project Structure

```
internal/
└── pool/
    ├── pool.go          # Pool struct, New(), Select(), Mark(), Stop()
    ├── classifier.go    # Classify(), ErrorClass type, provider keyword map
    ├── probe.go         # runProbeLoop(), sendProbe()
    └── pool_test.go     # unit tests covering Select, Mark, Classify, round-robin, -race
```

### Pattern 1: Pool Struct with RWMutex

**What:** The `Pool` struct holds a slice of runtime upstream entries (id, available bool, apiKey plaintext) and a round-robin index, all guarded by a single `sync.RWMutex`. `Select` takes a read-then-write lock only when advancing the index; `Mark` takes a write lock.

**When to use:** Any shared state accessed by many concurrent relay goroutines with infrequent writes (health state changes).

**Example:**
```go
// internal/pool/pool.go
// Source: Go stdlib sync package documentation

type entry struct {
    id       uint
    name     string
    baseURL  string
    apiKey   string
    available bool
}

type Pool struct {
    mu      sync.RWMutex
    entries []entry
    idx     int
    encKey  []byte
    stopCh  chan struct{}
}

// Select returns the next available upstream, cycling round-robin.
// Returns ErrNoUpstreams if all upstreams are unavailable.
func (p *Pool) Select(_ string) (*entry, error) {
    p.mu.Lock()
    defer p.mu.Unlock()
    n := len(p.entries)
    for i := 0; i < n; i++ {
        p.idx = (p.idx + 1) % n
        if p.entries[p.idx].available {
            e := p.entries[p.idx]
            return &e, nil
        }
    }
    return nil, ErrNoUpstreams
}
```

Note: `Select` takes a full write lock (not RLock) because it mutates `p.idx`. If write contention becomes measurable under load (only relevant at thousands of RPS), the counter can be split into a separate `sync/atomic` value; skip this optimization for Phase 2.

### Pattern 2: Error Classifier as Function + Keyword Map

**What:** `Classify(provider string, status int, body []byte) ErrorClass` looks up the provider's keyword list from a package-level map, then applies status-code rules first and substring rules second.

**When to use:** When classification rules are data-driven and per-provider, not behavioral.

**Example:**
```go
// internal/pool/classifier.go

type ErrorClass int

const (
    ClassTransient        ErrorClass = iota
    ClassRateLimited
    ClassCreditsExhausted
)

var creditsKeywords = []string{
    "insufficient", "quota", "balance", "out of credits",
    "no credit", "token limit", "exceeded_current_quota",
    "insufficient_quota", "recharge",
}

// providerOverrides can add or replace keywords per provider slug.
// Empty map means fall back to creditsKeywords for all providers.
var providerOverrides = map[string][]string{
    "glm": {"1113", "insufficient balance"},
    "minimax": {"1008", "insufficient balance"},
}

func Classify(provider string, status int, body []byte) ErrorClass {
    if status == http.StatusTooManyRequests {
        return ClassRateLimited
    }
    bodyStr := strings.ToLower(string(body))
    keywords := creditsKeywords
    if overrides, ok := providerOverrides[provider]; ok {
        keywords = overrides
    }
    for _, kw := range keywords {
        if strings.Contains(bodyStr, kw) {
            return ClassCreditsExhausted
        }
    }
    if status >= 500 {
        return ClassTransient
    }
    if status == http.StatusPaymentRequired { // 402
        return ClassCreditsExhausted
    }
    return ClassTransient
}
```

### Pattern 3: Background Probe Goroutine

**What:** Started in `New()`, the probe loop uses a `time.Ticker` set to 1 hour. On each tick it iterates unavailable upstreams and sends a minimal completion request; on success it calls `p.Mark(id, true)`.

**When to use:** Periodic background recovery without a scheduler dependency.

**Example:**
```go
// internal/pool/probe.go

func (p *Pool) runProbeLoop() {
    ticker := time.NewTicker(time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            p.probeAll()
        case <-p.stopCh:
            return
        }
    }
}

func (p *Pool) probeAll() {
    p.mu.RLock()
    toProbe := make([]entry, 0)
    for _, e := range p.entries {
        if !e.available {
            toProbe = append(toProbe, e)
        }
    }
    p.mu.RUnlock()

    for _, e := range toProbe {
        if ok := sendProbe(e); ok {
            p.Mark(e.id, true)
        }
    }
}

// sendProbe sends a minimal chat completion ("hi", max_tokens=1) to the upstream.
// Returns true only on a 200-class response with no error body.
func sendProbe(e entry) bool {
    // construct minimal OpenAI-format request body
    // POST e.baseURL + "/v1/chat/completions"
    // Authorization: Bearer e.apiKey
    // timeout: 10s
}
```

### Pattern 4: Pool Injection into Server

Following Phase 1's constructor pattern (`server.New(db, cfg)`), the pool is passed as a constructor argument:

```go
// internal/server/server.go
type Server struct {
    db   *gorm.DB
    cfg  *config.Config
    pool *pool.Pool  // added in Phase 2
}

func New(db *gorm.DB, cfg *config.Config, p *pool.Pool) *Server {
    return &Server{db: db, cfg: cfg, pool: p}
}
```

`main.go` constructs the pool before calling `server.New`, then calls `defer pool.Stop()` for clean shutdown.

### Pattern 5: Config Extension for Rate-Limit Backoff

Following existing Viper `mapstructure` convention:

```go
// internal/config/config.go addition
type PoolConfig struct {
    RateLimitBackoff time.Duration `mapstructure:"rate_limit_backoff"`
}

type Config struct {
    Server    ServerConfig     `mapstructure:"server"`
    Database  DatabaseConfig   `mapstructure:"database"`
    Pool      PoolConfig       `mapstructure:"pool"`
    Upstreams []UpstreamConfig `mapstructure:"upstreams"`
}
```

```yaml
# config.yaml.example addition
pool:
  rate_limit_backoff: 5s
```

Note: Viper reads duration strings as strings by default; use `v.GetDuration("pool.rate_limit_backoff")` and set default `v.SetDefault("pool.rate_limit_backoff", 5*time.Second)`.

### Anti-Patterns to Avoid

- **Reading config file instead of DB at pool startup:** D-15 requires the DB to be the single source of truth. The pool must query `SELECT * FROM upstreams WHERE enabled = true` via GORM, not re-parse `config.yaml`.
- **Storing plaintext API keys in the entry struct without decrypting at load time:** Decrypt once during `New()` using `upstream.DecryptAPIKey(encKey)`; never store ciphertext in the in-memory entry (decrypt-on-every-probe is wasteful and error-prone).
- **Probe goroutine holding pool lock while doing HTTP:** Collect unavailable entries under RLock, release, then probe without holding the lock. Never hold the lock across a network call.
- **Not closing the probe goroutine on shutdown:** `Stop()` must close `stopCh` to unblock the `select`. Without it, the goroutine leaks on server shutdown.
- **Using `sync.Mutex` instead of `sync.RWMutex`:** Select is called on every request; a full mutex blocks concurrent reads unnecessarily. Note: since `Select` mutates `idx`, it does need a write lock, so RWMutex's benefit here is that `Mark` (which is infrequent) is the only write-only operation — but `Select` itself is also a writer. Both patterns are correct; RWMutex signals intent even if both operations use write locks.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP client with timeout | Custom dialer | `http.Client{Timeout: 10*time.Second}` | Stdlib handles TLS, keep-alive, redirect |
| Duration parsing from config | Manual string parse | `viper.GetDuration()` | Parses "5s", "1m", "1h" automatically |
| JSON body decoding for probe response | Manual string scan | `encoding/json.Decoder` | Handles encoding, partial reads |
| Goroutine lifecycle | `sync.WaitGroup` + channels | `stopCh chan struct{}` + `close(stopCh)` | Simple, idiomatic for single background goroutine |

**Key insight:** The pool's complexity lives in correctness (concurrency, error classification) not in infrastructure. Every piece of infrastructure here is stdlib or already-imported.

## Common Pitfalls

### Pitfall 1: Kimi Returns 403 (Not 429) for Quota Exhaustion

**What goes wrong:** D-13 maps HTTP 429 to rate-limited. But Kimi returns HTTP 403 with `type: exceeded_current_quota_error` for credits exhaustion, not 402 or 429.

**Why it happens:** Moonshot AI treats quota exhaustion as an authorization failure, not a payment failure. This is non-standard.

**How to avoid:** The classifier must check body keywords before relying solely on status code. For Kimi specifically, body substring `"exceeded_current_quota_error"` or `"exceeded_current_quota"` signals credits-exhausted regardless of the HTTP status code. The general keyword `"quota"` in the default `creditsKeywords` list covers this without a Kimi-specific override.

**Warning signs:** An upstream being marked `transient` (and rotated away) instead of `unavailable` (and excluded) when Kimi credits run out.

### Pitfall 2: GLM Returns HTTP 429 for Out-of-Credits (Error Code 1113)

**What goes wrong:** GLM uses HTTP 429 and error code `1113` ("Insufficient balance or no resource package") for both rate limiting AND credits exhaustion. A naive HTTP-status-first classifier marks GLM credits-exhaustion as rate-limited.

**Why it happens:** GLM/Zhipu maps their custom error codes onto 429 for everything rate/quota related.

**How to avoid:** The GLM provider override must check body for `"1113"` or `"insufficient balance"` before the status-code 429 rule fires. Classifier must inspect body keywords first for GLM, or the body-keyword check must take priority over the 429 status for credits-related keywords.

**Concrete fix:** In `Classify`, check credits keywords before the `status == 429 → RateLimited` rule:

```go
// Check credits-exhausted keywords first, before the 429 → rate-limit rule.
bodyStr := strings.ToLower(string(body))
for _, kw := range keywords {
    if strings.Contains(bodyStr, kw) {
        return ClassCreditsExhausted
    }
}
if status == http.StatusTooManyRequests {
    return ClassRateLimited
}
```

**Warning signs:** GLM upstream never enters `unavailable` state despite running out of balance.

### Pitfall 3: Minimax Returns HTTP 500 for Insufficient Balance (Error Code 1008)

**What goes wrong:** Minimax returns HTTP 500 with body `{"error":{"type":"api_error","message":"insufficient balance (1008)"}}` when credits are exhausted. A 5xx classifier treating all 500s as transient will never mark Minimax unavailable.

**Why it happens:** Minimax maps billing errors to 500, conflating infrastructure failure with billing failure.

**How to avoid:** The classifier's body-keyword scan must happen before the 5xx → transient fallback. The keyword `"insufficient balance"` and `"1008"` in Minimax's provider override catch this.

**Warning signs:** Minimax rotates back into the pool immediately after every probe (probe also gets 500, which is misclassified as transient → available).

Probe handling note: `sendProbe` must treat a credits-exhausted classification on the probe response as failure (upstream stays unavailable), not as a transient error.

### Pitfall 4: Qwen Uses HTTP 429 with `insufficient_quota` Body for Credits

**What goes wrong:** Like GLM, Qwen returns HTTP 429 with `"insufficient_quota"` in the body for credits exhaustion, not rate limiting. The keyword `"insufficient_quota"` is in the default keyword list; this works only if body-keyword check precedes the 429 → rate-limit rule (see Pitfall 2).

**Why it happens:** DashScope maps quota exhaustion as a rate/quota limit violation.

**How to avoid:** Same fix as GLM — body keyword check before status code rule.

### Pitfall 5: `time.Duration` Not Unmarshalling from config.yaml

**What goes wrong:** Viper reads `rate_limit_backoff: 5s` as a string. `mapstructure` cannot convert a string to `time.Duration` automatically, resulting in zero value.

**Why it happens:** `time.Duration` is `int64` under the hood; mapstructure does not know to call `time.ParseDuration`.

**How to avoid:** Use `v.GetDuration("pool.rate_limit_backoff")` explicitly after `v.Unmarshal` and set the default with `v.SetDefault("pool.rate_limit_backoff", 5*time.Second)`. Do not rely on `mapstructure` struct tag for Duration fields.

### Pitfall 6: Race on Round-Robin Index with `-race` Flag

**What goes wrong:** Test with `-race` flag detects concurrent writes to the index field if the mutex is not held correctly.

**Why it happens:** Two goroutines calling `Select` concurrently both read then write `p.idx = (p.idx+1) % n` without a lock.

**How to avoid:** `Select` must hold the write lock for the entire read-modify-write of `idx`. The success criterion (ROUT-01) explicitly requires the `-race` test to pass.

## Code Examples

Verified patterns from official sources:

### Pool Load from DB at Startup

```go
// internal/pool/pool.go
// Source: GORM documentation (gorm.io/docs/query.html) [VERIFIED: codebase grep of existing database.go]

func New(db *gorm.DB, encKey []byte, cfg *config.PoolConfig) (*Pool, error) {
    var upstreams []models.Upstream
    if err := db.Where("enabled = ?", true).Find(&upstreams).Error; err != nil {
        return nil, fmt.Errorf("pool: load upstreams: %w", err)
    }
    entries := make([]entry, 0, len(upstreams))
    for _, u := range upstreams {
        apiKey, err := u.DecryptAPIKey(encKey)
        if err != nil {
            return nil, fmt.Errorf("pool: decrypt key for %s: %w", u.Name, err)
        }
        entries = append(entries, entry{
            id:        u.ID,
            name:      u.Name,
            baseURL:   u.BaseURL,
            apiKey:    apiKey,
            available: true,
        })
    }
    p := &Pool{
        entries: entries,
        encKey:  encKey,
        stopCh:  make(chan struct{}),
        backoff:  cfg.RateLimitBackoff,
    }
    go p.runProbeLoop()
    return p, nil
}
```

### Mark Upstream Available/Unavailable

```go
// Source: Go sync package [ASSUMED — idiomatic pattern]
func (p *Pool) Mark(id uint, available bool) {
    p.mu.Lock()
    defer p.mu.Unlock()
    for i := range p.entries {
        if p.entries[i].id == id {
            p.entries[i].available = available
            return
        }
    }
}
```

### Probe HTTP Call

```go
// Source: net/http stdlib [ASSUMED — idiomatic pattern]
// Model selection: use the cheapest/fastest model per provider.
// For Kimi: moonshot-v1-8k. For Qwen: qwen-turbo. For GLM: glm-4-flash.
// For Minimax: MiniMax-Text-01. For unknown providers: "gpt-3.5-turbo" (standard).

const probeBody = `{"model":"<model>","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`

func sendProbe(e entry) bool {
    body := strings.ReplaceAll(probeBody, "<model>", probeModel(e.name))
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        e.baseURL+"/v1/chat/completions", strings.NewReader(body))
    if err != nil {
        return false
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+e.apiKey)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    respBody, _ := io.ReadAll(resp.Body)
    class := Classify(e.name, resp.StatusCode, respBody)
    // Only return true if no error classification fired
    return class == ClassTransient && resp.StatusCode < 400
}
```

Wait — the probe must return `true` only on genuine success (2xx, no error body). The correct condition:

```go
return resp.StatusCode >= 200 && resp.StatusCode < 300 && class != ClassCreditsExhausted
```

### Test Pattern (round-robin with -race)

```go
// internal/pool/pool_test.go
func TestSelect_RoundRobin(t *testing.T) {
    p := newTestPool(t, "a", "b")  // helper creates pool with 2 entries
    seen := map[string]int{}
    for i := 0; i < 10; i++ {
        e, err := p.Select("")
        if err != nil {
            t.Fatalf("Select: %v", err)
        }
        seen[e.name]++
    }
    if seen["a"] != 5 || seen["b"] != 5 {
        t.Errorf("uneven distribution: %v", seen)
    }
}

// Run with: go test -race ./internal/pool/...
```

## Provider Error Response Reference

Documented from search results with confidence levels per source:

| Provider | Credits-Exhausted Signal | HTTP Status | Body Keyword | Source Confidence |
|----------|--------------------------|-------------|--------------|-------------------|
| Kimi (Moonshot) | Quota exceeded | 403 | `"exceeded_current_quota_error"`, `"quota"` | MEDIUM — [platform.moonshot.ai](https://platform.moonshot.ai/docs/guide/faq) |
| Qwen (DashScope) | Insufficient quota | 429 | `"insufficient_quota"`, `"quota"` | MEDIUM — [alibabacloud.com/help/en/model-studio/error-code](https://www.alibabacloud.com/help/en/model-studio/error-code) |
| GLM (Zhipu / Z.ai) | Error 1113 | 429 | `"1113"`, `"insufficient balance"` | MEDIUM — [github.com/vercel/ai/issues/9290](https://github.com/vercel/ai/issues/9290) |
| Minimax | Error 1008 | 500 | `"1008"`, `"insufficient balance"` | MEDIUM — [platform.minimax.io/docs/api-reference/errorcode](https://platform.minimax.io/docs/api-reference/errorcode) |
| Unknown / fallback | Any | any | `"insufficient"`, `"balance"`, `"quota"`, `"recharge"` | ASSUMED — general coverage |

**Important:** These keyword patterns are derived from reported errors and documentation, not from live API testing with exhausted keys. They should be validated during Phase 2 integration testing with actual provider keys. Fixture-based unit tests can be written now; live validation is needed before Phase 3.

## Runtime State Inventory

Step 2.5: SKIPPED — Phase 2 is a greenfield package addition (`internal/pool/`), not a rename or refactor. No existing stored data, service config, or OS-registered state references the pool package.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Build and test | Yes | 1.25.0 | — |
| SQLite (via glebarez) | Pool startup DB query | Yes | v1.11.0 in go.mod | — |
| `net/http` | Probe HTTP client | Yes | stdlib | — |
| Provider API endpoints (live) | Integration validation only | Unknown | — | Use httptest.Server for unit tests |

**Missing dependencies with no fallback:** None blocking unit tests. Live provider keys are needed only for integration validation of error keywords — not blocking for planning or initial implementation.

**Missing dependencies with fallback:** Live provider endpoints → use `httptest.NewServer` returning fixture error bodies in unit tests.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` package |
| Config file | None — Go testing is built in |
| Quick run command | `go test ./internal/pool/... -race` |
| Full suite command | `go test ./... -race` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ROUT-01 | Round-robin returns alternating upstreams | unit | `go test -race ./internal/pool/ -run TestSelect_RoundRobin` | Wave 0 |
| ROUT-01 | Select skips unavailable upstreams | unit | `go test -race ./internal/pool/ -run TestSelect_SkipsUnavailable` | Wave 0 |
| ROUT-01 | Select returns error when all unavailable | unit | `go test -race ./internal/pool/ -run TestSelect_NoUpstreams` | Wave 0 |
| UPST-02 | Mark unavailable excludes from subsequent Select | unit | `go test -race ./internal/pool/ -run TestMark_Unavailable` | Wave 0 |
| UPST-03 | Probe success marks upstream available | unit | `go test -race ./internal/pool/ -run TestProbe_RecoverOnSuccess` | Wave 0 |
| UPST-03 | Probe failure keeps upstream unavailable | unit | `go test -race ./internal/pool/ -run TestProbe_StayUnavailableOnFailure` | Wave 0 |
| ROUT-03 | Classifier: Kimi 403 + quota body → CreditsExhausted | unit | `go test ./internal/pool/ -run TestClassify_Kimi` | Wave 0 |
| ROUT-03 | Classifier: GLM 429 + 1113 body → CreditsExhausted (not RateLimited) | unit | `go test ./internal/pool/ -run TestClassify_GLM_1113` | Wave 0 |
| ROUT-03 | Classifier: Minimax 500 + 1008 body → CreditsExhausted | unit | `go test ./internal/pool/ -run TestClassify_Minimax_1008` | Wave 0 |
| ROUT-03 | Classifier: 429 + no credits keywords → RateLimited | unit | `go test ./internal/pool/ -run TestClassify_RateLimit` | Wave 0 |
| ROUT-03 | Classifier: 503 + unknown body → Transient | unit | `go test ./internal/pool/ -run TestClassify_Transient` | Wave 0 |
| ROUT-02 | Round-robin with -race flag (concurrent goroutines) | unit | `go test -race -count=10 ./internal/pool/ -run TestSelect_Concurrent` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test -race ./internal/pool/...`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/pool/pool_test.go` — covers ROUT-01, UPST-02, UPST-03, ROUT-02
- [ ] `internal/pool/classifier_test.go` — covers ROUT-03 with per-provider fixture bodies
- [ ] `internal/pool/testdata/` — fixture JSON files for each provider's error response (optional but clean)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | No | Pool does not authenticate clients |
| V3 Session Management | No | Stateless per-request selection |
| V4 Access Control | No | keyID scoping is out of scope for Phase 2 |
| V5 Input Validation | Yes | Upstream response body is untrusted; read with size cap |
| V6 Cryptography | Yes | `crypto.Decrypt` (AES-GCM) called in `New()` — use existing `internal/crypto` only |

### Input Validation Notes

The upstream response body is read during classification. An upstream could theoretically return a very large body. Use `io.LimitReader` when reading probe and classification bodies:

```go
// Cap body read to 64KB to prevent memory exhaustion from malicious/broken upstreams
respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
```

This applies to both the probe path and the relay classifier call (Phase 3 wires the classifier; mention this constraint now).

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Malicious upstream returning oversized error body | Denial of service | `io.LimitReader(resp.Body, 64*1024)` |
| API key logged in error messages | Information disclosure | Never log `entry.apiKey`; log upstream name only |
| AES key in pool memory | Information disclosure | Existing `OCP_ENCRYPTION_KEY` env var pattern; keys decrypted once at startup and held in-process — acceptable for threat model |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Kimi credits exhaustion body contains `"quota"` as substring | Provider Error Reference | Classifier misses Kimi exhaustion; upstream never marked unavailable — validate with live key |
| A2 | Qwen credits exhaustion is HTTP 429 with `"insufficient_quota"` body | Provider Error Reference | If Qwen uses a different status or keyword, classifier misfires |
| A3 | GLM credits exhaustion uses error code `1113` with body substring match | Provider Error Reference | GLM marked as rate-limited instead of credits-exhausted; rotated instead of excluded |
| A4 | Minimax credits exhaustion is HTTP 500 with `"1008"` in body | Provider Error Reference | All Minimax 500s treated as credits-exhausted if body check happens before 5xx rule |
| A5 | `sendProbe` using `"hi"` with `max_tokens=1` will complete successfully on a healthy upstream | Probe Pattern | Some providers reject `max_tokens=1` or require model-specific params — need live test |
| A6 | The cheapest probe model names (`moonshot-v1-8k`, `qwen-turbo`, `glm-4-flash`, `MiniMax-Text-01`) are valid at time of implementation | Code Examples | Probe returns 404 or model-not-found; upstream not recoverable by probe |

**All A-series items need live-key validation during Phase 2 implementation. Unit tests should use fixture bodies so they are not blocked by this assumption.**

## Open Questions

1. **Probe model names per provider**
   - What we know: Each provider has a "cheapest" model for completions.
   - What's unclear: Exact model IDs and whether they accept `max_tokens=1`.
   - Recommendation: Default to `"gpt-3.5-turbo"` as a probe model slug (most providers accept this for OpenAI-compatible APIs) and add per-provider overrides in the classifier map. Validate during Phase 2 with live keys.

2. **Xiao provider identity**
   - What we know: Listed in PROJECT.md as a named upstream; no public API found.
   - What's unclear: API base URL, error response format, authentication scheme.
   - Recommendation: The extensible keyword map handles unknown providers via the default keyword list. No Xiao-specific code needed; treat as config-only with no override until identified.

3. **Qwen region key**
   - What we know: CN (`dashscope.aliyuncs.com`) and international (`dashscope-intl.aliyuncs.com`) keys are not interchangeable.
   - What's unclear: Which key type is in use for the deployment inside Docker in China.
   - Recommendation: Pool stores the base URL from the DB (set by config at Phase 1 sync). No pool code change needed; operator must configure the correct base URL.

4. **Phase 3 interface contract**
   - What we know: Phase 3 (relay) will call `pool.Select(keyID)` and `pool.Mark(upstreamID, unavailable)`.
   - What's unclear: Whether Phase 3 needs `Select` to return a richer struct (latency metadata, etc.) beyond what D-17 specifies.
   - Recommendation: Expose `Select` returning `(*UpstreamEntry, error)` where `UpstreamEntry` has `{ID, Name, BaseURL, APIKey}`. Keep `Mark(id uint, available bool)` minimal. Add fields in Phase 3 if needed.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| External circuit-breaker lib (sony/gobreaker) | Custom two-state model with atomic counters | Project decision (D-01) | Fewer dependencies; simpler debugging; adequate for 5-10 upstreams |
| Separate health-check service | In-process background goroutine | Project decision (D-07) | No infra dependency; suitable for single-instance deployment |

**Not applicable here:**
- No deprecated Go patterns are in play. The `sync.RWMutex` + goroutine + channel approach is idiomatic modern Go.

## Sources

### Primary (HIGH confidence)

- Go stdlib `sync` package — `sync.RWMutex` semantics; `[VERIFIED: existing codebase uses stdlib extensively]`
- `internal/models/models.go` — `Upstream` struct fields and `DecryptAPIKey` method `[VERIFIED: file read]`
- `internal/crypto/crypto.go` — AES-GCM decrypt API `[VERIFIED: file read]`
- `internal/database/database.go` — GORM query pattern and encKey usage `[VERIFIED: file read]`
- `internal/config/config.go` — Viper mapstructure convention `[VERIFIED: file read]`
- `internal/server/server.go` — constructor injection pattern `[VERIFIED: file read]`
- GORM documentation — `db.Where().Find()` query pattern `[CITED: gorm.io/docs/query.html]`

### Secondary (MEDIUM confidence)

- Kimi/Moonshot error codes — HTTP 403 for quota exhaustion `[CITED: platform.moonshot.ai/docs/guide/faq via WebSearch]`
- Qwen/DashScope error codes — HTTP 429 + `insufficient_quota` `[CITED: alibabacloud.com/help/en/model-studio/error-code via WebSearch]`
- GLM/Zhipu error 1113 — HTTP 429 + body `[CITED: github.com/vercel/ai/issues/9290 via WebSearch]`
- Minimax error 1008 — HTTP 500 + body `[CITED: platform.minimax.io/docs/api-reference/errorcode via WebSearch]`

### Tertiary (LOW confidence)

- Probe model names (`moonshot-v1-8k`, `qwen-turbo`, `glm-4-flash`) — `[ASSUMED]` from general knowledge; needs live validation
- Xiao provider API endpoint — `[ASSUMED]` does not exist; treat as config-only

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all stdlib, existing go.mod dependencies confirmed
- Architecture patterns: HIGH — follows established Phase 1 patterns exactly
- Provider error keywords: MEDIUM — from documentation and issue reports, not live key testing
- Probe model names: LOW — assumed from training knowledge

**Research date:** 2026-04-16
**Valid until:** 2026-05-16 (provider error code formats are stable; model names may change if providers deprecate)
