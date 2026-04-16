# Phase 4: Anthropic Format Translation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-16
**Phase:** 04-anthropic-format-translation
**Areas discussed:** Model name handling, Streaming event format, Tool use translation depth, Translator code location

---

## Model Name Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Per-upstream model map in config | config.yaml gets optional model_override per upstream; replaces model field before forwarding | ✓ |
| Always strip / use upstream default | Remove model field entirely before forwarding | |
| Pass through as-is | Forward claude-* names unchanged; upstreams will likely reject with 400 | |

**Fallback behavior:**

| Option | Description | Selected |
|--------|-------------|----------|
| Optional — strip model field if no override set | If no model_override configured, remove model field entirely | ✓ |
| Optional — pass through if no override set | Forward claude-* name as-is if no override | |
| Required — startup error if not configured | Refuse to start without model_override on all enabled upstreams | |

**User's choice:** Per-upstream model_override in config.yaml; strip model field if no override is set. Echo back the original Anthropic request model name in responses.

---

## Streaming Event Format

| Option | Description | Selected |
|--------|-------------|----------|
| Full faithful translation | Parse OpenAI SSE chunks, re-emit complete Anthropic event sequence (message_start, content_block_start, content_block_delta, content_block_stop, message_delta, message_stop) | ✓ |
| Minimal / Claude Code only | Emit only events Claude Code requires; skip optional events | |

**Model in message_start:**

| Option | Description | Selected |
|--------|-------------|----------|
| Echo back the model from the original Anthropic request | Response model = what client sent | ✓ |
| Use the upstream's actual model name | Transparent about what ran | |
| Omit / hardcode a neutral value | Fixed string or upstream value | |

**User's choice:** Full faithful Anthropic SSE event sequence; echo original request model in message_start.

---

## Tool Use Translation Depth

| Option | Description | Selected |
|--------|-------------|----------|
| Full bidirectional mapping | Translate Anthropic tools ↔ OpenAI tools on request; tool_use blocks ↔ tool_calls on response; tool_result ↔ role:tool in multi-turn | ✓ |
| Minimal — enough to avoid 400s | Basic format translation without full multi-turn correctness | |
| Defer tool use to a later phase | Return error if request contains tools | |

**Streaming tool use:** User clarified that ocp's streaming layer handles tool call JSON as regular text delta chunks — no special `input_json_delta` parsing needed.

**User's choice:** Full bidirectional mapping; streaming tool use handled by existing chunk-level SSE translation.

---

## Translator Code Location

| Option | Description | Selected |
|--------|-------------|----------|
| New internal/translator/ package | Pure functions, independently unit-testable | ✓ |
| Extend internal/server/ inline | Translation functions in relay.go or server/anthropic.go | |

**User's choice:** New `internal/translator/` package with pure translation functions.

---

## Claude's Discretion

- Exact struct names in `internal/translator/`
- Whether `/v1/messages` handler goes in relay.go or a new anthropic.go file
- How `StreamTranslator` buffers partial SSE frames
- Whether to emit `ping` event (Anthropic spec includes it; Claude Code tolerates absence)
