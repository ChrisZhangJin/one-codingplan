# Phase 10: Rate Limit Portal - Research

**Researched:** 2026-04-18
**Domain:** React/TypeScript table and dialog UI, in-memory counter read-back, shadcn/ui component patterns
**Confidence:** HIGH

## Summary

Phase 10 is a pure frontend change with one targeted backend addition. The API already returns `rate_limit_per_minute` and `rate_limit_per_day` on every key response — both fields are present in the `keyResponse` struct and wired through `toKeyResponse()`. The frontend `KeyResponse` TypeScript interface already declares them. What is missing is their visibility in `KeyTable` (the table renders neither column) and the "current-day request count" which requires a new read path from the in-memory `perDayCounters` map.

Three concrete gaps must be closed:

1. **Table columns** — `KeyTable.tsx` does not render rate limit columns. Add three columns: per-minute limit, per-day limit, and today's request count. Display `0` values as "Unlimited".
2. **Edit dialog** — `EditKeyDialog.tsx` already has `rateLimitPerMinute` and `rateLimitPerDay` state, fields, and PATCH logic. The dialog is functionally complete for RATE-06. No changes needed there.
3. **Today's request count** — The in-memory `perDayCounters` sync.Map is never exposed via any HTTP endpoint. A new backend route `GET /api/keys/:id/usage-today` (or equivalent) must read the counter and return it, or the list endpoint must be augmented to include current-day counts.

**Primary recommendation:** Add a `DayUsage int` field to `keyResponse` in `admin.go`, populate it from `perDayCounters` in `toKeyResponse()`, and render it in the `KeyTable` as a read-only column. This avoids a separate per-key request from the frontend and keeps the data model flat.

---

## Project Constraints (from CLAUDE.md)

- Prefer editing existing files over creating new ones.
- Do not add comments, docstrings, or type annotations to code not changed.
- Do not add error handling or validation for scenarios that cannot happen.
- Do not introduce abstractions or helpers for one-time use.
- Keep diffs minimal — only change what is necessary.
- Follow conventions already present in the file being edited.
- Stack: Go + Gin + SQLite/GORM on backend; React + Vite + shadcn/ui on frontend.

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RATE-05 | Rate limit fields (per-minute, per-day, current usage) are visible per key in the web portal | `rate_limit_per_minute` and `rate_limit_per_day` already in `keyResponse`. Current-day count requires exposing `perDayCounters` value via the list endpoint. `KeyTable.tsx` needs new columns. |
| RATE-06 | Admin can update rate limits on existing keys via the edit key dialog in the portal | `EditKeyDialog.tsx` already has state, fields, and PATCH body logic for both rate limit fields. The dialog is functionally complete — no code change needed for RATE-06. |
</phase_requirements>

---

## Standard Stack

### Core (all existing — no new dependencies)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `react` | 18.x | Component rendering | Project standard |
| `shadcn/ui` Table | — | `Table`, `TableHead`, `TableCell`, `TableRow` | Already imported in `KeyTable.tsx` |
| `shadcn/ui` Badge | — | Status indicators | Already used for `Active`/`Blocked` in `KeyTable.tsx` |
| `gin-gonic/gin` | v1.10.1 | HTTP route handler | Project standard |
| `sync` (stdlib) | — | `sync.Map` for `perDayCounters` | Already used in `limit.go` |

**Installation:** No new packages required.

**Version verification:** All packages already present in `go.mod` and `package.json`. [VERIFIED: codebase inspection]

---

## Architecture Patterns

### Existing Project Structure (relevant files)

```
internal/
└── server/
    ├── limit.go          # perDayCounters sync.Map, checkRate, limitMiddleware
    └── admin.go          # keyResponse struct, toKeyResponse(), handleListKeys

web/src/
└── components/
    ├── KeyTable.tsx      # Table columns, KeyResponse interface, refetchKeys
    └── EditKeyDialog.tsx # PATCH /api/keys/:id with rate limit fields (ALREADY COMPLETE)
```

