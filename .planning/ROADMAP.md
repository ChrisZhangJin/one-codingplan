# Roadmap: one-codingplan (ocp)

## Milestones

- ✅ **v1.0 MVP** — Phases 1–8 (shipped 2026-04-17)
- ✅ **v1.1 Ops** — Phases 9–11 (complete 2026-04-18)
- 🔄 **v1.2 Codex + Portal UX** — Phases 12–13 (active)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1–8) — SHIPPED 2026-04-17</summary>

- [x] Phase 1: Project Skeleton & Data Layer (2/2 plans) — completed 2026-04-16
- [x] Phase 2: Upstream Pool & Health Monitor (2/2 plans) — completed 2026-04-16
- [x] Phase 3: Relay Pipeline (OpenAI Pass-Through) (2/2 plans) — completed 2026-04-16
- [x] Phase 4: Anthropic Format Translation (3/3 plans) — completed 2026-04-16
- [x] Phase 5: Management API (2/2 plans) — completed 2026-04-16
- [x] Phase 6: Web Portal (3/3 plans) — completed 2026-04-17
- [x] Phase 7: CLI (2/2 plans) — completed 2026-04-17
- [x] Phase 8: Upstream Format Flexibility (2/2 plans) — completed 2026-04-17

Full phase details: `.planning/milestones/v1.0-ROADMAP.md`

</details>

<details>
<summary>✅ v1.1 Ops (Phases 9–11) — COMPLETE 2026-04-18</summary>

- [x] Phase 9: Rate Limit Backend — Per-minute and per-day request caps enforced via admin API and middleware
- [x] Phase 10: Rate Limit Portal — Rate limit fields visible and editable in web portal per key
- [x] Phase 11: Docker Deployment — Service builds, runs, and persists via Dockerfile + docker-compose

</details>

### v1.2 Codex + Portal UX (Phases 12–13)

- [ ] **Phase 12: Responses API** - Codex CLI connects to ocp via `/v1/responses` with full translation, streaming, and middleware
- [ ] **Phase 13: Portal UX** - Operators can add upstreams and view per-key usage statistics from the web portal

## Phase Details

### Phase 9: Rate Limit Backend
**Goal**: Access keys enforce configurable per-minute and per-day request caps, rejectable with 429 responses
**Depends on**: Nothing (builds on existing v1.0 admin API and limit middleware)
**Requirements**: RATE-01, RATE-02, RATE-03, RATE-04
**Success Criteria** (what must be TRUE):
  1. Admin can set a per-day request limit on an access key via the admin API (PATCH/PUT key endpoint)
  2. A request from a key that has hit its per-minute limit receives a 429 response with no upstream call made
  3. A request from a key that has hit its per-day limit receives a 429 response with no upstream call made
  4. Per-minute and per-day limits independently enforce: exhausting one does not affect the other's counter
**Plans:** 1 plan
Plans:
- [x] 09-01-PLAN.md — Update 429 format to OpenAI-compatible, add per-day test, fix e2e field name

### Phase 10: Rate Limit Portal
**Goal**: Operators can see and update rate limit configuration for any key directly from the web portal
**Depends on**: Phase 9
**Requirements**: RATE-05, RATE-06
**Success Criteria** (what must be TRUE):
  1. Key management table displays per-minute limit, per-day limit, and current-day request count for each key
  2. Admin can open the edit dialog for an existing key and update per-minute or per-day limit; changes persist and take effect immediately
  3. A key with no rate limit set displays a clear "unlimited" indicator, not a zero or blank
**Plans:** 1/1 plans complete
Plans:
- [x] 10-01-PLAN.md — Add day_usage to backend keyResponse, add Rate/min, Rate/day, Today columns to KeyTable

