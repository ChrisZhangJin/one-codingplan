# Features Research — one-codingplan (ocp)

**Domain:** Self-hosted AI API proxy / gateway (Chinese provider aggregation)
**Researched:** 2026-04-16
**Overall confidence:** HIGH — core findings verified against official docs and multiple established tools

---

## Table Stakes

Features that must be present or the tool is effectively unusable.

| Feature | Why Required | Complexity | Notes |
|---------|--------------|------------|-------|
| OpenAI-compatible `/v1/chat/completions` endpoint | Claude Code, Codex, and every OpenAI SDK client expect this exact path | Low | Stateless HTTP handler; trivial with any Go/Node HTTP framework |
| Anthropic-compatible `/v1/messages` endpoint | Claude Code's native auth path sends to this endpoint; some users prefer native format | Low-Med | Requires extracting top-level `system` field; see Format Translation section |
| Bearer token auth on incoming requests | Without this, any process on the same machine can consume all upstream credits | Low | Standard middleware; `Authorization: Bearer <ocp-key>` |
| Transparent upstream forwarding | Proxy must correctly forward all fields the upstream provider requires | Med | See Format Translation section for the one hard part |
| Upstream key rotation / round-robin | Core value prop — silent failover when one plan exhausts | Med | Stateful: needs list of upstreams + current index |
| 402/429/5xx error detection for failover trigger | Must detect credit exhaustion and rate limits to know when to rotate | Low | Minimax: `406 Not enough credits`; Kimi: `429`; Qwen: separate BSS billing API |
| SSE streaming pass-through | All major coding tools use `stream: true`; blocking responses have unacceptable UX | Med | Must not buffer full response before forwarding; chunked transfer or SSE relay |
| Access key issuance | Users must have a way to generate ocp keys to give to tools | Low | CRUD, UUIDs, bcrypt or sha256 hash in DB |
| Key block/revoke | Without revoke, any leak requires full server restart | Low | Flip an `active` boolean in the key table |
| Persistent config storage | Upstream credentials, keys, and settings must survive restarts | Low | SQLite is sufficient for single-instance; no Postgres needed for personal use |
| Health/status endpoint | `GET /health` returning upstream states; Claude Code skill uses this | Low | Must return JSON; used by `/proxy-status` skill |

---

## Differentiators

Features that make ocp meaningfully better than manually editing config files.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Per-key allowed upstream set | Restrict a key to only use specific plans; natural model for team access | Low-Med | Many-to-many key↔upstream join table |
| Per-key token budget (lifetime or rolling) | Stop a key from exhausting all upstream credits | Med | Requires tracking tokens per key; infer from response `usage` field |
| Per-key rate limits (RPM / RPD) | Prevent a runaway agent from hammering all upstream quota | Med | In-memory or Redis sliding window; in-memory fine for single instance |
| Web dashboard — upstream health view | Show which plans are active, how much credit remains, last error | Med | Polling loop + simple React/HTMX page |
| Web dashboard — key management | Create / block / view usage per key without touching config files | Low-Med | Basic CRUD UI |
| `ocp status` CLI command | Terminal-friendly upstream health summary for terminal-first users | Low | `curl` wrapper or compiled binary; calls `/api/v1/status` |
| `ocp next` CLI command | Manually advance to next upstream, e.g. mid-session fix | Low | `curl` wrapper; calls `POST /api/v1/rotate` |
| `/proxy-status` Claude Code skill | Shows upstream health inside active Claude Code session | Low | Shell skill wrapping `curl`; see CLI Integration section |
| `/proxy-next` Claude Code skill | Triggers rotation from within Claude Code session | Low | Shell skill wrapping `curl` |
| Balance polling for providers that support it | Proactively detect near-empty plans before they fail | Med | Kimi has a documented endpoint; others require error-inference |
| Model aliasing | Let keys see `claude-3-5-sonnet` and route it to the cheapest available plan | Med | Routing table: alias → [upstream models]; useful when mixing Kimi and GLM |
| Request/response logging (recent N entries) | Debug why a request failed or which upstream served it | Med | Ring buffer in SQLite; no full log infra needed for personal use |

---

