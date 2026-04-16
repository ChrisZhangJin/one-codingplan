---
phase: 04-anthropic-format-translation
plan: 01
subsystem: translator
tags: [translator, anthropic, openai, format-translation, tdd]
dependency_graph:
  requires: []
  provides: [internal/translator/types.go, internal/translator/request.go]
  affects: [internal/config/config.go, internal/pool/pool.go]
tech_stack:
  added: []
  patterns: [pure-function-translator, json-rawmessage-passthrough, tdd-red-green]
key_files:
  created:
    - internal/translator/types.go
    - internal/translator/request.go
    - internal/translator/translator_test.go
  modified:
    - internal/config/config.go
    - internal/pool/pool.go
decisions:
  - D-01 (model override) implemented via modelOverride parameter to AnthropicToOpenAI
  - D-02 (model strip) implemented via omitempty on OpenAIRequest.Model
  - D-07 (tool_call_id round-trip) implemented in translateContentBlocks for tool_result blocks
  - tool_use input passed as json.RawMessage to avoid double-encoding
  - ModelOverride not stored in DB — applied post-construction via Pool.SetModelOverride
metrics:
  duration: "138s"
  completed_date: "2026-04-16"
  tasks_completed: 1
  tasks_total: 1
  files_created: 3
  files_modified: 2
---

# Phase 04 Plan 01: Anthropic-to-OpenAI Request Translator Summary

**One-liner:** Pure-function `AnthropicToOpenAI` translator with full tool use, system prompt, model override (D-01/D-02/D-07), and `ModelOverride` field wired through config and pool.

## What Was Built

The `internal/translator` package now provides:

- `types.go` — complete Anthropic and OpenAI struct definitions (`AnthropicRequest`, `AnthropicMessage`, `AnthropicContentBlock`, `AnthropicTool`, `AnthropicResponse`, `AnthropicUsage`, `OpenAIRequest`, `OpenAIMessage`, `OpenAIToolCall`, `OpenAIFunctionCall`, `OpenAITool`, `OpenAIFunction`, `OpenAIResponse`, `OpenAIChoice`, `OpenAIUsage`)
- `request.go` — `AnthropicToOpenAI(req *AnthropicRequest, modelOverride string) (*OpenAIRequest, error)` with helpers for content block translation, tool result extraction, and tool definition mapping
- `translator_test.go` — 9 unit tests covering all translation paths

Config and pool extended:
- `config.UpstreamConfig.ModelOverride string` (mapstructure `model_override`)
- `pool.UpstreamEntry.ModelOverride string`
- `pool.Pool.SetModelOverride(name, override string)` method

## TDD Execution

| Phase | Commit | Outcome |
|-------|--------|---------|
| RED | b16b88d | 9 tests fail (undefined types/functions) |
| GREEN | e8da2be | All 9 tests pass |
| REFACTOR | n/a | No cleanup needed |

## Tasks

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 (RED) | Failing tests for AnthropicToOpenAI | b16b88d | internal/translator/translator_test.go |
| 1 (GREEN) | Implementation + config/pool extension | e8da2be | internal/translator/types.go, request.go, internal/config/config.go, internal/pool/pool.go |

## Verification

```
go test ./internal/translator/... -run TestAnthropicToOpenAI -v  → 9/9 PASS
go test ./internal/pool/... -race                                 → PASS
go test ./...                                                     → all packages PASS
go vet ./internal/translator/... ./internal/config/... ./internal/pool/... → clean
```

## Acceptance Criteria Check

- [x] `internal/translator/types.go` contains `type AnthropicRequest struct`
- [x] `internal/translator/types.go` contains `type OpenAIRequest struct`
- [x] `internal/translator/types.go` contains `type AnthropicContentBlock struct`
- [x] `internal/translator/types.go` contains `type OpenAIToolCall struct`
- [x] `internal/translator/request.go` contains `func AnthropicToOpenAI(req *AnthropicRequest, modelOverride string) (*OpenAIRequest, error)`
- [x] `internal/translator/translator_test.go` contains `TestAnthropicToOpenAI_SimpleText`
- [x] `internal/translator/translator_test.go` contains `TestAnthropicToOpenAI_ToolResult`
- [x] `internal/translator/translator_test.go` contains `TestAnthropicToOpenAI_AssistantToolUse`
- [x] `internal/config/config.go` contains `ModelOverride string`
- [x] `internal/pool/pool.go` contains `ModelOverride string`
- [x] `internal/pool/pool.go` contains `func (p *Pool) SetModelOverride`
- [x] `go test ./internal/translator/... -v` exits 0
- [x] `go test ./internal/pool/... -race` exits 0
- [x] `go test ./internal/config/... -v` exits 0

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None. All translation logic is fully wired. `ModelOverride` in the pool is populated by `SetModelOverride` — the caller (main.go or future handler) must invoke it after pool construction for each upstream from config. This wiring point is documented in the pool method but not yet called — it will be connected in the handler plan (04-02 or equivalent).

## Threat Flags

No new threat surface beyond what was modeled in the plan's `<threat_model>`. `AnthropicContentBlock.Input` is passed as `json.RawMessage` (no eval). Tool result content is string-extracted only. No network endpoints added in this plan.

## Self-Check: PASSED

Files created:
- FOUND: internal/translator/types.go
- FOUND: internal/translator/request.go
- FOUND: internal/translator/translator_test.go

Files modified:
- FOUND: internal/config/config.go (ModelOverride field present)
- FOUND: internal/pool/pool.go (ModelOverride field + SetModelOverride method present)

Commits:
- FOUND: b16b88d (test RED)
- FOUND: e8da2be (feat GREEN)
