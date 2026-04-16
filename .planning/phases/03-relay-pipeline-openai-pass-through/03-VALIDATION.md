---
phase: 3
slug: relay-pipeline-openai-pass-through
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-16
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `net/http/httptest` |
| **Config file** | none — Wave 0 creates test file |
| **Quick run command** | `go test ./internal/server/... -run TestRelay -v` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/server/... -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** ~5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 3-01 | relay | 0 | PRXY-01 | — | Test stubs created before impl | unit | `go test ./internal/server/... -run TestRelay -v` | ❌ W0 | ⬜ pending |
| 3-02 | relay | 1 | PRXY-01 | T-3-auth | Missing token → 401, no upstream call | unit | `go test ./internal/server/... -run TestRelay_Auth_Missing` | ❌ W0 | ⬜ pending |
| 3-03 | relay | 1 | PRXY-01 | T-3-auth | Invalid token → 401, no upstream call | unit | `go test ./internal/server/... -run TestRelay_Auth_Invalid` | ❌ W0 | ⬜ pending |
| 3-04 | relay | 1 | PRXY-01 | — | Valid token + non-streaming → upstream response forwarded | unit | `go test ./internal/server/... -run TestRelay_NonStream` | ❌ W0 | ⬜ pending |
| 3-05 | relay | 1 | PRXY-01 | — | First upstream fails, second succeeds (failover) | unit | `go test ./internal/server/... -run TestRelay_Failover_Credits` | ❌ W0 | ⬜ pending |
| 3-06 | relay | 1 | PRXY-01 | — | All upstreams fail → 503 | unit | `go test ./internal/server/... -run TestRelay_AllFail` | ❌ W0 | ⬜ pending |
| 3-07 | relay | 2 | PRXY-03 | — | SSE frames arrive without buffering | unit | `go test ./internal/server/... -run TestRelay_Stream` | ❌ W0 | ⬜ pending |
| 3-08 | relay | 2 | PRXY-03 | — | Heartbeat comment sent on idle stream | unit | `go test ./internal/server/... -run TestRelay_Stream_Heartbeat` | ❌ W0 | ⬜ pending |
| 3-09 | relay | 2 | USGR-01 | — | Usage record written after successful request | unit | `go test ./internal/server/... -run TestRelay_Usage_Success` | ❌ W0 | ⬜ pending |
| 3-10 | relay | 2 | USGR-01 | — | Usage record written with Success=false on all-fail | unit | `go test ./internal/server/... -run TestRelay_Usage_Failure` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/server/relay_test.go` — stubs for PRXY-01, PRXY-03, USGR-01 (all test functions must exist and compile before implementation starts)

*Existing Go stdlib testing infrastructure covers all phase requirements — no new framework install required.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| SSE stream arrives token-by-token with no buffering delay (perceived latency) | PRXY-03 | Cannot assert subjective latency in unit test | Run `curl -N -H "Authorization: Bearer <key>" http://localhost:8080/v1/chat/completions -d '{"model":"...","messages":[...],"stream":true}'` and observe tokens appear progressively |
| Usage record survives proxy restart | USGR-01 | Requires process lifecycle | After a proxied request, `kill` the server and restart; verify record still in SQLite with `sqlite3 ocp.db "SELECT * FROM usage_records ORDER BY created_at DESC LIMIT 1;"` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
