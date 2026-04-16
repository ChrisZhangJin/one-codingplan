# Phase 1: Project Skeleton & Data Layer - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Bootable Go binary that: starts a Gin HTTP server, serves `GET /health`, reads a YAML config file, seeds upstream provider entries from config into SQLite, and exposes a correctly-schemaed database (upstreams, access_keys, usage_records tables) that survives restarts.

No SOCKS5 support. No management API endpoints (those are Phase 5). No routing logic (Phase 2).

</domain>

<decisions>
## Implementation Decisions

### Language & Runtime
- **D-01:** Go ≥ 1.24 required — use the latest 1.24.x release in go.mod

### Tech Stack (confirmed)
- **D-02:** HTTP framework: Gin v1.10.x
- **D-03:** ORM: GORM v2 with `glebarez/sqlite` pure-Go driver (no CGo, Alpine-safe)
- **D-04:** Config management: Viper (introduced in Phase 1, reused by Phase 7 CLI)
- **D-05:** No SOCKS5 outbound HTTP client — UPST-04 is dropped from scope entirely

### Upstream Config Entry Point
- **D-06:** Upstream providers are configured via a single YAML config file (not a seed API endpoint)
- **D-07:** The YAML config file covers everything: upstream definitions AND server runtime settings (port, DB path, admin key)
- **D-08:** Config file path: `--config` flag at startup, defaulting to `./config.yaml` in the working directory

### Config Loading
- **D-09:** Viper loads `config.yaml` with env var override support for every field (e.g., `OCP_PORT` overrides `server.port`)
- **D-10:** At startup, Viper reads config → seeds/syncs upstream entries into SQLite → server starts

### SQLite Schema
- **D-11:** GORM AutoMigrate creates tables on first run: `upstreams`, `access_keys`, `usage_records`
- **D-12:** `usage_records` table is created with correct schema in Phase 1 but not written to until Phase 3
- **D-13:** Database file path is configurable via config.yaml (`database.path`), defaulting to `./ocp.db`

### Claude's Discretion
- Go project layout follows standard convention: `cmd/ocp/main.go`, `internal/` for packages
- Gin router setup, middleware chain structure, and health endpoint response format
- Exact YAML config schema field names (e.g., `server.port`, `upstreams[].base_url`)
- GORM model struct design (field names, tags, indexes)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Requirements
- `.planning/REQUIREMENTS.md` — UPST-01 (upstream config), USGR-02 (SQLite persistence); UPST-04 is dropped
- `.planning/ROADMAP.md` §Phase 1 — Goal, success criteria, and dependency graph

### Project Context
- `.planning/PROJECT.md` — Core value, constraints, key decisions
- `CLAUDE.md` (project root) — Full tech stack rationale, library versions, reference projects

### External References (no local docs)
- glebarez/sqlite: pure-Go GORM driver — no CGo required
- GORM v2 AutoMigrate docs for schema management

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — greenfield project, no existing code

### Established Patterns
- None yet — Phase 1 establishes the conventions all later phases follow

### Integration Points
- `cmd/ocp/main.go` → entry point for all future phases
- `internal/` packages established here will be imported by Phases 2–7

</code_context>

<specifics>
## Specific Ideas

- Go ≥ 1.24 is a hard constraint (not just a preference)
- No SOCKS5 — removed entirely, not deferred
- Everything (server settings + upstream definitions) lives in one YAML file for simplicity

</specifics>

<deferred>
## Deferred Ideas

- SOCKS5 per-upstream proxy support (UPST-04) — removed from scope entirely by user decision
- Seed API endpoint for upstreams — covered by full management API in Phase 5
- Config hot-reload — not needed for Phase 1; revisit if operators need restart-free updates

</deferred>

---

*Phase: 01-project-skeleton-data-layer*
*Context gathered: 2026-04-16*
