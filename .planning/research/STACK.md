# Stack Research — one-codingplan (ocp)

**Researched:** 2026-04-16
**Overall confidence:** HIGH for core choices, MEDIUM for provider-specific details

---

## Recommended Stack

### Language: Go

Go is the correct choice for this project, not Python or Node.js.

**Rationale:**
- All existing production AI proxy projects with serious adoption are written in Go: `one-api` (songquanpeng), `new-api` (Calcium-Ion), Bifrost (Maxim AI). This is not coincidence.
- Go's `net/http` goroutine model handles thousands of concurrent long-lived SSE connections without the GIL bottleneck that caps Python/LiteLLM at ~250-300 RPS per instance.
- Streaming proxying maps directly onto Go's `io.Copy` + `http.Flusher` primitives — no async/await complexity.
- Single-binary deployment with `//go:embed` for the React portal fits the Docker/self-hosted target perfectly.
- The Chinese AI provider ecosystem (one-api, new-api) already has community knowledge and solved problems in Go — bugs, streaming quirks, and provider-specific workarounds are documented in Go.

**Go version:** 1.22+ (required for `net/http` routing improvements; `go.mod` should pin `go 1.22`)

---

### HTTP Framework: Gin

Use **Gin** (`github.com/gin-gonic/gin` v1.10.x).

**Rationale:**
- Most battle-tested Go HTTP framework (81k+ GitHub stars, ~48% of Go web developers).
- Both `one-api` and `new-api` use Gin — the streaming middleware patterns from those projects are directly reusable.
- Gin's `c.Stream()` and `c.SSEvent()` methods handle SSE without boilerplate.
- Excellent middleware ecosystem: CORS, JWT, rate limiting, request ID all have ready-made Gin middleware.
- Chi is more idiomatic but has a smaller ecosystem and no SSE helpers. Fiber uses Fasthttp which is **incompatible with `net/http`** — that rules it out because Go's standard `httputil.ReverseProxy` requires `net/http`.

**Do not use Fiber.** Fiber's `fasthttp` backend cannot use `httputil.ReverseProxy` or `net/http` middleware. For a proxy that needs to forward requests transparently, this incompatibility is a hard blocker.

---

### Database: SQLite via GORM (upgradeable to MySQL/Postgres)

Use **GORM** (`gorm.io/gorm` v2) with the **pure-Go SQLite driver** (`github.com/glebarez/sqlite`).

**Rationale:**
- At personal/small-team scale, SQLite is sufficient and removes all infrastructure dependencies — no database container to manage.
- The pure-Go driver (`glebarez/sqlite`) avoids CGo, meaning clean Docker builds on `golang:alpine` without a C compiler. The standard `mattn/go-sqlite3` requires CGo and breaks in stripped containers.
- GORM's `AutoMigrate` handles schema evolution across versions without manual migrations at this scale.
- Same pattern as `one-api`/`new-api`: SQLite default, MySQL via `SQL_DSN` env var for teams. GORM's driver abstraction makes this switchable with one env var.
- Usage tracking writes are async fire-and-forget — SQLite's single-writer lock is not a bottleneck for logging.

**When to switch to Postgres:** If you ever run multiple ocp instances behind a load balancer. Not needed for the current use case.

---

### Streaming: `httputil.ReverseProxy` + `FlushInterval: -1`

For proxying upstream SSE/chunked streams to clients:

```go
proxy := &httputil.ReverseProxy{
    Director:      rewriteRequest,
    FlushInterval: -1,  // flush immediately; auto-detected for text/event-stream
}
```

`FlushInterval: -1` tells Go's reverse proxy to flush each write immediately rather than buffering. As of Go 1.20+, this is automatically applied when the upstream response `Content-Type` is `text/event-stream`, but setting it explicitly is safer.

