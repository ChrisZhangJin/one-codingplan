---
phase: quick
plan: 260417-lff
subsystem: relay, pool, admin, portal
tags: [refactor, passthrough, format-removal, relay-simplification]
dependency_graph:
  requires: []
  provides: [passthrough-only-anthropic-relay, format-field-removed]
  affects: [internal/models, internal/pool, internal/server, internal/config, internal/database, web/portal]
tech_stack:
  added: []
  patterns: [passthrough-relay, unified-token-extraction]
key_files:
  created: []
  modified:
    - internal/models/models.go
    - internal/pool/pool.go
    - internal/config/config.go
    - internal/database/database.go
    - cmd/ocp/serve.go
    - internal/server/anthropic.go
    - internal/server/admin.go
    - internal/server/relay.go
    - internal/pool/pool_test.go
    - internal/server/anthropic_test.go
    - internal/server/e2e_test.go
    - web/src/components/EditUpstreamDialog.tsx
decisions:
  - "Remove Format field entirely from all layers — Chinese providers support Anthropic natively, translation degrades features"
  - "proxyBuffer token extraction extended to support both OpenAI and Anthropic usage formats"
metrics:
  duration: "~8 minutes"
  completed_date: "2026-04-17"
  tasks_completed: 2
  files_changed: 13
---

# Phase quick Plan 260417-lff: Remove Format Field from Upstream Config Summary

**One-liner:** Removed Format field from all layers and simplified Anthropic relay to pure passthrough — raw body forwarded to upstream /v1/messages, preserving tool use, thinking, and cache control.

## What Was Done

Removed the `Format` field that was used to branch between "translate Anthropic→OpenAI" and "passthrough" relay modes. Since Chinese AI providers support Anthropic format natively, translation was actively degrading features. The relay now always passes the raw Anthropic request body directly to upstream `/v1/messages`.

**Task 1: Go backend cleanup**

- `internal/models/models.go`: Removed `Format string` from `Upstream` struct
- `internal/pool/pool.go`: Removed `Format` from `UpstreamEntry`, `UpstreamInfo`, `UpdateEntry` signature (5 params), and removed the `SetFormat` method
- `internal/config/config.go`: Removed `Format` from `UpstreamConfig`
- `internal/database/database.go`: Removed `Format` from `SyncUpstreams` upsert columns
- `cmd/ocp/serve.go`: Removed `SetFormat` call block
- `internal/server/anthropic.go`: Rewrote `handleAnthropicRelay` as passthrough-only; removed `proxyAnthropicBuffer` and `proxyAnthropicStream` translation methods
- `internal/server/admin.go`: Removed `Format` from `patchUpstreamRequest` and `handleUpdateUpstream`

**Task 2: Tests and portal**

- `internal/pool/pool_test.go`: Removed `TestSetFormat`
- `internal/server/anthropic_test.go`: Updated non-stream/stream/failover/usage tests to use Anthropic-format fake upstreams; removed translation-specific tests (`ModelOverride`, `ModelStrip`, `ToolRoundTrip`); added `fakeAnthropicUpstreamServer` and `fakeAnthropicSSEUpstream` helpers
- `internal/server/e2e_test.go`: Replaced translate-path tests with passthrough tests; replaced `PassthroughVsTranslate` test with `BothUpstreams_ReceiveMessagesPath`; replaced `ModelOverride` test with `BodyForwardedVerbatim`
- `web/src/components/EditUpstreamDialog.tsx`: Removed `format` state, interface field, useEffect setter, handleSubmit comparison, and format input field div

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] database.go also referenced Format field**
- **Found during:** Task 1 build verification
- **Issue:** `internal/database/database.go` had `Format: u.Format` in `SyncUpstreams` and `"format"` in the upsert column list
- **Fix:** Removed both references from `database.go`
- **Files modified:** `internal/database/database.go`
- **Commit:** 60a1756

**2. [Rule 1 - Bug] proxyBuffer extracted tokens using only OpenAI format**
- **Found during:** Task 2 test run (TestAnthropicRelay_Usage failing)
- **Issue:** `proxyBuffer` parsed `prompt_tokens`/`completion_tokens` from upstream response, but passthrough returns Anthropic format with `input_tokens`/`output_tokens`
- **Fix:** Extended `chatResponse` struct with Anthropic fields; updated `proxyBuffer` to use whichever field is non-zero
- **Files modified:** `internal/server/relay.go`
- **Commit:** df79d5e

## Commits

| Hash | Message |
|------|---------|
| 60a1756 | refactor(quick-260417-lff): remove Format field and simplify relay to passthrough-only |
| df79d5e | refactor(quick-260417-lff): update tests and portal for passthrough-only relay |

## Known Stubs

None.

## Threat Flags

None — no new network endpoints, auth paths, or schema changes introduced.

## Self-Check

Files exist:
- internal/models/models.go: modified
- internal/pool/pool.go: modified
- internal/server/anthropic.go: rewritten
- web/src/components/EditUpstreamDialog.tsx: modified

Commits exist: verified above.
