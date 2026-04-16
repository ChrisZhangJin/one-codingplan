# Phase 4: Anthropic Format Translation - Research

**Researched:** 2026-04-16
**Domain:** Anthropic Messages API ↔ OpenAI Chat Completions translation in Go
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Model Name Handling**
- D-01: Add optional `model_override` field per upstream entry in `config.yaml`. Replace `model` field in OpenAI-format body with the upstream's `model_override` value before forwarding.
- D-02: If no `model_override` is configured, strip the `model` field entirely before forwarding (do not pass `claude-*` names upstream).
- D-03: Echo back the model name from the original Anthropic request body in the response (not the upstream's actual model name).

**Streaming Event Format**
- D-04: Full faithful Anthropic SSE event sequence — parse each OpenAI SSE chunk and re-emit the complete Anthropic event protocol:
  - First chunk → emit `message_start` (with echoed model + input token estimate) then `content_block_start` (index 0, type "text")
  - Each text delta → emit `content_block_delta` (type "text_delta")
  - On `[DONE]` or `finish_reason` set → emit `content_block_stop`, `message_delta` (with stop_reason), `message_stop`
- D-05: Streaming tool use — tool call JSON streamed from upstream arrives as regular text delta chunks. Emit as `content_block_delta` events. No special `input_json_delta` parsing required.

**Tool Use Translation**
- D-06: Full bidirectional mapping:
  - Request (Anthropic → OpenAI): `tools` array with `input_schema` → OpenAI `tools` array with `function.parameters`; `tool_result` blocks in `messages` → `role: "tool"` messages with `tool_call_id`
  - Response (OpenAI → Anthropic): `choices[0].message.tool_calls` → `content` array of `tool_use` blocks; `finish_reason: "tool_calls"` → `stop_reason: "tool_use"`
- D-07: `tool_call_id` round-tripping: preserve upstream's `tool_call_id` inside the `tool_use` block's `id` field. When client sends back `tool_result`, extract `tool_use_id` and use as `tool_call_id` in the translated OpenAI message.

**Translator Code Location**
- D-08: New package `internal/translator/` with pure functions:
  - `AnthropicToOpenAI(req *AnthropicRequest) (*OpenAIRequest, error)` — request translation
  - `OpenAIToAnthropic(resp *OpenAIResponse, originalModel string) (*AnthropicResponse, error)` — non-streaming response translation
  - `NewStreamTranslator(originalModel string) *StreamTranslator` — stateful SSE translator
- D-09: The `/v1/messages` handler calls into `internal/translator/`. The handler reuses the existing failover loop from `handleRelay` — no duplication of failover/auth/logging logic.

### Claude's Discretion
- Exact struct names for `AnthropicRequest`, `AnthropicResponse`, `OpenAIRequest`, `OpenAIResponse` in `internal/translator/`
- Whether `/v1/messages` handler is a new function in relay.go or a separate file `internal/server/anthropic.go`
- How `StreamTranslator` buffers partial chunks (e.g., incomplete SSE frames split across read calls)
- Whether to emit a `ping` event after `message_start` (Anthropic spec includes it; Claude Code tolerates its absence)

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PRXY-02 | Client can send Anthropic-format requests to `/v1/messages` and receive valid Anthropic-format responses | New route + `AnthropicToOpenAI` + `OpenAIToAnthropic` functions |
| PRXY-04 | Proxy correctly translates Anthropic request format to OpenAI upstream format and translates response back | Full bidirectional translator package with tool use, system prompt, and SSE event mapping |
</phase_requirements>

---

## Summary

Phase 4 adds a `/v1/messages` endpoint that accepts native Anthropic Messages API requests, translates them to OpenAI Chat Completions format for upstream providers, and translates responses back to Anthropic format — including the complete SSE event sequence for streaming.

The translation layer lives entirely in a new `internal/translator/` package of pure functions. The handler in `internal/server/` reuses the existing failover loop, auth middleware, and usage logging from Phase 3 — no duplication. The only new logic is the format conversion and a stateful `StreamTranslator` that re-frames OpenAI SSE chunks as Anthropic SSE events.

The two structural mismatches that require most careful handling are: (1) system prompts — Anthropic puts them in a top-level `system` field; OpenAI puts them as a `role: "system"` message; and (2) tool call round-tripping — `tool_call_id` must be preserved through the `tool_use.id` field so that subsequent `tool_result` messages translate correctly.

**Primary recommendation:** Implement `internal/translator/` as a standalone package with pure, table-tested functions first; then wire it into a new `internal/server/anthropic.go` handler that mirrors the structure of `handleRelay`.

---

## Standard Stack

### Core (already in go.mod — no new dependencies required)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `encoding/json` | stdlib | JSON marshal/unmarshal for translation structs | [VERIFIED: codebase] Already used throughout |
| `bufio` | stdlib | Line-by-line SSE frame parsing in `StreamTranslator` | [VERIFIED: codebase] Standard Go SSE parsing pattern |
| `bytes` | stdlib | Byte manipulation for SSE frame assembly | [VERIFIED: codebase] Already used in relay.go |
| `strings` | stdlib | String prefix/suffix operations on SSE lines | [VERIFIED: codebase] Already used in pool/classifier.go |
| `fmt` | stdlib | SSE line formatting (`event: ...\ndata: ...\n\n`) | [VERIFIED: codebase] Standard |
| `github.com/gin-gonic/gin` | v1.10.1 | Route registration for `/v1/messages` | [VERIFIED: go.mod] |

No new external dependencies needed. All translation logic uses stdlib only.

**Installation:** No new packages to install.

**Version verification:** All packages already present in go.mod. [VERIFIED: go.mod]

---

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── translator/
│   ├── types.go          # AnthropicRequest, AnthropicResponse, OpenAIRequest, OpenAIResponse structs
│   ├── request.go        # AnthropicToOpenAI() — request translation
│   ├── response.go       # OpenAIToAnthropic() — non-streaming response translation
│   ├── stream.go         # StreamTranslator — stateful SSE event translator
│   └── translator_test.go # Unit tests for all translation functions
├── server/
│   ├── relay.go          # Existing — handleRelay, proxyStream, proxyBuffer (unchanged)
│   ├── server.go         # Add: v1.POST("/v1/messages", s.handleAnthropicRelay)
│   └── anthropic.go      # New: handleAnthropicRelay, proxyAnthropicStream, proxyAnthropicBuffer
└── config/
    └── config.go         # Add: ModelOverride string to UpstreamConfig
```

### Pattern 1: Request Translation (Anthropic → OpenAI)

**What:** Convert an Anthropic `/v1/messages` request body into an OpenAI `/v1/chat/completions` request body.

**Field mapping:**

| Anthropic field | OpenAI field | Notes |
|----------------|--------------|-------|
| `system` (string) | `messages[0]` with `role: "system"` | Prepend as first message |
| `messages[].role` | `messages[].role` | Direct; Anthropic only has "user"/"assistant" |
| `messages[].content` (string) | `messages[].content` (string) | Direct if string |
| `messages[].content` (array of text blocks) | `messages[].content` (string) | Concatenate text blocks |
| `messages[].content` array with `tool_use` blocks | OpenAI `tool_calls` in assistant message | See tool use pattern |
| `messages[].content` array with `tool_result` blocks | `role: "tool"` message with `tool_call_id` | D-07: use `tool_use_id` as `tool_call_id` |
| `tools[].name` | `tools[].function.name` | Direct |
| `tools[].description` | `tools[].function.description` | Direct |
| `tools[].input_schema` | `tools[].function.parameters` | Direct (already JSON Schema) |
| `max_tokens` | `max_tokens` | Direct |
| `stream` | `stream` | Direct |
| `model` | `model` (overridden per D-01/D-02) | Replace with `model_override` or strip |

**Example:**
```go
// Source: Anthropic Messages API spec [CITED: docs.anthropic.com/en/api/messages]
// + OpenAI Chat Completions spec [ASSUMED: well-known mapping]

func AnthropicToOpenAI(req *AnthropicRequest, modelOverride string) (*OpenAIRequest, error) {
    out := &OpenAIRequest{
        MaxTokens: req.MaxTokens,
        Stream:    req.Stream,
    }
    // D-01/D-02: model override or strip
    if modelOverride != "" {
        out.Model = modelOverride
    }
    // system prompt → first message
    msgs := make([]OpenAIMessage, 0, len(req.Messages)+1)
    if req.System != "" {
        msgs = append(msgs, OpenAIMessage{Role: "system", Content: req.System})
    }
    for _, m := range req.Messages {
        translated, err := translateMessage(m)
        if err != nil {
            return nil, err
        }
        msgs = append(msgs, translated...)
    }
    out.Messages = msgs
    // tools
    if len(req.Tools) > 0 {
        out.Tools = translateTools(req.Tools)
    }
    return out, nil
}
```

### Pattern 2: Response Translation (OpenAI → Anthropic)

**What:** Convert an OpenAI non-streaming response into an Anthropic response.

**Field mapping:**

| OpenAI field | Anthropic field | Notes |
|-------------|----------------|-------|
| `id` | `id` | Prefix `msg_` if needed, or pass through |
| `choices[0].message.content` | `content[0].type: "text", text: ...` | Wrap in text block |
| `choices[0].message.tool_calls[]` | `content[]` with `type: "tool_use"` blocks | D-07: use `tool_call.id` as `tool_use.id` |
| `choices[0].finish_reason: "stop"` | `stop_reason: "end_turn"` | |
| `choices[0].finish_reason: "tool_calls"` | `stop_reason: "tool_use"` | |
| `choices[0].finish_reason: "length"` | `stop_reason: "max_tokens"` | |
| `usage.prompt_tokens` | `usage.input_tokens` | |
| `usage.completion_tokens` | `usage.output_tokens` | |
| `model` (upstream model name) | `model` (echoed from original request, D-03) | |

**Example:**
```go
// Source: Anthropic Messages API spec [CITED: docs.anthropic.com/en/api/messages]

func OpenAIToAnthropic(resp *OpenAIResponse, originalModel string) (*AnthropicResponse, error) {
    out := &AnthropicResponse{
        ID:    resp.ID,
        Type:  "message",
        Role:  "assistant",
        Model: originalModel, // D-03: echo back client's requested model
    }
    if len(resp.Choices) == 0 {
        return nil, fmt.Errorf("no choices in upstream response")
    }
    ch := resp.Choices[0]
    // translate content
    var content []AnthropicContentBlock
    if ch.Message.Content != "" {
        content = append(content, AnthropicContentBlock{Type: "text", Text: ch.Message.Content})
    }
    for _, tc := range ch.Message.ToolCalls {
        content = append(content, AnthropicContentBlock{
            Type:  "tool_use",
            ID:    tc.ID,          // D-07: preserve tool_call_id
            Name:  tc.Function.Name,
            Input: json.RawMessage(tc.Function.Arguments),
        })
    }
    out.Content = content
    out.StopReason = translateFinishReason(ch.FinishReason)
    out.Usage = AnthropicUsage{
        InputTokens:  resp.Usage.PromptTokens,
        OutputTokens: resp.Usage.CompletionTokens,
    }
    return out, nil
}
```

### Pattern 3: Streaming Translation (StreamTranslator)

**What:** Stateful translator that accepts raw bytes from the OpenAI upstream SSE stream and emits complete Anthropic SSE event lines.

**State machine:**
```
INIT → (first chunk with content delta) → emit message_start + content_block_start → STREAMING
STREAMING → (each delta chunk) → emit content_block_delta
STREAMING → ([DONE] or finish_reason present) → emit content_block_stop + message_delta + message_stop → DONE
```

**Key design points:**
- `StreamTranslator` holds: `started bool`, `model string`, `buf []byte` (partial frame accumulator)
- `Translate(chunk []byte) ([][]byte, error)` — accepts raw bytes, returns zero or more complete Anthropic SSE event byte slices
- OpenAI SSE lines: `data: {...}\n\n` — parse with `bufio.Scanner` using `\n\n` as delimiter or line-by-line
- Each OpenAI chunk has `choices[0].delta.content` (string, possibly empty) and optionally `choices[0].finish_reason`

**OpenAI streaming chunk → Anthropic events mapping:**

| Condition | Anthropic events emitted |
|-----------|--------------------------|
| First non-empty delta (started=false) | `message_start` → `content_block_start{index:0,type:"text"}` → `content_block_delta{text_delta}` |
| Subsequent non-empty delta | `content_block_delta{type:"text_delta",text:...}` |
| Empty delta, no finish_reason | (nothing) |
| finish_reason present (or `[DONE]`) | `content_block_stop{index:0}` → `message_delta{stop_reason,...}` → `message_stop` |

**SSE event wire format:**
```
event: message_start\ndata: {"type":"message_start","message":{...}}\n\n
event: content_block_start\ndata: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}\n\n
event: content_block_delta\ndata: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}\n\n
event: content_block_stop\ndata: {"type":"content_block_stop","index":0}\n\n
event: message_delta\ndata: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":0}}\n\n
event: message_stop\ndata: {"type":"message_stop"}\n\n
```

**Example struct:**
```go
// Source: Anthropic streaming spec [CITED: docs.anthropic.com/en/api/messages-streaming]

