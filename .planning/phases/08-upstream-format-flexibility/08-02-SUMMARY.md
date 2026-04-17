---
phase: 08-upstream-format-flexibility
plan: "02"
subsystem: pool/classifier, server/relay
tags: [error-classification, failover, model-not-supported, circuit-breaker]
dependency_graph:
  requires: []
  provides: [ClassModelNotSupported constant, 5xx model-error classification, relay handler model-error failover]
  affects: [internal/pool/classifier.go, internal/server/anthropic.go, internal/server/relay.go]
tech_stack:
  added: []
  patterns: [keyword-based error classification, 5xx status gate, iota error class constant]
key_files:
  created: []
  modified:
    - internal/pool/classifier.go
    - internal/pool/classifier_test.go
    - internal/server/anthropic.go
    - internal/server/relay.go
decisions:
  - "Model/config error check placed BEFORE credits keyword loop so 5xx model errors are not misclassified as transient"
  - "5xx gate prevents 4xx bodies with model keywords from triggering permanent upstream disable"
  - "ClassModelNotSupported treated identically to ClassCreditsExhausted in both relay handlers (Mark unavailable, rotate)"
metrics:
  duration: 10m
  completed_date: "2026-04-17T07:52:29Z"
  tasks_completed: 2
  tasks_total: 2
  files_changed: 4
---

# Phase 08 Plan 02: ClassModelNotSupported Error Classifier Summary

**One-liner:** Keyword-based 5xx model-error classifier that permanently marks misconfigured upstreams unavailable, stopping retry storms from unsupported-model responses.

## What Was Built

Added `ClassModelNotSupported` as the fourth `ErrorClass` iota constant in `internal/pool/classifier.go`. Upstreams returning 5xx responses whose bodies contain model/config error keywords (e.g., "not support model", "invalid model", "model does not exist") are now classified as `ClassModelNotSupported` rather than `ClassTransient`.

Both relay handlers (`handleAnthropicRelay` in `anthropic.go` and `handleRelay` in `relay.go`) now handle this class identically to `ClassCreditsExhausted` — calling `s.pool.Mark(current.ID, false)` to permanently disable the upstream for the session and rotating to the next available one.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add ClassModelNotSupported to classifier with keyword detection | 7e168c1 | internal/pool/classifier.go, internal/pool/classifier_test.go |
| 2 | Handle ClassModelNotSupported in both relay handlers | a59aa5a | internal/server/anthropic.go, internal/server/relay.go |

## Decisions Made

- **5xx gate is a correctness requirement (T-8-06 mitigation):** Without the `status >= 500` guard, a user-visible 400 error body containing a model-related word could silently disable a healthy upstream. The gate ensures only server-side config errors trigger permanent disabling.
- **Model check before credits check:** The `modelNotSupportedKeywords` loop runs before the credits keyword loop so that a 5xx body matching a model keyword is classified as `ClassModelNotSupported` even if it also contains a credits-adjacent word.
- **Identical handling to ClassCreditsExhausted:** A misconfigured upstream that repeatedly returns "not support model" should be treated the same as a credits-exhausted one — disabled immediately, not retried.

## Deviations from Plan

None - plan executed exactly as written.

## Verification Results

```
go build ./...          # exit 0
go test ./internal/pool/... -run TestClassify -v -count=1  # all 13 cases pass
go test ./... -count=1  # all packages pass
grep ClassModelNotSupported internal/server/anthropic.go internal/server/relay.go  # appears in both
grep -c "pool.Mark(current.ID, false)" internal/server/anthropic.go  # 2
grep -c "pool.Mark(current.ID, false)" internal/server/relay.go  # 2
```

## Known Stubs

None.

## Threat Flags

None - no new network endpoints, auth paths, or trust boundaries introduced. The `Classify` function receives already-limited (64KB) upstream response bodies as before.

## Self-Check: PASSED

- `internal/pool/classifier.go` — exists, contains `ClassModelNotSupported`, `modelNotSupportedKeywords`, `status >= 500` gate
- `internal/pool/classifier_test.go` — exists, contains new model/config test cases
- `internal/server/anthropic.go` — exists, contains `case pool.ClassModelNotSupported`
- `internal/server/relay.go` — exists, contains `case pool.ClassModelNotSupported`
- Commit 7e168c1 — exists
- Commit a59aa5a — exists
