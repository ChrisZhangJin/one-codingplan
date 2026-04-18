---
phase: 11-docker-deployment
reviewed: 2026-04-18T00:00:00Z
depth: standard
files_reviewed: 5
files_reviewed_list:
  - .env.example
  - Dockerfile
  - .dockerignore
  - config.yaml.example
  - docker-compose.yaml
findings:
  critical: 1
  warning: 3
  info: 2
  total: 6
status: issues_found
---

# Phase 11: Code Review Report

**Reviewed:** 2026-04-18
**Depth:** standard
**Files Reviewed:** 5
**Status:** issues_found

## Summary

Five deployment configuration files reviewed for the Docker deployment phase. The multi-stage Dockerfile is well-structured and the compose setup is functional. There is one critical issue: a non-existent Go base image tag that will cause all builds to fail. Three warnings cover a weak default secret in `config.yaml.example`, missing `healthcheck` in `docker-compose.yaml`, and unprotected host port binding. Two info items note the absence of a non-root runtime user and a missing `data/` directory declaration in compose.

---

## Critical Issues

### CR-01: Non-existent Go base image tag breaks all builds

**File:** `Dockerfile:10`
**Issue:** `golang:1.25.9-alpine` does not exist. As of April 2026 the latest stable Go release is 1.24.x; `1.25` is not yet released. The build stage will fail with `manifest unknown` when Docker pulls this image. The `go.mod` declares `go 1.25.0` (also unreleased), but Go module directives are forward-compatible, while Docker image tags must resolve exactly.
**Fix:**
```dockerfile
FROM golang:1.24-alpine AS go-builder
```
Use a minor-version floating tag (`1.24-alpine`) so patch updates are picked up automatically, or pin to the latest available patch (`golang:1.24.2-alpine`) for reproducibility. Also align `go.mod`:
```
go 1.24
```

---

## Warnings

### WR-01: Weak default `admin_key` in config.yaml.example ships as literal "change-me"

**File:** `config.yaml.example:3`
**Issue:** The value `"change-me"` is a well-known placeholder that operators frequently forget to replace. If a user copies this file verbatim and starts the container, the admin API is protected by a publicly known key. The application should refuse to start — or at minimum log a loud warning — when it detects this sentinel value, but that is a runtime concern. The config example itself should document the minimum entropy requirement and provide a generation command.
**Fix:**
```yaml
# Generate with: openssl rand -hex 32
admin_key: ""   # REQUIRED — must be set before running
```
Set the value to an empty string so the server fails fast at startup rather than accepting the placeholder silently.

### WR-02: No `healthcheck` defined in docker-compose.yaml

**File:** `docker-compose.yaml:8`
**Issue:** Without a `healthcheck`, Docker and any orchestrator (Swarm, Compose watch) cannot tell whether the `ocp` container is actually serving traffic. `restart: unless-stopped` will not restart a container that is running but hanging. The `ocp-cli` service has `depends_on: ocp` with no condition, so it may start before the server is ready.
**Fix:**
```yaml
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 10s
```
Then update the `ocp-cli` dependency:
```yaml
    depends_on:
      ocp:
        condition: service_healthy
```

### WR-03: Port binding `"8080:8080"` exposes the service on all host interfaces

**File:** `docker-compose.yaml:10`
**Issue:** `"8080:8080"` binds to `0.0.0.0`, making the service reachable from any network interface on the host. For a proxy that holds API keys this is a significant exposure if the host is internet-facing. The admin API and key management endpoints are served on the same port with no separate binding.
**Fix:**
```yaml
    ports:
      - "127.0.0.1:8080:8080"
```
Document that operators must remove the `127.0.0.1:` prefix (or configure a reverse proxy) when external access is needed.

---

## Info

### IN-01: Runtime container runs as root

**File:** `Dockerfile:20-26`
**Issue:** The final `alpine` stage has no `USER` directive, so the `ocp` binary runs as root inside the container. If the process is compromised the attacker has root inside the container. The binary only needs to write to `/data` (the SQLite path) and bind to a non-privileged port (8080).
**Fix:**
```dockerfile
RUN addgroup -S ocp && adduser -S ocp -G ocp
RUN mkdir -p /data && chown ocp:ocp /data
USER ocp
```
Add these lines before the `EXPOSE` directive.

### IN-02: `data/` volume directory is not declared in docker-compose.yaml

**File:** `docker-compose.yaml:11-13`
**Issue:** The compose file bind-mounts `./data:/data` but never creates the host directory. On first run Docker will create `./data` as root-owned, which conflicts with a non-root container user (IN-01 above) and surprises operators.
**Fix:** Add a named volume or document the required pre-run step:
```yaml
volumes:
  ocp-data:

services:
  ocp:
    volumes:
      - ocp-data:/data
      - ./config.yaml:/app/config.yaml:ro
```
Or add a setup note in the README instructing `mkdir -p data` before `docker compose up`.

---

_Reviewed: 2026-04-18_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
