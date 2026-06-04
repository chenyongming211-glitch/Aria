# Review Follow-ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining review follow-ups for forced password change UX, duplicate RBAC plumbing, and tenant-scoped hostname tools.

**Architecture:** Keep the current v2 `authorizeTenantPermission` path as the canonical RBAC enforcement point and remove unused middleware duplication. Add a dedicated forced-password-change route that works for both Velo-launched and standalone sessions. Add optional `tenant_id` selection to hostname-based admin and agent tools while preserving fail-closed ambiguity behavior when tenant is omitted.

**Tech Stack:** Vue 3, Pinia, Element Plus, Go Controller, sqlmock-backed tests, GitHub Actions for compile/build/test verification.

---

### Task 1: Forced Password Change Page

**Files:**
- Modify: `frontend/src/router/index.js`
- Modify: `frontend/src/views/Login.vue`
- Create: `frontend/src/views/ChangePassword.vue`
- Test: `frontend/tests/unit/routerPermissions.test.js`

- [ ] Add a route named `ChangePassword` at `/change-password` with `requiresAuth: true` and `allowPasswordChange: true`.
- [ ] Change route guard behavior so `aria_must_change_password=true` redirects protected pages to `/change-password`, allows `/change-password`, and redirects `/login` to `/change-password` when a valid forced-change session already exists.
- [ ] Move the forced password form out of the login dialog into `ChangePassword.vue`; after a successful change, logout and send the user to `/login`.
- [ ] Keep `Login.vue` login behavior simple: when login returns `requirePasswordChange`, route to `/change-password`.
- [ ] Add router unit tests for forced-change sessions.

### Task 2: RBAC Duplicate Middleware Cleanup

**Files:**
- Modify: `internal/api/middleware/permissions.go`
- Modify: `docs/code-review-findings.md`

- [ ] Remove unused `RequirePermission`, `GetPermissions`, and private permission-context helpers from middleware.
- [ ] Keep permission constants and `RoleSuperAdmin`.
- [ ] Leave v2 direct `authorizeTenantPermission` enforcement unchanged.
- [ ] Mark `AUTH-019` fixed in `docs/code-review-findings.md`.

### Task 3: Tenant-Scoped Hostname Tools

**Files:**
- Modify: `internal/agent/tools/node_lookup.go`
- Modify: `internal/agent/tools/route_management.go`
- Modify: `internal/agent/tools/tools.go`
- Modify: `internal/cli/admin_ban.go`
- Test: `internal/agent/tools/node_lookup_test.go`

- [ ] Add helper support for optional `tenant_id` in hostname lookups.
- [ ] Add `tenant_id` parameters to `add_route`, `remove_route`, `get_node_routes`, and `get_node_detail`.
- [ ] Add `--tenant-id` to `aria admin ban --hostname`.
- [ ] Preserve existing behavior when `tenant_id` is omitted: duplicate hostnames fail closed.
- [ ] Mark `HOST-001` fixed in `docs/code-review-findings.md`.

### Task 4: Verification and Landing

**Files:**
- No additional production files.

- [ ] Run only static local checks that do not compile the project.
- [ ] Commit the branch.
- [ ] Push to GitHub.
- [ ] Wait for GitHub Actions to run full Go/Rust/frontend verification.
- [ ] If Actions is green, merge to `master`.
