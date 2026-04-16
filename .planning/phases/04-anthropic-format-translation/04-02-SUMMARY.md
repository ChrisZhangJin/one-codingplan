---
phase: 04-anthropic-format-translation
plan: 02
subsystem: translator
tags: [translation, streaming, SSE, tool-use, anthropic, openai]
dependency_graph:
  requires: [internal/translator/types.go, internal/translator/request.go]
  provides: [internal/translator/response.go, internal/translator/stream.go]
  affects: [internal/server/anthropic.go (plan 04-03)]
tech_stack:
  added: []
  patterns: [TDD red-green, stateful SSE frame buffering, inline serialization structs]
key_files:
  created:
    - internal/translator/response.go
    - internal/translator/stream.go
  modified:
    - internal/translator/translator_test.go
decisions:
  - "translateFinishReason maps content_filter to end_turn (not stop_sequence) — closer semantic match per RESEARCH.md note"
  - "StreamTranslator skips unparseable JSON frames silently (nil, nil) per T-4-06 threat mitigation"
  - "emitClosing factored as a shared helper used by both [DONE] and finish_reason paths"
metrics:
  duration: ~15min
  completed: 2026-04-16T14:05:25Z
  tasks_completed: 2
  files_created: 2
  files_modified: 1
---

# Phase 04 Plan 02: Response Translator and StreamTranslator Summary

**One-liner:** OpenAI→Anthropic non-streaming response translator and stateful SSE StreamTranslator with partial frame buffering, tool_use block mapping, and model echo.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Non-streaming response translator (OpenAIToAnthropic) | 21c6e51 | response.go, translator_test.go |
| 2 | StreamTranslator stateful SSE re-framer | 3640d26 | stream.go, translator_test.go |

## What Was Built

### Task 1 — `internal/translator/response.go`

`OpenAIToAnthropic(resp *OpenAIResponse, originalModel string) (*AnthropicResponse, error)`:

- Returns error if `choices` is empty.
- Sets `type="message"`, `role="assistant"`, `model=originalModel` (D-03).
- Converts `choices[0].message.content` to a `type:"text"` content block.
- Converts each `choices[0].message.tool_calls` entry to a `type:"tool_use"` block preserving `id` (D-07).
- Text block is prepended before tool_use blocks when both are present.
- `translateFinishReason` maps: `stop`→`end_turn`, `tool_calls`→`tool_use`, `length`→`max_tokens`, default→`end_turn`.
- Maps `usage.prompt_tokens` / `completion_tokens` to `usage.input_tokens` / `output_tokens`.

### Task 2 — `internal/translator/stream.go`

`StreamTranslator` struct with `NewStreamTranslator(originalModel string) *StreamTranslator` and `Translate(chunk []byte) ([][]byte, error)`:

- Appends each `chunk` to `st.buf`, scans for `\n\n`-delimited complete SSE frames.
- For `[DONE]` frames: emits `content_block_stop` + `message_delta` (with `stop_reason`) + `message_stop`.
- For first non-empty delta: emits `message_start` (with `model=st.model`, D-03) + `content_block_start{index:0,type:"text"}` + `content_block_delta`.
- For subsequent non-empty deltas: emits only `content_block_delta`.
- For frames with `finish_reason` present: emits delta (if content non-empty) then the 3 closing events.
- Skips frames with empty delta and no finish_reason (0 events emitted).
- Malformed JSON frames skipped silently per T-4-06 mitigation.
- `formatSSEEvent(eventType, payload)` produces `event: {type}\ndata: {json}\n\n`.

## Test Results

All 22 tests pass:
- 9 pre-existing `AnthropicToOpenAI` tests (from plan 04-01) — no regressions
- 6 `OpenAIToAnthropic` tests: Text, ToolUse, TextAndToolUse, ModelEcho, FinishReasons, NoChoices
- 7 `StreamTranslator` tests: Start, Subsequent, Done, FinishReason, Partial, EmptyDelta, ModelEcho

`go vet ./internal/translator/...` — clean.

## Success Criteria Met

- [x] OpenAIToAnthropic correctly translates text and tool_use responses
- [x] D-03 (model echo) implemented and tested for both non-streaming and streaming
- [x] D-06 (bidirectional tool mapping) response direction implemented and tested
- [x] D-07 (tool_call_id preservation) response direction implemented and tested
- [x] StreamTranslator produces correct Anthropic SSE event sequence per D-04
- [x] Partial frame buffering handles TCP segmentation (TestStreamTranslator_Partial)

## Deviations from Plan

### Auto-fixed Issues

None — plan executed exactly as written.

**One minor implementation choice:** `translateFinishReason` maps `content_filter` to `end_turn` (not `stop_sequence` as suggested in RESEARCH.md code comments). The PLAN.md acceptance criteria specifies `"content_filter"→"end_turn" (fallback)` explicitly, which was followed. This is not a bug — the plan took precedence over the research comment.

## Known Stubs

None — all functions are fully implemented with no placeholder values or TODO markers.

## Threat Flags

None — no new network endpoints or trust boundaries introduced. All threat mitigations from the plan's threat model were applied:
- T-4-04: `json.RawMessage` used for `tool_calls.arguments` (no eval)
- T-4-06: Malformed SSE frames skipped silently via nil return in `translateFrame`

## Self-Check: PASSED

- [x] `internal/translator/response.go` exists
- [x] `internal/translator/stream.go` exists
- [x] Commit `21c6e51` exists (Task 1)
- [x] Commit `3640d26` exists (Task 2)
- [x] All 22 tests pass