## Anti-features (defer or skip)

Features to deliberately not build for v1. Each has a reason.

| Anti-Feature | Why Skip | What to Do Instead |
|--------------|----------|--------------------|
| Multi-tenant billing / reselling | Explicitly out of scope per PROJECT.md; adds auth, invoicing, payments complexity | Never — not the product |
| Semantic caching | 20-40% cost saving but: cache invalidation is hard, stale responses in coding context are dangerous | Consider v2 if demand |
| Prompt filtering / content moderation | Not a firewall product; adds latency and false-positive risk | Rely on upstreams' own moderation |
| Per-session upstream pinning | PROJECT.md explicitly excludes this; force-next covers 95% of needs | Keep force-next only |
| Redis / Postgres | Overkill for single-instance personal/team use; adds ops burden | SQLite until there's a reason |
| SSO / SAML / OIDC | No enterprise auth needed; simple bearer tokens are sufficient | Add only if sharing across an org |
| Response format transformation (non-compat) | All upstreams expose OpenAI-compatible or Anthropic-compatible formats already per PROJECT.md | No translation needed beyond system-prompt extraction |
| Webhook cost alerts / Slack notifications | Nice-to-have; adds integration surface area | Expose a status API; let the user wire their own alerting |
| Agentic loop detection | Useful but complex to implement correctly; premature for v1 | Monitor token burn rate instead |
| Gemini format support | No Gemini upstream planned | Skip entirely |
| Plugin / extension system | Adds architectural complexity before core works | Config-driven extensibility is enough |

---

## Upstream Provider Balance APIs

Research findings on each of the five named upstream providers.

### Kimi (Moonshot AI) — CONFIRMED, HIGH confidence
Source: https://platform.moonshot.ai/docs/api/balance (redirects to platform.kimi.ai)

```
GET https://api.moonshot.ai/v1/users/me/balance
Authorization: Bearer {MOONSHOT_API_KEY}

Response:
{
  "code": 0,
  "data": {
    "available_balance": 12.50,   // cash + voucher; <=0 means blocked
    "voucher_balance": 5.00,
    "cash_balance": 7.50
  },
  "status": true
}
```

Error codes to treat as credit-exhausted: `available_balance <= 0`, HTTP `429`.

### Minimax — NO public balance endpoint, MEDIUM confidence
Source: platform.minimax.io/docs/api-reference/api-overview + community forum

No documented `GET /balance` or equivalent. Balance is web-dashboard-only.
Error to detect credit exhaustion: HTTP `406 Not Enough Credits` (documented).

**Polling strategy:** Use error-inference only. On `406`, mark upstream unhealthy.

### GLM (Zhipu AI / open.bigmodel.cn) — NO public balance endpoint, MEDIUM confidence
Source: open.bigmodel.cn/dev/api + MetaGLM GitHub SDKs

No balance REST endpoint in official SDKs (Python, Java, Node). Account balance is
web-only at open.bigmodel.cn/usercenter.

**Polling strategy:** Error-inference. On HTTP `402` or quota-exceeded error codes,
mark upstream unhealthy.

### Qwen (Alibaba DashScope) — NO public balance endpoint, LOW-MEDIUM confidence
Source: alibabacloud.com/help/en/model-studio + DashScope PyPI docs

Balance management is through Alibaba Cloud BSS API (separate product, separate auth).
Not practical to integrate directly. DashScope does not expose `/balance` on the model
inference base URL.

OpenAI-compatible base URLs:
- China: `https://dashscope.aliyuncs.com/compatible-mode/v1`
- International: `https://dashscope-intl.aliyuncs.com/compatible-mode/v1`

Keys are region-specific — China and international keys are not interchangeable.

**Polling strategy:** Error-inference only. Detect standard OpenAI-format quota errors.

### iFlytek Spark (讯飞星火) — OpenAI-compatible, no balance endpoint known, LOW confidence
Source: xfyun.cn/doc/spark/HTTP + trufflesecurity/trufflehog issue

OpenAI-compatible base URL: `https://spark-api-open.xf-yun.com/v1`
Auth: `Authorization: Bearer {APIPassword}` (obtained from iFlytek console)
No balance endpoint documented.

