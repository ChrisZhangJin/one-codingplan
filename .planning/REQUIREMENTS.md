# Requirements: v1.1 Ops

## Milestone Goal

Add per-key rate limiting and production-ready Docker deployment.

---

## v1.1 Requirements

### Rate Limiting

- [ ] **RATE-01**: Admin can set a per-minute request limit on an access key via the admin API
- [ ] **RATE-02**: Admin can set a per-day request limit on an access key via the admin API
- [ ] **RATE-03**: Requests from a key that exceeds its per-minute limit receive a 429 response
- [ ] **RATE-04**: Requests from a key that exceeds its per-day limit receive a 429 response
- [ ] **RATE-05**: Rate limit fields (per-minute, per-day, current usage) are visible per key in the web portal
- [ ] **RATE-06**: Admin can update rate limits on existing keys via the edit key dialog in the portal

### Docker Deployment

- [ ] **DOCK-01**: Service builds successfully via `docker build` using the provided Dockerfile
- [ ] **DOCK-02**: Service starts and serves requests via `docker compose up`
- [ ] **DOCK-03**: Database file persists across container restarts via mounted volume
- [ ] **DOCK-04**: CLI controller (`ocp-cli`) runs on-demand via `docker compose run`

---

## Future Requirements (Deferred)

- Proactive upstream balance polling (Kimi `/v1/users/me/balance` every 5 min)
- Web portal: credits remaining and request counts per upstream
- Usage charts (requests/tokens over time, per-upstream breakdown)
- Prometheus-compatible `/metrics` endpoint
- Per-token rate limits (in addition to per-request)

---

## Out of Scope (v1.1)

- Per-token day/minute limits — per-request limits are sufficient for this milestone
- Rate limit persistence across server restarts for in-flight windows — reset on restart is acceptable
- OAuth/SSO — static admin key is sufficient

---

## Traceability

| REQ-ID  | Description                        | Phase    | Status  |
|---------|------------------------------------|----------|---------|
| RATE-01 | Admin sets per-minute limit        | Phase 9  | Pending |
| RATE-02 | Admin sets per-day limit           | Phase 9  | Pending |
| RATE-03 | 429 on per-minute exceeded         | Phase 9  | Pending |
| RATE-04 | 429 on per-day exceeded            | Phase 9  | Pending |
| RATE-05 | Rate limit visible in portal       | Phase 10 | Pending |
| RATE-06 | Edit rate limits via portal        | Phase 10 | Pending |
| DOCK-01 | docker build succeeds              | Phase 11 | Pending |
| DOCK-02 | docker compose up serves requests  | Phase 11 | Pending |
| DOCK-03 | DB persists via volume             | Phase 11 | Pending |
| DOCK-04 | ocp-cli runs via compose run       | Phase 11 | Pending |
