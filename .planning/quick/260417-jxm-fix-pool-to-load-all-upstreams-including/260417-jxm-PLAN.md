---
phase: quick
plan: 260417-jxm
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/pool/pool.go
  - internal/pool/pool_test.go
autonomous: true
must_haves:
  truths:
    - "pool.New() loads ALL upstreams from DB, including disabled ones"
    - "Disabled upstreams appear in pool.List() with Enabled=false, Available=false"
    - "pool.Select() still skips disabled upstreams (they have available=false)"
    - "pool.SetEnabled(name, true) works for previously-disabled upstreams"
  artifacts:
    - path: "internal/pool/pool.go"
      provides: "Pool constructor loading all upstreams"
      contains: "db.Find(&upstreams)"
    - path: "internal/pool/pool_test.go"
      provides: "Tests covering disabled upstream visibility and toggle"
  key_links:
    - from: "internal/pool/pool.go New()"
      to: "entry.available / entry.enabled"
      via: "set from u.Enabled DB field"
      pattern: "available:\\s+u\\.Enabled"
---

<objective>
Fix pool.New() to load all upstreams (including disabled) so the portal can display and toggle them.

Purpose: Currently disabled upstreams are invisible to the portal and cannot be re-enabled via SetEnabled because they were never loaded into pool.entries.
Output: pool.New() loads all upstreams; disabled ones have available=false, enabled=false; Select() routing unchanged.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@internal/pool/pool.go
@internal/pool/pool_test.go
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Update pool.New() to load all upstreams and fix tests</name>
  <files>internal/pool/pool.go, internal/pool/pool_test.go</files>
  <behavior>
    - Test: pool.New() with 2 enabled + 1 disabled upstream results in List() returning 3 entries
    - Test: The disabled upstream in List() has Enabled=false and Available=false
    - Test: Select() still only returns enabled upstreams (does not return the disabled one)
    - Test: SetEnabled(disabledName, true) makes the upstream selectable again
  </behavior>
  <action>
1. In pool.go `New()` function (line 53): Remove the `.Where("enabled = ?", true)` filter so the query becomes just `db.Find(&upstreams)`.

2. In the entry construction loop (lines 62-71): Set `available` and `enabled` based on `u.Enabled` from the DB record instead of hardcoding both to `true`:
   ```
   available: u.Enabled,
   enabled:   u.Enabled,
   ```

3. Update the comment on line 49 from "loads all enabled upstreams" to "loads all upstreams" (drop "enabled").

4. In pool_test.go `TestNew_LoadsFromDB` (line 297):
   - Change the assertion: instead of checking that disabled `z` does NOT appear via Select(), verify that List() returns 3 entries total.
   - Verify `z` appears in List() with Enabled=false and Available=false.
   - Verify Select() still only returns `x` and `y` (not `z`).
   - Add a sub-test: call SetEnabled("z", true), then verify Select() can now return `z`.

5. In pool_test.go `newTestPool` helper: No changes needed — it only creates enabled upstreams, so behavior is unchanged.
  </action>
  <verify>
    <automated>cd /root/workspace/one-codingplan && go test ./internal/pool/ -v -count=1</automated>
  </verify>
  <done>
    - pool.New() loads all upstreams regardless of enabled state
    - Disabled upstreams visible in List() with correct Enabled/Available flags
    - Select() unchanged — only picks available upstreams
    - SetEnabled() works for upstreams that were loaded as disabled
    - All existing tests pass (updated where needed)
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

No new trust boundaries introduced. This is an internal data-loading change.

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-quick-01 | I (Information Disclosure) | pool.List() | accept | Disabled upstreams were already in DB and visible via admin API; List() already excludes API keys. No new exposure. |
</threat_model>

<verification>
```bash
cd /root/workspace/one-codingplan && go test ./internal/pool/ -v -count=1
```
All pool tests pass, including updated TestNew_LoadsFromDB covering disabled upstream visibility.
</verification>

<success_criteria>
- `go test ./internal/pool/` passes with zero failures
- Disabled upstreams appear in pool.List() output
- Select() never returns a disabled upstream
- SetEnabled() can re-enable a previously-disabled upstream
</success_criteria>

<output>
After completion, create `.planning/quick/260417-jxm-fix-pool-to-load-all-upstreams-including/260417-jxm-SUMMARY.md`
</output>
