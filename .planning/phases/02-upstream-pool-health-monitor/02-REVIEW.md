---
phase: 02-upstream-pool-health-monitor
reviewed: 2026-04-16T00:00:00Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - cmd/ocp/main.go
  - config.yaml.example
  - internal/config/config.go
  - internal/pool/classifier.go
  - internal/pool/classifier_test.go
  - internal/pool/pool.go
  - internal/pool/pool_test.go
  - internal/pool/probe.go
  - internal/pool/probe_test.go
  - internal/server/server.go
  - internal/server/server_test.go
findings:
  critical: 0
  warning: 4
  info: 3
  total: 7
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-04-16T00:00:00Z
**Depth:** standard
**Files Reviewed:** 11
**Status:** issues_found

## Summary

This phase implements the upstream pool with round-robin selection, error classification, and a background probe loop. The code is well-structured and the test coverage is good. No security vulnerabilities or data-loss bugs were found. There are four warnings covering real correctness and reliability risks: a redundant timeout on probe requests that does not protect what it appears to protect, a missing guard for a zero-value `cfg` pointer, an untested edge case in the classifier's keyword matching for Kimi (the body keyword `"quota"` in `defaultCreditsKeywords` matches the Kimi 403 body, but the test credits the match to the wrong keyword — this is a latent maintenance hazard rather than a current bug), and a config loading order issue where `AutomaticEnv()` is called after `ReadInConfig()` but env-var overrides for the port and admin key are then re-applied manually, creating an inconsistency that is partly papered over by the explicit re-reads.

## Warnings

### WR-01: Redundant context timeout inside `sendProbe` — outer client timeout already applies

**File:** `internal/pool/probe.go:88`
**Issue:** `probeClient` is created at package scope with `Timeout: 10*time.Second` (line 15). Inside `sendProbe`, a `context.WithTimeout(context.Background(), 10*time.Second)` is also created (line 88) and attached to the request. The two timeouts race each other; whichever fires first wins. The intent is clearly to bound the probe to 10 seconds, but maintaining two identical 10-second timeouts in two separate places means they will drift apart during future edits. If someone changes the client timeout but not the context timeout (or vice versa), the effective limit will silently be whichever is smaller. The context timeout here provides no meaningful additional protection over the client-level timeout and should be removed to keep a single authoritative source.

**Fix:** Remove the `ctx`/`cancel` lines and use `http.NewRequest` (without context) so the client-level timeout is the only control knob:
```go
req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
if err != nil {
    return false
}
```
The `context` import can then be removed from probe.go.

---

### WR-02: `Pool.cfg` is never guarded against nil — panic if `New` is called with `cfg == nil`

**File:** `internal/pool/pool.go:117`
**Issue:** `Backoff()` dereferences `p.cfg.RateLimitBackoff` unconditionally. `New` stores whatever `cfg` pointer it receives directly into the struct (line 71). The exported `New` function accepts `cfg *Config` — callers outside this package can pass `nil`. If they do, both `Backoff()` and anything that reads `cfg` will panic. The server tests in `server_test.go` already pass `nil` for pool (line 15, 25, 37), but those tests do not call `Backoff()`. A future route handler that calls `p.Backoff()` on a server constructed with `nil` pool would panic.

**Fix:** Add a nil guard in `New`:
```go
if cfg == nil {
    cfg = &Config{RateLimitBackoff: 5 * time.Second}
}
```

---

### WR-03: `Classify` for Kimi credits exhaustion relies on the generic keyword `"quota"` — fragile and undocumented

**File:** `internal/pool/classifier.go:49-57`
**Issue:** The `defaultCreditsKeywords` list (line 28-31) includes the word `"quota"`. The Kimi test case (classifier_test.go line 21) passes because the Kimi 403 body contains `"quota"` (`"Your account's current quota has been exhausted"`). This match is implicit and not called out anywhere. The keyword `"quota"` is broad enough to match provider error bodies that signal a per-minute rate limit (e.g. `"rate quota exceeded"` from some providers), which would misclassify a transient rate-limit as a permanent credits exhaustion and permanently disable an upstream rather than backing off temporarily. There is no test case that verifies a rate-limit body containing the word "quota" is NOT misclassified.

