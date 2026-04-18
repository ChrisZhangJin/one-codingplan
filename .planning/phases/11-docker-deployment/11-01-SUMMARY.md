---
phase: 11-docker-deployment
plan: "01"
subsystem: deployment
tags: [docker, dockerfile, docker-compose, config, deployment]
dependency_graph:
  requires: []
  provides: [docker-build, docker-compose-run, persistent-volume, cli-compose]
  affects: [Dockerfile, docker-compose.yaml, config.yaml.example, .dockerignore, .env.example]
tech_stack:
  added: []
  patterns: [multi-stage-docker-build, volume-persistence, compose-profiles]
key_files:
  created:
    - .env.example
  modified:
    - Dockerfile
    - .dockerignore
    - config.yaml.example
    - docker-compose.yaml
decisions:
  - "Split Dockerfile ENTRYPOINT/CMD so compose command override works correctly"
  - "Added .env.example to document required env vars"
  - "config.yaml.example uses /data/ocp.db to match the ./data:/data volume mount"
metrics:
  duration: "~10 minutes"
  completed: "2026-04-18T11:31:37Z"
  tasks_completed: 2
  tasks_total: 3
  files_changed: 5
---

# Phase 11 Plan 01: Docker Deployment Summary

**One-liner:** Fixed Dockerfile ENTRYPOINT/CMD split, .dockerignore secrets exclusions, db path for volume persistence, and compose depends_on — all deployment config artifacts are now correct for `docker compose up`.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Fix Dockerfile and .dockerignore | 32e1e46 | Dockerfile, .dockerignore |
| 2 | Fix config, compose, and env | 1b8cfe9 | config.yaml.example, docker-compose.yaml, .env.example |
| 3 | Build and verify (env limitation) | N/A | No files modified — verification only |

## What Was Built

**Task 1 — Dockerfile and .dockerignore fixes:**
- Dockerfile: Split `ENTRYPOINT ["ocp", "serve"]` into `ENTRYPOINT ["ocp"]` + `CMD ["serve"]`. This is required because `docker-compose.yaml` passes `command: ["serve", "--config", "/app/config.yaml"]` which replaces CMD (not ENTRYPOINT). The old setup produced `ocp serve serve --config /app/config.yaml` — wrong.
- .dockerignore: Added `*.db`, `config.yaml`, `.env`, `.git`, `run_ocp.sh` to prevent secrets and large files from entering the build context.

**Task 2 — Config, compose, and env fixes:**
- `config.yaml.example`: Changed `database.path` from `"./ocp.db"` to `"/data/ocp.db"` to match the `./data:/data` volume mount in docker-compose.yaml. Added logging section. Removed example upstream entries (cleaner template).
- `docker-compose.yaml`: Added `depends_on: [ocp]` to the `ocp-cli` service so it waits for the ocp server container to start before the CLI runs.
- `.env.example`: Created new file documenting the two required env vars (`OCP_ENCRYPTION_KEY`, `OCP_ADMIN_KEY`) with placeholder values.

**Task 3 — Docker verification:**
Task 3 is verification-only (no file modifications). The Docker daemon is not available in the build/execution environment (container without Docker-in-Docker). All code artifacts have been verified correct by static inspection and grep checks. Full end-to-end verification (docker build + docker compose up + curl /health + docker compose run ocp-cli status) must be run on a host with Docker installed.

## Deviations from Plan

### Environment Limitation

**Task 3 (Docker verification) could not run end-to-end**
- **Found during:** Task 3 execution
- **Issue:** `docker` command not found in execution environment (running inside a container without Docker daemon access)
- **Impact:** DOCK-01 through DOCK-04 runtime verification not performed; static correctness of all config artifacts verified manually
- **Resolution required:** Run `docker build -t one-codingplan:latest . && docker compose up -d && curl http://localhost:8080/health` on a host with Docker installed

### No Other Deviations

The plan's static fix instructions were applied exactly as specified. No Rule 1/2/3 auto-fixes were needed.

## Known Stubs

None — no placeholder data flows or stub components introduced.

## Threat Flags

No new security surface introduced beyond what is covered in the plan's threat model (T-11-01 through T-11-05). All mitigations applied:
- T-11-01: config.yaml and .env added to .dockerignore
- T-11-03: config.yaml mounted read-only (`:ro`) in docker-compose.yaml — already present, preserved

## Self-Check: PASSED

- FOUND: .planning/phases/11-docker-deployment/11-01-SUMMARY.md
- FOUND: commit 32e1e46 (Task 1 — Dockerfile + .dockerignore)
- FOUND: commit 1b8cfe9 (Task 2 — config + compose + .env.example)
- Dockerfile verified: `golang:1.25-alpine`, `ENTRYPOINT ["ocp"]`, `CMD ["serve"]`
- .dockerignore verified: config.yaml, .env, .git, *.db all excluded
- config.yaml.example verified: `/data/ocp.db` path
- docker-compose.yaml verified: ocp-cli has `depends_on: [ocp]`
- .env.example verified: OCP_ENCRYPTION_KEY and OCP_ADMIN_KEY present
