# Phase 6: Web Portal - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 6 delivers a React SPA embedded in the compiled Go binary. An admin authenticates with the admin key, views upstream health status with enabled/disabled toggle, and manages access keys (list, create, block/unblock, view details). The portal is a pure consumer of Phase 5's `/api/*` routes — no new backend API endpoints are added in this phase.

**In scope:** React SPA scaffold, Vite build wired into Go embed, login flow, upstream status dashboard, key management table.
**Not in scope:** New API endpoints, CLI, upstream polling logic, non-admin views.

</domain>

<decisions>
## Implementation Decisions

### Auth / Session Handling
- **D-01:** Admin key is stored in `localStorage` after successful login. Persists across browser restarts and tabs. Chosen for operator convenience on a personal/team tool with no third-party JS.
- **D-02:** Login UI is a full-page centered login form. The portal content is never partially visible before authentication — unauthenticated state redirects to the login screen at the same URL root.
- **D-03:** Any `401` response from the `/api/*` endpoints clears the stored admin key from `localStorage` and redirects to the login screen. No toast/stay — clean redirect on invalid or revoked key.

### Claude's Discretion
- UI component library choice (shadcn/ui recommended in CLAUDE.md, but Claude may use simpler primitives if the scope is narrow)
- Data refresh strategy for upstream status (polling interval, manual refresh button, or fetch-on-mount only)
- Key create/edit UX pattern (modal dialog, inline, or sidebar)
- Vite build integration with Go `//go:embed` (Makefile target vs `go:generate` vs shell script)
- Exact visual layout of dashboard vs key table (single-page tabs, two-panel, or separate routes)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Requirements
- `.planning/REQUIREMENTS.md` — PORT-01, PORT-02 (the two requirements covered by this phase)

### Prior Phase Decisions
- `.planning/phases/05-management-api/05-CONTEXT.md` — complete API contract (routes, request/response shapes, admin auth pattern) that the portal consumes
- `.planning/phases/01-project-skeleton-data-layer/01-CONTEXT.md` — D-07 (admin_key in config), embedded binary pattern

### Technology Guidance
- `CLAUDE.md` (project root) — Recommended stack: React + Vite, `//go:embed`, shadcn/ui (MEDIUM confidence), no Ant Design

No external specs — requirements fully captured in decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/server/server.go` — route registration point; the SPA will be served from the same Gin server via a catch-all route
- `internal/server/admin.go` — all `/api/*` handlers the portal calls; read for exact request/response JSON shapes before building API client
- `internal/models/models.go` — `AccessKey` struct defines the data shape for the key management table

### Established Patterns
- Gin handler pattern: `func (s *Server) handleXxx(c *gin.Context)` — the catch-all SPA handler will follow the same shape
- Error envelope: `{"error": "message"}` — portal's fetch wrapper should surface this on non-200 responses
- Admin Bearer auth: `Authorization: Bearer <admin_key>` — portal attaches this header to every `/api/*` request from localStorage

### Integration Points
- `//go:embed web/dist` on a `fs.FS` variable, served by Gin's static file middleware — new `web/` directory at project root
- Gin catch-all `r.NoRoute(...)` or `r.GET("/*path", ...)` to serve `index.html` for all non-`/api` and non-`/v1` paths (SPA routing)
- No changes to existing relay or admin handlers — portal is additive only

</code_context>

<specifics>
## Specific Ideas

- Login form: single password/key input field, centered, minimal — no username field (admin key is the only credential)
- On 401: clear `localStorage` key, redirect to login — no intermediate error page
- The portal is for one operator: keep the UI functional over decorative

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 06-web-portal*
*Context gathered: 2026-04-17*
