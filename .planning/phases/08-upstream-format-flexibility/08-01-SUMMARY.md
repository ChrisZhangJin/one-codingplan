---
phase: 08-upstream-format-flexibility
plan: "01"
subsystem: upstream-routing
tags: [upstream, format, passthrough, anthropic, config]
dependency_graph:
  requires: []
  provides: [upstream-format-field, anthropic-passthrough-relay]
  affects: [internal/server/anthropic.go, internal/pool/pool.go, internal/config/config.go, cmd/ocp/main.go]
tech_stack:
  added: []
  patterns: [format-branch-relay, verbatim-passthrough]
key_files:
  created: []
  modified:
    - internal/config/config.go
    - internal/pool/pool.go
    - cmd/ocp/main.go
    - internal/server/anthropic.go
    - internal/server/anthropic_test.go
    - internal/pool/pool_test.go
decisions:
  - "Reuse proxyStream/proxyBuffer from relay.go for passthrough response path rather than adding new functions — they do no translation and already handle SSE + heartbeat correctly"
  - "config.yaml is gitignored (contains real API keys) so the format: anthropic update is on-disk only, not committed — this is expected behavior"
metrics:
  duration: "~8 minutes"
  completed: "2026-04-17"
  tasks_completed: 2
  tasks_total: 2
  files_changed: 6
  commits: 2
---

# Phase 08 Plan 01: Upstream Format Flexibility Summary

**One-liner:** Per-upstream `format` field enabling verbatim Anthropic passthrough to native Anthropic upstreams (mimo) without OpenAI translation.

## What Was Built

Added a `format` config field that controls whether the Anthropic relay handler translates requests to OpenAI format (default, empty) or forwards the original request body verbatim to an Anthropic-native upstream (format: anthropic).

The mimo upstream at `token-plan-cn.xiaomimimo.com/anthropic` speaks Anthropic format natively. Before this change, sending it a translated OpenAI body caused errors. Now `format: anthropic` in config routes it to the verbatim passthrough branch.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Add Format field to UpstreamConfig and UpstreamEntry, wire via SetFormat | 16193f1 | internal/config/config.go, internal/pool/pool.go, cmd/ocp/main.go, internal/pool/pool_test.go |
| 2 | Passthrough branch in handleAnthropicRelay + tests + config.yaml update | 189a618 | internal/server/anthropic.go, internal/server/anthropic_test.go |

## Decisions Made

1. **Reuse proxyStream/proxyBuffer for passthrough responses** — These functions in relay.go do no translation; they copy bytes verbatim with SSE heartbeat support. Adding new functions would duplicate ~60 lines for identical behavior.

2. **config.yaml gitignored** — The file contains real API keys and is excluded from git. The `format: anthropic` update for mimo is applied on disk. This is the existing project pattern; the change is tested via the code path and confirmed working.

## Verification Results

- `go build ./...` — passes
- `go test ./internal/pool/... -run TestSetFormat` — passes
- `go test ./internal/server/... -run TestAnthropic` — 13 tests pass (10 existing + 3 new)
- `go test ./... -race` — no data races, all packages pass

## New Tests Added

- `TestSetFormat` (pool_test.go) — verifies Format field set/get and no-op on unknown name
- `TestAnthropicPassthrough_NonStream` — verifies anthropic-format upstream receives verbatim body at /v1/messages
- `TestAnthropicTranslate_NonStream` — verifies openai-format upstream receives translated body at /v1/chat/completions
- `TestAnthropicPassthrough_UpstreamError` — verifies 500 on passthrough upstream causes failover to next upstream

## Deviations from Plan

None — plan executed exactly as written. The config.yaml gitignore situation is a pre-existing project constraint, not a deviation.

## Known Stubs

None — all data flows are wired end-to-end.

## Threat Flags

No new threat surface introduced. Body size limit (T-8-01, 10MB) was already in place before the format branch, covering both passthrough and translate paths. API key handling (T-8-02) follows the identical pattern as the existing translate path.

## Self-Check

Files created/modified:
- internal/config/config.go — FOUND
- internal/pool/pool.go — FOUND
- cmd/ocp/main.go — FOUND
- internal/server/anthropic.go — FOUND
- internal/server/anthropic_test.go — FOUND
- internal/pool/pool_test.go — FOUND

Commits:
- 16193f1 — FOUND
- 189a618 — FOUND

## Self-Check: PASSED
