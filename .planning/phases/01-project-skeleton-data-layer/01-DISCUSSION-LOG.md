# Phase 1: Project Skeleton & Data Layer - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-16
**Phase:** 01-project-skeleton-data-layer
**Areas discussed:** Upstream config entry point, SOCKS5 scope (removed), Server config loading

---

## Upstream Config Entry Point

| Option | Description | Selected |
|--------|-------------|----------|
| YAML config file | Upstream definitions in config.yaml, loaded at startup | ✓ |
| Seed API endpoint | POST /api/upstreams for Phase 1 seeding | |
| DB seeding only | No user-facing config in Phase 1, direct DB fixtures | |

**User's choice:** YAML config file

---

| Option | Description | Selected |
|--------|-------------|----------|
| Upstreams only in YAML; server settings via env vars | Clean separation of provider data vs runtime config | |
| Everything in one YAML file | Server settings + upstream definitions in one file | ✓ |

**User's choice:** Everything in one YAML file

---

## SOCKS5 Scope

**User's decision:** Removed entirely — no SOCKS5 support needed. UPST-04 dropped from Phase 1 scope.

---

## Server Config Loading

| Option | Description | Selected |
|--------|-------------|----------|
| Viper from day one | Reads config.yaml + env var overrides; reused by Phase 7 CLI | ✓ |
| Simple yaml.Unmarshal | go-yaml direct unmarshal, no Viper until Phase 7 | |

**User's choice:** Viper from day one

---

| Option | Description | Selected |
|--------|-------------|----------|
| --config flag + default ./config.yaml | Flexible, standard Go service pattern | ✓ |
| Fixed path only (./config.yaml) | Simpler, less flexible for Docker | |

**User's choice:** --config flag + default ./config.yaml

---

## Claude's Discretion

- Go project layout (cmd/ocp/, internal/)
- Gin router and middleware structure
- YAML config schema field names
- GORM model struct design

## Deferred Ideas

- SOCKS5 per-upstream proxy (UPST-04) — removed by user
- Seed API endpoint — Phase 5 management API
- Config hot-reload — not needed now
