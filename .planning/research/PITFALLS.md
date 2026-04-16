# Pitfalls Research — one-codingplan (ocp)

**Researched:** 2026-04-16
**Overall confidence:** MEDIUM-HIGH (verified against official docs and multiple technical sources)

---

## Critical Pitfalls (project-killers if missed)

### P1: Nginx proxy buffering kills SSE streaming

**What goes wrong:** By default, Nginx (and most reverse proxies) buffer upstream responses. An LLM streaming response that trickles in as small SSE chunks gets held in the proxy buffer and delivered in one burst — or not at all. The client perceives the connection as hanging, then receives a wall of text (or a timeout).

**Why it happens:** Nginx defaults `proxy_buffering on`. Each `data: {...}\n\n` SSE event is only a few hundred bytes; Nginx accumulates them until the buffer fills before flushing downstream. For very long responses the buffer spills to disk, adding severe overhead.

**Consequences:** Streaming appears broken to every client (Claude Code, Codex, curl). The most common user report is "responses arrive all at once" or "connection times out on long generations."

**Warning signs:**
- Tokens arrive in batches, not one at a time
- `proxy_buffering` is not explicitly set in Nginx config
- gzip compression is enabled globally

**Prevention:**
```nginx
location /v1/ {
    proxy_buffering off;
    proxy_cache off;
    proxy_http_version 1.1;
    proxy_set_header Connection '';
    proxy_read_timeout 600s;
    gzip off;
    # Or use the header approach (per-response, more flexible):
    # upstream must set: X-Accel-Buffering: no
}
```
Also disable gzip globally for streaming routes — gzip compression requires buffering the full response before compressing.

**Phase:** Proxy core (Phase 1). Must be in place before any streaming test.

---

### P2: All 429 errors look the same — credits vs. rate limit confusion

**What goes wrong:** "Credits exhausted" and "rate limited" both return HTTP 429 from every Chinese provider (and OpenAI). If the proxy treats all 429s identically as "rate limited — retry after cooldown," it will retry a key that is permanently dead, burning time and requests. Conversely, if it treats all 429s as "credits gone — mark key dead," a transient rate limit causes permanent failover away from a healthy key.

**Why it happens:** OpenAI's spec defines 429 for both rate limits and quota exhaustion. Providers frequently use the same status code for both conditions, relying on a message field or a custom error code to distinguish them. Kimi (Moonshot) is a documented example: when account balance reaches zero the API returns 429 with `type: exceeded_current_quota_error`, identical HTTP status to a rate-limit 429.

**Consequences:** Proxy gets stuck in a bad rotation loop. Either it never uses a key that has remaining rate-limit capacity, or it keeps hammering a depleted key.

**Warning signs:**
- All upstreams being marked unhealthy within minutes
- Error logs show 429 with "quota" or "balance" in the message but proxy still retries
- Upstream key appears healthy in the dashboard but is never used

**Prevention:** Inspect the error body, not just the status code. Define a classification table per provider:

| Provider | Signal | Classification |
|----------|--------|----------------|
| Kimi (Moonshot) | `type: exceeded_current_quota_error` | Credits exhausted → mark key dead |
| Kimi (Moonshot) | `type: rate_limit_error` | Rate limited → back off, keep key alive |
| Qwen (DashScope) | `error.code: Throttling.RateQuota` | Rate limited |
| Qwen (DashScope) | `error.code: InvalidApiKey` or billing message | Credits exhausted |
| MiniMax | `base_resp.status_code: 1008` (credits) vs `1002` (rate limit) | See MiniMax error code reference |
| GLM (Zhipu) | HTTP 429 + message containing "balance" | Credits exhausted |

Also handle HTTP 402 (rare but used by some providers for billing failures) and HTTP 403 (used by some providers when balance is zero).

**Phase:** Upstream health tracking (Phase 2). Required before round-robin is trustworthy.

---

### P3: Round-robin state race condition corrupts key selection

**What goes wrong:** Two concurrent requests both read the current-index pointer at the same time, both advance it, and both select the same upstream. Or one request advances the index past a just-marked-dead key, and another request lands on that dead key before the write propagates.

**Why it happens:** Without a mutex around the read-increment-write of the round-robin index, Go goroutines (or Node event loop callbacks sharing state with async ops) produce data races. The race is invisible under low concurrency but surfaces in production.

**Consequences:** Requests sent to dead upstreams; uneven distribution; the same upstream getting hammered while others sit idle; crash if index goes out of bounds on a live-modified slice.

