---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Codex + Portal UX
status: roadmap_ready
last_updated: "2026-04-18T00:00:00.000Z"
progress:
  total_phases: 2
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-18 after v1.1)

**Core value:** A single endpoint that never goes down — when one upstream coding plan runs out of credits or hits a rate limit, ocp silently rotates to the next available one.
**Current focus:** Phase 12 — Responses API (Codex support)

---

## Milestone History

- ✅ **v1.0 MVP** — 8 phases, 18 plans, shipped 2026-04-17. Archive: `.planning/milestones/v1.0-ROADMAP.md`
- ✅ **v1.1 Ops** — 3 phases, complete 2026-04-18
- 🔄 **v1.2 Codex + Portal UX** — 2 phases, roadmap created 2026-04-18

## Current Phase

Phase: Not started (roadmap ready)
Plan: —
Status: Ready to plan Phase 12
Last activity: 2026-04-18 — v1.2 roadmap created (Phases 12–13)

---

## Performance Metrics

| Metric | Value |
|--------|-------|
| v1.0 phases | 8/8 complete |
| v1.0 plans | 18/18 complete |
| v1.0 requirements | 26/26 mapped |
| v1.1 phases | 3/3 complete |
| v1.1 requirements | 10/10 complete |
| v1.2 phases | 0/2 complete |
| v1.2 requirements | 11 mapped, 0 complete |
| Timeline | v1.0: 2026-04-16 → 2026-04-17 (2 days) |

---

## Accumulated Context

### Key Decisions (v1.2)

- **Phase 12 backend only:** Responses API translation is a pure backend addition — no portal changes. Keeps the translation layer separate from UI work.
- **Phase 13 bundles UPST + STAT:** Both are portal-side features with a shared backend API pattern (admin endpoints + React components). One phase avoids artificial split for two small feature areas.
- **STAT-03 backend in Phase 13:** The usage aggregation endpoint is small (one SQL GROUP BY query) and tightly coupled to the portal page that consumes it — no benefit to splitting into its own phase.

### Known Codebase Entry Points (v1.2)

- `internal/server/relay.go` — existing relay handler; Responses API handler will follow same pattern
- `internal/server/admin.go` — admin endpoints; usage aggregation endpoint goes here
- `internal/pool/classifier.go` — upstream error classification; no changes expected for v1.2
- `web/src/components/` — existing portal components; add upstream form and usage page here
- `web/src/App.tsx` (or router) — navigation; add Usage nav link

### Open Questions (Carried from v1.1)

1. **Balance APIs for Minimax, GLM, Qwen.** Only Kimi has a documented programmatic balance endpoint. Deferred to v1.3+.
2. **"Xiao" provider identity.** Listed in PROJECT.md but no public API found. Config-only until identified.
3. **Qwen region key compatibility.** China vs international DashScope keys not interchangeable — confirm in use.

### Open Questions (v1.2)

4. **Responses API event format.** Codex uses `response.output_text.delta` SSE events — need to confirm exact schema from OpenAI Responses API spec before implementing translation.
5. **Phase 9 status.** ROADMAP.md shows Phase 9 (Rate Limit Backend) as "In progress" but git log shows commits for it. Verify completion before starting Phase 12.

---

## Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260417-jxm | Fix pool to load all upstreams including disabled ones so portal can toggle them | 2026-04-17 | 957541f | [260417-jxm-fix-pool-to-load-all-upstreams-including](./quick/260417-jxm-fix-pool-to-load-all-upstreams-including/) |
| 260417-k6u | Add edit/modify functionality for upstreams and access keys | 2026-04-17 | 83ed963 | [260417-k6u-add-edit-modify-functionality-for-upstre](./quick/260417-k6u-add-edit-modify-functionality-for-upstre/) |
| 260417-lff | Remove Format field from upstream config and simplify relay to passthrough-only | 2026-04-17 | df79d5e | [260417-lff-remove-format-field-from-upstream-config](./quick/260417-lff-remove-format-field-from-upstream-config/) |
| 260417-mdn | Add provider adapter pattern: create internal/pool/adapter.go with ProviderAdapter interface, DefaultAdapter, MinimaxAdapter, and registry; wire into anthropic.go and relay.go; update minimax base_url in DB | 2026-04-17 | 307bed5 | [260417-mdn-add-provider-adapter-pattern-create-inte](./quick/260417-mdn-add-provider-adapter-pattern-create-inte/) |

---

## Session Continuity

**Last updated:** 2026-04-18 — v1.2 roadmap created (2 phases: 12 Responses API, 13 Portal UX)
**Next action:** Run `/gsd-plan-phase 12` to plan Phase 12: Responses API
