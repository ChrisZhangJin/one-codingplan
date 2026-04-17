# Phase 7: CLI - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-17
**Phase:** 07-cli
**Areas discussed:** CLI framework, Server start behavior, Connection config, Output format

---

## CLI framework

| Option | Description | Selected |
|--------|-------------|----------|
| Add Cobra | github.com/spf13/cobra — auto help, clean subcommand structure, standard for Go CLIs | ✓ |
| Plain os.Args dispatch | No new dep, manual switch on os.Args[1], simpler for 3 commands | |
| You decide | Claude picks | |

**User's choice:** Add Cobra
**Notes:** User asked for clarification on what Cobra is before selecting.

---

## Server start behavior

| Option | Description | Selected |
|--------|-------------|----------|
| ocp serve | Server becomes `ocp serve`, bare `ocp` shows help. Clean Cobra convention. | ✓ |
| bare ocp still starts server | No args = start server (current behavior). Root command RunE handles it. | |
| You decide | Claude picks | |

**User's choice:** `ocp serve`
**Notes:** Accepted the breaking change to start command for cleaner Cobra structure.

---

## Connection config

| Option | Description | Selected |
|--------|-------------|----------|
| Flags + env vars | --host and --admin-key flags with OCP_HOST / OCP_ADMIN_KEY env var fallback | ✓ |
| Config file via Viper | ~/.ocp/config.yaml — Viper already in go.mod | |
| Env vars only | OCP_HOST and OCP_ADMIN_KEY must be set, no flags | |

**User's choice:** Flags + env vars
**Notes:** No config file in this phase — keep it simple.

---

## Output format

| Option | Description | Selected |
|--------|-------------|----------|
| Plain tabwriter table | stdlib text/tabwriter, no color, no deps | ✓ |
| Plain table + --json flag | Same default + --json for scripting | |
| Colored table output | fatih/color or similar — adds dep, breaks in non-TTY | |

**User's choice:** Plain tabwriter table
**Notes:** Functional over decorative.

---

## Claude's Discretion

- Whether persistent flags live on root command or per-subcommand
- Exact Cobra project layout (cmd/ocp/cmd/ sub-package vs inline)
- Precise tabwriter column widths and padding
- Viper BindEnv vs manual os.Getenv for env var fallback

## Deferred Ideas

None.