**Key patterns required:**
- Set `X-Accel-Buffering: no` on responses so nginx (if present) disables buffering.
- Strip `Content-Length` from proxied streaming responses (it will be wrong).
- Send SSE heartbeat comments (`: heartbeat\n\n`) on a 30-second ticker to prevent proxy/client timeouts on idle streams.
- Detect stream vs. non-stream by checking upstream response `Content-Type` before deciding to buffer or pass through.

---

### Web Portal: React + Vite, embedded in the binary

Use **React 18/19 + Vite + TypeScript**, built and embedded via `//go:embed dist`.

**Rationale:**
- The portal is a management UI for one operator, not a public SaaS. It does not need to be a separate service. Embedding it in the proxy binary means one `docker run` deploys everything.
- `//go:embed` is a compile-time directive (Go 1.16+) — the built `dist/` folder from Vite gets baked into the binary. Zero runtime file system dependency.
- Dev workflow: run `vite dev` with a proxy to the Go server for HMR; prod build runs `vite build` before `go build`.
- The `one-api`/`new-api` pattern of Gin serving both `/api/` routes and the embedded SPA root is proven and well-documented.

**UI component library:** Use **shadcn/ui** (Radix UI + Tailwind). It generates unstyled accessible components that you own — no version lock-in, no conflicting CSS, works cleanly with Vite. Avoid Ant Design (heavy, opinionated) and Material UI (excessive for a management portal).

**Do NOT build the portal as a separate service.** There is no scaling requirement that justifies the operational complexity of two deployments and CORS configuration.

---

### CLI Tool: Cobra + Viper

Use **Cobra** (`github.com/spf13/cobra` v1.8+) for the `ocp` CLI, with **Viper** (`github.com/spf13/viper` v1.19+) for config.

**Rationale:**
- Industry standard for Go CLIs (used by kubectl, hugo, gh).
- Cobra handles subcommands (`ocp status`, `ocp keys`, `ocp next`), flag inheritance, and help generation automatically.
- Viper reads config from `~/.ocp/config.yaml` and environment variables — the CLI just needs to know the proxy's base URL and an admin API key.
- The CLI is a thin HTTP client that calls the proxy's management API. Keep it that way — no embedded business logic, no direct DB access.

---

### Upstream Health / Circuit Breaker: Custom, not a library

Do not import a circuit breaker library. Implement the three-state machine (Closed → Open → Half-Open) directly.

**Rationale:**
- The state per upstream is simple: a failure counter, a `lastFailure` timestamp, and a status enum. 20 lines of Go with atomic operations.
- Upstream health for AI providers is not TCP health — it's HTTP-semantic: 429 = rate limited (exponential backoff), 402/insufficient credits = disable until manually re-enabled, 5xx = transient (circuit breaker). These rules require custom logic regardless of library.
- Libraries like `sony/gobreaker` or `cenkalti/backoff` add a dependency for logic you'll customize anyway.

---

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Language | Go | Python/LiteLLM | GIL limits concurrency for streaming; heavier Docker image; less appropriate for pure proxy work |
| Language | Go | Node.js/Fastify | Streaming semantics are workable but the ecosystem for this specific proxy pattern is Go-native |
| Language | Go | Rust/Axum | Correct performance characteristics but steep learning curve, no comparable reference projects in the AI proxy space |
| HTTP framework | Gin | Fiber | Fasthttp incompatibility with `net/http` is a hard blocker for transparent proxying |
| HTTP framework | Gin | Chi | More idiomatic but no SSE helpers, smaller ecosystem, no reference project |
| HTTP framework | Gin | Echo | Comparable to Gin; less community reference material for AI proxies; no meaningful advantage |
| Database | SQLite/GORM | Redis | Overkill; Redis is appropriate if rate limiting needs sub-millisecond shared state across instances — not needed here |
| Database | SQLite/GORM | Postgres | No infrastructure to manage; SQLite is sufficient; GORM makes the switch a one-liner when needed |
| Frontend | React+Vite embedded | Separate SPA service | Operational complexity without benefit; CORS configuration; two deployments for one operator |
| Frontend | shadcn/ui | Ant Design | Ant Design is 2MB+ of CSS, opinionated component behavior, not Tailwind-compatible |
| Build-on | Greenfield | Fork one-api/new-api | Both are AGPLv3 (new-api) or have architectural decisions (billing, reselling) baked in that conflict with ocp's simpler model; learning from, not inheriting |

