# Phase 2: Upstream Pool & Health Monitor - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-16
**Phase:** 02-upstream-pool-health-monitor
**Areas discussed:** State machine topology, Rate-limit handling, Re-test probe strategy, Error classifier architecture

---

## State machine topology

The user described the intended model directly (not via AskUserQuestion options):
> "if it can be calling normally, it stays available. if api returns 'no fee, no credit, no more token' or anything regarding out of token, we take it unavailable. we should have a background routine to check if the unavailable upstream is still alive or not per hours by sending 'hi' to its api. if the api returns normal response, turn it back to available."

**User's choice:** Two states — available / unavailable. Credits-exhausted = unavailable. Hourly background probe recovers it.
**Notes:** User explicitly rejected the three-state (healthy/cooling/dead) model in favor of simplicity. No per-provider balance API polling for now.

---

## Rate-limit handling

| Option | Description | Selected |
|--------|-------------|----------|
| Rotate to next upstream | Treat rate-limit same as transient — just pick next upstream | |
| Backoff and retry same upstream | Wait and retry the same upstream before rotating | ✓ |
| Rotate + short cooldown on rate-limited upstream | Rotate away and suppress the rate-limited upstream briefly | |

**User's choice:** Backoff and retry same upstream
**Notes:** Keeps upstream "loyalty" — if the same upstream is rate-limited, wait for it rather than wasting a different upstream's quota.

## Rate-limit backoff duration

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed 5 seconds | Simple, predictable | |
| Read Retry-After header, else 5s | Honors provider guidance when available | |
| Configurable in config.yaml, default 5s | Operator-tunable | ✓ |

**User's choice:** Configurable in config.yaml, default 5s

---

## Re-test probe strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal chat completion: "hi" with max_tokens=1 | Real API call, works universally | ✓ |
| Provider-specific balance API where available, fallback | More informative, requires per-provider code | |
| No active probe — wait for real traffic | Zero extra calls, but slower recovery | |

**User's choice:** Minimal chat completion ("hi", max_tokens=1) every hour

---

## Error classifier architecture

| Option | Description | Selected |
|--------|-------------|----------|
| Provider-keyed map of pattern matchers | map[string]Classifier, one entry per provider | ✓ |
| Per-provider struct implementing interface | KimiClassifier, QwenClassifier, etc. | |
| Generic classifier based on common patterns | Single classifier, per-provider overrides only when needed | |

**User's choice:** Provider-keyed map of pattern matchers

## Error pattern matching

| Option | Description | Selected |
|--------|-------------|----------|
| HTTP status + response body substring match | Check status code then scan body for keywords | ✓ |
| HTTP status code only | Simpler, misses providers that use 400 for out-of-credits | |
| Full JSON parse against provider-specific schema | Most precise, most brittle | |

**User's choice:** HTTP status code + response body substring match

---

## Claude's Discretion

- Exact struct/interface names for Classifier and Pool
- Round-robin index granularity (global vs per-key)
- Concurrency primitives for pool state
- Pool injection into Server
- Model/prompt used for probe calls per provider

## Deferred Ideas

- Kimi balance API polling (v2 HLTH-01)
- Per-upstream configurable probe interval
- Admin manual re-enable of unavailable upstreams (Phase 5)
