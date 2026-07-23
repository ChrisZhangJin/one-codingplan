# Feasibility: Codex (Responses API) → Anthropic Upstream

Status: **deferred** — assessed 2026-05-16, not yet scheduled.

## Why this came up

Today OCP's `/v1/responses` handler only translates Responses → OpenAI chat
completions, then POSTs `upstream/v1/chat/completions`. A Claude-only proxy
backend (only speaks `/v1/messages`) returns 500 on that path. There is no
config workaround — OCP needs a new translation chain.

## Verdict

Feasible. No architectural blockers. The translator package is already
organized for this shape of work — every piece has a near-mirror analog.

## What needs to be built

Three pieces, all anchored in existing code in `internal/translator/`:

| Piece | Existing analog | New work |
|---|---|---|
| Request: Responses → Anthropic `/v1/messages` | `ResponsesRequestToOpenAI` (`responses_request.go`) parses input items, tools, instructions | Re-emit as Anthropic `messages` + `system` + `tools`. Map `function_call` → `tool_use` content block; `function_call_output` → `tool_result` content block. |
| Response (buffered): Anthropic → Responses | `OpenAIToResponsesAPI` (`responses_response.go`) | Same shape, but read from `AnthropicResponse.Content` blocks instead of `OpenAIResponse.Choices`. ~40 lines. |
| SSE stream: Anthropic → Responses | `ResponsesStreamTranslator` (`responses_stream.go`) does OpenAI SSE → Responses SSE | New `AnthropicResponsesStreamTranslator`. Event mapper plus `output_index`/`content_index` bookkeeping. The OpenAI translator already handles tool-call argument deltas, so the pattern is proven. ~300 lines. |

## Mapping confidence

| Concern | Verdict |
|---|---|
| Role-based messages, multi-turn input | Direct. `input` items → `messages[]`. |
| `instructions` field | Direct → Anthropic `system`. |
| Function tools | Direct — `function` tool def → Anthropic tool (`name`, `description`, `input_schema`). |
| Tool call round-trip (assistant call + tool output) | Works. Reuses the same `call_id ↔ tool_use_id` pairing the existing OpenAI path handles (`responses_request.go:103`, `:156`). |
| Stop reasons | `end_turn`→`completed`, `max_tokens`/`stop_sequence`→`incomplete`, `tool_use`→`completed` with function_call items. |
| Usage accounting | Anthropic gives `input_tokens` on `message_start`, `output_tokens` on `message_delta`. Both feed `ResponsesUsage`. |
| Image input | Supported on Anthropic side; Codex rarely sends multimodal, can defer. |

## Friction points worth flagging before building

1. **`max_tokens` is mandatory on Anthropic.** Responses requests from Codex
   usually omit it. Server-side default required (config knob, e.g. `8192`).
2. **Model name.** Codex sends `model="default"`. Anthropic backends reject
   that. `model_override` on the upstream is required — e.g.
   `claude-3-5-sonnet-latest`.
3. **SSE state machine.** Anthropic's `content_block_index` is per-message;
   Responses' `output_index`/`content_index`/`sequence_number` is per-stream.
   Translator must keep `anthropic_block_index → (output_index, content_index,
   opened_kind)`. Mechanical but the place real bugs hide. Use the existing
   test pattern in `responses_stream_test.go`.
4. **Heartbeats/connection lifetime.** Reuse `proxyResponsesStream` as-is.
5. **Interleaved blocks.** Anthropic sometimes emits text + tool_use blocks in
   the same message — emit one Responses `output_item` per content block, not
   per message.

## Wiring

- `pool.Select` already supports protocol filtering. Have `/v1/responses`
  accept anthropic-protocol upstreams (today it only asks for `ProtocolOpenAI`
  at `responses.go:77`).
- Branch on `up.Protocol` after selection to choose which translation chain to
  run, and which adapter URL (`OpenAIURL` vs `AnthropicURL`).
- Header injection: switch from `Authorization: Bearer …` only to also setting
  `x-api-key` (same as `anthropic.go:126-127`).

## Effort estimate

- Non-streaming path (request translator + response translator + handler
  branch + tests): ~half day, low risk.
- Streaming path (the SSE translator + tests): ~1 day, medium risk, mostly
  because SSE state bugs are annoying to reproduce.
- Total: ~1.5 days.

## When to pick this up

Once there is a real need to run Codex against the Anthropic-only proxy
(currently `my-claude` at `http://10.0.3.248:3000/api`). Until then, point
Codex at an OpenAI-shaped upstream.