---

## Key Libraries

| Library | Version | Purpose |
|---------|---------|---------|
| `github.com/gin-gonic/gin` | v1.10.x | HTTP router, SSE helpers, middleware chain |
| `gorm.io/gorm` | v2 latest | ORM, AutoMigrate, driver abstraction |
| `github.com/glebarez/sqlite` | latest | Pure-Go SQLite driver (no CGo, Alpine-safe) |
| `github.com/spf13/cobra` | v1.8+ | CLI subcommands, flag parsing, help generation |
| `github.com/spf13/viper` | v1.19+ | Config file + env var management for CLI |
| `github.com/golang-jwt/jwt/v5` | v5 latest | Access key signing and validation |
| `golang.org/x/time/rate` | stdlib-adjacent | Token bucket rate limiter (per-key req/min enforcement) |
| `github.com/google/uuid` | v1 | Access key ID generation |

**Standard library usage (no extra dependency needed):**
- `net/http/httputil.ReverseProxy` — upstream request forwarding
- `net/http` — HTTP server
- `embed` — bake React dist into binary
- `sync/atomic` — lock-free upstream health counters
- `crypto/rand` — key generation entropy

---

## What NOT to Use

**LiteLLM (Python):** Not a building block — it IS a competing product. Its Python-based architecture has a GIL-limited concurrency ceiling (~250-300 RPS) that makes it inappropriate as an internal component. Reference its provider compatibility list, don't import it.

**Fiber (`gofiber/fiber`):** The `fasthttp` HTTP engine is incompatible with Go's standard `net/http`. `httputil.ReverseProxy`, standard middleware, and most Go HTTP libraries require `net/http`. Using Fiber would require reimplementing the reverse proxy from scratch or maintaining a fork. Not worth it.

**mattn/go-sqlite3 (CGo SQLite):** Requires a C compiler at build time. Breaks Alpine-based Docker builds. Use `glebarez/sqlite` instead — it's slower (pure Go) but the throughput of a management DB is not the bottleneck.

**GORM v1:** Use GORM v2 (`gorm.io/gorm`). GORM v1 is `github.com/jinzhu/gorm` — it's unmaintained and the import path is different. Do not confuse the two.

**Gorilla/mux or httprouter:** Replaced by Gin or Chi. No reason to use naked routers when Gin provides middleware chains, context, and SSE helpers.

**sony/gobreaker or cenkalti/backoff for circuit breaking:** The upstream failure semantics (429 vs 402 vs 5xx) require custom branching logic. A generic library adds a dependency without eliminating the custom code.

**Redis for state:** There is only one ocp instance. Redis would be needed for shared rate-limit counters across replicas. At this scale, in-memory atomic counters are correct.

**Separate Next.js or Remix frontend:** SSR is irrelevant for an admin portal used by one person. Next.js adds a Node.js runtime dependency to the deployment. React+Vite+embed gives you zero external runtime.

---

## Reference Projects (Learn From, Don't Fork)

**`songquanpeng/one-api`** (Go, Gin, GORM, SQLite/MySQL, React)
- MIT license. The closest architectural match.
- Study: channel (upstream) management model, relay controller pattern, streaming response handling, balance polling patterns per provider.
- Do not copy: billing/quota reselling model, user management complexity, channel priority weight system (ocp uses simpler round-robin).
- GitHub: https://github.com/songquanpeng/one-api

**`Calcium-Ion/new-api`** (Go, Gin, GORM, React)
- AGPLv3 — cannot copy code, but architecture is instructive.
- Study: Claude/Gemini format translation layer, newer provider support.
- Do not fork: AGPLv3 infects the entire codebase.

