---
phase: 06-web-portal
plan: 02
subsystem: web-portal
tags: [react, typescript, shadcn, auth, localStorage, login, dashboard]
dependency_graph:
  requires: [06-01]
  provides: [login-page, auth-context, api-client, dashboard-shell]
  affects: [web/src/App.tsx, web/src/lib/auth.ts, web/src/lib/api.ts]
tech_stack:
  added: []
  patterns:
    - localStorage token for operator console auth (D-01)
    - Raw fetch for login validation to avoid 401 redirect loop
    - apiFetch wrapper with Bearer header and 401 clearToken+redirect (D-03)
    - State-based routing (loggedIn boolean) instead of React Router
key_files:
  created:
    - web/src/lib/auth.ts
    - web/src/lib/api.ts
    - web/src/pages/LoginPage.tsx
    - web/src/pages/DashboardPage.tsx
    - web/src/components/ui/separator.tsx
  modified:
    - web/src/App.tsx
    - web/src/lib/api.ts
    - web/tsconfig.json
decisions:
  - "Used raw fetch (not apiFetch) for login validation to avoid 401 redirect loop during credential check"
  - "State-based routing via useState(!!getToken()) — no React Router needed for two-state app"
  - "ApiError class uses explicit property assignment instead of parameter properties to satisfy erasableSyntaxOnly"
metrics:
  duration: "~3 minutes"
  completed: "2026-04-17"
  tasks_completed: 2
  tasks_total: 2
  files_created: 5
  files_modified: 3
---

# Phase 06 Plan 02: Login Page, Auth Context, API Client, Dashboard Shell — Summary

**One-liner:** localStorage-based auth with login page (360px card, password input, inline error), API fetch wrapper with Bearer token and 401 redirect, and dashboard shell with Upstream Status and Access Keys section headings.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Auth helpers and API client | d156b87 | web/src/lib/auth.ts, web/src/lib/api.ts, web/tsconfig.json |
| 2 | Login page and Dashboard shell with routing | 86ba9c6 | web/src/pages/LoginPage.tsx, web/src/pages/DashboardPage.tsx, web/src/App.tsx, web/src/components/ui/separator.tsx |

## Verification Results

- `npm run build` exits 0, produces `dist/index.html`
- All 16 acceptance criteria pass (ocp_admin_key, Authorization, clearToken, redirect, title, Sign in, error text, password input, 360px width, Upstream Status, Access Keys, getToken, LoginPage, DashboardPage, Toaster, no react-router)
- TypeScript compiles without errors (`tsc -b` clean)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] ApiError parameter property syntax rejected by erasableSyntaxOnly**
- **Found during:** Task 2 (build verification)
- **Issue:** `constructor(public status: number, ...)` is not allowed when `erasableSyntaxOnly` is enabled in tsconfig.app.json (required for Vite's esbuild mode)
- **Fix:** Changed to explicit `status: number` class field with assignment in constructor body
- **Files modified:** `web/src/lib/api.ts`
- **Commit:** 86ba9c6

**2. [Rule 3 - Blocking] tsconfig.json missing ignoreDeprecations for baseUrl**
- **Found during:** Task 1 (tsc --noEmit)
- **Issue:** TypeScript emitted TS5101 error for `baseUrl` deprecation; tsc exited non-zero blocking verification
- **Fix:** Added `"ignoreDeprecations": "6.0"` to `web/tsconfig.json` (same fix applied in Plan 01 to tsconfig.app.json; root tsconfig.json was missed)
- **Files modified:** `web/tsconfig.json`
- **Commit:** d156b87

## Known Stubs

- `web/src/pages/DashboardPage.tsx` renders "Loading..." placeholder text in both sections — intentional shell, Plan 03 will replace with real data components (upstream cards, keys table)

## Threat Flags

No new security-relevant surface beyond what the plan's threat model covers. Login uses raw fetch (no token stored before 200 response). API client Bearer header only attached when token exists.

## Self-Check: PASSED