**Warning signs:**
- Running `go test -race` reveals a data race on the index field
- One upstream in logs gets 10x more traffic than others
- Intermittent 500s that disappear on retry

**Prevention:**
- Use an `atomic.Int64` or `sync/atomic` for the index counter in Go; it is lock-free and safe for simple increment.
- Wrap health state reads/writes in a per-upstream `sync.RWMutex` — read lock for selection, write lock for marking dead.
- Keep the critical section minimal: read index, copy upstream slice reference, advance index atomically, release.
- For the health map, prefer an immutable replacement pattern: build a new `[]Upstream` slice each time health changes and swap the pointer atomically.

**Phase:** Proxy core / round-robin implementation (Phase 1).

---

### P4: Anthropic tool_use / tool_result ordering violation breaks multi-turn

**What goes wrong:** Anthropic's API requires that every `tool_use` block in an assistant message be immediately followed by a `tool_result` block in the next user message, with no intervening messages. When the proxy translates OpenAI function-call history into Anthropic messages format, intermediate messages or out-of-order buffering causes: `messages.1: tool_use ids were found without tool_result blocks immediately after`.

**Why it happens:** OpenAI's function calling places tool results as `role: tool` messages that can appear anywhere. Anthropic's is stricter: the conversation must alternate correctly. A naive 1-to-1 mapping breaks multi-step agentic workflows.

**Consequences:** Claude Code agentic sessions (which make heavy use of tool calls) fail on the second or third turn with a cryptic 400 error. The session cannot continue.

**Warning signs:**
- 400 errors containing "tool_use ids were found" in the body
- Error appears only on the second or later turn, not on first request
- Error appears only in streaming mode (due to buffering timing)

**Prevention:**
- Implement a message history normalizer in the Anthropic translation layer that:
  1. Ensures every `tool_use` assistant block has a matching `tool_result` in the immediately following user message
  2. Preserves the `reasoning_content` field from Kimi/thinking models (see P7) when rebuilding history
- Test with multi-turn tool-call sequences, not just single-turn requests.

**Phase:** Format translation layer (Phase 1/2). Must be proven before advertising Anthropic compatibility.

---

### P5: Streaming delta with empty `role` field breaks OpenAI SDK clients

**What goes wrong:** MiniMax's streaming API emits `"role": ""` (empty string) in some delta chunks instead of omitting the field or setting it to `"assistant"`. The OpenAI Python and Node SDKs validate the role field and raise a parsing error when they see an empty string.

**Why it happens:** MiniMax has a known streaming spec deviation. It is a provider bug, but it manifests as a proxy failure because the proxy passes the chunk through unmodified.

**Consequences:** All streaming requests to MiniMax fail mid-stream with a client-side SDK exception. The user sees a partial response followed by an error.

**Warning signs:**
- Streaming works in non-streaming mode (`stream: false`)
- Error message from client mentions unexpected `role` value or empty string
- Specific to MiniMax as the active upstream

**Prevention:** The proxy's streaming pass-through layer must sanitize delta chunks from each upstream:
- If `role` field is present and empty string, remove the field from the delta object before forwarding.
- This normalization belongs in a per-provider stream transformer, not a global one.

**Phase:** Provider adapters (Phase 2). MiniMax adapter specifically.

---

## Common Implementation Mistakes

### M1: Token count tracking diverges from actual upstream consumption

**What goes wrong:** Different providers use different tokenizers. The same message costs 8 tokens on Kimi but 11 tokens on Qwen. If the proxy pre-counts tokens using one tokenizer and all upstreams bill on a different count, the budget tracking drifts. Budget enforcement becomes unreliable — keys get marked exhausted prematurely or overdraw.

**Warning signs:**
- Dashboard token totals don't match provider invoices
- Keys hit token budget limit before the invoice balance is zero

**Prevention:**
- Never pre-count tokens with a local tokenizer for billing/budget purposes.
- Always read `usage.prompt_tokens` and `usage.completion_tokens` from the upstream response body.
- For streaming responses, usage is typically in the final `data: [DONE]` chunk or a trailing `usage` SSE event — ensure the proxy reads the stream to completion even after forwarding the last content chunk.
- Accept that streaming token counts arrive late (after the response ends) and budget deduction must be post-hoc. Design the budget check as: "if current spend > budget, reject the *next* request" not "block mid-stream."

**Phase:** Usage tracking (Phase 2). Critical for per-key budget enforcement.

---

### M2: Forgetting to flush the final SSE chunk