### Pattern 1: Exposing in-memory counter value in keyResponse

**What:** `perDayCounters` is a package-level `sync.Map` in `limit.go`. `toKeyResponse()` in `admin.go` is in the same package (`package server`) and can read the map directly.

**How to read current count without taking a lock:**
```go
// [VERIFIED: internal/server/limit.go — rateCounter struct and perDayCounters var]
func currentDayCount(keyID string) int {
    val, ok := perDayCounters.Load(keyID)
    if !ok {
        return 0
    }
    rc := val.(*rateCounter)
    rc.mu.Lock()
    defer rc.mu.Unlock()
    // Validate that the stored window matches today; if not, count is 0
    if rc.windowID != time.Now().UTC().YearDay() {
        return 0
    }
    return rc.count
}
```

This is a read path only — it does not increment, does not create an entry if absent, and is safe to call from any goroutine. [VERIFIED: rateCounter.mu protects count and windowID; sync.Map.Load is safe concurrently]

**Where to call it:** Inside `toKeyResponse()` in `admin.go`, after computing `input, output := s.usageTotals(key.ID)`. Add `DayUsage: currentDayCount(key.ID)` to the returned struct.

**keyResponse struct addition:**
```go
// [VERIFIED: internal/server/admin.go — keyResponse struct at line 46]
type keyResponse struct {
    // ... existing fields ...
    RateLimitPerMinute int `json:"rate_limit_per_minute"`
    RateLimitPerDay    int `json:"rate_limit_per_day"`
    DayUsage           int `json:"day_usage"`   // <-- new field
    // ... rest of fields ...
}
```

### Pattern 2: Adding table columns in KeyTable.tsx

**What:** `KeyTable.tsx` currently has 7 columns: Name, Token, Status, Budget, Expires, Usage, Actions. Three new columns are needed: Rate/min, Rate/day, Today's Requests.

**Column header pattern (existing style):**
```tsx
// [VERIFIED: web/src/components/KeyTable.tsx lines 94-100]
<TableHead className="text-xs font-normal">Rate/min</TableHead>
<TableHead className="text-xs font-normal">Rate/day</TableHead>
<TableHead className="text-xs font-normal">Today</TableHead>
```

**Cell rendering with "Unlimited" for 0:**
```tsx
// [VERIFIED: existing pattern in KeyTable.tsx line 118 — token_budget uses 0 = Unlimited]
<TableCell className="text-sm">
  {key.rate_limit_per_minute === 0 ? 'Unlimited' : key.rate_limit_per_minute}
</TableCell>
<TableCell className="text-sm">
  {key.rate_limit_per_day === 0 ? 'Unlimited' : key.rate_limit_per_day}
</TableCell>
<TableCell className="text-sm">
  {key.day_usage}
</TableCell>
```

**TypeScript interface addition:**
```ts
// [VERIFIED: web/src/components/KeyTable.tsx — KeyResponse interface at line 22]
interface KeyResponse {
  // ... existing fields ...
  day_usage: number   // <-- new field
}
```

The same `KeyResponse` interface is duplicated in `EditKeyDialog.tsx`. That copy does not need `day_usage` since the edit dialog does not display current usage — but it should be added for consistency so both interfaces stay in sync if the type is ever shared.

### Pattern 3: EditKeyDialog is already complete for RATE-06

**What:** The edit dialog already has:
- `rateLimitPerMinute` state initialized from `keyData.rate_limit_per_minute`
- `rateLimitPerDay` state initialized from `keyData.rate_limit_per_day`
- Both inputs (`edit-key-rpm`, `edit-key-rpd`) rendered in the form
- PATCH body includes `rate_limit_per_minute` and `rate_limit_per_day` when changed

**Verification:** [VERIFIED: web/src/components/EditKeyDialog.tsx lines 49-50, 87-90, 155-174]

No changes to `EditKeyDialog.tsx` are required to satisfy RATE-06. The requirement is already implemented — the only gap was the table not showing the columns, which blocked users from seeing current values before editing.

