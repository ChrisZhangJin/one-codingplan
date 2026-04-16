# Requirements: one-codingplan (ocp)

**Defined:** 2026-04-16
**Core Value:** A single endpoint that never goes down — when one upstream coding plan runs out of credits or hits a rate limit, ocp silently rotates to the next available one.

## v1 Requirements

### Proxy Endpoints

- [ ] **PRXY-01**: Client can send OpenAI-format chat completion requests to `/v1/chat/completions` and receive valid OpenAI-format responses
- [ ] **PRXY-02**: Client can send Anthropic-format requests to `/v1/messages` and receive valid Anthropic-format responses
- [ ] **PRXY-03**: Proxy streams SSE responses token-by-token without buffering (both OpenAI and Anthropic formats)
- [ ] **PRXY-04**: Proxy correctly translates Anthropic request format to OpenAI upstream format and translates response back

### Routing & Failover

- [ ] **ROUT-01**: Proxy selects upstreams via round-robin across the key's allowed upstream pool (all upstreams if unrestricted, allowed subset if restricted)
- [ ] **ROUT-02**: Proxy automatically rotates to next available upstream when current upstream returns credits-exhausted, rate-limit, or error/timeout response
- [ ] **ROUT-03**: Proxy classifies upstream error responses per-provider to distinguish credits-exhausted (mark upstream unhealthy) from rate-limited (backoff + retry same upstream) from transient error (retry next upstream)
- [ ] **ROUT-04**: Admin can force-rotate the active upstream via `ocp next` CLI command

### Upstream Management

- [ ] **UPST-01**: Admin can configure upstream providers (name, base URL, API key, enabled/disabled) via config file or management API
- [ ] **UPST-02**: Proxy detects unhealthy upstreams reactively via error-inference and marks them with a cooldown period
- [ ] **UPST-03**: Proxy re-tests cooled-down upstreams and returns them to the active pool when healthy
- ~~**UPST-04**: Proxy supports SOCKS5 outbound HTTP client for upstreams that require proxy to reach through GFW~~ *(removed — all target providers are reachable without proxy; descoped by D-05)*

### Access Keys

- [ ] **KEY-01**: Admin can issue access keys (ocp-prefixed bearer tokens) via management API
- [ ] **KEY-02**: Admin can list all access keys with their status, limits, and usage summary
- [ ] **KEY-03**: Admin can block and unblock an access key (blocked keys receive 401)
- [ ] **KEY-04**: Admin can set a token budget on a key; requests that would exceed the budget are rejected with 429
- [ ] **KEY-05**: Admin can restrict a key to a specific subset of upstream providers; the key's round-robin pool is limited to that subset
- [ ] **KEY-06**: Admin can set an expiry date on a key; expired keys receive 401

### Usage Tracking

- [ ] **USGR-01**: Every proxied request is logged with: key ID, upstream used, input tokens, output tokens, timestamp, latency, and success/error status
- [ ] **USGR-02**: Usage is persisted to SQLite and survives proxy restarts

### Management Portal

- [ ] **PORT-01**: Web portal is accessible at the proxy's base URL (`/`) and requires admin authentication
- [ ] **PORT-02**: Portal displays the key management table (list, create, block/unblock, view limits and usage per key)

### CLI

- [ ] **CLI-01**: `ocp status` command displays all configured upstreams with their health status and current round-robin position
- [ ] **CLI-02**: `ocp next` command forces the proxy to rotate to the next available upstream
- [ ] **CLI-03**: `ocp keys` command lists all access keys with their limits and usage

---

## v2 Requirements

### Health Monitoring

- **HLTH-01**: Proxy polls Kimi balance API (`GET /v1/users/me/balance`) every 5 minutes and marks Kimi unhealthy proactively when balance is zero
- **HLTH-02**: Upstream status dashboard in web portal shows real-time health, credits remaining, and request counts per provider

### Key Limits

- **KEY-07**: Admin can set per-minute and per-day request rate limits on a key; requests exceeding limits receive 429

### Claude Code Integration

- **INTG-01**: Claude Code slash command `/proxy-status` calls proxy management API and injects current upstream health into conversation context
- **INTG-02**: Claude Code slash command `/proxy-next` calls `POST /api/upstreams/rotate` to force-rotate active upstream

### Observability

- **OBS-01**: Web portal shows usage charts (requests/tokens over time, per upstream breakdown)
- **OBS-02**: Proxy exposes Prometheus-compatible `/metrics` endpoint

---

## Out of Scope

| Feature | Reason |
|---------|--------|
| Reselling / billing | ocp is a personal/team routing layer, not a commercial gateway |
| Session pinning to specific upstream | Force-next is sufficient; pinning adds state complexity |
| Semantic caching | Not core to the failover value prop; adds significant complexity |
| OAuth / SSO for portal | Static admin key is sufficient for personal/team use |
| Redis / Postgres backend | SQLite is sufficient; multi-instance not a v1 requirement |
| Non-OpenAI/non-Anthropic upstream formats | All target providers expose compatible APIs |
| Plugin system / hooks | Scope creep; configure upstreams via config, not plugins |
| Webhook alerts | Defer; not needed while portal + CLI cover visibility |

---

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| UPST-01 | Phase 1 | Pending |
| ~~UPST-04~~ | ~~Phase 1~~ | Removed |
| USGR-02 | Phase 1 | Pending |
| UPST-02 | Phase 2 | Pending |
| UPST-03 | Phase 2 | Pending |
| ROUT-01 | Phase 2 | Pending |
| ROUT-02 | Phase 2 | Pending |
| ROUT-03 | Phase 2 | Pending |
| PRXY-01 | Phase 3 | Pending |
| PRXY-03 | Phase 3 | Pending |
| USGR-01 | Phase 3 | Pending |
| PRXY-02 | Phase 4 | Pending |
| PRXY-04 | Phase 4 | Pending |
| KEY-01 | Phase 5 | Pending |
| KEY-02 | Phase 5 | Pending |
| KEY-03 | Phase 5 | Pending |
| KEY-04 | Phase 5 | Pending |
| KEY-05 | Phase 5 | Pending |
| KEY-06 | Phase 5 | Pending |
| ROUT-04 | Phase 5 | Pending |
| PORT-01 | Phase 6 | Pending |
| PORT-02 | Phase 6 | Pending |
| CLI-01 | Phase 7 | Pending |
| CLI-02 | Phase 7 | Pending |
| CLI-03 | Phase 7 | Pending |

**Coverage:**
- v1 requirements: 24 total (UPST-04 removed)
- Mapped to phases: 24
- Unmapped: 0

---
*Requirements defined: 2026-04-16*
*Last updated: 2026-04-16 after roadmap creation*
