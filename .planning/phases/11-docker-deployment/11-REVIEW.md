---
phase: 11-docker-deployment
reviewed: 2026-04-18T00:00:00Z
depth: standard
files_reviewed: 5
files_reviewed_list:
  - .dockerignore
  - .env.example
  - Dockerfile
  - config.yaml.example
  - docker-compose.yaml
findings:
  critical: 1
  warning: 3
  info: 3
  total: 7
status: issues_found
---

# Phase 11: Code Review Report

**Reviewed:** 2026-04-18
**Depth:** standard
**Files Reviewed:** 5
**Status:** issues_found

## Summary

Five Docker deployment artifacts reviewed. The multi-stage Dockerfile is well-structured: correct pure-Go SQLite driver for Alpine (no CGo), GOPROXY and npmmirror mirrors are appropriate for the China deployment target, and secrets are kept out of the image via environment variable injection. One critical issue exists — a non-existent Go toolchain version that will cause every build to fail with a manifest-not-found error. Three warnings address a weak default admin key, missing container healthcheck, and all-interface port binding. Three info items note the root runtime user, missing `data/` directory creation, and `OCP_ADMIN_KEY` not wired to the server service.

---

## Critical Issues

### CR-01: Non-existent Go base image tag `golang:1.25-alpine` breaks all builds

**File:** `Dockerfile:10`
**Issue:** `golang:1.25-alpine` does not exist on Docker Hub. Go 1.25 has not been released; the latest stable series is 1.24.x. `docker build` will fail with `manifest unknown` when pulling this image from a clean environment. The `go.mod` also declares `go 1.25.0` for the same reason — this is a pre-release version number.
**Fix:**
```dockerfile
FROM golang:1.24-alpine AS go-builder
```
Align `go.mod` to match:
```
go 1.24
```
Use `1.24-alpine` (minor-version floating) for automatic patch updates, or pin to a specific patch (`golang:1.24.2-alpine`) for full reproducibility.

---

## Warnings

### WR-01: Weak default `admin_key` in `config.yaml.example` ships as literal `"change-me"`

**File:** `config.yaml.example:3`
**Issue:** `admin_key: "change-me"` is a publicly known placeholder. Operators who copy this file verbatim will expose the admin API with a well-known credential. The server should refuse to start or log a prominent warning when it detects this sentinel value at runtime.
**Fix:** Replace the placeholder with an empty string and a generation hint so the server fails fast rather than accepting the sentinel silently:
```yaml
# Generate with: openssl rand -hex 32
admin_key: ""   # REQUIRED — must be set before running
```

### WR-02: No `healthcheck` in `docker-compose.yaml`; `ocp-cli` races the server

**File:** `docker-compose.yaml:8`, `docker-compose.yaml:23`
**Issue:** Without a `healthcheck`, Docker cannot distinguish a running-but-not-ready container from a healthy one. `restart: unless-stopped` will not restart a hung process. The `ocp-cli` service uses `depends_on: ocp` with no condition, meaning it may issue CLI commands before the server is listening on port 8080.
**Fix:**
```yaml
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 10s
```
Update the `ocp-cli` dependency accordingly:
```yaml
    depends_on:
      ocp:
        condition: service_healthy
```

### WR-03: Port binding `"8080:8080"` exposes the service on all host interfaces

**File:** `docker-compose.yaml:10`
**Issue:** `"8080:8080"` binds to `0.0.0.0`, making the admin API and key management endpoints reachable from any network interface on the host. For a service that stores upstream API keys this is a significant exposure if the host is internet-facing.
**Fix:**
```yaml
    ports:
      - "127.0.0.1:8080:8080"
```
Document that operators should remove `127.0.0.1:` or configure a reverse proxy when external access is intentionally needed.

---

## Info

### IN-01: Runtime container runs as root

**File:** `Dockerfile:20-26`
**Issue:** The final `alpine` stage has no `USER` directive. The `ocp` binary runs as root inside the container. The binary only needs to write to `/data` and bind to the non-privileged port 8080.
**Fix:**
```dockerfile
RUN addgroup -S ocp && adduser -S ocp -G ocp
RUN mkdir -p /data && chown ocp:ocp /data
USER ocp
```
Add these lines before the `EXPOSE` directive.

### IN-02: `./data` host directory is not created before bind mount

**File:** `docker-compose.yaml:11-12`
**Issue:** The bind mount `./data:/data` causes Docker to create `./data` as a root-owned directory on first run when it does not exist. If a non-root user is added inside the container (IN-01), the database write will fail on first start.
**Fix:** Either switch to a named volume which Docker manages with correct ownership:
```yaml
volumes:
  ocp-data:

services:
  ocp:
    volumes:
      - ocp-data:/data
      - ./config.yaml:/app/config.yaml:ro
```
Or document a required pre-run step (`mkdir -p data`) in operator instructions.

### IN-03: `OCP_ADMIN_KEY` is not injected into the `ocp` server service

**File:** `docker-compose.yaml:14-16`
**Issue:** `.env.example` defines `OCP_ADMIN_KEY` and `docker-compose.yaml` passes it to `ocp-cli`, but not to the main `ocp` server. If the server reads the admin key from the `OCP_ADMIN_KEY` environment variable, operators who rely on `.env` for secrets rather than the config file will find the admin API inaccessible. Currently the only way to set a non-placeholder admin key is to edit `config.yaml` directly.
**Fix:** Either expose the variable to the server service:
```yaml
    environment:
      OCP_ENCRYPTION_KEY: ${OCP_ENCRYPTION_KEY}
      OCP_ADMIN_KEY: ${OCP_ADMIN_KEY}
```
Or explicitly document in `config.yaml.example` that `admin_key` must be set there and cannot be overridden by environment variable.

---

_Reviewed: 2026-04-18_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
