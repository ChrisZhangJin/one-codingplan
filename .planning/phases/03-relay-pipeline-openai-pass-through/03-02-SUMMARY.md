---
phase: 03-relay-pipeline-openai-pass-through
plan: 02
subsystem: api
tags: [gin, sse, streaming, heartbeat, race-free, openai]

requires:
  - phase: 03-01
    provides: proxyStream stub in relay.go, handleRelay dispatch, logUsage, authMiddleware

provides:
  - Real proxyStream replacing Plan 01 stub: SSE passthrough with per-chunk flushing
  - Heartbeat goroutine sending ': heartbeat' comments every 30s on idle streams
  - sync.Mutex serializing all c.Writer writes between heartbeat goroutine and main loop
  - HeartbeatInterval package-level var (30s default) overridable in tests
  - 5 new streaming tests covering SSE frames, heartbeat, mid-stream failure, usage logging, failover

affects:
  - 04-anthropic-pass-through (proxyStream pattern reusable for /v1/messages streaming)

tech-stack:
  added: []
  patterns:
    - "Context cancel deferred into proxy functions: cancel() called by proxyBuffer/proxyStream after body reads complete, not immediately after relayClient.Do()"
    - "Mutex-guarded writer helper: writeAndFlush([]byte) acquires mu, writes to c.Writer, flushes — prevents data race between heartbeat goroutine and main read loop"
    - "Heartbeat interval captured before goroutine launch: hbInterval := HeartbeatInterval avoids data race with test overrides"
    - "buf[:n] slice passed to writeAndFlush inside read loop: n bytes valid, reused 4096-byte buffer"

key-files:
  created: []
  modified:
    - internal/server/relay.go
    - internal/server/relay_test.go

key-decisions:
  - "sync.Mutex for c.Writer serialization: heartbeat goroutine and main read loop both write to c.Writer (not concurrency-safe); mutex is the minimal correct fix that works in tests and production"
  - "context.CancelFunc passed into proxyBuffer/proxyStream: cancel() must not be called until after body reads finish, otherwise resp.Body.Read returns context.Canceled mid-stream"
  - "HeartbeatInterval exported (not unexported): allows test override without reflection; low coupling risk since it is a package-level tuning var"

requirements-completed: [PRXY-03, USGR-01]

duration: 7min
completed: 2026-04-16
---

# Phase 03 Plan 02: SSE Streaming Passthrough Summary

**proxyStream implemented with per-chunk flushing, mutex-guarded heartbeat goroutine, and context lifetime fix ensuring resp.Body remains readable for full stream duration**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-04-16T06:13:43Z
- **Completed:** 2026-04-16T06:20:30Z
- **Tasks:** 1 (TDD: RED + GREEN)
- **Files modified:** 2

## Accomplishments

- `proxyStream` replaces the Plan 01 stub: sets `text/event-stream`, `X-Accel-Buffering: no`, `Cache-Control: no-cache`, `Connection: keep-alive` headers before first write
- Per-chunk flush loop reads up to 4096 bytes at a time from `resp.Body`, writes and flushes immediately via `writeAndFlush`
- Heartbeat goroutine sends `: heartbeat\n\n` every `HeartbeatInterval` (default 30s); `defer close(done)` ensures it exits on all return paths (T-3-08)
- `sync.Mutex` serializes all `c.Writer` access between the heartbeat goroutine and the main read loop (race-free under `-race`)
- `context.CancelFunc` passed into proxy functions and deferred — `cancel()` fires after body reads complete, not after `relayClient.Do()` returns
- `HeartbeatInterval` exported package var; tests override to 50ms for fast heartbeat verification
- 5 streaming tests added; all pass with `-race`; all 16 relay tests (11 from Plan 01 + 5 new) pass

## Task Commits

1. **RED: Streaming tests** - `30a6b36` (test)
2. **GREEN: proxyStream implementation + context fix** - `9da3a16` (feat)

## Files Created/Modified

- `internal/server/relay.go` — HeartbeatInterval var, proxyStream implementation, cancel deferred into proxyBuffer/proxyStream, sync import, TODO(plan-02) removed
- `internal/server/relay_test.go` — 5 streaming tests + fakeStreamingUpstream/fakeSlowStreamingUpstream/fakeMidFailureUpstream helpers

## Decisions Made

- `sync.Mutex` wrapping all `c.Writer` writes: heartbeat goroutine and main read loop run concurrently; `c.Writer` (backed by `httptest.ResponseRecorder` in tests or `net/http` in production) is not concurrency-safe — mutex is the minimal correct fix
- Context cancel deferred into proxy functions: `cancel()` called immediately after `relayClient.Do()` cancelled `resp.Body` mid-stream; fix passes `cancel` as parameter and defers it
- `HeartbeatInterval` exported: enables test overrides without test hacks; small surface, justified

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed context cancel aborting resp.Body mid-stream**
- **Found during:** Task 1 (TestRelay_Stream failing with only 1 frame received under -race)
- **Issue:** `cancel()` called immediately after `relayClient.Do()` cancelled the context used by the request, causing `resp.Body.Read` to return `context.Canceled` after the first chunk
- **Fix:** Pass `context.CancelFunc` into `proxyBuffer` and `proxyStream`; defer `cancel()` inside each function so the context lives for the full response body read
- **Files modified:** internal/server/relay.go
- **Commit:** 9da3a16

**2. [Rule 1 - Bug] Fixed data race: concurrent writes to c.Writer from heartbeat goroutine and main loop**
- **Found during:** Task 1 (data race reported by -race on TestRelay_Stream_Heartbeat)
- **Issue:** Heartbeat goroutine called `fmt.Fprint(c.Writer, ...)` while main loop called `c.Writer.Write(buf[:n])` — Gin's ResponseWriter is not concurrency-safe
- **Fix:** Added `sync.Mutex` guarding a `writeAndFlush([]byte)` helper used by both goroutines; all writes to `c.Writer` now serialized
- **Files modified:** internal/server/relay.go
- **Commit:** 9da3a16

**3. [Rule 1 - Bug] Fixed data race on HeartbeatInterval global var**
- **Found during:** Task 1 (first -race run)
- **Issue:** Test deferred restore of `server.HeartbeatInterval` while heartbeat goroutine still running read it concurrently
- **Fix:** Capture `hbInterval := HeartbeatInterval` before spawning goroutine; goroutine uses local copy
- **Files modified:** internal/server/relay.go
- **Commit:** 9da3a16

---

**Total deviations:** 3 auto-fixed (3 Rule 1 bugs discovered during GREEN phase)
**Impact on plan:** All fixes improve production correctness (context lifetime, concurrency safety). Plan's proposed implementation was accurate for structure but missed these concurrency details.

## Issues Encountered

All three bugs were concurrency-related and found by the race detector. All fixed before final commit.

## User Setup Required

None.

## Next Phase Readiness

- `/v1/chat/completions` now handles both streaming and non-streaming correctly with failover
- `proxyStream` mutex pattern is reusable for Phase 04 Anthropic streaming
- `authMiddleware` on `/v1` group automatically protects `/v1/messages` when Phase 04 adds it

---
*Phase: 03-relay-pipeline-openai-pass-through*
*Completed: 2026-04-16*

## Self-Check: PASSED

- internal/server/relay.go: FOUND
- internal/server/relay_test.go: FOUND
- .planning/phases/03-relay-pipeline-openai-pass-through/03-02-SUMMARY.md: FOUND
- commit 30a6b36 (RED: streaming tests): FOUND
- commit 9da3a16 (GREEN: proxyStream implementation): FOUND
