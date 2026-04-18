---
phase: 09-rate-limit-backend
reviewed: 2026-04-18T00:00:00Z
depth: standard
files_reviewed: 3
files_reviewed_list:
  - internal/server/limit.go
  - internal/server/admin_test.go
  - internal/server/e2e_test.go
findings:
  critical: 1
  warning: 2
  info: 1
  total: 4
status: issues_found
---

# Phase 09: Code Review Report

**Reviewed:** 2026-04-18
**Depth:** standard
**Files Reviewed:** 3
**Status:** issues_found

## Summary

Three files were reviewed: the new rate-limiting middleware (`limit.go`) and the two test files that exercise both the admin CRUD API and the end-to-end HTTP stack. The middleware logic is clean and well-structured, but contains one critical correctness bug in the per-minute window ID calculation that allows stale counters from a previous day to bleed into the current day. Two additional warnings cover an unchecked DB error in the token budget query and a broken (nil-returning) helper function in the e2e test file.

---

## Critical Issues

### CR-01: Per-minute window ID collides across calendar days

**File:** `internal/server/limit.go:80`

**Issue:** `minuteWindow` is computed as `now.Hour()*60 + now.Minute()`, producing values 0–1439. This value is identical for the same clock time on any two calendar days. `sync.Map` entries are never evicted, so a `*rateCounter` created at, say, 14:37 on day N persists indefinitely. At 14:37 on day N+1 `currentWindow` is again 877 (`14*60+37`). Because `rc.windowID == currentWindow`, the guard `if rc.windowID != currentWindow` does **not** fire and the counter is **not** reset. Any count accumulated during that minute on day N carries over into day N+1, incorrectly blocking or partially consuming the per-minute budget.

The per-day window (using `YearDay()`) does not have this problem because `YearDay()` is unique within a year.

**Fix:** Include the date component in the minute window ID so it is unique across days. The simplest approach uses Unix epoch minutes:

```go
// Replace:
minuteWindow := now.Hour()*60 + now.Minute()

// With:
minuteWindow := int(now.Unix() / 60) // unique per calendar minute across all days
```

This produces a monotonically increasing integer that never repeats for a given wall-clock minute on different days. No other change is needed; the reset logic in `checkRate` handles the transition correctly.

---

## Warnings

### WR-01: Token budget DB error silently treated as zero usage

**File:** `internal/server/limit.go:61-64`

**Issue:** The token budget check uses `Row().Scan(&totalInput, &totalOutput)`. If `Row()` returns an error (disk full, SQLite locked, driver error), `Scan` populates neither variable and they remain 0. The code then compares `0 >= key.TokenBudget` — for any positive budget this is false, so the request is allowed through. A transient DB error causes the limit to be silently bypassed rather than failing safely.

```go
s.db.Model(&models.UsageRecord{}).
    Select("COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)").
    Where("key_id = ?", key.ID).
    Row().Scan(&totalInput, &totalOutput)   // error discarded
```

**Fix:** Capture the `Row()` result, call `Scan`, and check the error:

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

### WR-02: `e2eServerWithDB` returns nil DB, panics on use

**File:** `internal/server/e2e_test.go:39-47`

**Issue:** `e2eServerWithDB` declares a return type of `interface{ First(interface{}, ...interface{}) interface{} }` but unconditionally returns `nil` as the db value (line 46). Any caller that dereferences the returned db will panic. The function is currently uncalled within the file, so it does not fail today, but it is broken dead code that will cause a nil pointer panic if ever used.

```go
func e2eServerWithDB(t *testing.T) (serverURL string, db interface{ First(interface{}, ...interface{}) interface{} }, cleanup func()) {
    // ...
    return ts.URL, nil, ts.Close   // nil returned for db
}
```

**Fix:** Either return the actual `gormDB` (which satisfies the declared interface) or remove the function entirely if it is not needed:

```go
// Option A: return the real DB
return ts.URL, gormDB, ts.Close

// Option B: remove the function — it has no callers
```

---

## Info

### IN-01: Unused-import sentinel `var _ = time.Now` in admin_test.go

**File:** `internal/server/admin_test.go:719`

**Issue:** The sentinel `var _ = time.Now` exists solely to suppress an "imported and not used" compile error for the `time` package. The `time` import at line 9 is kept because `time.Time` appears in struct types used indirectly, but the compiler does not see a direct usage. The sentinel is a code smell — either the import is genuinely unused (and should be removed) or it is used in a way that could be made explicit.

**Fix:** Verify whether `time` is actually needed. If `makeLimitTestKey` or any seeded model uses `time.Time` through a GORM field, the import is only needed for that type. If the compiler is satisfied without it, remove both the import and the sentinel. If it is required, add a real usage such as a test that checks timestamp fields, so the sentinel is not necessary.

---

_Reviewed: 2026-04-18_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
