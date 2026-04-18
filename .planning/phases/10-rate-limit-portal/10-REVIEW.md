---
phase: 10-rate-limit-portal
reviewed: 2026-04-18T00:00:00Z
depth: standard
files_reviewed: 5
files_reviewed_list:
  - internal/server/limit.go
  - internal/server/admin.go
  - internal/server/admin_test.go
  - web/src/components/KeyTable.tsx
  - web/src/components/EditKeyDialog.tsx
findings:
  critical: 0
  warning: 4
  info: 3
  total: 7
status: issues_found
---

# Phase 10: Code Review Report

**Reviewed:** 2026-04-18T00:00:00Z
**Depth:** standard
**Files Reviewed:** 5
**Status:** issues_found

## Summary

This phase adds per-minute and per-day rate limiting plus a token budget check in the server middleware, exposes rate limit fields in the admin API, and adds UI controls in the React portal to view and edit those limits. The implementation is generally clean and follows existing patterns. There are no security vulnerabilities or data-loss risks. Four warnings cover logic gaps that could cause silent incorrect behavior in production; three info items cover dead code and minor clarity issues.

## Warnings

### WR-01: Token budget query ignores errors silently

**File:** `internal/server/limit.go:88-92`
**Issue:** The token budget query uses `.Row().Scan(...)` without checking the error returned by `Scan`. If the query fails (e.g., DB overloaded, context cancelled), `totalInput` and `totalOutput` remain zero, causing the middleware to allow the request through rather than failing safely. The budget check then compares `0 >= key.TokenBudget`, which is false for any positive budget — so a DB error grants access instead of denying it.
**Fix:**
```go
row := s.db.Model(&models.UsageRecord{}).
    Select("COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)").
    Where("key_id = ?", key.ID).
    Row()
if err := row.Scan(&totalInput, &totalOutput); err != nil {
    c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
        "error": gin.H{"message": "internal error", "type": "server_error", "code": "internal_error"},
    })
    return
}
```

### WR-02: `minuteWindow` collides across days

**File:** `internal/server/limit.go:107-108`
**Issue:** The per-minute window ID is computed as `now.Hour()*60 + now.Minute()`, which produces values 0–1439. This value repeats identically every 24 hours. If a key exhausts its per-minute limit at 14:30 on day N, the counter is still in memory at 14:30 on day N+1 with the same `windowID`. The `checkRate` function sees `rc.windowID == currentWindow` and does not reset — so the key remains rate-limited at the same clock minute the next day. Only a process restart or a request at a different minute clears the stale count.

The impact is low in practice (the counter resets the next minute), but the semantic is wrong: the window should encode day+hour+minute, not just hour+minute.
**Fix:**
```go
now := time.Now().UTC()
minuteWindow := now.YearDay()*1440 + now.Hour()*60 + now.Minute()
```

### WR-03: `handleUpdateKey` reload ignores DB error

**File:** `internal/server/admin.go:234`
**Issue:** After applying updates, the handler reloads the key with `s.db.First(&key, "id = ?", id)` but discards the error. If the reload fails, the handler calls `s.toKeyResponse(key, false)` with stale pre-update data and returns HTTP 200 with incorrect values. The caller receives a successful response with wrong field values.
**Fix:**
```go
if err := s.db.First(&key, "id = ?", id).Error; err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reload key"})
    return
}
```

### WR-04: `expiresAt` clearing is impossible from the UI

**File:** `web/src/components/EditKeyDialog.tsx:84-87`
**Issue:** The submit handler only sends `expires_at` if `expiresAt` is truthy. If a user clears the datetime field (making `expiresAt` an empty string), the field is omitted from the PATCH body entirely, so the expiry is never cleared. There is no way to remove an expiration date once set.

Additionally, the backend `patchKeyRequest.ExpiresAt` is `*time.Time`, and the update map only sets the field when `req.ExpiresAt != nil` (admin.go:216). An explicit null is needed to clear it, but the UI never sends one.
**Fix:** In `handleSubmit`, handle the clearing case explicitly:
```ts
if (expiresAt) {
  const newExpiry = new Date(expiresAt).toISOString()
  if (newExpiry !== keyData.expires_at) body.expires_at = newExpiry
} else if (keyData.expires_at) {
  // user cleared the field — send null to remove expiry
  body.expires_at = null
}
```
The backend also needs a corresponding change to handle null `expires_at` in the patch (set `expires_at` to NULL in DB when the pointer value is explicitly provided as null vs. omitted — currently the pointer-nil check cannot distinguish the two cases from JSON).

## Info

### IN-01: Dead UI button — `console.log` placeholder

**File:** `web/src/components/KeyTable.tsx:169`
**Issue:** The "View key details" Info button has an `onClick` handler that only calls `console.log('detail', key.id)`. This is unimplemented placeholder behavior and leaves a debug log statement in production code.
**Fix:** Either implement the detail view or remove the button until the feature is built.

### IN-02: Duplicate `KeyResponse` interface definition

**File:** `web/src/components/EditKeyDialog.tsx:16-31` and `web/src/components/KeyTable.tsx:21-36`
**Issue:** `KeyResponse` is defined identically in both files. If the API shape changes, both definitions must be updated in sync.
**Fix:** Extract to a shared types file (e.g., `web/src/lib/types.ts`) and import from both components.

### IN-03: `usageTotals` issues N+1 queries in `handleListKeys`

**File:** `internal/server/admin.go:72-79` called from `internal/server/admin.go:171-174`
**Issue:** `handleListKeys` calls `s.toKeyResponse` for each key, and each call invokes `s.usageTotals` which executes a separate `SELECT SUM(...)` query per key. For N keys this is N+1 queries. This is an info-level note since performance is out of v1 scope, but it is worth flagging because it also affects the accuracy of `DayUsage` (each call to `currentDayCount` reads from the in-memory map, which is consistent, but the SQL totals are N separate round-trips with no transaction).
**Fix (when ready):** Batch the usage aggregation with a single `GROUP BY key_id` query before the loop.

---

_Reviewed: 2026-04-18T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
