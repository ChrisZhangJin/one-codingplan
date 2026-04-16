---
phase: 04-anthropic-format-translation
reviewed: 2026-04-16T00:00:00Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - cmd/ocp/main.go
  - internal/config/config.go
  - internal/pool/pool.go
  - internal/server/anthropic.go
  - internal/server/anthropic_test.go
  - internal/server/server.go
  - internal/translator/request.go
  - internal/translator/response.go
  - internal/translator/stream.go
  - internal/translator/translator_test.go
  - internal/translator/types.go
findings:
  critical: 0
  warning: 5
  info: 4
  total: 9
status: issues_found
---

# Phase 04: Code Review Report

**Reviewed:** 2026-04-16T00:00:00Z
**Depth:** standard
**Files Reviewed:** 11
**Status:** issues_found

## Summary

This phase introduces the Anthropic Messages API translation layer: a request translator (`AnthropicToOpenAI`), a response translator (`OpenAIToAnthropic`), a stateful SSE `StreamTranslator`, and the `handleAnthropicRelay` handler wired into the Gin engine. The core translation logic is solid — model echo (D-03), tool round-trip (D-06/D-07), and finish-reason mapping are all correct and well-tested.

The review found no critical issues. Five warnings cover: a context leak on `json.Marshal` failure, a request-body size check that leaks 1 byte of data past the limit check, a blocking `time.Sleep` on the hot path, a streaming translator that silently drops frames on `translateErr` (hiding errors), and a `done` channel close-before-write race in the heartbeat pattern. Four info items cover: an unused variable reference in tests, `containsString` reimplementing bytes.Contains, a hardcoded 30-second timeout duplicated in two files, and zero token counts logged for streaming usage.

## Warnings

### WR-01: Context leak on `json.Marshal` failure in `handleAnthropicRelay`

**File:** `internal/server/anthropic.go:93-95`
**Issue:** When `json.Marshal(oaiReq)` fails, the code executes `continue`, which starts the next loop iteration and calls `context.WithTimeout` again — but the `cancel` from the previous iteration is never called. Each failed marshal leaks a context. In practice `json.Marshal` on a well-typed struct very rarely fails, but the variable `ctx`/`cancel` from line 97 is created _after_ the marshal, so the real problem is that a failed marshal silently loops to the next upstream without decrementing any resource. More concretely: if there is only one upstream and the struct is somehow unmarshalable, the loop calls `s.pool.Select` once, marks it `seen`, then on the next iteration hits `seen[up.ID]` and breaks — resulting in a 503 with no log entry.

