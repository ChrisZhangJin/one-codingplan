---
phase: 4
slug: anthropic-format-translation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-16
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` package (stdlib) |
| **Config file** | none — standard `go test` |
| **Quick run command** | `go test ./internal/translator/... -v` |
| **Full suite command** | `go test ./... -race` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/translator/... -v`
- **After every plan wave:** Run `go test ./... -race`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 4-W0-01 | W0 | 0 | PRXY-04 | — | N/A | unit | `go test ./internal/translator/... -v` | ❌ W0 | ⬜ pending |
| 4-W0-02 | W0 | 0 | PRXY-02 | — | N/A | integration | `go test ./internal/server/... -run TestAnthropicRelay -v` | ❌ W0 | ⬜ pending |
| 4-01-01 | 01 | 1 | PRXY-04 | T-V5 | json.Unmarshal errors return 400 | unit | `go test ./internal/translator/... -run TestAnthropicToOpenAI -v` | ❌ W0 | ⬜ pending |
| 4-01-02 | 01 | 1 | PRXY-04 | T-V5 | tool_result blocks translate correctly | unit | `go test ./internal/translator/... -run TestAnthropicToOpenAI_ToolResult -v` | ❌ W0 | ⬜ pending |
| 4-01-03 | 01 | 1 | PRXY-04 | — | OpenAI text response translates correctly | unit | `go test ./internal/translator/... -run TestOpenAIToAnthropic -v` | ❌ W0 | ⬜ pending |
| 4-01-04 | 01 | 1 | PRXY-04 | — | tool_calls translate to tool_use blocks | unit | `go test ./internal/translator/... -run TestOpenAIToAnthropic_ToolUse -v` | ❌ W0 | ⬜ pending |
| 4-01-05 | 01 | 1 | PRXY-04 | — | originalModel echoed in response (D-03) | unit | `go test ./internal/translator/... -run TestModelEcho -v` | ❌ W0 | ⬜ pending |
| 4-02-01 | 02 | 1 | PRXY-04 | — | StreamTranslator emits message_start on first delta | unit | `go test ./internal/translator/... -run TestStreamTranslator_Start -v` | ❌ W0 | ⬜ pending |
| 4-02-02 | 02 | 1 | PRXY-04 | — | StreamTranslator emits message_stop on [DONE] | unit | `go test ./internal/translator/... -run TestStreamTranslator_Done -v` | ❌ W0 | ⬜ pending |
| 4-02-03 | 02 | 1 | PRXY-04 | — | StreamTranslator handles partial frame buffering | unit | `go test ./internal/translator/... -run TestStreamTranslator_Partial -v` | ❌ W0 | ⬜ pending |
| 4-03-01 | 03 | 2 | PRXY-02 | T-V5 | /v1/messages returns valid Anthropic response schema | integration | `go test ./internal/server/... -run TestAnthropicRelay -v` | ❌ W0 | ⬜ pending |
| 4-03-02 | 03 | 2 | PRXY-02 | — | /v1/messages SSE delivers correct event sequence | integration | `go test ./internal/server/... -run TestAnthropicStream -v` | ❌ W0 | ⬜ pending |
| 4-03-03 | 03 | 2 | PRXY-02 | — | Multi-turn tool use round-trip (D-07) | integration | `go test ./internal/server/... -run TestAnthropicToolRoundTrip -v` | ❌ W0 | ⬜ pending |
| 4-03-04 | 03 | 2 | PRXY-01 | — | OpenAI pass-through unaffected (regression) | regression | `go test ./internal/server/... -run TestRelay -v` | ✅ relay_test.go | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/translator/translator_test.go` — unit test stubs for all PRXY-04 translation functions
- [ ] `internal/server/anthropic_test.go` — integration test stubs for PRXY-02 handler tests
- [ ] `internal/translator/types.go` — struct definitions (AnthropicRequest, AnthropicResponse, etc.)
- [ ] `internal/translator/request.go` — AnthropicToOpenAI() function
- [ ] `internal/translator/response.go` — OpenAIToAnthropic() function
- [ ] `internal/translator/stream.go` — StreamTranslator struct and Translate() method

*Existing `internal/server/relay_test.go` covers PRXY-01 regression — no new test file needed for that.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Claude Code native Anthropic mode end-to-end | PRXY-02 | Requires live Claude Code pointed at running server | Point Claude Code at `http://localhost:8080` with `ANTHROPIC_BASE_URL`, send a message, confirm response |
| Streaming token-by-token delivery (no buffering) | PRXY-02 | Visual verification of stream timing | Watch SSE events arrive in real-time via `curl -N` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