### Phase 11: Docker Deployment
**Goal**: Service runs cleanly in a container with persistent storage, and the CLI is accessible via compose
**Depends on**: Phase 9 (rate limit model changes must be in before baking binary)
**Requirements**: DOCK-01, DOCK-02, DOCK-03, DOCK-04
**Success Criteria** (what must be TRUE):
  1. `docker build` completes without error using the repo Dockerfile
  2. `docker compose up` starts the service and it responds to `GET /health` and proxies requests
  3. Stopping and restarting the container via `docker compose down` then `docker compose up` preserves all upstreams, keys, and usage records
  4. `docker compose run ocp-cli status` returns current upstream pool status without error
**Plans:** 2/2 plans complete
Plans:
- [x] 11-01-PLAN.md — Fix Dockerfile, compose, and config for working Docker deployment

### Phase 12: Responses API
**Goal**: Codex CLI can send requests to ocp's `/v1/responses` endpoint and receive correctly translated responses, with full streaming, auth, failover, and rate-limit enforcement
**Depends on**: Nothing (builds on existing relay pipeline and middleware; no v1.2 prerequisite)
**Requirements**: RESP-01, RESP-02, RESP-03, RESP-04, RESP-05
**Success Criteria** (what must be TRUE):
  1. Codex CLI can target ocp (`OPENAI_BASE_URL=http://ocp`) and complete a coding request without configuration changes on the Codex side
  2. A non-streaming request to `/v1/responses` returns a response in Responses API format (not chat completions format)
  3. A streaming request to `/v1/responses` delivers incremental output events in Responses API SSE format end-to-end
  4. A request to `/v1/responses` with an invalid or missing access key receives a 401 response
  5. A request to `/v1/responses` that triggers upstream failover succeeds transparently (Codex sees no error)
**Plans:** 2 plans
Plans:
- [ ] 12-01-PLAN.md — Responses API types and bidirectional translation functions (request + response)
- [ ] 12-02-PLAN.md — Stream translator, handler, route registration, and e2e tests

### Phase 13: Portal UX
**Goal**: Operators can add new upstreams directly from the web portal and view per-key usage statistics without touching config.yaml or the admin API
**Depends on**: Phase 12 (no hard dependency; can run in parallel, but sequential keeps scope clean)
**Requirements**: UPST-01, UPST-02, UPST-03, STAT-01, STAT-02, STAT-03
**Success Criteria** (what must be TRUE):
  1. Operator fills in the add-upstream form (name, base URL, API key, model override) and submits; the new upstream appears in the upstream list immediately without a page reload
  2. The newly added upstream is immediately included in the round-robin pool and receives requests
  3. A "Usage" link is present in the portal navigation and opens the usage statistics page
  4. The usage page displays a table with one row per access key showing total requests, total input tokens, and total output tokens
**Plans**: TBD
**UI hint**: yes

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Skeleton & Data Layer | v1.0 | 2/2 | Complete | 2026-04-16 |
| 2. Upstream Pool & Health | v1.0 | 2/2 | Complete | 2026-04-16 |
| 3. Relay Pipeline (OpenAI) | v1.0 | 2/2 | Complete | 2026-04-16 |
| 4. Anthropic Translation | v1.0 | 3/3 | Complete | 2026-04-16 |
| 5. Management API | v1.0 | 2/2 | Complete | 2026-04-16 |
| 6. Web Portal | v1.0 | 3/3 | Complete | 2026-04-17 |
| 7. CLI | v1.0 | 2/2 | Complete | 2026-04-17 |
| 8. Upstream Format Flexibility | v1.0 | 2/2 | Complete | 2026-04-17 |
| 9. Rate Limit Backend | v1.1 | 1/1 | Complete | 2026-04-18 |
| 10. Rate Limit Portal | v1.1 | 1/1 | Complete | 2026-04-18 |
| 11. Docker Deployment | v1.1 | 2/2 | Complete | 2026-04-18 |
| 12. Responses API | v1.2 | 0/2 | Not started | - |
| 13. Portal UX | v1.2 | 0/? | Not started | - |

---
*Roadmap created: 2026-04-16 · v1.0 archived: 2026-04-17 · v1.1 phases added: 2026-04-18 · v1.2 phases added: 2026-04-18*
