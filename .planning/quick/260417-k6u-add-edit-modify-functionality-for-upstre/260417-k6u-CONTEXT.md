---
name: 260417-k6u context
description: Edit/modify feature decisions for upstreams and access keys
type: project
---

# Quick Task 260417-k6u: Add edit/modify functionality for upstreams and access keys - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning

<domain>
## Task Boundary

Add edit/modify functionality for upstreams and access keys: backend PATCH endpoints + portal UI edit dialogs

</domain>

<decisions>
## Implementation Decisions

### Upstream editable fields
- All fields: name, base_url, api_key, format, model_override
- Same field set as add/seed — full control over the upstream

### Key editable fields
- name, token_budget, rate_limit_per_minute, rate_limit_per_day, allowed_upstreams, expires_at
- Token value itself is NOT regenerated or changed

### Edit UI pattern
- Modal dialog for both upstreams and keys
- Consistent with existing CreateKeyDialog pattern
- Edit button opens dialog pre-populated with current values

### API key display in upstream edit dialog
- Show masked current key as placeholder (e.g. "•••abc")
- Leave blank = keep existing key unchanged
- Type new value = replace the key
- Never expose the actual key value in the response

### Claude's Discretion
- Backend endpoint shape: PATCH /api/upstreams/:id and PATCH /api/keys/:id
- Pool in-memory state update after upstream edit (name, base_url, model_override need to sync)
- Dialog component naming and file placement

</decisions>

<specifics>
## Specific Ideas

- Upstream edit: if api_key field is empty in PATCH body, keep existing encrypted key
- Pool must update in-memory entry when upstream is edited (name/baseURL/modelOverride/format may change)
- Key edit dialog reuses same form fields as CreateKeyDialog
- Masked key placeholder: use the existing maskToken() helper output from GET /api/upstreams response (but upstreams currently don't return a masked key — may need to add masked_key field to UpstreamInfo)

</specifics>

<canonical_refs>
## Canonical References

- internal/server/admin.go — existing PATCH key handler pattern (handlePatchKey), toggle handler
- internal/pool/pool.go — pool.SetEnabled, need similar Update method
- web/src/components/CreateKeyDialog.tsx — dialog pattern to follow
- web/src/components/UpstreamStatus.tsx — upstream card where edit button goes
- web/src/components/KeyTable.tsx — key table where edit button goes

</canonical_refs>
