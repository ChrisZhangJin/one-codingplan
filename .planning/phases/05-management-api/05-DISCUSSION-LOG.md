# Phase 5: Management API - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-16
**Phase:** 05-management-api
**Areas discussed:** AllowedUpstreams storage, Rate limit counters, Usage totals in GET /api/keys, Key creation request/response shape

---

## AllowedUpstreams storage

| Option | Description | Selected |
|--------|-------------|----------|
| JSON string column | Store as JSON array in TEXT column; empty = unrestricted | ✓ |
| Comma-separated names | Lightest option — just a string; slightly awkward queries | |
| Separate join table | Most normalized; overkill at personal scale | |

**User's choice:** JSON string column
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Pool.Select filters at request time | authMiddleware sets keyID; Pool.Select reads allowed list from DB | ✓ |
| Middleware pre-filter | Reads allowed list in middleware, passes as parameter | |
| You decide | Implementation detail | |

**User's choice:** Pool.Select filters at request time
**Notes:** Aligns with D-17 design from Phase 2

---

## Rate limit counters

| Option | Description | Selected |
|--------|-------------|----------|
| In-memory map | sync.Map, reset on restart; zero DB writes per request | ✓ |
| DB row updated per request | Persistent; conflicts with async-fire-and-forget logging design | |
| Hybrid: in-memory + periodic flush | Complex; overkill for personal proxy | |

**User's choice:** In-memory map
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed minute boundary | Count resets at :00; track count + minute-of-hour | ✓ |
| Sliding window (last 60s) | More accurate; ring buffer per key | |

**User's choice:** Fixed minute boundary
**Notes:** Same fixed-boundary model applies to per-day limit (resets at UTC midnight)

---

## Usage totals in GET /api/keys

| Option | Description | Selected |
|--------|-------------|----------|
| Aggregate from usage_records on request | SELECT SUM(...) per key; accurate; key_id index makes it fast | ✓ |
| Running totals on AccessKey | Fast reads; requires atomic update in async goroutine | |

**User's choice:** Aggregate from usage_records
**Notes:** key_id index already present from Phase 3

| Option | Description | Selected |
|--------|-------------|----------|
| All keys, no pagination | Flat array; personal proxy has dozens of keys | ✓ |
| Pagination (limit/offset) | Conventional but overkill | |

**User's choice:** All keys, no pagination
**Notes:** —

---

## Key creation request/response shape

| Option | Description | Selected |
|--------|-------------|----------|
| All limits upfront | POST /api/keys accepts name + all optional limits in one body | ✓ |
| Name only, update separately | POST takes name; PATCH sets limits | |
| You decide | Leave schema to planner | |

**User's choice:** All limits upfront
**Notes:** Minimizes round trips for key provisioning

| Option | Description | Selected |
|--------|-------------|----------|
| Return token only at creation | POST response has full token; GET/list returns masked | ✓ |
| Always return full token | GET /api/keys returns plaintext token for all keys | |

**User's choice:** Return token only at creation
**Notes:** Standard pattern; user copies token once at creation time

| Option | Description | Selected |
|--------|-------------|----------|
| PATCH /api/keys/:id for updates | Partial update; enables portal edit flow | ✓ |
| Create-and-delete only | No update endpoint; simpler API surface | |

**User's choice:** PATCH /api/keys/:id
**Notes:** Needed for portal Phase 6 to have edit-without-recreate UX

---

## Claude's Discretion

- Admin middleware vs. relay middleware implementation details
- AllowedUpstreams JSON serialization approach (GORM serializer vs. manual)
- Token masking algorithm specifics
- Whether rate limit and budget checks are a separate middleware or extend authMiddleware

## Deferred Ideas

None.
