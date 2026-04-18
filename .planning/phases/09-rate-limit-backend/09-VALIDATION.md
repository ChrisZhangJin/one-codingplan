---
phase: 9
slug: rate-limit-backend
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-18
---

# Phase 9 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none |
| **Quick run command** | `go test ./internal/server/ -run TestLimit -v` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/server/ -run TestLimit -v`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 9-01-01 | 01 | 1 | RATE-01 | — | 429 body uses OpenAI nested format | unit | `go test ./internal/server/ -run TestLimit -v` | ✅ | ⬜ pending |
| 9-01-02 | 01 | 1 | RATE-02 | — | Per-minute 429 returns OpenAI format | unit | `go test ./internal/server/ -run TestLimitMiddleware_RatePerMinute` | ✅ | ⬜ pending |
| 9-01-03 | 01 | 1 | RATE-03 | — | Per-day 429 returns OpenAI format | unit | `go test ./internal/server/ -run TestLimitMiddleware_RatePerDay` | ❌ W0 | ⬜ pending |
| 9-01-04 | 01 | 1 | RATE-04 | — | e2e field name parses correctly | integration | `go test ./internal/server/ -run TestE2E -v` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/server/limit.go` — export `ResetPerDayCounters()` function
- [ ] `internal/server/admin_test.go` — add `TestLimitMiddleware_RatePerDay` test stub

*Wave 0 must pass before Wave 1 implementation tasks run.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| 429 response body visible to API client | RATE-01 | End-to-end format check | `curl -H "Authorization: Bearer <key>" http://localhost:8080/v1/chat/completions` after exhausting rate limit |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
