---
phase: 1
slug: project-skeleton-data-layer
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-16
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | stdlib `testing` package (Go 1.24.13 built-in) |
| **Config file** | none — `go test ./...` requires no config file |
| **Quick run command** | `go test ./... -count=1` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -count=1`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 0 | UPST-01 | — | N/A | unit | `go test ./internal/config/... -run TestConfigLoad -v` | ❌ W0 | ⬜ pending |
| 1-01-02 | 01 | 0 | USGR-02 | — | N/A | integration | `go test ./internal/database/... -run TestAutoMigrate -v` | ❌ W0 | ⬜ pending |
| 1-01-03 | 01 | 0 | USGR-02 | — | N/A | integration | `go test ./internal/database/... -run TestPersistence -v` | ❌ W0 | ⬜ pending |
| 1-01-04 | 01 | 1 | UPST-01 | — | N/A | integration | `go test ./internal/database/... -run TestSyncUpstreams -v` | ❌ W0 | ⬜ pending |
| 1-01-05 | 01 | 1 | (SC-1) | — | N/A | unit | `go test ./internal/server/... -run TestHealthEndpoint -v` | ❌ W0 | ⬜ pending |
| 1-01-06 | 01 | 2 | UPST-04 | — | N/A | integration | `go test ./internal/httpclient/... -run TestSOCKS5Proxy -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/config/config_test.go` — stubs for UPST-01 config loading + env override
- [ ] `internal/database/database_test.go` — stubs for USGR-02 AutoMigrate + persistence + upstream sync
- [ ] `internal/server/server_test.go` — stub for health endpoint (SC-1)
- [ ] `internal/httpclient/httpclient_test.go` — stub for SOCKS5 proxy (UPST-04)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `go run ./cmd/ocp` starts without error | SC-1 | Binary smoke test requires running process | `go run ./cmd/ocp & sleep 2 && curl -s http://localhost:8080/health` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
