<!-- GSD:project-start source:PROJECT.md -->
## Project

**one-codingplan (ocp)**

one-codingplan (ocp) aggregates multiple AI coding plan credentials (Minimax, Kimi, Xiao, GLM, Qwen and others) behind a single OpenAI-compatible and Anthropic-compatible endpoint. Users point their tools — Claude Code, Codex, or any API client — at ocp using one URL and one key, and ocp handles routing, failover, and credit tracking transparently.

**Core Value:** A single endpoint that never goes down: when one upstream coding plan runs out of credits or hits a rate limit, ocp silently rotates to the next available one.

### Constraints

- **Network**: Deployed in China — upstream API calls may need proxy configuration for some providers
- **Compatibility**: Must accept requests formatted for both OpenAI and Anthropic APIs and forward correctly to whichever upstream is active
- **Extensibility**: New upstream providers must be addable without code changes to routing logic (config-driven)
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Recommended Stack
### Language: Go
- All existing production AI proxy projects with serious adoption are written in Go: `one-api` (songquanpeng), `new-api` (Calcium-Ion), Bifrost (Maxim AI). This is not coincidence.
- Go's `net/http` goroutine model handles thousands of concurrent long-lived SSE connections without the GIL bottleneck that caps Python/LiteLLM at ~250-300 RPS per instance.
- Streaming proxying maps directly onto Go's `io.Copy` + `http.Flusher` primitives — no async/await complexity.
- Single-binary deployment with `//go:embed` for the React portal fits the Docker/self-hosted target perfectly.
- The Chinese AI provider ecosystem (one-api, new-api) already has community knowledge and solved problems in Go — bugs, streaming quirks, and provider-specific workarounds are documented in Go.
### HTTP Framework: Gin
- Most battle-tested Go HTTP framework (81k+ GitHub stars, ~48% of Go web developers).
- Both `one-api` and `new-api` use Gin — the streaming middleware patterns from those projects are directly reusable.
- Gin's `c.Stream()` and `c.SSEvent()` methods handle SSE without boilerplate.
- Excellent middleware ecosystem: CORS, JWT, rate limiting, request ID all have ready-made Gin middleware.
- Chi is more idiomatic but has a smaller ecosystem and no SSE helpers. Fiber uses Fasthttp which is **incompatible with `net/http`** — that rules it out because Go's standard `httputil.ReverseProxy` requires `net/http`.
### Database: SQLite via GORM (upgradeable to MySQL/Postgres)
- At personal/small-team scale, SQLite is sufficient and removes all infrastructure dependencies — no database container to manage.
- The pure-Go driver (`glebarez/sqlite`) avoids CGo, meaning clean Docker builds on `golang:alpine` without a C compiler. The standard `mattn/go-sqlite3` requires CGo and breaks in stripped containers.
- GORM's `AutoMigrate` handles schema evolution across versions without manual migrations at this scale.
- Same pattern as `one-api`/`new-api`: SQLite default, MySQL via `SQL_DSN` env var for teams. GORM's driver abstraction makes this switchable with one env var.
- Usage tracking writes are async fire-and-forget — SQLite's single-writer lock is not a bottleneck for logging.
### Streaming: `httputil.ReverseProxy` + `FlushInterval: -1`
- Set `X-Accel-Buffering: no` on responses so nginx (if present) disables buffering.
- Strip `Content-Length` from proxied streaming responses (it will be wrong).
- Send SSE heartbeat comments (`: heartbeat\n\n`) on a 30-second ticker to prevent proxy/client timeouts on idle streams.
- Detect stream vs. non-stream by checking upstream response `Content-Type` before deciding to buffer or pass through.
### Web Portal: React + Vite, embedded in the binary
- The portal is a management UI for one operator, not a public SaaS. It does not need to be a separate service. Embedding it in the proxy binary means one `docker run` deploys everything.
- `//go:embed` is a compile-time directive (Go 1.16+) — the built `dist/` folder from Vite gets baked into the binary. Zero runtime file system dependency.
- Dev workflow: run `vite dev` with a proxy to the Go server for HMR; prod build runs `vite build` before `go build`.
- The `one-api`/`new-api` pattern of Gin serving both `/api/` routes and the embedded SPA root is proven and well-documented.
### CLI Tool: Cobra + Viper
- Industry standard for Go CLIs (used by kubectl, hugo, gh).
- Cobra handles subcommands (`ocp status`, `ocp keys`, `ocp next`), flag inheritance, and help generation automatically.
- Viper reads config from `~/.ocp/config.yaml` and environment variables — the CLI just needs to know the proxy's base URL and an admin API key.
- The CLI is a thin HTTP client that calls the proxy's management API. Keep it that way — no embedded business logic, no direct DB access.
### Upstream Health / Circuit Breaker: Custom, not a library
- The state per upstream is simple: a failure counter, a `lastFailure` timestamp, and a status enum. 20 lines of Go with atomic operations.
- Upstream health for AI providers is not TCP health — it's HTTP-semantic: 429 = rate limited (exponential backoff), 402/insufficient credits = disable until manually re-enabled, 5xx = transient (circuit breaker). These rules require custom logic regardless of library.
- Libraries like `sony/gobreaker` or `cenkalti/backoff` add a dependency for logic you'll customize anyway.
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
- `net/http/httputil.ReverseProxy` — upstream request forwarding
- `net/http` — HTTP server
- `embed` — bake React dist into binary
- `sync/atomic` — lock-free upstream health counters
- `crypto/rand` — key generation entropy
## What NOT to Use
## Reference Projects (Learn From, Don't Fork)
- MIT license. The closest architectural match.
- Study: channel (upstream) management model, relay controller pattern, streaming response handling, balance polling patterns per provider.
- Do not copy: billing/quota reselling model, user management complexity, channel priority weight system (ocp uses simpler round-robin).
- GitHub: https://github.com/songquanpeng/one-api
- AGPLv3 — cannot copy code, but architecture is instructive.
- Study: Claude/Gemini format translation layer, newer provider support.
- Do not fork: AGPLv3 infects the entire codebase.
- MIT. Newer, cleaner codebase. Less Chinese-provider-specific but good Go proxy patterns.
- GitHub: https://github.com/Helicone/ai-gateway
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
## Provider-Specific Notes
| Provider | Base URL | Notes |
|----------|----------|-------|
| Qwen | `dashscope.aliyuncs.com` (CN) / `dashscope-intl.aliyuncs.com` (intl) | Two endpoints depending on network context |
| Kimi | `api.moonshot.ai` | Standard OpenAI-compatible |
| GLM | `open.bigmodel.cn` (CN) / `api.z.ai` (intl) | Two endpoints |
| Minimax | `api.minimax.io` | Standard; coding plan key separate from pay-as-you-go key |
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
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, or `.github/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