### Anti-Patterns to Avoid

- **Separate API endpoint for day count:** Don't add `GET /api/keys/:id/usage-today` that requires N frontend calls for N keys. Embed the counter in the list response.
- **Reading perDayCounters without checking windowID:** The counter's windowID must match `time.Now().UTC().YearDay()`. If the server has been running across midnight, the stored count is for the previous day — return 0, not the stale count.
- **Sharing TypeScript interface between files via import:** Both `KeyTable.tsx` and `EditKeyDialog.tsx` define their own local `KeyResponse` interface. Follow the existing pattern — update both locally. Do not introduce a shared types file (CLAUDE.md: no abstractions for one-time use).

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Table with sortable columns | Custom sort logic | Existing `<Table>` from shadcn/ui | Phase only needs display, not sorting |
| Badge for "Unlimited" indicator | Custom styled span | Existing text convention (`'Unlimited'`) | Matches `token_budget` column pattern already in `KeyTable.tsx` |
| Separate day-usage polling endpoint | `GET /api/keys/:id/usage-today` per key | Single `day_usage` field in list response | Avoids N+1 frontend requests |

---

## Common Pitfalls

### Pitfall 1: windowID staleness in currentDayCount

**What goes wrong:** `perDayCounters` stores a `rateCounter` with `windowID = YearDay()` at the time the first request was made that day. If the day rolls over, the windowID is stale. Reading `rc.count` without checking `rc.windowID != time.Now().UTC().YearDay()` returns yesterday's count.

**Why it happens:** The `checkRate` function resets the count when `windowID` changes, but only when a new request arrives. Between midnight and the first request of the new day, the stored count is from the previous day.

**How to avoid:** The `currentDayCount` helper must validate `rc.windowID == time.Now().UTC().YearDay()` before returning `rc.count`. If they differ, return 0.

**Warning signs:** `day_usage` shows a non-zero count at the start of a new day before any requests have been made.

### Pitfall 2: Missing day_usage in KeyResponse TypeScript interface

**What goes wrong:** Backend returns `day_usage` in the JSON but the frontend interface does not declare it. TypeScript compiles fine (extra JSON fields are ignored), but `key.day_usage` is `undefined` at runtime instead of `0`. The column renders blank or `NaN`.

**Why it happens:** The `KeyResponse` interface in `KeyTable.tsx` is a local copy; it does not auto-update when the backend struct changes.

**How to avoid:** Add `day_usage: number` to the `KeyResponse` interface in `KeyTable.tsx` in the same edit that adds the backend field.

### Pitfall 3: Table column count mismatch causing misaligned cells

**What goes wrong:** Adding `<TableHead>` entries without matching `<TableCell>` entries (or vice versa) causes columns to shift. shadcn Table does not enforce column count alignment at compile time.

**Why it happens:** `<TableHeader>` and `<TableBody>` rows are independent JSX trees.

**How to avoid:** Add both the `<TableHead>` and `<TableCell>` entries in the same edit. Count columns before and after: currently 7 headers, 7 cells. After change: 10 headers, 10 cells.

### Pitfall 4: EditKeyDialog KeyResponse interface diverging from KeyTable

**What goes wrong:** `EditKeyDialog.tsx` and `KeyTable.tsx` each define a local `KeyResponse`. If `day_usage` is added to `KeyTable.tsx` but not `EditKeyDialog.tsx`, `keyData.day_usage` is `undefined` in the dialog — benign now, but will cause confusion in future phases.

**How to avoid:** Add `day_usage: number` to `EditKeyDialog.tsx`'s `KeyResponse` interface as well, even though the dialog does not render it.

---

## Code Examples

### currentDayCount helper (new function in limit.go)

```go
// [VERIFIED: pattern consistent with rateCounter struct at limit.go:12 and perDayCounters at limit.go:19]
func currentDayCount(keyID string) int {
    val, ok := perDayCounters.Load(keyID)
    if !ok {
        return 0
    }
    rc := val.(*rateCounter)
    rc.mu.Lock()
    defer rc.mu.Unlock()
    if rc.windowID != time.Now().UTC().YearDay() {
        return 0
    }
    return rc.count
}
```

