# Requirements: v1.2 Codex + Portal UX

## Milestone Goal

Add Codex CLI support via OpenAI Responses API translation, and fill two portal gaps — upstream creation and per-key usage visibility.

---

## v1.2 Requirements

### Responses API (Codex Support)

- [ ] **RESP-01**: Codex CLI can send requests to ocp's `/v1/responses` endpoint using an ocp access key
- [ ] **RESP-02**: ocp translates `/v1/responses` request body to `/v1/chat/completions` format before forwarding to upstream
- [ ] **RESP-03**: ocp translates the upstream `/v1/chat/completions` response back to Responses API format before returning to Codex
- [ ] **RESP-04**: Streaming responses via `/v1/responses` work end-to-end (Codex uses streaming by default)
- [ ] **RESP-05**: `/v1/responses` requests go through the same auth, failover, and rate-limit middleware as other endpoints

### Upstream Management

- [ ] **UPST-01**: Operator can create a new upstream via the web portal without editing config.yaml
- [ ] **UPST-02**: Add upstream form includes: name, base URL, API key, and model override fields
- [ ] **UPST-03**: Newly created upstream is immediately active in the pool and visible in the upstream list

### Usage Statistics

- [ ] **STAT-01**: Portal has a Usage page accessible from the main navigation
- [ ] **STAT-02**: Usage page shows per-key totals: total requests, total input tokens, total output tokens
- [ ] **STAT-03**: Backend exposes an API endpoint that aggregates usage records grouped by access key

---

## Future Requirements (Deferred)

- Proactive upstream balance polling (Kimi `/v1/users/me/balance` every 5 min)
- Web portal: credits remaining and request counts per upstream
- Usage charts (requests/tokens over time, per-upstream breakdown)
- Prometheus-compatible `/metrics` endpoint
- Per-token rate limits (in addition to per-request)
- Usage breakdown per upstream (in addition to per-key)
- Time-series view on usage statistics page

---

## Out of Scope (v1.2)

- Full Responses API feature parity (file search, code interpreter, web search tools) — translation covers text/streaming only
- Usage statistics per upstream — per-key is sufficient for this milestone
- Time-series charts — aggregate totals per key are sufficient

---

## Traceability

| REQ-ID  | Description                                     | Phase    | Status  |
|---------|-------------------------------------------------|----------|---------|
| RESP-01 | Codex connects to /v1/responses                 | Phase 12 | Pending |
| RESP-02 | Translate /v1/responses → /v1/chat/completions  | Phase 12 | Pending |
| RESP-03 | Translate response back to Responses API format | Phase 12 | Pending |
| RESP-04 | Streaming via /v1/responses                     | Phase 12 | Pending |
| RESP-05 | Auth + failover + rate-limit on /v1/responses   | Phase 12 | Pending |
| UPST-01 | Create upstream via portal                      | Phase 13 | Pending |
| UPST-02 | Add form: name, URL, key, model override        | Phase 13 | Pending |
| UPST-03 | New upstream immediately active in pool         | Phase 13 | Pending |
| STAT-01 | Usage page in portal nav                        | Phase 13 | Pending |
| STAT-02 | Per-key totals: requests + tokens               | Phase 13 | Pending |
| STAT-03 | Backend usage aggregation API                   | Phase 13 | Pending |