**Polling strategy:** Error-inference only.

### "Xiao" Provider — UNCLEAR, LOW confidence
The PROJECT.md lists "Xiao" as a named provider but no publicly documented AI coding
plan API matching this name was found. Candidates:
- Xiaomi's AI service?
- A specific reseller plan not publicly indexed?

**Recommendation:** Treat "Xiao" as a config-only upstream (base URL + key fields in
config) with no balance polling until the provider is identified. The extensible config
model handles this gracefully — no code change needed.

### Balance Polling Strategy Summary

| Provider | Balance API | Polling Strategy |
|----------|-------------|-----------------|
| Kimi | `GET /v1/users/me/balance` (documented) | Poll every N minutes; mark unhealthy if `available_balance <= 0` |
| Minimax | None | Error-inference on HTTP 406 |
| GLM | None | Error-inference on 402/quota error |
| Qwen | None (BSS is separate) | Error-inference on quota error |
| Spark | None known | Error-inference |
| Xiao | Unknown | Error-inference until identified |

---

## Anthropic ↔ OpenAI Format Translation

ocp must accept both OpenAI and Anthropic format requests and forward them to whichever
upstream is active (all of which expose OpenAI-compatible APIs per PROJECT.md).

### The one hard case: system prompt extraction

OpenAI format places system prompt inside `messages` array:
```json
{ "messages": [{"role": "system", "content": "You are..."}, {"role": "user", ...}] }
```

Anthropic native format places it at top level:
```json
{ "system": "You are...", "messages": [{"role": "user", ...}] }
```

When ocp receives an **Anthropic-format** request on `/v1/messages` and needs to forward
to an **OpenAI-compatible** upstream:
1. Extract `request.system` → inject as `{"role": "system", "content": ...}` at index 0
   of `messages`
2. Map Anthropic `max_tokens` (required) → OpenAI `max_tokens` (optional but pass through)
3. Map Anthropic `stop_sequences` → OpenAI `stop`
4. Drop Anthropic-only fields: `metadata`, `top_k` (no OpenAI equivalent)
5. Response: wrap OpenAI `choices[0].message.content` into Anthropic `content[0].text`
   block format

When ocp receives an **OpenAI-format** request on `/v1/chat/completions`:
- Forward as-is (all upstreams are OpenAI-compatible)
- No translation needed unless upstream enforces strict alternating turns

### Strict alternating turns (Anthropic upstream risk — not applicable here)

All ocp upstreams use OpenAI-compatible format, so strict alternating-turn enforcement
(an Anthropic-specific constraint) does not apply. No mitigation needed.

### Streaming

Both formats support SSE streaming. Anthropic streaming uses `event: content_block_delta`
events; OpenAI uses `data: {"choices": [{"delta": ...}]}`. When translating
Anthropic-in → OpenAI-upstream, translate the response stream from OpenAI SSE format
back to Anthropic SSE format if the client connected to `/v1/messages`. This requires
a streaming transformer, not just field remapping.

**Recommendation:** Implement translation as a thin middleware layer inserted between
the client handler and the upstream forwarder. Keep translation logic isolated — it is
the most likely source of bugs.

---

## Claude Code / Codex Integration Patterns

### Claude Code Skills (current recommended pattern, April 2026)

Skills supersede legacy `/commands` but both still work. Skills use a `SKILL.md` file
with YAML frontmatter. For ocp, simple slash commands (user-invoked) are the right
choice — not auto-invoked agent skills.

Recommended structure for ocp:
```
.claude/commands/
  proxy-status.md    # /proxy-status
  proxy-next.md      # /proxy-next
```

Each file contains a shell invocation using the `!command` syntax that substitutes
output before Claude sees it:

**proxy-status.md:**
```markdown
Show the current ocp proxy status.

!curl -s -H "Authorization: Bearer $OCP_ADMIN_KEY" http://localhost:8080/api/v1/status
```

**proxy-next.md:**
```markdown
Rotate ocp to the next upstream provider.

!curl -s -X POST -H "Authorization: Bearer $OCP_ADMIN_KEY" http://localhost:8080/api/v1/rotate
```

