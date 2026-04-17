# Phase 7: CLI - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 7 adds `ocp status`, `ocp next`, and `ocp keys` subcommands to the same binary that runs the server. The CLI is a thin HTTP client that calls the existing Phase 5 management API. No new backend endpoints are added.

**In scope:** Cobra integration, `ocp serve` server command, three CLI subcommands, connection config via flags/env vars, tabwriter table output.
**Not in scope:** New API endpoints, web portal changes, upstream polling, config file management commands.

</domain>

<decisions>
## Implementation Decisions

### CLI Framework
- **D-01:** Add `github.com/spf13/cobra` to go.mod. Use standard Cobra subcommand structure with a root command and three child commands (`status`, `next`, `keys`).

### Server Start Behavior
- **D-02:** The server becomes `ocp serve`. Bare `ocp` (no subcommand) shows Cobra help. Any Docker `CMD` or startup scripts must be updated from `ocp` to `ocp serve`.
- **D-03:** `ocp serve` accepts the same `--config` flag as the current `main.go` (`-config` with `flag` package → `--config` with Cobra persistent flag).

### Connection Config
- **D-04:** Two persistent root-level flags available to all subcommands:
  - `--host` (default: `http://localhost:8080`) — ocp server base URL
  - `--admin-key` — admin bearer token (no default; required for CLI commands)
- **D-05:** Env var fallback: `OCP_HOST` and `OCP_ADMIN_KEY`. Cobra's persistent flags bind to these via `os.Getenv` or Viper's `AutomaticEnv`. Flag value takes precedence over env var.
- **D-06:** No config file for the CLI in this phase. Flags + env vars only.

### Output Format
- **D-07:** All three commands use `text/tabwriter` (stdlib) for column-aligned table output. No color library, no `--json` flag. Plain text readable in terminals, pipes, and log files.

### Error Handling
- **D-08:** When the server is unreachable (connection refused, timeout), print a clear one-line error to stderr and exit with code 1. No stack trace. Example:
  `Error: cannot reach ocp server at http://localhost:8080 — is it running?`
- **D-09:** Non-200 API responses print the `{"error": "..."}` message from the response body and exit 1.

### Command Behavior
- **D-10:** `ocp status` calls `GET /api/upstreams`. Table columns: Name, Healthy, Position (round-robin index), Last Error (if any).
- **D-11:** `ocp next` calls `POST /api/upstreams/rotate`. Prints one confirmation line: `Rotated to: <upstream-name>`.
- **D-12:** `ocp keys` calls `GET /api/keys`. Table columns: ID (short), Name, Token (masked), Enabled, Budget, Rate Limit, Expires, Usage (in/out tokens).

### Claude's Discretion
- Whether `--host` and `--admin-key` are persistent flags on the root command or per-subcommand flags
- Exact Cobra project layout: `cmd/ocp/cmd/root.go`, `cmd/ocp/cmd/serve.go`, etc. vs inline in `main.go`
- Precise tabwriter column widths and padding
- Whether to use Viper's `BindEnv` or manual `os.Getenv` for env var fallback

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Requirements
- `.planning/REQUIREMENTS.md` — CLI-01, CLI-02, CLI-03 (the three requirements for this phase)

### Prior Phase Decisions
- `.planning/phases/05-management-api/05-CONTEXT.md` — D-16 (full API route inventory the CLI calls), D-17 (error envelope format `{"error": "..."}`)
- `.planning/phases/01-project-skeleton-data-layer/01-CONTEXT.md` — D-07 (admin_key in config, existing `--config` flag)

### Technology Guidance
- `CLAUDE.md` (project root) — Cobra + Viper recommendation, existing go.mod dependencies

### Existing Code to Read
- `cmd/ocp/main.go` — current entry point using `flag` package; will be restructured to Cobra
- `internal/server/admin.go` — exact JSON response shapes for upstreams and keys endpoints
- `go.mod` — current dependencies; Viper already present, Cobra must be added

No external specs — requirements fully captured in decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `github.com/spf13/viper` — already in go.mod; can bind env vars for `--host` and `--admin-key` via `viper.AutomaticEnv()`
- `internal/server/admin.go` — `keyResponse` and upstream response types define the JSON the CLI will decode
- `text/tabwriter` — stdlib, no new deps needed for table output

### Established Patterns
- Handler/config pattern: `cfg` struct passed down from `main.go` — CLI can follow same pattern with a `cliConfig` holding host + adminKey
- Error response: `gin.H{"error": "..."}` flat envelope — CLI decodes `struct{ Error string }` from non-200 responses
- Admin Bearer auth: `Authorization: Bearer <admin_key>` header — CLI attaches this to every request

### Integration Points
- `cmd/ocp/main.go` — restructure to Cobra root command; `ocp serve` subcommand wraps existing server startup logic
- New CLI subcommands live in `cmd/ocp/` (same package) or a `cmd/ocp/cmd/` sub-package
- No changes to `internal/` packages — CLI is a consumer, not a modifier

</code_context>

<specifics>
## Specific Ideas

- `ocp serve` flag: `--config` (matches current `-config` flag, just renamed with double-dash for Cobra convention)
- Connection flags available on all subcommands (persistent root flags)
- Env vars: `OCP_HOST`, `OCP_ADMIN_KEY` as fallback without needing to type flags every time

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 07-cli*
*Context gathered: 2026-04-17*
