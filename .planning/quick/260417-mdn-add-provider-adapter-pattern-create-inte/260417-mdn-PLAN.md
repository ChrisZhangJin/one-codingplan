---
phase: quick
plan: 260417-mdn
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/pool/adapter.go
  - internal/pool/adapter_test.go
  - internal/server/anthropic.go
  - internal/server/relay.go
autonomous: true
requirements: []
must_haves:
  truths:
    - "Each provider resolves to the correct upstream URL per protocol"
    - "DefaultAdapter appends /v1/messages for Anthropic and /v1/chat/completions for OpenAI"
    - "MinimaxAdapter appends /anthropic/v1/messages for Anthropic, delegates OpenAI to default"
    - "Unknown providers fall back to DefaultAdapter"
    - "Relay handlers use adapter instead of hardcoded path concatenation"
  artifacts:
    - path: "internal/pool/adapter.go"
      provides: "ProviderAdapter interface, DefaultAdapter, MinimaxAdapter, registry, GetAdapter()"
      exports: ["ProviderAdapter", "DefaultAdapter", "MinimaxAdapter", "GetAdapter"]
    - path: "internal/pool/adapter_test.go"
      provides: "Unit tests for all adapter URL methods and registry lookup"
  key_links:
    - from: "internal/server/anthropic.go"
      to: "internal/pool/adapter.go"
      via: "pool.GetAdapter(up.Name).AnthropicURL(up.BaseURL)"
      pattern: "GetAdapter.*AnthropicURL"
    - from: "internal/server/relay.go"
      to: "internal/pool/adapter.go"
      via: "pool.GetAdapter(up.Name).OpenAIURL(up.BaseURL)"
      pattern: "GetAdapter.*OpenAIURL"
---

<objective>
Add a ProviderAdapter pattern so URL construction per provider is centralized and extensible, replacing hardcoded path concatenation in relay handlers.

Purpose: Minimax uses non-standard paths (`/anthropic/v1/messages` instead of `/v1/messages`). Other providers may have similar quirks. A registry of adapters makes this config-driven rather than scattered across handler code.

Output: `internal/pool/adapter.go` with interface + implementations + registry; updated relay handlers; tests.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@internal/pool/pool.go (UpstreamEntry struct, pool.Select returns *UpstreamEntry with Name, BaseURL)
@internal/pool/classifier.go (existing pool package pattern)
@internal/server/anthropic.go (handleAnthropicRelay — line 80: `up.BaseURL+"/v1/messages"`)
@internal/server/relay.go (handleRelay — line 152: `current.BaseURL+"/v1/chat/completions"`)
@internal/models/models.go (Upstream model — BaseURL field)

<interfaces>
<!-- Key types the executor needs -->

From internal/pool/pool.go:
```go
type UpstreamEntry struct {
    ID            uint
    Name          string
    BaseURL       string
    APIKey        string
    ModelOverride string
}
```

From internal/server/anthropic.go line 80 (to be replaced):
```go
up.BaseURL+"/v1/messages"
```

