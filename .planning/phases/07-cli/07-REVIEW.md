---
phase: 07-cli
reviewed: 2026-04-17T00:00:00Z
depth: standard
files_reviewed: 9
files_reviewed_list:
  - cmd/ocp/api.go
  - cmd/ocp/keys.go
  - cmd/ocp/main.go
  - cmd/ocp/next.go
  - cmd/ocp/root.go
  - cmd/ocp/serve.go
  - cmd/ocp/status.go
  - go.mod
  - internal/pool/pool.go
findings:
  critical: 0
  warning: 4
  info: 3
  total: 7
status: issues_found
---

# Phase 07: Code Review Report

**Reviewed:** 2026-04-17T00:00:00Z
**Depth:** standard
**Files Reviewed:** 9
**Status:** issues_found

## Summary

Reviewed the CLI tool (`cmd/ocp/`) and the upstream pool (`internal/pool/pool.go`). The code is generally clean and follows Go idioms. Four warnings were found: two relate to `os.Exit` being called inside functions that also return errors (making the error return dead code and complicating testing), one is a `Select` off-by-one behaviour when all upstreams are unavailable, and one is unchecked `io.ReadAll` in error paths. Three info items cover `go.mod` version anomaly, a misleading struct field name, and a stale comment.

## Warnings

### WR-01: `apiGet`/`apiPost` call `os.Exit(1)` but also return `([]byte, error)`

**File:** `cmd/ocp/api.go:25-28`, `cmd/ocp/api.go:56-59`
**Issue:** Both `apiGet` and `apiPost` call `os.Exit(1)` on network error rather than returning an error. The `error` return value from these functions is therefore never populated for the connection-failure case, making the return type misleading. Callers in `keys.go`, `next.go`, and `status.go` check `err != nil` after these calls, but they will never see an error for the most common failure mode (server not running). This pattern also makes unit testing impossible without process-exiting the test runner.
**Fix:** Return an error instead of calling `os.Exit`:
```go
if err != nil {
    return nil, fmt.Errorf("cannot reach ocp server at %s: %w", flagHost, err)
}
```
Move the `os.Exit` to `main.go` or rely on Cobra's `RunE` chain to print and exit.

---

### WR-02: `io.ReadAll` error silently ignored in both `apiGet` and `apiPost`

**File:** `cmd/ocp/api.go:31`, `cmd/ocp/api.go:63`
**Issue:** `body, _ := io.ReadAll(resp.Body)` discards the read error. On a truncated or reset connection this will silently produce a partial body, which is then passed to `json.Unmarshal`. The JSON parse will fail but the error message will be "decode response" rather than pointing to the real cause.
**Fix:**
```go
body, err := io.ReadAll(resp.Body)
if err != nil {
    return nil, fmt.Errorf("read response body: %w", err)
}
```

---

### WR-03: `Select` skips the current position on the first call, even when only one upstream exists

**File:** `internal/pool/pool.go:120-127`
**Issue:** `Select` always increments `p.idx` before checking availability (`p.idx = (p.idx + 1) % n`). On the very first call with a single upstream, `idx` starts at 0, increments to 0 again (mod 1), and works. But consider two upstreams where only index 0 is available: `idx` starts at 0, the loop increments to 1 (unavailable), then wraps to 0 (available) — two iterations to find it. This is a minor inefficiency but has a correctness edge case: if `ForceRotate` has moved `idx` to the last entry and then `Select` is called on a pool with exactly one available upstream at position `idx`, the loop correctly wraps, but this interaction is non-obvious and asymmetric with `ForceRotate`'s identical pattern. The real bug is that both `Select` and `ForceRotate` share a single `idx` cursor — a `ForceRotate` call changes where `Select` will start next, meaning an operator-triggered rotation permanently shifts the round-robin start point. This is likely intentional but is not documented.

More concretely: if there is exactly one upstream and `ForceRotate` is called, it advances `idx` from 0 to 0 (mod 1 = 0) and reports success. Then the next `Select` call does the same. This works. However, if there are two upstreams and both are available, `ForceRotate` advances to upstream B. The next `Select` call then advances to upstream A — effectively the rotate had no lasting effect after one request. Document the intended semantics or use a separate cursor for `ForceRotate`.
**Fix:** Add a comment clarifying that `ForceRotate` and `Select` share a cursor and that `ForceRotate` only guarantees the next *single* request goes to the rotated-to upstream.

---

### WR-04: `serve.go` — encryption key length check allows zero-length key on empty env var

**File:** `cmd/ocp/serve.go:27-30`
**Issue:** `encKey := []byte(os.Getenv("OCP_ENCRYPTION_KEY"))`. If `OCP_ENCRYPTION_KEY` is not set, `os.Getenv` returns `""` and `len(encKey)` is 0. The condition `len(encKey) != 16 && len(encKey) != 24 && len(encKey) != 32` correctly rejects 0, so the process exits with an error. This is not a security bug per se, but the error message says "got %d bytes" which for an unset env var prints "got 0 bytes" — a clear enough signal. The actual risk is that a value like `OCP_ENCRYPTION_KEY=` (set but empty) also produces 0 bytes and the same rejection path. This is handled correctly. **However**, the key is taken directly from an environment variable as raw bytes with no derivation or stretching — if the operator provides a short-entropy string like `"password1234xxxx"` (16 bytes, ASCII), it will be accepted as a valid AES-128 key. This is documented nowhere and may surprise operators expecting the key to be a passphrase. This is an info-level concern absent a documented security model, but it warrants a warning given that key material security is the responsibility of this code path.
**Fix:** Add a comment at the key-loading site: `// OCP_ENCRYPTION_KEY must be 16, 24, or 32 random bytes (not a passphrase). Use: openssl rand -hex 16`.

## Info

### IN-01: `go.mod` declares `go 1.25.0` which does not exist yet

**File:** `go.mod:3`
**Issue:** The module declares `go 1.25.0`. As of April 2026 the latest stable Go release is 1.24.x; 1.25 has not been released. This likely results from a typo or auto-generated value. While `go build` will still work on a newer toolchain, CI/CD environments pinned to current stable Go will warn or fail.
**Fix:** Change to `go 1.24.0` (or whichever version is actually being used to build the project).

---

### IN-02: `upstreamStatus.Position` field name is misleading

**File:** `cmd/ocp/status.go:27`
**Issue:** `Position bool` — a boolean field named "position" is confusing. The field communicates "is this the current round-robin cursor position?" A more expressive name would be `Current bool` or `Active bool`. The JSON tag would also benefit from clarity: `json:"current"`.
**Fix:** Rename to `Current bool \`json:"current"\`` and update the tabwriter header from `POSITION` to `CURRENT`.

---

### IN-03: Stale comment in `pool.go`

**File:** `internal/pool/pool.go:51`
**Issue:** The comment on `New` says "It does not start any background goroutine (that is Plan 02)." Plan 02 has long since been implemented — `StartProbeLoop` exists in the same file and is called from `serve.go`. The comment is misleading for anyone reading the code.
**Fix:** Update the comment to: `// New loads all enabled upstreams from db, decrypts their API keys, and returns a ready Pool.`

---

_Reviewed: 2026-04-17T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
