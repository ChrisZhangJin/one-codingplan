---
phase: 2
slug: upstream-pool-health-monitor
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-16
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — stdlib test runner |
| **Quick run command** | `go test ./internal/pool/... -count=1` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/pool/... -count=1`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | UPST-02 | — | N/A | unit | `go test ./internal/pool/... -run TestSelect -race` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | UPST-02 | — | N/A | unit | `go test ./internal/pool/... -run TestRoundRobin -race` | ❌ W0 | ⬜ pending |
| 02-01-03 | 01 | 1 | UPST-03 | — | N/A | unit | `go test ./internal/pool/... -run TestMark -race` | ❌ W0 | ⬜ pending |
| 02-02-01 | 02 | 1 | ROUT-01 | — | N/A | unit | `go test ./internal/pool/... -run TestClassify -race` | ❌ W0 | ⬜ pending |
| 02-02-02 | 02 | 1 | ROUT-02 | — | N/A | unit | `go test ./internal/pool/... -run TestCooldown -race` | ❌ W0 | ⬜ pending |
| 02-03-01 | 03 | 2 | ROUT-03 | — | N/A | unit | `go test ./internal/pool/... -run TestProbe -race` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/pool/pool_test.go` — stubs for UPST-02, UPST-03, ROUT-01, ROUT-02, ROUT-03
- [ ] `internal/pool/classify_test.go` — error classification fixtures per provider

*If existing test infra covers everything, remove Wave 0 tasks.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Probe model ID accuracy per provider | ROUT-03 | Live key required; model IDs not verifiable without real credentials | Run probe with each provider key; confirm 200 response with minimal token usage |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
