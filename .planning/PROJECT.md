# one-codingplan (ocp)

## What This Is

one-codingplan (ocp) aggregates multiple AI coding plan credentials (Minimax, Kimi, Xiao, GLM, Qwen and others) behind a single OpenAI-compatible and Anthropic-compatible endpoint. Users point their tools — Claude Code, Codex, or any API client — at ocp using one URL and one key, and ocp handles routing, failover, and credit tracking transparently.

## Core Value

A single endpoint that never goes down: when one upstream coding plan runs out of credits or hits a rate limit, ocp silently rotates to the next available one.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Expose OpenAI-compatible and Anthropic-compatible proxy endpoints
- [ ] Support upstream plans: Minimax, Kimi, Xiao, GLM, Qwen (extensible)
- [ ] Auto-switch upstream on: credits exhausted, rate limit hit, upstream error/timeout
- [ ] Round-robin across allowed upstreams per access key (all plans if unrestricted, allowed subset if restricted)
- [ ] Track upstream plan health: poll balance APIs where available, detect from errors where not
- [ ] Issue and manage access keys with configurable limits: token budget, allowed upstreams, rate limit (req/min and req/day), expiry date
- [ ] Block and unblock access keys
- [ ] Web portal: dashboard showing upstream status, key management, usage stats
- [ ] CLI (`ocp`): `ocp status`, `ocp next`, `ocp keys` commands
- [ ] Claude Code skill and Codex hook: `/proxy-status`, `/proxy-next` slash commands that call proxy management API

### Out of Scope

- Reselling or billing — this is a personal/team routing layer, not a commercial gateway
- Supporting non-OpenAI/non-Anthropic API formats — all upstreams expose compatible APIs
- Pinning a session to a specific upstream — force-next only (not pin-to-plan)

## Context

- User currently manages multiple Chinese AI provider subscriptions manually, switching config files each time a plan is exhausted — this is the pain being eliminated
- All upstream providers expose OpenAI-compatible or Anthropic-compatible APIs (same request/response format, different base URLs and keys)
- ocp runs as a self-hosted service (Docker container environment, China network context — GFW applies)
- Claude Code supports custom skills (slash commands backed by shell scripts); Codex has similar hooks — both can `curl` the proxy management API
- Unknown: which upstream providers expose a balance/credits API — research required to determine polling strategy per provider

## Constraints

- **Network**: Deployed in China — upstream API calls may need proxy configuration for some providers
- **Compatibility**: Must accept requests formatted for both OpenAI and Anthropic APIs and forward correctly to whichever upstream is active
- **Extensibility**: New upstream providers must be addable without code changes to routing logic (config-driven)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Round-robin (not priority-list) failover | Simpler UX — user doesn't need to rank plans; keys define allowed set | — Pending |
| Single force-next command (not pin-to-plan) | Keeps CLI surface minimal; most sessions don't need pinning | — Pending |
| Both web portal + CLI | Web for visibility and key management; CLI for quick ops from terminal | — Pending |
| Per-key allowed upstreams restrict the round-robin pool | Natural model: unrestricted key = all plans, restricted key = subset | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-16 after initialization*