**What goes wrong:** After forwarding the last content chunk from upstream, the proxy's write buffer may still hold a partial SSE frame. If the upstream closes the connection before the proxy explicitly flushes its outbound buffer, the client never receives the `[DONE]` sentinel or the last token of the response.

**Warning signs:**
- Responses are consistently missing the last word or sentence
- `finish_reason` is never received by the client
- Bug is absent when testing with `stream: false`

**Prevention:**
- After forwarding each upstream chunk, explicitly flush the downstream ResponseWriter/stream.
- After the upstream stream ends (upstream sends `data: [DONE]`), forward `data: [DONE]\n\n` to client and flush before closing.
- In Go: use `http.Flusher` interface; in Node: call `res.flush()` or `res.write()` explicitly.

**Phase:** Proxy core streaming (Phase 1).

---

### M3: Upstream timeout kills long-running generations mid-stream

**What goes wrong:** A long LLM generation (e.g., a 4000-token response) takes 90 seconds. The proxy's upstream HTTP client has a default 30-second read timeout. The proxy receives an upstream timeout error mid-stream, closes the downstream connection, and the client gets a truncated response with no error message.

**Warning signs:**
- Long requests fail but short requests succeed
- Error appears exactly at a consistent time boundary (30s, 60s, 120s)
- Client receives partial response then connection drop

**Prevention:**
- Set upstream HTTP client read timeout to 600s minimum for generation endpoints.
- Distinguish the upstream `dial` timeout (short, e.g. 10s) from the `read` timeout (long, e.g. 600s).
- Emit SSE heartbeat comments (`:\n\n`) every 15-30 seconds to prevent intermediate proxies from killing idle connections.

**Phase:** Proxy core (Phase 1).

---

### M4: Proxy key entropy is insufficient

**What goes wrong:** Proxy-issued access keys generated with `math/rand` (not `crypto/rand`), sequential IDs, or short random strings are guessable. An attacker who obtains one key can brute-force others or predict future keys.

**Warning signs:**
- Keys are short (< 32 chars)
- Keys are base-10 numeric or use a small charset
- Key generation uses a seeded PRNG

**Prevention:**
- Generate keys with `crypto/rand`, 32 bytes minimum, base62 or hex-encoded = 43+ character keys.
- Use the format `ocp_<random>` to namespace keys and make them grep-able in logs.
- Store only the SHA-256 hash of the key in the database; compare submitted keys by hashing then comparing digests (constant-time comparison).
- Never log the full key value — log only the first 8 chars + `...` as an identifier.

**Phase:** Key management (Phase 2).

---

### M5: Health poller marks upstreams dead too aggressively

**What goes wrong:** A single transient network error causes the poller to immediately mark an upstream dead. All in-flight requests are rerouted. The upstream recovers in 2 seconds but stays dead for the full health-check interval (e.g., 60 seconds). The active pool is reduced to fewer upstreams than necessary.

**Warning signs:**
- Upstreams flip between healthy/unhealthy every few minutes in logs
- Pool size frequently drops to 1 even when all providers are reachable

**Prevention:**
- Use a circuit-breaker with a failure threshold (e.g., 3 consecutive failures within 60 seconds → mark dead).
- Use exponential backoff for recovery probes: first re-check after 30s, then 60s, then 120s, cap at 300s.
- Distinguish between "connection refused" (hard failure → mark dead immediately) and "timeout" (soft failure → increment counter).

**Phase:** Upstream health tracking (Phase 2).

---

### M6: Retrying non-retryable errors wastes upstream credits

**What goes wrong:** The proxy retries requests on 400 errors. But a 400 means the request itself is malformed — retrying it against the next upstream sends the same bad request again, consuming input tokens on every upstream until the pool is exhausted.

**Warning signs:**
- All upstreams show 400 errors for the same request in the same second
- Token usage spikes on error conditions

**Prevention:**
- Only retry on: 429 (rate limit type only, not quota), 5xx, network timeouts, connection errors.
- Never retry on: 400 (bad request), 401 (auth), 403 (forbidden), 404 (model not found), 413 (too large).
- Log the original error and forward the last upstream error to the client when all retries are exhausted.

**Phase:** Proxy core / failover logic (Phase 1).

---

## Provider-Specific Quirks

### Q1: Kimi (Moonshot) — `reasoning_content` required in conversation history

