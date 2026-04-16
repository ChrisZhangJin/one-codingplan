---
phase: 5
slug: management-api
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-16
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package |
| **Config file** | none — existing infrastructure |
| **Quick run command** | `go test ./internal/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 5-01-01 | 01 | 1 | KEY-01 | — | Admin token required for all /api/* routes | unit | `go test ./internal/api/...` | ❌ W0 | ⬜ pending |
| 5-01-02 | 01 | 1 | KEY-02 | — | GET /api/keys returns correct JSON shape | unit | `go test ./internal/api/...` | ❌ W0 | ⬜ pending |
| 5-01-03 | 01 | 1 | KEY-03 | — | POST /api/keys/:id/block returns 401 for next proxied request | integration | `go test ./internal/api/...` | ❌ W0 | ⬜ pending |
| 5-01-04 | 01 | 1 | KEY-04 | — | Token budget exceeded returns 429 | unit | `go test ./internal/api/...` | ❌ W0 | ⬜ pending |
| 5-01-05 | 01 | 1 | KEY-05 | — | Allowed upstreams restriction enforced | unit | `go test ./internal/api/...` | ❌ W0 | ⬜ pending |
| 5-01-06 | 01 | 1 | KEY-06 | — | Expiry enforced (401 after expiry) | unit | `go test ./internal/api/...` | ❌ W0 | ⬜ pending |
| 5-01-07 | 01 | 1 | ROUT-04 | — | POST /api/upstreams/rotate advances round-robin | unit | `go test ./internal/pool/...` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/api/keys_test.go` — stubs for KEY-01 through KEY-06
- [ ] `internal/api/upstreams_test.go` — stubs for ROUT-04

*Existing Go test infrastructure covers the framework; only new test files needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| End-to-end key block via curl | KEY-03 | Requires live proxy + upstream mock | Create key, block it, attempt proxy request, verify 401 |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