**`Helicone/ai-gateway`** (Go)
- MIT. Newer, cleaner codebase. Less Chinese-provider-specific but good Go proxy patterns.
- GitHub: https://github.com/Helicone/ai-gateway

---

## Confidence Levels

| Area | Confidence | Basis |
|------|------------|-------|
| Go as language | HIGH | Multiple production projects; ecosystem evidence; concurrency requirements |
| Gin as framework | HIGH | Used by one-api/new-api; SSE support verified; 81k stars |
| SQLite + glebarez driver | HIGH | Official GORM docs; Alpine CGo constraint confirmed |
| GORM v2 | HIGH | Official docs; used by both reference projects |
| `httputil.ReverseProxy` + FlushInterval | HIGH | Go stdlib source; upstream Go issue tracker confirmed behavior |
| React+Vite embed pattern | HIGH | Multiple verified tutorials; Go 1.16+ `embed` is stable |
| shadcn/ui for portal | MEDIUM | Good reputation, but UI library choice has low impact on project success |
| Cobra + Viper for CLI | HIGH | Industry standard; kubernetes/hugo/gh all use it |
| Custom circuit breaker (no library) | MEDIUM | Architectural judgment; sony/gobreaker is a reasonable alternative if complexity grows |
| Provider balance APIs exist | LOW | Unclear which of Minimax/Kimi/GLM/Qwen expose a dedicated balance endpoint — requires per-provider investigation in a later phase |

---

## Provider-Specific Notes

All four target providers (Minimax, Kimi/Moonshot, GLM/Zhipu, Qwen/Alibaba) expose OpenAI-compatible endpoints. The base URLs are:

| Provider | Base URL | Notes |
|----------|----------|-------|
| Qwen | `dashscope.aliyuncs.com` (CN) / `dashscope-intl.aliyuncs.com` (intl) | Two endpoints depending on network context |
| Kimi | `api.moonshot.ai` | Standard OpenAI-compatible |
| GLM | `open.bigmodel.cn` (CN) / `api.z.ai` (intl) | Two endpoints |
| Minimax | `api.minimax.io` | Standard; coding plan key separate from pay-as-you-go key |

**Balance/credits API:** No public evidence that any of these providers exposes a dedicated balance polling API. Realistic strategy: detect exhaustion from error responses (402, specific error codes) rather than polling. This needs verification in Phase 1 with actual API keys. Mark as LOW confidence / requires research.

**China network context:** All four providers are Chinese companies with mainland endpoints. Direct access from within China (the deployment target) should work without proxy for these upstreams. The Go HTTP client should use the system proxy env vars (`HTTP_PROXY`, `HTTPS_PROXY`) for any providers that might need it (configurable per-upstream in the channel config).

---

## Sources

- [songquanpeng/one-api GitHub](https://github.com/songquanpeng/one-api) — reference architecture
- [Calcium-Ion/new-api GitHub](https://github.com/Calcium-Ion/new-api) — fork with Claude/Gemini support
- [Helicone/ai-gateway GitHub](https://github.com/Helicone/ai-gateway) — Go, MIT-licensed
- [glebarez/sqlite — pure-Go GORM driver](https://github.com/glebarez/sqlite)
- [Go httputil.ReverseProxy FlushInterval issue #27816](https://github.com/golang/go/issues/27816)
- [Designing AI Proxy Server in Go (2026)](https://dasroot.net/posts/2026/02/designing-ai-proxy-server-go/)
- [Top Go frameworks 2025 — LogRocket](https://blog.logrocket.com/top-go-frameworks-2025/)
- [Embed Vite app in Go binary](https://www.tushar.ch/writing/embed-vite-app-in-go-binary)
- [Cobra CLI framework](https://cobra.dev/)
- [AI Coding Plan Comparison 2026](https://codingplan.org/en/) — provider endpoint reference
- [cc-compatible-models — Chinese provider endpoints](https://github.com/Alorse/cc-compatible-models)