**What goes wrong:** When Kimi's thinking mode is active, every assistant message containing a tool call must include a `reasoning_content` field when it appears in subsequent-turn conversation history. If the proxy strips this field during message normalization (e.g., when translating to OpenAI format for other upstreams), and the session later lands back on Kimi, the API returns: `400 - thinking is enabled but reasoning_content is missing in assistant tool call message`.

**Prevention:**
- Preserve `reasoning_content` in stored conversation history even though it has no OpenAI equivalent.
- When forwarding to non-Kimi upstreams, strip it. When forwarding to Kimi with thinking enabled, include it.
- This requires per-upstream message reconstruction, not a single normalization pass.

**Endpoints:**
- International: `api.moonshot.ai/v1`
- Domestic (China): `api.moonshot.cn/v1` — important: keys from `platform.moonshot.ai` return 401 on the `.cn` endpoint

**Balance API:** `GET /v1/users/me/balance` returns `available_balance`, `voucher_balance`, `cash_balance`.

---

### Q2: MiniMax — `reasoning_details` not in streaming, empty role in delta

Two distinct bugs in MiniMax's OpenAI-compatible streaming:
1. `"role": ""` in delta chunks (see P5 above)
2. When `reasoning_split: true` is in the request, non-streaming returns `reasoning_details` correctly but streaming wraps thinking in `<think>` tags inside the `content` field with no `reasoning_details` field

**Prevention:**
- Strip empty `role` fields in the MiniMax stream transformer.
- When MiniMax is the active upstream and thinking/reasoning is requested, default to non-streaming or accept that `<think>` tag content will appear in the streamed output.
- MiniMax ignores: `presence_penalty`, `frequency_penalty`, `logit_bias` — do not forward these fields.

**Balance:** No programmatic balance API documented. Detect credits exhaustion from error messages.

---

### Q3: Qwen (DashScope) — model ID versioning and endpoint selection

**What goes wrong:** Model aliases like `qwen-plus` or `qwen-plus-latest` stop resolving or return 503 without notice. Pinned version names like `qwen-plus-2025-12-01` are required in production.

**Regional endpoints:**
- China: `https://dashscope.aliyuncs.com/compatible-mode/v1`
- International/Singapore: `https://dashscope-intl.aliyuncs.com/compatible-mode/v1`

For a China-hosted ocp instance, use the domestic endpoint to avoid GFW latency.

**Free tier limit:** 2,000 requests/day cap on the free preview tier. At free tier, data is used for training and TTFT averages 11.5s. Use a paid plan for production.

**Prevention:**
- Store full versioned model IDs in the upstream config (e.g., `qwen-plus-2025-12-01` not `qwen-plus`).
- Handle 503 with body containing "No available channels" as a model-not-found error (do not retry; escalate to operator).

---

### Q4: GLM (Zhipu / Z.AI) — dual endpoint and format options

GLM exposes both an OpenAI-compatible endpoint and an Anthropic-compatible endpoint:
- OpenAI: `https://open.bigmodel.cn/api/paas/v4/`
- Anthropic: `https://api.z.ai/api/anthropic`

Using the wrong endpoint base for an OpenAI-formatted request results in silent format mismatch errors.

**No documented programmatic balance API.** Credits must be monitored via the web console at `open.bigmodel.cn`. Detect exhaustion from error response bodies.

**Tool calling:** GLM 4.5+ has strong tool-call reliability (90.6% success rate cited). Earlier GLM-4 variants had known tool-calling failures.

---

### Q5: Provider content filtering adds unexpected refusals

Chinese providers apply domestic content filters that may refuse prompts that OpenAI/Anthropic would accept. This appears as a 200 response with `finish_reason: content_filter` or a 400 with a policy message, not as a 5xx.

**Prevention:**
- Treat `finish_reason: content_filter` as a non-retryable error (retrying on a different Chinese provider will likely trigger the same filter).
- Log the refusal with enough context to distinguish from other errors.
- Do not expose the raw filter message to the end user; return a sanitized message.

---

### Q6: Thinking/reasoning tokens inflate input token counts in multi-turn

When reasoning mode is active, the model's internal reasoning tokens (returned in `reasoning_content` or `<think>` blocks) count against the context window on the next turn if included in the conversation history. A session that starts with a 2000-token context can hit the context limit within 3-4 turns if reasoning tokens are naively included.

**Prevention:**
- Strip `reasoning_content` / `<think>` content before computing effective context length.
- Warn or refuse requests where projected context (including pending reasoning) exceeds 90% of the model's limit.

---

## Deployment Pitfalls

### D1: Upstream API credentials visible in process environment and logs