### keyResponse struct with new field (admin.go)

```go
// [VERIFIED: internal/server/admin.go lines 46-60 — existing struct layout]
type keyResponse struct {
    ID                 string     `json:"id"`
    Name               string     `json:"name"`
    Token              string     `json:"token"`
    Enabled            bool       `json:"enabled"`
    TokenBudget        int64      `json:"token_budget"`
    AllowedUpstreams   []string   `json:"allowed_upstreams"`
    ExpiresAt          *time.Time `json:"expires_at,omitempty"`
    RateLimitPerMinute int        `json:"rate_limit_per_minute"`
    RateLimitPerDay    int        `json:"rate_limit_per_day"`
    DayUsage           int        `json:"day_usage"`
    UsageTotalInput    int64      `json:"usage_total_input"`
    UsageTotalOutput   int64      `json:"usage_total_output"`
    CreatedAt          time.Time  `json:"created_at"`
    UpdatedAt          time.Time  `json:"updated_at"`
}
```

### toKeyResponse with day_usage populated (admin.go)

```go
// [VERIFIED: internal/server/admin.go lines 93-114 — existing toKeyResponse body]
func (s *Server) toKeyResponse(key models.AccessKey, exposeToken bool) keyResponse {
    tok := key.Token
    if !exposeToken {
        tok = maskToken(key.Token)
    }
    input, output := s.usageTotals(key.ID)
    return keyResponse{
        // ... existing fields ...
        RateLimitPerMinute: key.RateLimitPerMinute,
        RateLimitPerDay:    key.RateLimitPerDay,
        DayUsage:           currentDayCount(key.ID),
        UsageTotalInput:    input,
        UsageTotalOutput:   output,
        // ...
    }
}
```

### KeyTable.tsx new columns (table header + body cell pair)

