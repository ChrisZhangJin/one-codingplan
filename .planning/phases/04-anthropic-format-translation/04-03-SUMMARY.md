---
phase: 04-anthropic-format-translation
plan: "03"
subsystem: server
tags: [anthropic, handler, relay, sse, integration-tests]
dependency_graph:
  requires: [04-01, 04-02]
  provides: [anthropic-http-handler, route-registration, model-override-wiring]
  affects: [internal/server, cmd/ocp]
tech_stack:
  added: []
  patterns: [failover-loop-mirror, sync-mutex-sse, stream-translator-integration]
key_files:
  created:
    - internal/server/anthropic.go
    - internal/server/anthropic_test.go
  modified:
    - internal/server/server.go
    - cmd/ocp/main.go
decisions:
  - "Anthropic error format used for all error responses from /v1/messages (type+error object) matching real Anthropic API behavior"
  - "Translation errors on the path (json.Unmarshal of upstream response) return 502 not 500 — upstream is the source of badness"
  - "proxyAnthropicBuffer uses c.JSON (not c.Data) so AnthropicResponse is marshaled directly — avoids double-marshal of raw bytes"
metrics:
  duration_min: 15
  completed_date: "2026-04-16"
  tasks_completed: 2
  tasks_total: 2
  files_changed: 4
---

# Phase 04 Plan 03: Anthropic Handler and Route Registration Summary

**One-liner:** POST /v1/messages handler wiring translator package into Gin server with failover loop, streaming SSE translation, and 11 integration tests.

## What Was Built

Created `internal/server/anthropic.go` implementing three functions that glue the translator package (Plans 01-02) into the existing server:

- **`handleAnthropicRelay`**: mirrors `handleRelay` failover loop — reads body with 10MB limit, parses `AnthropicRequest`, rotates through pool upstreams, translates to OpenAI format via `translator.AnthropicToOpenAI`, forwards to upstream `/v1/chat/completions`, branches on `req.Stream`
- **`proxyAnthropicBuffer`**: reads OpenAI response, calls `translator.OpenAIToAnthropic`, writes Anthropic JSON with usage tokens extracted from translated response
- **`proxyAnthropicStream`**: SSE proxy using `sync.Mutex` + heartbeat goroutine pattern from `proxyStream`, pipes upstream bytes through `translator.NewStreamTranslator` which emits Anthropic SSE event sequence

Registered `v1.POST("/messages", s.handleAnthropicRelay)` in `server.Engine()` — inherits `authMiddleware` from the `v1` group.

Wired `ModelOverride` from `cfg.Upstreams` to `p.SetModelOverride` in `cmd/ocp/main.go` after pool construction.

## Test Coverage

11 integration tests in `internal/server/anthropic_test.go`:

| Test | What it verifies |
|------|-----------------|
| TestAnthropicRelay_NonStream | 200 response with valid Anthropic JSON (type, role, content, stop_reason, model echo) |
| TestAnthropicRelay_Stream | SSE response contains message_start, content_block_delta, message_stop |
| TestAnthropicRelay_Auth | 401 for missing Authorization header |
| TestAnthropicRelay_Failover | First upstream 500 → rotates to second → client gets 200 |
| TestAnthropicRelay_AllFail | Single 500 upstream → 503 with Anthropic overloaded_error format |
| TestAnthropicRelay_Usage | UsageRecord written with correct keyID, upstreamID, token counts |
| TestAnthropicRelay_ModelOverride | D-01: forwarded body contains override model; D-03: response echoes original |
| TestAnthropicRelay_ModelStrip | D-02: forwarded body has no claude- model when no override set |
| TestAnthropicRelay_InvalidBody | 400 with invalid_request_error format for malformed JSON |
| TestAnthropicRelay_ToolRoundTrip | D-06/D-07: tool_use→tool_calls and tool_result→role:tool translation verified; tool_use block in response with correct ID |
| TestRelay_OpenAI_Regression | /v1/chat/completions still returns 200 (regression green) |

All pass with `-race` flag.

## Deviations from Plan

None — plan executed exactly as written.

## Threat Mitigations Applied

| Threat ID | Status |
|-----------|--------|
| T-4-07 | Applied — `json.Unmarshal` into typed `AnthropicRequest`; malformed JSON returns 400 |
| T-4-08 | Applied — `io.LimitReader(body, 10MB+1)` with size check |
| T-4-09 | Applied — route registered under `v1` group with `authMiddleware`; 401 tested |
| T-4-10 | Applied — `cloneHeaders` strips hop-by-hop; `Authorization` replaced not forwarded |
| T-4-11 | Applied — `sync.Mutex` guards all `c.Writer` writes in `proxyAnthropicStream` |

## Known Stubs

None.

## Threat Flags

None — no new network endpoints or auth paths beyond what was planned.

## Self-Check: PASSED

- `internal/server/anthropic.go` exists and contains `handleAnthropicRelay`, `proxyAnthropicBuffer`, `proxyAnthropicStream`
- `internal/server/server.go` contains `v1.POST("/messages"`
- `internal/server/anthropic_test.go` contains all 11 test functions
- `cmd/ocp/main.go` contains `SetModelOverride`
- `go build ./cmd/ocp` exits 0
- `go test ./... -race` exits 0

## Commits

| Hash | Message |
|------|---------|
| 3a6fa5e | feat(04-03): handleAnthropicRelay handler with failover, non-stream, stream paths |
| 81b0728 | test(04-03): integration tests for /v1/messages endpoint |