type StreamTranslator struct {
    model   string
    started bool
    buf     []byte  // accumulates incomplete SSE frames across Read() calls
}

func NewStreamTranslator(originalModel string) *StreamTranslator {
    return &StreamTranslator{model: originalModel}
}

// Translate accepts raw upstream bytes, returns 0+ Anthropic SSE event byte slices.
func (st *StreamTranslator) Translate(chunk []byte) ([][]byte, error) {
    st.buf = append(st.buf, chunk...)
    var events [][]byte
    // parse complete SSE frames from st.buf (split on \n\n)
    for {
        idx := bytes.Index(st.buf, []byte("\n\n"))
        if idx < 0 {
            break
        }
        frame := st.buf[:idx]
        st.buf = st.buf[idx+2:]
        emitted, err := st.translateFrame(frame)
        if err != nil {
            return nil, err
        }
        events = append(events, emitted...)
    }
    return events, nil
}
```

### Pattern 4: Handler Wiring (anthropic.go)

**What:** New Gin handler `handleAnthropicRelay` that mirrors `handleRelay` but calls translator functions.

**Differences from handleRelay:**
1. Parse request as `AnthropicRequest` (not raw passthrough)
2. Call `AnthropicToOpenAI(req, upstream.ModelOverride)` to produce the upstream request body
3. Forward to upstream `/v1/chat/completions` (same endpoint as OpenAI pass-through)
4. For non-streaming: call `proxyAnthropicBuffer` which reads OpenAI response, calls `OpenAIToAnthropic`, returns JSON
5. For streaming: call `proxyAnthropicStream` which instantiates `StreamTranslator` and pumps translated events to client

**Streaming detect:** Check `req.Stream` field (bool) from the parsed `AnthropicRequest` — same approach as `rb.Stream` in handleRelay.

**Reused unchanged:** `authMiddleware`, `logUsage`, `relayClient`, `cloneHeaders`, failover loop structure, `pool.Select/Mark/Backoff`, `pool.Classify`.

### Pattern 5: Config Extension (model_override)

**What:** Add `ModelOverride` field to `UpstreamConfig`.

```go
// internal/config/config.go
type UpstreamConfig struct {
    Name          string `mapstructure:"name"`
    BaseURL       string `mapstructure:"base_url"`
    APIKey        string `mapstructure:"api_key"`
    Enabled       bool   `mapstructure:"enabled"`
    ModelOverride string `mapstructure:"model_override"` // D-01: optional model replacement
}
```

The `ModelOverride` value must be propagated through `pool.UpstreamEntry` so the handler can access it at relay time.

**pool.UpstreamEntry extension:**
```go
// internal/pool/pool.go
type UpstreamEntry struct {
    ID            uint
    Name          string
    BaseURL       string
    APIKey        string
    ModelOverride string // new — passed to translator
}
```

### Anti-Patterns to Avoid

- **Passing `claude-*` model names to OpenAI-compatible upstreams.** Upstreams will reject with 400. Always apply D-01/D-02 model stripping before forwarding.
- **Duplicating the failover loop.** The handler should share the same loop structure as `handleRelay`, not copy-paste diverging logic.
- **Buffering the entire SSE stream before translating.** Translate frame-by-frame in `StreamTranslator.Translate()` so tokens arrive at the client without delay.
- **Using `httputil.ReverseProxy` for the Anthropic handler.** Body must be re-readable across retry attempts; manual `http.NewRequest` + copy is required (same as Phase 3 choice).
- **Assuming OpenAI `content` is always a string.** In multi-turn tool-use conversations, `content` may be null with tool calls in `tool_calls`. Handle both cases.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON schema for tool parameters | Custom schema type | Pass `input_schema` / `parameters` as `json.RawMessage` | Avoids re-encoding and round-trip loss |
| SSE frame framing | Custom delimiter scanner | `bufio.Scanner` with `ScanLines` or manual `bytes.Index("\n\n")` | Handles partial reads correctly |
| Anthropic API types | From scratch | Reference `github.com/liushuangls/go-anthropic` for field names | Field names are a source of bugs |

**Key insight:** The translator is a pure data transformation — no I/O, no goroutines, no dependencies. Keep it that way. All networking/streaming stays in the handler.

---

## Common Pitfalls

### Pitfall 1: Strict Role Alternation
**What goes wrong:** Anthropic API rejects requests where consecutive messages have the same role. Multi-turn tool-use conversations often result in back-to-back user messages (first the user query, then the `tool_result`). However, since ocp is translating *from* Anthropic format (which already enforces alternation), the incoming messages array should be well-formed — no merging needed. The risk is in the reverse direction if somehow malformed input arrives.
**Why it happens:** Anthropic's training assumes strict user/assistant alternation.
**How to avoid:** The translator should pass messages through as-is from the (already-Anthropic-format) client. Do not attempt to merge/reorder. If needed, validate on input and return 400.
**Warning signs:** 400 response from upstream with "messages must alternate" error body.

### Pitfall 2: `max_tokens` Required by Anthropic
**What goes wrong:** Anthropic requires `max_tokens` in every request. If the client sends a request without it, the translation passes a zero value to upstream, which may fail.
**Why it happens:** Anthropic's API has no default for `max_tokens` unlike OpenAI.
**How to avoid:** In `AnthropicToOpenAI`, if `req.MaxTokens == 0`, set a safe default (e.g., 4096) or return a 400 to the client immediately.
**Warning signs:** Upstream returns 400 with "max_tokens is required".

### Pitfall 3: Partial SSE Frame Across Read() Calls
**What goes wrong:** `resp.Body.Read()` may return a buffer that cuts mid-frame (e.g., `data: {"choices":[{"delta":{"content":"He` without the closing `}}\n\n`). Parsing this naively produces JSON decode errors.
**Why it happens:** TCP segmentation; OS read buffer boundaries.
**How to avoid:** `StreamTranslator` accumulates bytes in `st.buf` and only processes complete frames (those ending in `\n\n`). Any trailing bytes wait for the next `Translate()` call.
**Warning signs:** Intermittent JSON parse errors in streaming tests with large token output.

### Pitfall 4: tool_call_id Round-Trip Corruption
**What goes wrong:** If `tool_use.id` is not faithfully preserved from the upstream `tool_call.id`, subsequent `tool_result` messages will have a mismatched `tool_use_id` → `tool_call_id` translation, causing the upstream to return 400 "tool_call_id does not match".
**Why it happens:** Forgetting to map `tool_call.id` → `tool_use.id` in response translation, and `tool_result.tool_use_id` → `tool_call_id` in request translation.
**How to avoid:** D-07 is the solution. Tests must cover a two-turn tool-use conversation: turn 1 gets `tool_use` block, turn 2 sends `tool_result` with the same ID.
**Warning signs:** 400 errors specifically on the second turn of a tool-use exchange.

### Pitfall 5: Model Name Leakage
**What goes wrong:** Upstream provider name (e.g., `qwen-max`) appears in the Anthropic response `model` field instead of the client's requested model (e.g., `claude-opus-4-5`). Clients that validate the response model field against their request will fail.
**Why it happens:** Forwarding the upstream response `model` field verbatim without applying D-03.
**How to avoid:** Pass `originalModel` from the parsed Anthropic request into `OpenAIToAnthropic()` and always set `out.Model = originalModel`.
**Warning signs:** Claude Code logs "model mismatch" warnings or validation errors.

### Pitfall 6: `system` Field in Multi-Turn Messages
**What goes wrong:** Anthropic requests have a top-level `system` field (not in `messages`). When translating, if `system` is non-empty, it must become a `role: "system"` message prepended to the OpenAI messages array. If omitted, the upstream has no system prompt.
**Why it happens:** Forgetting that Anthropic's `system` is separate from `messages`.
**How to avoid:** Explicit check in `AnthropicToOpenAI`: if `req.System != ""`, prepend `{role: "system", content: req.System}`.
**Warning signs:** Upstream ignores system instructions; test with a system prompt and verify it appears in the forwarded request body.

---

## Code Examples

### Anthropic Request Struct (key fields)
```go
// Source: Anthropic Messages API spec [CITED: docs.anthropic.com/en/api/messages]
// Field names verified against official spec documentation.

type AnthropicRequest struct {
    Model     string             `json:"model"`
    Messages  []AnthropicMessage `json:"messages"`
    System    string             `json:"system,omitempty"`
    MaxTokens int                `json:"max_tokens"`
    Stream    bool               `json:"stream,omitempty"`
    Tools     []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMessage struct {
    Role    string      `json:"role"`
    Content interface{} `json:"content"` // string OR []AnthropicContentBlock
}

type AnthropicContentBlock struct {
    Type    string          `json:"type"`             // "text", "image", "tool_use", "tool_result"
    Text    string          `json:"text,omitempty"`    // type=text
    ID      string          `json:"id,omitempty"`      // type=tool_use: upstream tool_call_id
    Name    string          `json:"name,omitempty"`    // type=tool_use
    Input   json.RawMessage `json:"input,omitempty"`   // type=tool_use: JSON object
    ToolUseID string        `json:"tool_use_id,omitempty"` // type=tool_result: matches tool_use.id
    Content interface{}     `json:"content,omitempty"` // type=tool_result: string or blocks
}

type AnthropicTool struct {
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    InputSchema json.RawMessage `json:"input_schema"` // JSON Schema object
}
```

### Anthropic Response Struct (key fields)
```go
// Source: Anthropic Messages API spec [CITED: docs.anthropic.com/en/api/messages]

type AnthropicResponse struct {
    ID           string                  `json:"id"`
    Type         string                  `json:"type"`          // always "message"
    Role         string                  `json:"role"`          // always "assistant"
    Content      []AnthropicContentBlock `json:"content"`
    Model        string                  `json:"model"`         // echoed from request (D-03)
    StopReason   string                  `json:"stop_reason"`   // "end_turn", "max_tokens", "tool_use", "stop_sequence"
    StopSequence *string                 `json:"stop_sequence"` // null or matched sequence
    Usage        AnthropicUsage          `json:"usage"`
}

type AnthropicUsage struct {
    InputTokens  int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
}
```

### finish_reason → stop_reason Mapping
```go
// Source: [ASSUMED] — standard mapping documented in multiple proxy implementations
func translateFinishReason(reason string) string {
    switch reason {
    case "stop":
        return "end_turn"
    case "tool_calls":
        return "tool_use"
    case "length":
        return "max_tokens"
    case "content_filter":
        return "stop_sequence" // closest analog
    default:
        return "end_turn"
    }
}
```

### SSE Event Serialization
```go
// Source: Anthropic streaming spec [CITED: docs.anthropic.com/en/api/messages-streaming]
// Wire format: "event: {type}\ndata: {json}\n\n"

func formatSSEEvent(eventType string, payload interface{}) ([]byte, error) {
    data, err := json.Marshal(payload)
    if err != nil {
        return nil, err
    }
    return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}
```

### message_start payload shape
```json
{
  "type": "message_start",
  "message": {
    "id": "msg_placeholder",
    "type": "message",
    "role": "assistant",
    "content": [],
    "model": "<echoed from request>",
    "stop_reason": null,
    "stop_sequence": null,
    "usage": { "input_tokens": 0, "output_tokens": 1 }
  }
}
```
[CITED: docs.anthropic.com/en/api/messages-streaming — verified via WebSearch result with exact payloads]

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Pass model name through | Strip/override model per upstream (D-01/D-02) | Phase 4 design | Prevents 400 from upstream rejecting `claude-*` names |
| Global SSE passthrough | Per-frame SSE re-emission in Anthropic format | Phase 4 design | Enables native Anthropic clients (Claude Code) |
| No tool use support | Full bidirectional tool use translation | Phase 4 design | Enables multi-turn agentic workflows |

**Not needed in this phase:**
- `input_json_delta` streaming (D-05 explicitly deferred — tool input arrives as text deltas, treated as regular text_delta)
- `ping` events (Claude Code tolerates absence per D-04 discretion)
- Extended thinking (`thinking_delta`) — out of scope

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `finish_reason: "stop"` → `stop_reason: "end_turn"` mapping | Code Examples | If upstreams use non-standard finish_reason values, stop_reason may be wrong. Low risk — "stop" is universal. |
| A2 | Multi-turn Anthropic requests already have strictly alternating roles (no merging needed in translator) | Pitfalls | If clients send malformed requests, translator will pass them through and upstream returns 400. Add input validation if this occurs. |
| A3 | Claude Code tolerates absence of `ping` event | Architecture Patterns (streaming) | If Claude Code breaks without `ping`, add `ping` emission after `message_start`. Easy to add. |
| A4 | `UpstreamEntry` in pool.go can safely receive a new `ModelOverride string` field | Architecture Patterns | If pool construction logic changes between now and implementation, field wiring may need adjustment. Very low risk. |

---

## Open Questions

1. **`max_tokens` default when client omits it**
   - What we know: Anthropic *requires* `max_tokens` but OpenAI-compatible upstreams do not. The Anthropic SDK always sends it.
   - What's unclear: Should the translator return 400 to the client, or silently supply a default?
   - Recommendation: Supply a safe default (e.g., 4096) and log a warning. Client omitting `max_tokens` is a client bug, but failing silently is better UX than an opaque 400.

2. **`tool_use` block `input` field type in response**
   - What we know: OpenAI returns `tool_calls[].function.arguments` as a JSON *string* (not object). Anthropic `tool_use.input` is a JSON *object*.
   - What's unclear: Whether to parse-and-re-encode `arguments` or treat it as `json.RawMessage`.
   - Recommendation: Treat `arguments` as `json.RawMessage` — it is already valid JSON. Set `tool_use.input = json.RawMessage(tc.Function.Arguments)`. This avoids double-encoding.

---

## Environment Availability

Step 2.6: SKIPPED — Phase 4 is purely code changes to the existing Go codebase. No new external services, databases, or CLI tools are required beyond what Phase 3 already exercises.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` package (stdlib) |
| Config file | none — standard `go test` |
| Quick run command | `go test ./internal/translator/... -v` |
| Full suite command | `go test ./... -race` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PRXY-02 | `/v1/messages` returns valid Anthropic response schema | integration | `go test ./internal/server/... -run TestAnthropicRelay -v` | Wave 0 |
| PRXY-02 | `/v1/messages` SSE streaming emits correct event types | integration | `go test ./internal/server/... -run TestAnthropicStream -v` | Wave 0 |
| PRXY-04 | `AnthropicToOpenAI` translates system prompt correctly | unit | `go test ./internal/translator/... -run TestAnthropicToOpenAI -v` | Wave 0 |
| PRXY-04 | `AnthropicToOpenAI` translates tool definitions correctly | unit | `go test ./internal/translator/... -run TestAnthropicToOpenAI_Tools -v` | Wave 0 |
| PRXY-04 | `AnthropicToOpenAI` translates tool_result messages correctly | unit | `go test ./internal/translator/... -run TestAnthropicToOpenAI_ToolResult -v` | Wave 0 |
| PRXY-04 | `OpenAIToAnthropic` translates text response correctly | unit | `go test ./internal/translator/... -run TestOpenAIToAnthropic -v` | Wave 0 |
| PRXY-04 | `OpenAIToAnthropic` translates tool_calls to tool_use blocks | unit | `go test ./internal/translator/... -run TestOpenAIToAnthropic_ToolUse -v` | Wave 0 |
| PRXY-04 | `OpenAIToAnthropic` echoes originalModel (D-03) | unit | `go test ./internal/translator/... -run TestModelEcho -v` | Wave 0 |
| PRXY-04 | `StreamTranslator` emits message_start + content_block_start on first delta | unit | `go test ./internal/translator/... -run TestStreamTranslator_Start -v` | Wave 0 |
| PRXY-04 | `StreamTranslator` emits message_stop on [DONE] | unit | `go test ./internal/translator/... -run TestStreamTranslator_Done -v` | Wave 0 |
| PRXY-04 | `StreamTranslator` handles partial frame buffering | unit | `go test ./internal/translator/... -run TestStreamTranslator_Partial -v` | Wave 0 |
| PRXY-02 | Multi-turn tool use round-trip (tool_call_id, D-07) | integration | `go test ./internal/server/... -run TestAnthropicToolRoundTrip -v` | Wave 0 |
| PRXY-01 | OpenAI pass-through regression (unaffected by translation layer) | regression | `go test ./internal/server/... -run TestRelay -v` | ✅ relay_test.go |

### Sampling Rate
- **Per task commit:** `go test ./internal/translator/... -v`
- **Per wave merge:** `go test ./... -race`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/translator/translator_test.go` — unit tests for all translation functions (PRXY-04)
- [ ] `internal/server/anthropic_test.go` — integration tests for `/v1/messages` handler (PRXY-02)
- [ ] `internal/translator/types.go`, `request.go`, `response.go`, `stream.go` — all new (no existing code)
- [ ] `internal/config/config.go` — add `ModelOverride` field
- [ ] `internal/pool/pool.go` — add `ModelOverride` to `UpstreamEntry`

*(Existing relay_test.go covers PRXY-01 regression — no new test file needed for that.)*

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Phase 3 auth middleware reused unchanged |
| V3 Session Management | no | Stateless HTTP, no sessions |
| V4 Access Control | no | Auth enforced at middleware level, same as Phase 3 |
| V5 Input Validation | yes | Validate `AnthropicRequest` on parse; return 400 for malformed bodies |
| V6 Cryptography | no | No new crypto operations |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Malformed JSON in translation structs | Tampering | `json.Unmarshal` returns error; handler returns 400 |
| Tool `input_schema` containing malicious JSON | Tampering | Treat as `json.RawMessage` — pass through verbatim, no eval |
| Oversized request body | DoS | Existing 10MB limit in `handleRelay` — replicate in `handleAnthropicRelay` |
| `tool_call_id` injection via crafted responses | Spoofing | IDs are opaque strings passed through; no execution occurs |

---

## Sources

### Primary (HIGH confidence)
- Anthropic Messages API spec — request/response field names and streaming event sequence [CITED: docs.anthropic.com/en/api/messages + messages-streaming — verified via WebSearch returning exact JSON payloads from official docs]
- Existing codebase: `internal/server/relay.go`, `internal/pool/pool.go`, `internal/config/config.go`, `internal/server/relay_test.go` [VERIFIED: codebase — read directly]
- `go.mod` — confirmed no new external dependencies needed [VERIFIED: go.mod]

### Secondary (MEDIUM confidence)
- Streaming SSE event payloads (message_start, content_block_delta etc.) — exact JSON shapes confirmed via WebSearch returning official Anthropic doc content [CITED: docs.anthropic.com/en/api/messages-streaming]
- tool_calls ↔ tool_use bidirectional mapping — confirmed via multiple sources including tokligence/openai-anthropic-endpoint-translation Go package and liteLLM documentation [CITED: pkg.go.dev/github.com/tokligence/openai-anthropic-endpoint-translation/pkg/translator]

### Tertiary (LOW confidence)
- `finish_reason: "content_filter"` → `stop_reason: "stop_sequence"` mapping [ASSUMED — no official mapping documented; content_filter is rare, stop_sequence is closest analog]

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all stdlib, all already in go.mod
- Architecture: HIGH — based on actual existing code patterns in relay.go + verified Anthropic API spec
- Pitfalls: HIGH — based on actual API spec requirements (max_tokens required, strict alternation) + well-known streaming implementation issues
- Translation field mapping: HIGH — Anthropic spec field names confirmed from official docs via WebSearch

**Research date:** 2026-04-16
**Valid until:** 2026-07-16 (stable API — both Anthropic Messages API v1 and OpenAI Chat Completions are stable; 90-day window)