```tsx
// [VERIFIED: web/src/components/KeyTable.tsx — existing column pattern at lines 94-100, 118]
// In <TableHeader>:
<TableHead className="text-xs font-normal">Rate/min</TableHead>
<TableHead className="text-xs font-normal">Rate/day</TableHead>
<TableHead className="text-xs font-normal">Today</TableHead>

// In <TableBody> row:
<TableCell className="text-sm">
  {key.rate_limit_per_minute === 0 ? 'Unlimited' : key.rate_limit_per_minute}
</TableCell>
<TableCell className="text-sm">
  {key.rate_limit_per_day === 0 ? 'Unlimited' : key.rate_limit_per_day}
</TableCell>
<TableCell className="text-sm">{key.day_usage}</TableCell>
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| KeyTable shows no rate limit data | Table shows per-minute limit, per-day limit, and current day count | Phase 10 | Operators can see rate limit configuration at a glance |
| EditKeyDialog fields existed but were hidden behind implementation gap | Dialog already fully wired — no new code needed for RATE-06 | Phase 10 analysis | RATE-06 is zero-change; gap was only in RATE-05 table display |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Phase 9 is complete and `perDayCounters` and `ResetPerDayCounters` are present in `limit.go` before this phase executes | Architecture Patterns | If Phase 9 is not complete, `currentDayCount` references `perDayCounters` which may not yet be exported — compile error |

**Note on A1:** Reading `limit.go` confirms `perDayCounters` and `ResetPerDayCounters` are already present [VERIFIED: limit.go lines 19, 32]. Phase 9 appears to be already implemented in the codebase, ahead of its formal plan execution. Risk is LOW.

---

## Open Questions

1. **Column placement in KeyTable**
   - What we know: Table currently has 7 columns. The last column is "Actions".
   - What's unclear: Whether to insert rate columns before or after "Usage".
   - Recommendation: Insert Rate/min, Rate/day, Today between "Usage" and "Actions" to keep actions rightmost. This is a discretion call with no functional impact.

2. **day_usage when key has no RateLimitPerDay set**
   - What we know: If `RateLimitPerDay == 0`, `limitMiddleware` skips the per-day counter check, so `perDayCounters` never gets an entry for that key.
   - What's unclear: Should "Today" show `0` or `—` for keys with no day limit?
   - Recommendation: Always show the numeric count (0 if no counter entry). The column heading "Today" implies "requests today", which is 0 if no check has run. Showing `—` only makes sense if the counter is intentionally not tracked, which would need a separate day-request-count path (out of scope). Display `0`.

---

## Environment Availability

Step 2.6: SKIPPED — phase is purely code changes (Go + React) with no external service dependencies. Go toolchain and Node.js are confirmed available from prior phases.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard `testing` for backend; manual browser verification for React UI |
| Config file | none |
| Quick run command | `go test ./internal/server/... -run TestListKeys -v` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RATE-05 | List keys API response includes `day_usage` field | unit | `go test ./internal/server/... -run TestListKeys -v` | ❌ Wave 0 |
| RATE-05 | `day_usage` is 0 for a key with no requests today | unit | `go test ./internal/server/... -run TestListKeys -v` | ❌ Wave 0 |
| RATE-05 | Table renders Rate/min, Rate/day, Today columns | manual | Browser: open portal, inspect key table | N/A — UI-only |
| RATE-05 | Zero rate limit displays "Unlimited", not "0" | manual | Browser: create key with no rate limits, verify table | N/A — UI-only |
| RATE-06 | Updating rate limits via edit dialog persists and takes effect | manual | Browser: edit key, save, verify table reflects new limits | N/A — UI-only |

### Sampling Rate

- **Per task commit:** `go test ./internal/server/... -v`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green + manual portal smoke test before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `TestListKeys_IncludesDayUsage` in `internal/server/admin_test.go` — verify `day_usage` field is present and zero when no requests made
- [ ] No framework install needed — existing Go test infrastructure covers backend; no frontend test framework in this project

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | — |
| V3 Session Management | no | — |
| V4 Access Control | yes | Existing `adminMiddleware` gates all `/api/keys` routes — no change |
| V5 Input Validation | no | Phase is read-only display + existing validated PATCH path |
| V6 Cryptography | no | — |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| `day_usage` reveals request volume to non-admins | Information Disclosure | `handleListKeys` is already behind `adminMiddleware`; no public exposure |
| Stale counter leaking prior-day counts | Information Disclosure | `currentDayCount` validates `windowID == YearDay()` before returning count; returns 0 if stale |

---

## Sources

### Primary (HIGH confidence)

- `internal/server/limit.go` — verified `perDayCounters sync.Map`, `rateCounter` struct (mu, count, windowID), `checkRate` implementation [VERIFIED: codebase read]
- `internal/server/admin.go` — verified `keyResponse` struct fields, `toKeyResponse()` body, all five key handler signatures [VERIFIED: codebase read]
- `web/src/components/KeyTable.tsx` — verified `KeyResponse` interface, column structure (7 headers, 7 cells), existing `0 = Unlimited` pattern for `token_budget` [VERIFIED: codebase read]
- `web/src/components/EditKeyDialog.tsx` — verified rate limit state, form fields, and PATCH body logic fully implemented [VERIFIED: codebase read]
- `.planning/phases/09-rate-limit-backend/09-RESEARCH.md` — Phase 9 decisions and field name conventions [VERIFIED: planning artifact read]

### Secondary (MEDIUM confidence)

- None — all findings sourced directly from codebase.

### Tertiary (LOW confidence)

- None — no web searches performed; all required information found in codebase.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new dependencies; all existing
- Architecture: HIGH — currentDayCount pattern derived directly from reading limit.go; no inference
- Pitfalls: HIGH — all identified from direct code inspection (windowID staleness, interface divergence, column count)

**Research date:** 2026-04-18
**Valid until:** 2026-05-18 (stable Go + React codebase, no external dependencies)
