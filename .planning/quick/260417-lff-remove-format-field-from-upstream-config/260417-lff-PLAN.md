---
phase: quick
plan: 260417-lff
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/models/models.go
  - internal/pool/pool.go
  - internal/config/config.go
  - cmd/ocp/serve.go
  - internal/server/anthropic.go
  - internal/server/admin.go
  - web/src/components/EditUpstreamDialog.tsx
  - internal/pool/pool_test.go
  - internal/server/anthropic_test.go
  - internal/server/e2e_test.go
  - internal/pool/probe_test.go
autonomous: true
must_haves:
  truths:
    - "POST /v1/messages always forwards raw Anthropic body to upstream /v1/messages without translation"
    - "POST /v1/chat/completions always forwards raw OpenAI body to upstream /v1/chat/completions without translation"
    - "No format field in admin API, pool structs, or portal UI"
    - "All existing tests pass (adapted to new passthrough-only behavior)"
  artifacts:
    - path: "internal/server/anthropic.go"
      provides: "Passthrough-only Anthropic relay"
    - path: "internal/pool/pool.go"
      provides: "Pool structs without Format field"
    - path: "web/src/components/EditUpstreamDialog.tsx"
      provides: "Edit dialog without format field"
---

<objective>
Remove the `Format` field from the upstream configuration model and all layers (pool, admin API, portal UI). Simplify `handleAnthropicRelay` to always passthrough raw Anthropic request body to upstream `/v1/messages` — no translation branch. Chinese AI providers support Anthropic format natively, so translation loses features (tool use, thinking, cache control).

Purpose: Eliminate unnecessary translation that degrades Anthropic-specific features; simplify the relay path to pure passthrough.
Output: Cleaned codebase with format field removed from all layers; all relay handlers are passthrough-only.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@internal/models/models.go
@internal/pool/pool.go
@internal/config/config.go
@cmd/ocp/serve.go
@internal/server/anthropic.go
@internal/server/admin.go
@web/src/components/EditUpstreamDialog.tsx

<interfaces>
<!-- Pool and model structs that will be modified -->

From internal/models/models.go:
```go
type Upstream struct {
    // ... other fields ...
    Format        string `gorm:"default:''"` // REMOVE this field
    ModelOverride string `gorm:"column:model_override;default:''"`
}
```

From internal/pool/pool.go:
```go
type UpstreamEntry struct {
    ID, Name, BaseURL, APIKey, ModelOverride string
    Format string // REMOVE
}

type UpstreamInfo struct {
    // ... other fields ...
    Format string `json:"format,omitempty"` // REMOVE
}

func (p *Pool) UpdateEntry(id uint, name, baseURL, apiKey, modelOverride, format string) // REMOVE format param
func (p *Pool) SetFormat(name, format string) // REMOVE entire method
```

From internal/server/admin.go:
```go
type patchUpstreamRequest struct {
    Format *string `json:"format"` // REMOVE
}
// handleUpdateUpstream references Format in updates map and pool.UpdateEntry call
```
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Remove Format from Go structs, pool, config, admin API, and relay handlers</name>
  <files>
    internal/models/models.go
    internal/pool/pool.go
    internal/config/config.go
    cmd/ocp/serve.go
    internal/server/anthropic.go
    internal/server/admin.go
  </files>
  <action>
1. **internal/models/models.go**: Remove the `Format string` field from the `Upstream` struct. Leave all other fields. The DB column will remain but GORM will simply ignore it.

2. **internal/pool/pool.go**:
   - Remove `Format string` from `UpstreamEntry` struct.
   - Remove `Format string` from `UpstreamInfo` struct.
   - In `New()`: remove `Format: u.Format` from the UpstreamEntry initialization (around line 69).
   - In `List()`: remove `Format: e.Format` from UpstreamInfo initialization (around line 166).
   - Change `UpdateEntry` signature from `(id uint, name, baseURL, apiKey, modelOverride, format string)` to `(id uint, name, baseURL, apiKey, modelOverride string)`. Remove `p.entries[i].Format = format` line inside.
   - Remove the entire `SetFormat` method (lines 249-258).

3. **internal/config/config.go**: Remove `Format string` field from `UpstreamConfig` struct.

4. **cmd/ocp/serve.go**: Remove the `if u.Format != "" { p.SetFormat(u.Name, u.Format) }` block (lines 56-58).

5. **internal/server/anthropic.go** — This is the critical change. Simplify `handleAnthropicRelay`:
   - Remove the `if current.Format == "anthropic"` / `else` branch structure in the request-forwarding loop. Keep ONLY the passthrough path (the `"anthropic"` branch body). All requests go to `current.BaseURL + "/v1/messages"` with the raw `bodyBytes`.
   - Remove the `translator` import if no longer used in this file.
   - In the success path (after status check), remove the format branching for stream/non-stream. For streaming: always call `s.proxyStream(c, resp, cancel, keyID, current.ID, start)`. For non-streaming: always call `s.proxyBuffer(c, resp, cancel, keyID, current.ID, start)`.
   - Remove the `proxyAnthropicBuffer` method entirely (it translates OpenAI response back to Anthropic — no longer needed).
   - Remove the `proxyAnthropicStream` method entirely (it translates OpenAI SSE to Anthropic SSE — no longer needed).
   - The `translator.AnthropicRequest` struct is still used to parse `req.Stream` and `req.Model`. Keep parsing the request for stream detection, but do NOT translate it. If `req.Model` is not needed downstream, simplify accordingly. Actually, keep `req.Stream` parsing so the handler knows whether to call `proxyStream` or `proxyBuffer`.

