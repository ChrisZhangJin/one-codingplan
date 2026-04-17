---
phase: quick
plan: 260417-mdn
subsystem: pool/server
tags: [adapter, provider, url-construction, minimax, relay]
dependency_graph:
  requires: []
  provides: [pool.GetAdapter, pool.ProviderAdapter, pool.DefaultAdapter, pool.MinimaxAdapter]
  affects: [internal/server/anthropic.go, internal/server/relay.go]
tech_stack:
  added: []
  patterns: [registry pattern, adapter pattern]
key_files:
  created:
    - internal/pool/adapter.go
    - internal/pool/adapter_test.go
  modified:
    - internal/server/anthropic.go
    - internal/server/relay.go
decisions:
  - "Registry initialized via init() with MinimaxAdapter; GetAdapter returns DefaultAdapter for all unknown providers"
  - "MinimaxAdapter embeds DefaultAdapter so OpenAIURL delegation is automatic and DRY"
metrics:
  duration: 69s
  completed: "2026-04-17T16:10:08Z"
  tasks_completed: 2
  files_changed: 4
---

# Quick Task 260417-mdn: Provider Adapter Pattern Summary

**One-liner:** ProviderAdapter registry centralizes per-provider URL construction, with MinimaxAdapter overriding Anthropic path to `/anthropic/v1/messages`.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Create ProviderAdapter interface, implementations, registry, tests | d41e674 | internal/pool/adapter.go, internal/pool/adapter_test.go |
| 2 | Wire adapters into relay handlers | 307bed5 | internal/server/anthropic.go, internal/server/relay.go |

## What Was Built

- `ProviderAdapter` interface (`AnthropicURL`, `OpenAIURL` methods) in `internal/pool/adapter.go`
- `DefaultAdapter`: appends `/v1/messages` and `/v1/chat/completions` to trimmed base URL
- `MinimaxAdapter`: embeds `DefaultAdapter`, overrides `AnthropicURL` to append `/anthropic/v1/messages`
- `GetAdapter(provider string)` registry: maps `"minimax"` to `MinimaxAdapter`, all others fall back to `DefaultAdapter`
- Table-driven tests cover all adapter behaviors including trailing-slash edge case
- `handleAnthropicRelay`: replaced `up.BaseURL+"/v1/messages"` with `pool.GetAdapter(up.Name).AnthropicURL(up.BaseURL)`
- `handleRelay`: replaced `current.BaseURL+"/v1/chat/completions"` with `pool.GetAdapter(current.Name).OpenAIURL(current.BaseURL)`

## Verification

- `go build ./...` passes
- `go test ./internal/pool/...` passes (all 5 new adapter tests + existing pool tests)
- `go test ./internal/server/...` passes with no modifications
- `grep GetAdapter` confirms usage in both handler files

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints, auth paths, or trust boundary changes introduced.

## Self-Check: PASSED

- internal/pool/adapter.go: FOUND
- internal/pool/adapter_test.go: FOUND
- Commits d41e674 and 307bed5: verified via git log
