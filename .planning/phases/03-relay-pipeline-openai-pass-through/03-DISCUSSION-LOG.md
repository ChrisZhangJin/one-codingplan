# Phase 3: Relay Pipeline (OpenAI Pass-Through) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-16
**Phase:** 03-relay-pipeline-openai-pass-through
**Areas discussed:** Auth token lookup strategy, Usage log on failure, Mid-stream failure handling, Failover retry ceiling

---

## Auth Token Lookup Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| DB lookup every request | Query SQLite on every request. Simple, always current. | ✓ |
| In-memory cache | Load tokens at startup, refresh periodically. Faster but stale window. | |
| You decide | Leave to planner | |

**User's choice:** DB lookup every request
**Notes:** None — default was clear.

| Option | Description | Selected |
|--------|-------------|----------|
| Token exists + Enabled=true only | Phase 3 scope only: is token real and not blocked? | ✓ |
| Token + Enabled + basic rate limit | Add req/min enforcement now. Requires schema changes. | |
| You decide | Leave to planner | |

**User's choice:** Token exists + Enabled=true only
**Notes:** Rate limiting deferred to Phase 5.

---

## Usage Log on Failure

| Option | Description | Selected |
|--------|-------------|----------|
| Always log, zeros for token counts | Write usage_record for every authenticated request. | ✓ |
| Log only on success | Skip usage_record on failure. | |
| You decide | Leave to planner | |

**User's choice:** Always log, zeros for token counts

| Option | Description | Selected |
|--------|-------------|----------|
| Async fire-and-forget | Goroutine after response. Low latency impact. | ✓ |
| Synchronous before response | Guaranteed record, adds SQLite write to critical path. | |
| You decide | Leave to planner | |

**User's choice:** Async fire-and-forget

---

## Mid-Stream Failure Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Close connection, no retry | Truncated stream. Retrying would corrupt SSE framing. | ✓ |
| Inject error SSE event, then close | Send error event before closing. | |
| You decide | Leave to planner | |

**User's choice:** Close connection, no retry

| Option | Description | Selected |
|--------|-------------|----------|
| Check request body stream:true | OpenAI spec field. Works regardless of Accept header. | ✓ |
| Check Accept: text/event-stream | Less reliable — many clients don't set this. | |
| Both checks | More compatible but complicates detection. | |

**User's choice:** Check request body stream:true field

---

## Failover Retry Ceiling

| Option | Description | Selected |
|--------|-------------|----------|
| Try all available upstreams once | One pass through pool. Predictable. | ✓ |
| Cap at 3 retries | Hard cap regardless of pool size. | |
| Configurable retry limit | relay.max_retries in config. | |

**User's choice:** Try all available upstreams once

| Option | Description | Selected |
|--------|-------------|----------|
| 503 + OpenAI error JSON | Matches OpenAI's own behavior. Claude Code retries 503. | ✓ |
| 502 + OpenAI error JSON | Less semantically accurate. | |
| You decide | Leave to planner | |

**User's choice:** 503 + OpenAI error JSON
**Notes:** Researched before deciding — user asked to look up what OpenAI clients actually expect. OpenAI returns 503 for "no healthy upstream"; Claude Code auto-retries 503 with exponential backoff.

---

## Claude's Discretion

- Request forwarding approach (httputil.ReverseProxy vs manual)
- Per-upstream HTTP client timeout value
- Gin middleware vs handler structure
- Request body buffering strategy for retry

## Deferred Ideas

None.
