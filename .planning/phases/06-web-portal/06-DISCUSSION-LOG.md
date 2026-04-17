# Phase 6: Web Portal - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-17
**Phase:** 06-web-portal
**Areas discussed:** Auth/session handling

---

## Auth/session handling

### Q1: Where should the admin key be stored after the user enters it?

| Option | Description | Selected |
|--------|-------------|----------|
| localStorage (persists across tabs/restarts) | User enters once; survives browser restarts. Standard for single-operator admin tools. | ✓ |
| sessionStorage (survives tab refresh, not restart) | Cleared when browser tab is closed. User re-enters key after every session. | |
| In-memory only (cleared on page reload) | Prompted every page load. Most secure, least convenient. | |

**User's choice:** localStorage
**Notes:** Personal/team tool — convenience over strict isolation.

---

### Q2: What should the login UI look like?

| Option | Description | Selected |
|--------|-------------|----------|
| Full-page centered login form | Redirects to blank login screen before portal loads. Clean separation. | ✓ |
| Inline prompt inside the portal shell | Portal layout renders with a prompt in content areas. Same URL throughout. | |
| Browser native prompt() dialog | Minimal UI — fast to implement, ugly UX. | |

**User's choice:** Full-page centered login form
**Notes:** Portal content never partially visible before auth.

---

### Q3: What should happen when the admin key is wrong or revoked mid-session?

| Option | Description | Selected |
|--------|-------------|----------|
| Redirect to login + clear stored key | Any 401 clears localStorage and bounces to login. Simple and predictable. | ✓ |
| Show error toast, stay on current page | User sees error but stays in place before redirect. | |
| You decide | Claude picks what's natural given the login flow. | |

**User's choice:** Redirect to login + clear stored key
**Notes:** Clean, predictable — no intermediate state.

---

## Claude's Discretion

- UI component library (shadcn/ui recommended)
- Data refresh strategy for upstream status
- Key create/edit UX pattern
- Vite/Go embed build integration approach
- Dashboard layout (tabs, two-panel, separate routes)

## Deferred Ideas

None.