6. **internal/server/admin.go**:
   - Remove `Format *string` from `patchUpstreamRequest` struct.
   - In `handleUpdateUpstream`: remove the `if req.Format != nil` block that sets `updates["format"]`.
   - Update the `s.pool.UpdateEntry(...)` call to remove the `upstream.Format` argument (now 5 args instead of 6).
   - In the fallback response JSON at the end of `handleUpdateUpstream`, remove `"format": upstream.Format`.
  </action>
  <verify>
    <automated>cd /root/workspace/one-codingplan && go build ./...</automated>
  </verify>
  <done>
    - Format field removed from all Go structs (Upstream, UpstreamEntry, UpstreamInfo, UpstreamConfig, patchUpstreamRequest)
    - SetFormat method removed from pool
    - UpdateEntry has 5 params (no format)
    - handleAnthropicRelay always does passthrough to /v1/messages
    - proxyAnthropicBuffer and proxyAnthropicStream methods removed
    - No translation called from relay handlers
    - Code compiles cleanly
  </done>
</task>

<task type="auto">
  <name>Task 2: Update tests and portal UI to remove format references</name>
  <files>
    internal/pool/pool_test.go
    internal/server/anthropic_test.go
    internal/server/e2e_test.go
    internal/pool/probe_test.go
    web/src/components/EditUpstreamDialog.tsx
  </files>
  <action>
1. **internal/pool/pool_test.go**:
   - Remove `TestSetFormat` entirely (tests a method that no longer exists).
   - Update any `UpstreamEntry` literal that includes `Format:` field — remove that field.

2. **internal/server/anthropic_test.go**:
   - `TestAnthropicPassthrough_NonStream` (line ~595): Remove the `p.SetFormat("mimo", "anthropic")` call. The test should still work because passthrough is now the ONLY behavior. Update the test name if needed to reflect it's testing the standard path. Verify the test expects the request forwarded to `/v1/messages` with raw Anthropic body.
   - `TestAnthropicTranslate_NonStream` (line ~658): This test verifies the translate path which no longer exists. Convert it to test passthrough behavior OR delete it. Since passthrough is already tested above, DELETE this test.
   - The failover test at line ~706: Remove `p.SetFormat("up-500-anthropic", "anthropic")` call. Update to reflect that all upstreams now use passthrough. Both upstreams should have their requests sent to `/v1/messages`.

3. **internal/server/e2e_test.go**:
   - `TestE2E_Anthropic_Passthrough_FormatField` (line ~259): Remove `p.SetFormat("mimo", "anthropic")`. The test should still pass since passthrough is default.
   - Any test around line ~357 with `p.SetFormat`: remove the call. If the test was specifically testing translate-vs-passthrough branching, simplify to test passthrough only.

4. **internal/pool/probe_test.go**: Check `TestSendProbe_RequestFormat` (line 220). If it references the `Format` field on pool entries, update accordingly. This likely tests probe request format (HTTP), not the upstream Format config field — read the test first to confirm before changing.

5. **web/src/components/EditUpstreamDialog.tsx**:
   - Remove `format?: string` from the `UpstreamInfo` interface.
   - Remove the `const [format, setFormat] = useState('')` state.
   - Remove `setFormat(upstream.format ?? '')` from the useEffect.
   - Remove the format comparison and `body.format = format` from handleSubmit.
   - Remove the entire format input field div (Label + Input for "Format").
  </action>
  <verify>
    <automated>cd /root/workspace/one-codingplan && go test ./internal/pool/... ./internal/server/... -v -count=1 2>&1 | tail -30</automated>
  </verify>
  <done>
    - All tests pass with no references to Format field or SetFormat method
    - Translation-specific tests removed or converted to passthrough tests
    - Portal edit dialog has no format field
    - `go test ./...` passes
  </done>
</task>

</tasks>

<verification>
1. `go build ./...` — compiles without errors
2. `go test ./internal/pool/... -v` — all pool tests pass, no Format references
3. `go test ./internal/server/... -v` — all server tests pass, no translation paths exercised
4. `grep -rn "\.Format\b" internal/ cmd/ --include="*.go" | grep -v _test.go | grep -v translator/` — returns no matches (Format removed from all non-translator production code)
5. `grep -n "format" web/src/components/EditUpstreamDialog.tsx` — returns no matches
</verification>

<success_criteria>
- Format field completely removed from: Upstream model, UpstreamEntry, UpstreamInfo, UpstreamConfig, patchUpstreamRequest, EditUpstreamDialog
- SetFormat method removed from pool
- handleAnthropicRelay always passes through raw body to /v1/messages — no translation branch
- proxyAnthropicBuffer and proxyAnthropicStream removed (translation response handlers)
- translator package still exists but is not imported by relay handlers
- All tests pass
- Portal UI has no format field in edit dialog
</success_criteria>

<output>
After completion, create `.planning/quick/260417-lff-remove-format-field-from-upstream-config/260417-lff-SUMMARY.md`
</output>