**Fix:** Either add a test that verifies a rate-limit body containing "quota" is correctly classified, or narrow the keyword to `"quota exceeded"` / `"quota has been exhausted"` to reduce false-positive surface area. If the intent is that "quota" alone is safe, document the rationale in a comment.

---

### WR-04: `config.Load` calls `v.AutomaticEnv()` after `v.ReadInConfig()` — env override does not apply to `Unmarshal`

**File:** `internal/config/config.go:59-75`
**Issue:** `v.AutomaticEnv()` is called on line 61, after `v.ReadInConfig()` on line 55. In Viper, `AutomaticEnv` only takes effect for `v.Get*` calls made after it is registered; it does not retroactively affect keys already bound or the state used by `v.Unmarshal`. The code then calls `v.Unmarshal` (line 64) and separately re-reads four scalar fields via `v.GetInt` / `v.GetString` (lines 72-75). The re-reads ensure the four scalars pick up env overrides, but any field added to `Config` in the future (e.g., a new `pool.probe_interval` key) will silently ignore the env override unless a developer remembers to add another explicit `v.Get*` call. The `Upstreams` slice is correctly documented as non-env-injectable (line 69 comment), but the silent "works for some fields, not others" contract is a maintenance trap.

**Fix:** Register `AutomaticEnv` and `BindEnv` calls before `ReadInConfig`, or add a comment block that explicitly lists every field that is env-overridable and requires a `v.Get*` call here, so future maintainers know the contract:
```go
// ENV OVERRIDE: Every scalar field that should be env-overridable MUST be re-read
// below via v.Get* after Unmarshal. Viper does not propagate AutomaticEnv into
// Unmarshal results. Add a re-read here whenever a new top-level scalar is added.
```

## Info

### IN-01: Duplicate test functions in `classifier_test.go` — table test and standalone functions test the same cases

**File:** `internal/pool/classifier_test.go:9-133`
**Issue:** `TestClassify` (line 9) is a table-driven test covering 9 cases. Lines 95-133 duplicate five of those nine cases as individual top-level functions (`TestClassify_Kimi_CreditsExhausted`, `TestClassify_GLM_1113`, etc.). Both sets exercise identical inputs and assertions. This doubles the test count with no coverage gain and will require double-maintenance if a case changes.

**Fix:** Remove the individual top-level functions (lines 95-133) and keep only the table-driven test. The table is already comprehensive.

---

### IN-02: `config.yaml.example` contains placeholder `admin_key: "change-me"` that `config.Load` explicitly rejects

**File:** `config.yaml.example:3`
**Issue:** The example file ships with `admin_key: "change-me"`. `config.Load` (config.go line 77) rejects this value with a fatal error at startup. A new user who copies the example file verbatim and starts the server will see an immediate fatal error without a clear path forward unless they read the source or error message carefully.

**Fix:** Change the placeholder to a more instructive value, or add an inline comment:
```yaml
  admin_key: "REPLACE_WITH_A_STRONG_SECRET"  # must not be "change-me"; ocp will refuse to start
```

---

### IN-03: `probeModels` fallback is `"gpt-3.5-turbo"` — will fail on all Chinese providers

**File:** `internal/pool/probe.go:29`
**Issue:** `probeModel` returns `"gpt-3.5-turbo"` for any provider not in the `probeModels` map. All target providers (Kimi, Qwen, GLM, Minimax) are in the map, but if a new upstream is added to the config without a corresponding entry in `probeModels`, the probe will send a request for `gpt-3.5-turbo`, which will be rejected by every Chinese provider. This will cause the probe to always fail and the new upstream will never be re-enabled after it is marked unavailable. The fallback silently defeats the recovery mechanism.

**Fix:** Either log a warning when a provider is not found in `probeModels` (so the operator knows to add an entry), or document that every new provider must have an entry in `probeModels` with a comment at the map declaration:
```go
// probeModels maps provider name to a cheap model for liveness probes.
// REQUIRED: Every upstream provider in config must have an entry here.
// Upstreams without an entry will use "gpt-3.5-turbo" which fails on all
// Chinese providers, causing permanent unavailability after first failure.
var probeModels = map[string]string{
```

---

_Reviewed: 2026-04-16T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
