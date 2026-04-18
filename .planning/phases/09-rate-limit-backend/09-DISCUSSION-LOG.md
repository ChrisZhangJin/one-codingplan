# Phase 9: Rate Limit Backend - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-18
**Phase:** 09-rate-limit-backend
**Areas discussed:** Error response format

---

## Pre-Discussion Finding

Codebase scout revealed Phase 9 backend is largely already implemented:
- `RateLimitPerMinute` + `RateLimitPerDay` fields exist on AccessKey model
- `limitMiddleware` already enforces both limits with 429
- Admin API create/update handlers accept and persist both fields
- CLI `ocp keys` displays RPM and RPD columns

Remaining gaps: error format (flat vs OpenAI), missing per-day test, e2e field name bug.

---

## Error Response Format

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — add Retry-After header | Standard HTTP practice, helps clients back off | |
| No — keep it simple | No header, current behavior | ✓ |

**User's choice:** No Retry-After header.

---

| Option | Description | Selected |
|--------|-------------|----------|
| OpenAI format | `{"error": {"message": "...", "type": "requests", "code": "rate_limit_exceeded"}}` | ✓ |
| Current flat format | `{"error": "per-minute rate limit exceeded"}` | |

**User's choice:** OpenAI format for the 429 error body.

---

## Claude's Discretion

- Exact wording of error messages inside the nested format
- Whether to export `ResetPerDayCounters` as separate function or combined reset

## Deferred Ideas

None.