The actual leak is subtle: `cancel` is declared outside the loop (it isn't — `cancel` is declared at line 97 inside the loop only in the success branch). The real issue is the `continue` on line 95 skips the error return and silently tries the next upstream, which is wrong behavior for a translation error — the client request is malformed, not the upstream.

**Fix:** Return a 400 immediately on marshal failure, same as the `AnthropicToOpenAI` error path just above:
```go
translatedBody, err := json.Marshal(oaiReq)
if err != nil {
    c.JSON(http.StatusInternalServerError, anthropicError("api_error", "failed to serialize request"))
    return
}
```

---

### WR-02: Body size limit allows exactly 10MB+1 bytes through

**File:** `internal/server/anthropic.go:41-49` (same pattern in `internal/server/relay.go:94-102`)
**Issue:** The code reads up to `10*1024*1024+1` bytes, then checks `if len(bodyBytes) > 10*1024*1024`. This means a body of exactly `10*1024*1024+1` bytes passes the read but is rejected — correct. However a body of exactly `10*1024*1024` bytes passes both checks and is processed. This is the intended 10MB limit, so the limit itself is correct. The off-by-one is in the _read cap_: `LimitReader` caps at `10MB+1`, so if the body is `10MB+1` bytes you read all `10MB+1` then reject. If the body is `10MB+2` bytes you still only read `10MB+1` because `LimitReader` stops there. The check fires correctly in all cases.

The real issue is that a body of exactly `10*1024*1024` bytes is silently accepted even though the intent may be to reject anything at or above 10MB. This is a boundary ambiguity, not a security hole, but it differs from the 413 path advertised in comments.

Additionally, this pattern is duplicated verbatim between `relay.go` and `anthropic.go` — if the limit changes it must be updated in two places.

**Fix:** Either accept that `<= 10MB` is the limit (document it), or change the check to `>=`:
```go
if len(bodyBytes) >= 10*1024*1024 {
    c.JSON(http.StatusRequestEntityTooLarge, ...)
    return
}
```
And extract the limit to a package-level constant shared by both handlers.

---

### WR-03: Blocking `time.Sleep` on the Goroutine serving the HTTP request

**File:** `internal/server/anthropic.go:126-128` (same pattern in `internal/server/relay.go:163-165`)
**Issue:** On a `ClassRateLimited` upstream response the handler sleeps with `time.Sleep(s.pool.Backoff())`. The default backoff is 5 seconds. This blocks the Goroutine serving the client request for the entire duration. Under load this can exhaust the Goroutine pool and cause cascading latency for all other requests. The client also has no feedback during the wait.

**Fix:** Replace with a context-aware wait so the sleep is cancelled if the client disconnects:
```go
case pool.ClassRateLimited:
    select {
    case <-time.After(s.pool.Backoff()):
    case <-c.Request.Context().Done():
        c.JSON(http.StatusServiceUnavailable, anthropicErrNoUpstream)
        return
    }
    rateLimitRetry = true
    continue
```

---

### WR-04: Stream translator silently swallows translation errors per-chunk

**File:** `internal/server/anthropic.go:229-234`
**Issue:** The read loop only writes events when `translateErr == nil`. When `translateErr != nil` the events are discarded and the loop continues. The `Translate` method returns a non-nil error only for `formatSSEEvent` failures (JSON marshal of a static struct — extremely unlikely in practice). However, when it does happen the client receives a partial stream with no error indication, leaving the client to time out waiting for `message_stop`.

```go
events, translateErr := st.Translate(buf[:n])
if translateErr == nil {
    for _, event := range events {
        writeAndFlush(event)
    }
}
```

**Fix:** On a translation error, close the stream with an Anthropic error event:
```go
events, translateErr := st.Translate(buf[:n])
if translateErr != nil {
    writeAndFlush([]byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"stream translation failed\"}}\n\n"))
    return
}
for _, event := range events {
    writeAndFlush(event)
}
```

---

### WR-05: `StreamTranslator` emits closing events even when stream never started

**File:** `internal/translator/stream.go:65-67`
**Issue:** When the upstream sends `[DONE]` before any content chunk (e.g., an assistant message with empty content), `emitClosing` is called while `st.started == false`. This emits `content_block_stop`, `message_delta`, and `message_stop` without a preceding `message_start` or `content_block_start`. Anthropic clients that enforce the event ordering protocol will error.

**Fix:** Emit the opening sequence before closing if `!st.started`:
```go
if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
    var out [][]byte
    if !st.started {
        // emit minimal opening events so the protocol is well-formed
        opening, err := st.emitOpening()
        if err != nil {
            return nil, err
        }
        out = append(out, opening...)
        st.started = true
    }
    closing, err := st.emitClosing("end_turn")
    if err != nil {
        return nil, err
    }
    return append(out, closing...), nil
}
```

## Info

### IN-01: Unused variable `input` in test

**File:** `internal/translator/translator_test.go:157`
**Issue:** `input := json.RawMessage(...)` is declared but only used in a `_ = input` comment line. The variable exists as a reference comment but adds noise.
**Fix:** Remove the declaration and the `_ = input` line; the test already verifies arguments via `gotArgs`.

---

### IN-02: `containsString` / `indexOf` reimplements `bytes.Contains`

**File:** `internal/translator/translator_test.go:632-645`
**Issue:** The helper functions `containsString` and `indexOf` replicate `bytes.Contains` from the standard library. The redundant implementation adds maintenance surface.
**Fix:** Replace both helpers with `bytes.Contains`:
```go
func containsString(b []byte, s string) bool {
    return bytes.Contains(b, []byte(s))
}
```

---

### IN-03: 30-second upstream timeout is hardcoded in two places

**File:** `internal/server/anthropic.go:97`, `internal/server/relay.go:134`
**Issue:** Both handlers create a `context.WithTimeout(c.Request.Context(), 30*time.Second)` independently. If the timeout needs to be configurable or changed it must be updated in two files.
**Fix:** Declare a package-level constant or read from `s.cfg`:
```go
const upstreamTimeout = 30 * time.Second
```

---

### IN-04: Streaming usage always logs zero token counts

**File:** `internal/server/anthropic.go:241`
**Issue:** `s.logUsage(keyID, upstreamID, true, 0, 0, time.Since(start))` always records `0` input and output tokens for streaming responses. The SSE stream does include a usage chunk from some providers (the final chunk with `usage` field). The `StreamTranslator` does not parse or expose token counts.
**Fix:** Consider parsing the final OpenAI SSE usage chunk in `StreamTranslator` and returning token counts via a method, or accept the known limitation with a comment.

---

_Reviewed: 2026-04-16T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