**What goes wrong:** Upstream provider API keys stored in Docker environment variables (`-e MINIMAX_KEY=xxx`) are visible in `docker inspect`, `ps aux`, and any log line that dumps env vars. In a shared host or CI environment this is a credential leak.

**Prevention:**
- Use Docker secrets (`/run/secrets/`) or mount a config file that is not part of the image.
- Never log environment variables. Log only the first 8 chars of any key for identification.
- Do not put provider keys in `docker-compose.yml` directly; use a `.env` file and add it to `.gitignore`.

**Phase:** Deployment setup (Phase 3).

---

### D2: Container state loss on restart loses health and usage state

**What goes wrong:** Round-robin index, upstream health state, and accumulated token usage are stored in memory. A container restart (OOM kill, deploy update) resets all state. Upstreams that were dead are treated as healthy; token budgets are reset; per-key usage counters vanish.

**Prevention:**
- Persist health state and usage counters to SQLite or a small Postgres instance mounted as a Docker volume.
- On startup, reload persisted health state. Apply a brief re-validation probe before putting a previously-dead upstream back in rotation.
- Separate data volume from application container so restarts don't lose data.

**Phase:** Phase 2 (when usage tracking is added). Phase 1 can use in-memory state with a documented caveat.

---

### D3: Outbound proxy configuration for upstream API calls

**What goes wrong:** ocp runs in China. Some upstream providers may require outbound HTTP requests to go through a SOCKS5 proxy (GFW context). If the upstream HTTP client does not pick up `HTTP_PROXY` / `HTTPS_PROXY` environment variables, requests silently time out.

**Warning signs:**
- API calls to a provider work in the host shell but fail inside the Docker container
- Timeout errors, not connection-refused

