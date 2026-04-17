---
phase: 06-web-portal
plan: 01
subsystem: web-portal
tags: [react, vite, tailwind, shadcn, go-embed, spa]
dependency_graph:
  requires: []
  provides: [web-scaffold, spa-embed, makefile-build]
  affects: [internal/server/server.go]
tech_stack:
  added:
    - react 19
    - vite 8
    - tailwindcss 4 (@tailwindcss/vite)
    - shadcn/ui (base-nova, neutral theme)
    - lucide-react
  patterns:
    - go:embed all:web_dist for SPA binary embedding
    - gin r.NoRoute for SPA catch-all fallback
    - Vite dev proxy to Go backend on localhost:8080
key_files:
  created:
    - web/package.json
    - web/vite.config.ts
    - web/tsconfig.json
    - web/tsconfig.app.json
    - web/tsconfig.node.json
    - web/index.html
    - web/src/main.tsx
    - web/src/App.tsx
    - web/src/index.css
    - web/src/lib/utils.ts
    - web/src/components/ui/button.tsx
    - web/src/components/ui/card.tsx
    - web/src/components/ui/input.tsx
    - web/src/components/ui/label.tsx
    - web/src/components/ui/sonner.tsx
    - web/components.json
    - internal/server/spa.go
    - internal/server/web_dist/.gitkeep
    - Makefile
  modified:
    - internal/server/server.go
    - .gitignore
decisions:
  - "Used ignoreDeprecations: 6.0 in tsconfig.app.json to allow baseUrl alongside paths aliases required by shadcn"
  - "web_dist directory lives inside internal/server/ so go:embed directive resolves relative to source file"
  - "Removed web/.git (created by Vite scaffold) to prevent submodule tracking in main repo"
  - "shadcn base-nova style selected by -d flag defaults; neutral color theme"
metrics:
  duration: "~15 minutes"
  completed: "2026-04-17"
  tasks_completed: 2
  tasks_total: 2
  files_created: 19
  files_modified: 2
---

# Phase 06 Plan 01: React+Vite+shadcn Scaffold and Go Embed — Summary

**One-liner:** React+Vite+Tailwind+shadcn/ui project scaffolded in `web/`, wired to Go binary via `go:embed all:web_dist` with SPA catch-all handler and Makefile build pipeline.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Scaffold React+Vite+shadcn in web/ | 00c9f2b | web/package.json, web/vite.config.ts, web/src/index.css, web/components.json, web/src/lib/utils.ts, shadcn ui components |
| 2 | Wire Go embed + SPA serving + Makefile | 9ecfd02 | internal/server/spa.go, internal/server/server.go, Makefile |

## Verification Results

- `cd web && npm run build` exits 0, produces `web/dist/index.html`
- `go build ./...` exits 0 with only `.gitkeep` in `web_dist/`
- All shadcn components (button, card, input, label, sonner) installed
- Dev proxy configured: `/api`, `/v1`, `/health` -> `localhost:8080`
- SPA handler skips `/api`, `/v1`, `/health` paths — existing routes unaffected

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] tsconfig missing path aliases for shadcn init**
- **Found during:** Task 1
- **Issue:** `npx shadcn@latest init -d` failed with "No import alias found in your tsconfig.json file" because the Vite template does not include `paths` in tsconfig
- **Fix:** Added `baseUrl` and `paths: {"@/*": ["./src/*"]}` to both `tsconfig.json` and `tsconfig.app.json`; added `"ignoreDeprecations": "6.0"` to suppress TS 7.0 deprecation warning for `baseUrl`
- **Files modified:** `web/tsconfig.json`, `web/tsconfig.app.json`
- **Commit:** 00c9f2b

**2. [Rule 3 - Blocking] Vite scaffold created embedded .git directory**
- **Found during:** Task 1 commit
- **Issue:** `npm create vite` initialized a `.git` directory inside `web/`, causing git to track it as a submodule (160000 mode) rather than regular files
- **Fix:** Removed `web/.git`, staged individual source files explicitly; amended commit to proper file tracking
- **Files affected:** All files under `web/`
- **Commit:** 00c9f2b (corrected from 8ea6c57)

## Known Stubs

- `web/src/App.tsx` renders a placeholder `<p>ocp portal</p>` — intentional scaffold stub, will be replaced in Plans 02-03 with login and dashboard UI

## Threat Flags

No new security-relevant surface introduced beyond what the plan's threat model covers. SPA is served from embedded FS (compile-time integrity). No VITE_-prefixed env vars used — no secrets in bundle.

## Self-Check: PASSED

All created files exist on disk. Both task commits (00c9f2b, 9ecfd02) verified in git log. `go build ./...` exits 0.
