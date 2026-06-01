# Code Review Bugfix Batches Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close verified review bugs in small batches, pushing each batch to GitHub and waiting for GitHub Actions to pass before starting the next batch.

**Architecture:** Keep each batch focused on one failure domain so CI failures are easy to isolate. Add regression coverage before production changes where the repo already has a local test harness, but run compilation and test verification only in GitHub Actions per project workflow.

**Tech Stack:** Go controller, Rust agent, Vue/Vitest frontend, GitHub Actions Build workflow.

---

## Batch Order

### Batch 1: P0 Auth And Policy Result Semantics

**Scope:**
- `AUTH-001`: queued token-refresh requests must keep `X-Tenant-ID`.
- `AUTH-003`: first boot must not create a predictable default super admin password.
- `POLICY-001`: policy mutation dispatch failure must not return HTTP 200 success.

**Files:**
- Modify: `frontend/src/composables/useApi.js`
- Modify: `frontend/tests/unit/useApiSession.test.js`
- Modify: `internal/cli/controller_serve.go`
- Modify: `internal/cli/super_admin_test.go`
- Modify: `internal/api/v2/setup.go`
- Modify: `internal/api/v2/policy_dispatch_test.go`

- [ ] Add regression tests for tenant header propagation during refresh queueing.
- [ ] Add regression tests for missing `ARIA_SUPER_ADMIN_PASSWORD`.
- [ ] Add regression tests for policy dispatch failure status.
- [ ] Implement minimal fixes.
- [ ] Format changed Go files and run static diff checks.
- [ ] Push branch and wait for GitHub Actions green.

### Batch 2: ACL And Policy Sync Correctness

**Scope:**
- `ACL-001`: unify v2 ACL write/read paths so agent sync sees per-node ACL rules.
- `SYNC-001`: sync must fail on peer query errors instead of returning empty topology.
- `SYNC-002`: sync peers must exclude deleted/suspended nodes.
- `ACL-002`: ACL region filtering must use tenant-scoped node data.
- `BL-001`: blacklist mutations must bump desired state version.
- `POLICY-002`: QoS policy list must surface category load errors.

**Files:**
- Modify: `pkg/controllerstorage/network_policy.go`
- Modify: `pkg/controllerstorage/postgres.go`
- Modify: `internal/cli/controller_serve.go`
- Modify: `internal/api/v2/setup.go`
- Add or modify focused Go tests near existing storage and v2 tests.

- [ ] Add regression tests for v2 ACL appearing in agent sync payload.
- [ ] Add regression tests for sync peer DB errors and filtered statuses.
- [ ] Add regression tests for blacklist desired-version bumps.
- [ ] Implement minimal storage and sync changes.
- [ ] Format changed Go files and run static diff checks.
- [ ] Push branch and wait for GitHub Actions green.

### Batch 3: Auth, RBAC, And Session Consistency

**Scope:**
- `AUTH-004`, `AUTH-006`, `AUTH-007`, `AUTH-008`, `AUTH-009`, `AUTH-010`, `AUTH-014`, `AUTH-015`, `AUTH-016`, `AUTH-017`, `AUTH-018`, `AUTH-019`, `AUTH-020`.

**Files:**
- Modify: `internal/api/handlers/auth.go`
- Modify: `internal/api/handlers/tenant.go`
- Modify: `internal/api/middleware/jwt_auth.go`
- Modify: `internal/api/middleware/permissions.go`
- Modify: `internal/api/v2/setup.go`
- Modify: `pkg/controllerstorage/postgres.go`
- Modify: `frontend/src/router/index.js`
- Modify: `frontend/src/composables/usePermission.js`
- Modify: focused Go and Vitest unit tests.

- [ ] Add regression tests for refresh reading the current user from DB.
- [ ] Add regression tests for owner role mapping.
- [ ] Add regression tests for safe JWT context getters.
- [ ] Add frontend session teardown and cached-permission tests.
- [ ] Implement minimal fixes.
- [ ] Format changed Go files and run static diff checks.
- [ ] Push branch and wait for GitHub Actions green.

### Batch 4: Enrollment, Tokens, Certificates, And Agent Command Lifecycle

**Scope:**
- `ENROLL-002`, `TOKEN-001`, `TOKEN-002`, `TOKEN-003`, `LIFECYCLE-001`, `LIFECYCLE-002`, `LIFECYCLE-003`, `GRPC-003`, `GRPC-004`.

**Files:**
- Modify: `internal/cli/controller_serve.go`
- Modify: `internal/api/v2/platform.go`
- Modify: `internal/controller/grpc/server.go`
- Modify: `internal/token/validator.go`
- Modify: `internal/token/store.go`
- Modify: `pkg/controllerstorage/node_lifecycle.go`
- Modify: `pkg/controllerstorage/postgres.go`
- Modify: focused Go tests.

- [ ] Add token semantics and token-list redaction tests.
- [ ] Add enrollment rollback and lifecycle certificate/command tests.
- [ ] Implement minimal fixes.
- [ ] Format changed Go files and run static diff checks.
- [ ] Push branch and wait for GitHub Actions green.

### Batch 5: Monitoring, Routing, Operations, And Storage Error Handling

**Scope:**
- `MON-001`, `MON-002`, `ROUTE-001`, `ROUTE-002`, `OPS-001`, `STORAGE-001`, `STORAGE-002`, `HOST-001`.

**Files:**
- Modify: `internal/api/v2/monitoring.go`
- Modify: `internal/api/v2/operations.go`
- Modify: `internal/api/v2/setup.go`
- Modify: `internal/cli/controller_serve.go`
- Modify: `pkg/controllerstorage/postgres.go`
- Modify: focused Go tests.

- [ ] Add tests for tenant-scoped monitoring and hostname ambiguity.
- [ ] Add tests for invalid node IDs in batch commands.
- [ ] Add storage rows error tests.
- [ ] Implement minimal fixes.
- [ ] Format changed Go files and run static diff checks.
- [ ] Push branch and wait for GitHub Actions green.

### Batch 6: Frontend Tenant Workflow And UI Truthfulness

**Scope:**
- `FE-001`, `FE-002`, `FE-004`, `FE-005`, `FE-006`, `FE-007`, `FE-008`, `FE-009`, `FE-010`, `FE-012`, `FE-013`, `FE-014`, and the remaining Monitoring resolve write guard.

**Files:**
- Modify: `frontend/src/views/TenantManagement.vue`
- Modify: `frontend/src/views/Tokens.vue`
- Modify: `frontend/src/views/ACLRules.vue`
- Modify: `frontend/src/views/Dashboard.vue`
- Modify: `frontend/src/views/BandwidthControl.vue`
- Modify: `frontend/src/views/Monitoring.vue`
- Modify: `frontend/src/stores/node.js`
- Modify: `frontend/src/composables/useAclApi.js`
- Modify: focused Vitest tests.

- [ ] Add tests for tenant-change cache clearing and ACL map scoping.
- [ ] Add tests for pagination, null-safe search, and response-based node updates.
- [ ] Replace fake tenant actions or remove misleading controls.
- [ ] Implement minimal UI fixes.
- [ ] Run static diff checks.
- [ ] Push branch and wait for GitHub Actions green.

## Execution Rule

Do not start the next batch until the current batch has been pushed to GitHub and the Build workflow is green for that branch.