Key patterns from research:
- `!command` executes before Claude reads the skill; output is substituted inline
- `$ARGUMENTS` expands to full argument string (e.g. `/proxy-next kimi` passes `kimi`)
- `$OCP_ADMIN_KEY` env var must be set in the shell where Claude Code runs
- Skills load at session start; changes require new session
- Command name = filename without extension → `/proxy-status`

### Codex Hooks

Codex supports similar hook patterns through its config. The same `curl` calls work.
ocp should expose a simple JSON REST API for management actions (not just the proxy
passthrough) so both tools can integrate without special logic.

### Access from tools

The management API (`/api/v1/*`) should be separate from the proxy endpoint
(`/v1/chat/completions`, `/v1/messages`) to avoid auth confusion:
- Proxy endpoints: authenticated with ocp access keys (user-issued)
- Management endpoints: authenticated with admin key (from env/config, not DB)

This separation means the skill can use a stable admin key in env, while users get
their own scoped keys for API access.

---

## Complexity Notes

### What is simpler than it looks

- **Upstream round-robin**: A mutex-protected index into a slice. Not complex.
- **SQLite persistence**: Embedded, no server, Go has `database/sql` + `modernc.org/sqlite`
  or `mattn/go-sqlite3`. Migrations with a plain `CREATE TABLE IF NOT EXISTS` are fine
  for personal use.
- **Bearer auth middleware**: 10 lines; hash incoming token, compare to DB.
- **Claude Code skill integration**: Two markdown files. Done.

### What is harder than it looks

- **SSE streaming translation** (Anthropic ↔ OpenAI format): Event-by-event transformation
  while maintaining backpressure and connection lifecycle. Easiest approach: use `io.Pipe`
  or a channel to relay chunks; do field mapping per chunk. Test with both streaming
  and non-streaming clients.
- **Reliable credit exhaustion detection**: Each provider uses different error codes
  and messages. Needs a per-provider error classifier, not a generic HTTP status check.
  Kimi returns `429`; Minimax returns `406`; GLM and Qwen use OpenAI-format error
  objects with provider-specific `code` fields.
- **Balance polling for Kimi only**: Adding a background goroutine that polls
  `GET /v1/users/me/balance` every 5 minutes requires careful lifecycle management
  (startup, shutdown, restart-on-panic). Worth the complexity — it enables proactive
  failover rather than reactive.
- **Web dashboard**: The backend is simple, but building a usable UI takes real time.
  Use HTMX + Tailwind for minimal JS; avoids a full SPA build pipeline.

### What should not be built in v1

- Distributed state (Redis, Postgres): Single instance is the target deployment.
- Plugin system: The upstream interface is already the extension point.
- Full request/response body logging: Privacy risk; ring buffer of metadata (token
  counts, latency, upstream used, status) is sufficient.

---

## Sources

- Kimi/Moonshot balance API: https://platform.moonshot.ai/docs/api/balance
- Minimax API reference: https://platform.minimax.io/docs/api-reference/api-overview
- Zhipu AI SDK: https://github.com/MetaGLM/zhipuai-sdk-python-v4
- Qwen/DashScope: https://www.alibabacloud.com/help/en/model-studio/compatibility-of-openai-with-dashscope
- iFlytek Spark: https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html
- one-api: https://github.com/songquanpeng/one-api
- new-api: https://github.com/QuantumNous/new-api
- liteLLM proxy: https://docs.litellm.ai/docs/simple_proxy
- liteLLM load balancing: https://docs.litellm.ai/docs/proxy/load_balancing
- liteLLM budgets/rate limits: https://docs.litellm.ai/docs/proxy/users
- Anthropic vs OpenAI format: https://portkey.ai/blog/open-ai-responses-api-vs-chat-completions-vs-anthropic-anthropic-messages-api/
- Claude Code skills: https://code.claude.com/docs/en/skills
- Claude Code skills tutorial: https://supalaunch.com/blog/claude-code-skills-tutorial-custom-slash-commands-and-automations-guide
- Per-key budget patterns: https://portkey.ai/docs/product/administration/enforce-budget-and-rate-limit