From internal/server/relay.go line 152 (to be replaced):
```go
current.BaseURL+"/v1/chat/completions"
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Create ProviderAdapter interface, implementations, and registry with tests</name>
  <files>internal/pool/adapter.go, internal/pool/adapter_test.go</files>
  <behavior>
    - DefaultAdapter.AnthropicURL("https://api.example.com") -> "https://api.example.com/v1/messages"
    - DefaultAdapter.OpenAIURL("https://api.example.com") -> "https://api.example.com/v1/chat/completions"
    - MinimaxAdapter.AnthropicURL("https://api.minimaxi.com") -> "https://api.minimaxi.com/anthropic/v1/messages"
    - MinimaxAdapter.OpenAIURL("https://api.minimaxi.com") -> "https://api.minimaxi.com/v1/chat/completions" (delegates to default)
    - GetAdapter("minimax") returns MinimaxAdapter
    - GetAdapter("kimi") returns DefaultAdapter (unknown provider falls back)
    - GetAdapter("") returns DefaultAdapter
    - BaseURL with trailing slash is handled: "https://api.example.com/" + "/v1/messages" does not produce double slash
  </behavior>
  <action>
Create `internal/pool/adapter.go`:

```go
// ProviderAdapter constructs the full upstream URL from a base URL for each protocol.
type ProviderAdapter interface {
    AnthropicURL(baseURL string) string
    OpenAIURL(baseURL string) string
}
```

`DefaultAdapter` struct (empty, methods use `strings.TrimRight(baseURL, "/")` + path).

`MinimaxAdapter` struct embedding `DefaultAdapter`. Overrides `AnthropicURL` to return `base/anthropic/v1/messages`. `OpenAIURL` delegates to embedded `DefaultAdapter`.

Package-level `var adapters = map[string]ProviderAdapter{}` registry, populated in `init()` with `"minimax": MinimaxAdapter{}`.

`func GetAdapter(provider string) ProviderAdapter` — looks up by provider name (lowercase), returns `DefaultAdapter{}` if not found.

Create `internal/pool/adapter_test.go` with table-driven tests covering all behaviors listed above.
  </action>
  <verify>
    <automated>cd /root/workspace/one-codingplan && go test ./internal/pool/ -run TestAdapter -v</automated>
  </verify>
  <done>All adapter tests pass. GetAdapter returns correct adapter per provider. URL construction is correct for both protocols, both adapters, and edge cases (trailing slash, empty provider).</done>
</task>

<task type="auto">
  <name>Task 2: Wire adapters into relay handlers</name>
  <files>internal/server/anthropic.go, internal/server/relay.go</files>
  <action>
In `internal/server/anthropic.go` line 80, replace:
```go
up.BaseURL+"/v1/messages"
```
with:
```go
pool.GetAdapter(up.Name).AnthropicURL(up.BaseURL)
```

In `internal/server/relay.go` line 152, replace:
```go
current.BaseURL+"/v1/chat/completions"
```
with:
```go
pool.GetAdapter(current.Name).OpenAIURL(current.BaseURL)
```

The `pool` package is already imported in both files (used for `pool.Classify`, `pool.ErrNoUpstreams`), so no new imports needed.

Do NOT change any other logic in these handlers. Minimal diff.
  </action>
  <verify>
    <automated>cd /root/workspace/one-codingplan && go build ./... && go test ./internal/server/ -v -count=1 2>&1 | tail -30</automated>
  </verify>
  <done>Both relay handlers use adapter-based URL construction. Project compiles. Existing server tests pass unchanged.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| adapter input | baseURL comes from DB (admin-controlled), provider name from DB — both trusted |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-q-01 | T (Tampering) | adapter.go | accept | baseURL is admin-written to DB via management API which already validates input; adapter only concatenates paths |
| T-q-02 | S (Spoofing) | GetAdapter | accept | provider name from DB, not user input; wrong adapter = wrong URL = upstream 404, not a security issue |
</threat_model>

<verification>
- `go build ./...` succeeds
- `go test ./internal/pool/ -run TestAdapter -v` all pass
- `go test ./internal/server/ -v -count=1` existing tests pass
- `grep -n "GetAdapter" internal/server/anthropic.go internal/server/relay.go` shows adapter usage in both handlers
- No hardcoded `/v1/messages` or `/v1/chat/completions` remain in anthropic.go or relay.go
</verification>

<success_criteria>
- ProviderAdapter interface exists with AnthropicURL and OpenAIURL methods
- DefaultAdapter handles standard providers (all current ones except minimax anthropic path)
- MinimaxAdapter overrides only the Anthropic path
- Registry returns correct adapter by provider name, defaults to DefaultAdapter
- Both relay handlers use GetAdapter instead of hardcoded string concatenation
- All existing tests pass with no modifications
</success_criteria>

<output>
After completion, create `.planning/quick/260417-mdn-add-provider-adapter-pattern-create-inte/260417-mdn-SUMMARY.md`
</output>