**Prevention:**
- Explicitly configure the outbound proxy in the upstream HTTP client per provider (not globally — some providers like domestic DashScope don't need a proxy).
- In Go: use `golang.org/x/net/proxy` for SOCKS5; do not rely on env var pass-through for SOCKS5 (env var support is HTTP/HTTPS only in standard library).
- Test each provider endpoint connectivity from inside the container during startup and log the result.

**Phase:** Phase 1 (proxy core). Must work before any provider integration test.

---

### D4: Missing keep-alive between proxy and upstream causes reconnection overhead

**What goes wrong:** Each API request opens a new TCP+TLS connection to the upstream provider. For high-frequency usage this adds 200-500ms per request for the TLS handshake and is a significant overhead, especially to providers outside China where latency is already high.

**Prevention:**
- Use a persistent connection pool (HTTP keep-alive) for upstream connections.
- In Go: configure `http.Transport` with `MaxIdleConns`, `IdleConnTimeout`, and `DisableKeepAlives: false`.
- Set `proxy_http_version 1.1` and `proxy_set_header Connection ''` in Nginx (the default HTTP/1.0 closes connections after every request).

**Phase:** Phase 1. Performance concern that becomes painful at moderate usage.

---

### D5: Self-signed TLS certificate breaks Claude Code's certificate validation

**What goes wrong:** ocp is deployed with a self-signed certificate (or no TLS) behind a local reverse proxy. Claude Code validates TLS certificates and refuses connections to endpoints with self-signed certs unless the cert is added to the system trust store or `NODE_EXTRA_CA_CERTS` is set.

**Prevention:**
- Use Let's Encrypt (Caddy auto-provisioning is simplest) if the ocp host is reachable from the internet.
- For LAN-only: generate a local CA, add it to the system trust store on all client machines, and issue a cert from that CA.
- Document the trust store setup step explicitly in the README.
- Alternatively, configure Claude Code with `ANTHROPIC_API_URL=http://...` if running entirely on localhost (no TLS needed for loopback).

**Phase:** Phase 3 (deployment). Block on client integration if missed.

---

## Phase Mapping

| Phase | Component | Pitfall to Address |
|-------|-----------|-------------------|
| Phase 1: Proxy core | SSE streaming | P1 (Nginx buffering), M2 (flush), M3 (timeout) |
| Phase 1: Proxy core | Failover logic | P3 (race condition), M6 (retry non-retryable) |
| Phase 1: Proxy core | Format translation | P4 (tool_use ordering) |
| Phase 1: Proxy core | Outbound networking | D3 (SOCKS5 proxy for GFW), D4 (keep-alive) |
| Phase 2: Provider adapters | MiniMax adapter | P5 (empty role), Q2 (reasoning_details) |
| Phase 2: Provider adapters | Kimi adapter | P2 (credits vs rate limit), Q1 (reasoning_content history) |
| Phase 2: Provider adapters | Qwen adapter | P2 (quota error codes), Q3 (model ID versioning) |
| Phase 2: Provider adapters | GLM adapter | P2 (error classification), Q4 (endpoint selection) |
| Phase 2: Provider adapters | All providers | Q5 (content filter), Q6 (reasoning token inflation) |
| Phase 2: Health tracking | Poller | M5 (aggressive marking), P2 (error classification) |
| Phase 2: Usage tracking | Token counters | M1 (token count divergence) |
| Phase 2: Key management | Key issuance | M4 (key entropy) |
| Phase 3: Deployment | Docker config | D1 (credential exposure), D2 (state loss) |
| Phase 3: Deployment | TLS / networking | D5 (self-signed cert), D3 (outbound proxy) |
| Phase 4: Claude Code integration | Slash commands | Context budget limits (skill description truncation); verify ANTHROPIC_BASE_URL is read by the installed Claude Code version before shipping the slash command |

---

## Sources

- Nginx SSE buffering: [oneuptime.com blog](https://oneuptime.com/blog/post/2025-12-16-server-sent-events-nginx/view), [objectgraph.com](https://objectgraph.com/blog/optimizing-sse-nginx-streaming/)
- OpenAI error codes: [platform.openai.com/docs/guides/error-codes](https://developers.openai.com/api/docs/guides/error-codes), [OpenAI community](https://community.openai.com/t/encountering-ratelimiterror-despite-having-available-credits-and-rpm/616122)
- Kimi error codes and balance API: [platform.moonshot.ai/docs/guide/faq](https://platform.moonshot.ai/docs/guide/faq), [platform.moonshot.cn/docs/api/balance](https://platform.moonshot.cn/docs/api/balance)
- MiniMax streaming bug: [MiniMax-AI/MiniMax-M2.5 issue #2](https://github.com/MiniMax-AI/MiniMax-M2.5/issues/2), [MiniMax error codes](https://platform.minimax.io/docs/api-reference/errorcode)
- Kimi/Moonshot rate limit masking insufficient funds: [openclaw issue #43447](https://github.com/openclaw/openclaw/issues/43447), [moonshot .cn baseUrl silent 401](https://github.com/openclaw/openclaw/issues/6222)
- Anthropic tool_use ordering in streaming: [openai/openai-agents-python issue #1863](https://github.com/openai/openai-agents-python/issues/1863)
- Format translation (LiteLLM): [docs.litellm.ai/docs/providers/anthropic](https://docs.litellm.ai/docs/providers/anthropic)
- Chinese provider API quirks: [cc-compatible-models](https://github.com/Alorse/cc-compatible-models), [maniac.ai comparison](https://www.maniac.ai/blog/chinese-frontier-models-compared-glm5-minimax-kimi-qwen)
- Kimi reasoning_content 400 bug: [agentscope-ai/QwenPaw issue #388](https://github.com/agentscope-ai/QwenPaw/issues/388)
- Qwen DashScope endpoints: [alibabacloud.com docs](https://www.alibabacloud.com/help/en/model-studio/compatibility-of-openai-with-dashscope)
- SSE premature close / Go/Node: [dev.to SSE agents article](https://dev.to/abhishek_chatterjee_33b9d/why-sse-for-ai-agents-keeps-breaking-at-2am-55ie)
- Docker deployment pitfalls: [liteLLM production docs](https://docs.litellm.ai/docs/proxy/prod), [Docker proxy guide](https://www.datacamp.com/tutorial/docker-proxy)
- JWT/key security: [pinusx.com JWT security 2026](https://tools.pinusx.com/blog/jwt-security-best-practices-2026), [levo.ai API security](https://www.levo.ai/resources/blogs/rest-api-security-best-practices)
- LiteLLM supply chain incident: [trendmicro.com](https://www.trendmicro.com/en_us/research/26/c/inside-litellm-supply-chain-compromise.html)
- Claude Code slash commands: [code.claude.com/docs/en/slash-commands](https://code.claude.com/docs/en/slash-commands), [alexop.dev guide](https://alexop.dev/posts/claude-code-customization-guide-claudemd-skills-subagents/)
- MiniMax reasoning_details streaming bug: [litellm issue #22392](https://github.com/BerriAI/litellm/issues/22392)
- Round-robin concurrency: [dev.to Go load balancer](https://dev.to/vivekalhat/building-a-simple-load-balancer-in-go-70d)
