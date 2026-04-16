# Phase 4: Anthropic Format Translation - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Add `/v1/messages` endpoint that accepts native Anthropic-format requests, translates them to OpenAI format for upstream providers, receives OpenAI-format responses, and translates back to Anthropic format (including SSE event sequence) before returning to the client.

No management API (Phase 5). No web portal (Phase 6). No CLI (Phase 7).
Auth, failover, usage logging, and the existing `/v1/chat/completions` endpoint are unchanged.

</domain>

<decisions>
## Implementation Decisions

### Model Name Handling
- **D-01:** Add an optional `model_override` field per upstream entry in `config.yaml`. Before forwarding the request to an upstream, replace the `model` field in the OpenAI-format body with the upstream's `model_override` value.
- **D-02:** If no `model_override` is configured for the selected upstream, strip the `model` field entirely before forwarding (do not pass `claude-*` names to upstream — they will reject with 400).
- **D-03:** In the Anthropic-format response (both streaming and non-streaming), echo back the model name from the original Anthropic request body (not the upstream's actual model name). Client sees exactly what it asked for.

### Streaming Event Format
- **D-04:** Full faithful Anthropic SSE event sequence — parse each OpenAI SSE chunk and re-emit the complete Anthropic event protocol:
  - First chunk → emit `message_start` (with echoed model + input token estimate) then `content_block_start` (index 0, type "text")
  - Each text delta → emit `content_block_delta` (type "text_delta")
  - On `[DONE]` or `finish_reason` set → emit `content_block_stop`, `message_delta` (with stop_reason), `message_stop`
- **D-05:** Streaming tool use: tool call JSON streamed from upstream arrives as regular text delta chunks. The SSE translation layer emits them as `content_block_delta` events. No special `input_json_delta` parsing required — the existing chunk-level translation covers it.

### Tool Use Translation
- **D-06:** Full bidirectional mapping, covering:
  - **Request (Anthropic → OpenAI):** `tools` array with `input_schema` → OpenAI `tools` array with `function.parameters`; `tool_result` blocks in `messages` → `role: "tool"` messages with `tool_call_id`
  - **Response (OpenAI → Anthropic):** `choices[0].message.tool_calls` → `content` array of `tool_use` blocks; `finish_reason: "tool_calls"` → `stop_reason: "tool_use"`
- **D-07:** `tool_call_id` round-tripping: preserve the upstream's `tool_call_id` inside the `tool_use` block's `id` field. When the client sends back a `tool_result`, extract `tool_use_id` and use it as `tool_call_id` in the translated OpenAI message.

### Translator Code Location
- **D-08:** New package `internal/translator/` with pure functions:
  - `AnthropicToOpenAI(req *AnthropicRequest) (*OpenAIRequest, error)` — request translation
  - `OpenAIToAnthropic(resp *OpenAIResponse, originalModel string) (*AnthropicResponse, error)` — non-streaming response translation
  - `NewStreamTranslator(originalModel string) *StreamTranslator` — stateful SSE translator that accepts OpenAI chunks and emits Anthropic events
- **D-09:** The `/v1/messages` handler in `internal/server/relay.go` (or a new `internal/server/anthropic.go`) calls into `internal/translator/`. The handler reuses the existing failover loop from `handleRelay` — no duplication of failover/auth/logging logic.

### Claude's Discretion
- Exact struct names for `AnthropicRequest`, `AnthropicResponse`, `OpenAIRequest`, `OpenAIResponse` in `internal/translator/`
- Whether `/v1/messages` handler is a new function in relay.go or a separate file `internal/server/anthropic.go`
- How `StreamTranslator` buffers partial chunks (e.g., incomplete SSE frames split across read calls)
- Whether to emit a `ping` event after `message_start` (Anthropic spec includes it; Claude Code tolerates its absence)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Requirements
- `.planning/REQUIREMENTS.md` — PRXY-02 (Anthropic-format requests), PRXY-03 (SSE streaming both formats), PRXY-04 (bidirectional format translation)
- `.planning/ROADMAP.md` §Phase 4 — Goal, success criteria, dependency on Phase 3

### Project Context
- `.planning/PROJECT.md` — Core value, constraints, extensibility principle
- `CLAUDE.md` (project root) — Tech stack rationale, streaming approach, SSE heartbeat pattern

### Prior Phase Output (read before implementing)
- `.planning/phases/03-relay-pipeline-openai-pass-through/03-CONTEXT.md` — All Phase 3 decisions (auth middleware, failover retry logic, streaming/buffer proxy, usage logging)
- `internal/server/relay.go` — `handleRelay`, `proxyStream`, `proxyBuffer`, `authMiddleware`, `logUsage` — reuse/extend these
- `internal/server/server.go` — Server struct; new `/v1/messages` route registers here
- `internal/pool/pool.go` — `Select`, `Mark`, `Backoff`, `ErrNoUpstreams`
- `internal/pool/classifier.go` — `Classify` — same error classification used for Anthropic requests
- `internal/models/models.go` — `AccessKey`, `UsageRecord` — unchanged
- `internal/config/config.go` — `Config` struct — needs `model_override` field added to upstream entry

### No external specs — requirements fully captured in decisions above

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `handleRelay` failover loop — copy or extract to share with `/v1/messages` handler; the retry-over-upstreams logic is identical
- `proxyStream` / `proxyBuffer` — used for OpenAI pass-through; Phase 4 needs new streaming function that re-formats SSE events instead of passing through raw
- `authMiddleware` — reused unchanged on `/v1/messages` route
- `logUsage` — reused unchanged
- `relayClient` — shared HTTP client, reused

### Established Patterns
- Gin handlers on `Server` struct
- GORM queries via `s.db`
- Pool injected as `s.pool`
- Async usage logging via goroutine
- Streaming: `http.Flusher` + `sync.Mutex` guard + heartbeat goroutine (see `proxyStream`)

### Integration Points
- New route registered in `server.go` (or `Server.Engine()`): `POST /v1/messages`
- `config.go` UpstreamConfig struct needs `ModelOverride string` field
- New package `internal/translator/` — no existing code, pure greenfield

</code_context>

<specifics>
## Specific Ideas

- For streaming, the `StreamTranslator` is stateful (needs to know whether `message_start` has been emitted yet, current block index, etc.) — design it as a struct with a `Translate(chunk []byte) ([]byte, error)` method rather than a stateless function.
- The heartbeat goroutine pattern from `proxyStream` should be replicated in the new Anthropic streaming handler.
- Token counts in `message_start.message.usage.input_tokens`: can be populated from the non-streaming response or set to 0 for streaming (same as Phase 3 behavior for usage logging).

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 04-anthropic-format-translation*
*Context gathered: 2026-04-16*
