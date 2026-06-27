# Confirmed Bugfix Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the currently scheduled non-AI bugs in `docs/confirmed-bugs.md` without mixing unrelated UI, dependency, AI Agent, or architecture refactors.

**Architecture:** Treat the product loops as the repair boundary: onboarding fail-closed first, control-plane write consistency second, command-state consistency third, monitoring semantics last. Legacy AI write behavior is deferred because the AI Agent will be replaced by Hermes Agent; Hermes must get a fresh confirmation and policy-delivery design instead of extending the old AI path.

**Tech Stack:** Go Controller, PostgreSQL storage, gRPC Agent API, Rust Agent protocol consumer, Vue 3 + Pinia + Vue Router + Element Plus frontend, Vitest.

---

## Current Open Bug Inventory

| Batch | Bugs | Theme | Stop rule |
| --- | --- | --- | --- |
| B1 | BUG-28, BUG-29, BUG-30, BUG-33, BUG-34 | Tenant/user/node lifecycle fail-closed | Inactive tenants and inactive nodes cannot register, re-register, login, refresh permissions, or self-delete through privileged side paths. |
| B2 | BUG-27, BUG-32 | Non-AI route writes must enter policy delivery | Any non-AI route mutation creates/updates desired state, queues an Agent command, records policy delivery, and requires `routes:write`. |
| B3 | BUG-35 | Agent command status state machine | Unknown command status is rejected, terminal status cannot be downgraded, timeout cleanup still catches all non-terminal supported states. |
| B4 | BUG-31 | Monitoring inactive node semantics | Suspended/banned nodes never appear as online/eligible traffic participants even if `last_seen` is recent. |
| Deferred | BUG-25, BUG-26 | Hermes Agent phase | Old AI write and AI route tools are not extended in this plan; Hermes must reintroduce them through backend confirmation and policy delivery. |

## File Responsibility Map

- `internal/cli/controller_serve.go`: southbound HTTP registration/unregister/network handlers and registration authorization.
- `internal/controller/grpc/server.go`: gRPC Sync, Metrics, CommandStream runtime identity and command status handling.
- `internal/controller/grpc/auth_interceptor.go`: runtime token extraction and validation entrypoint.
- `internal/token/validator.go` and token store code: enrollment token validation and tenant binding.
- `internal/api/handlers/auth.go`: login, refresh, and permission response behavior.
- `internal/api/v2/setup.go`: tenant-scoped API routing, tenant/node update, route mutation, monitoring response helpers.
- `internal/api/v2/platform.go`: tenant token creation/list/detail/delete behavior.
- `pkg/controllerstorage/postgres.go`: schema, node save/reuse helpers, tenant/node/token persistence.
- `pkg/controllerstorage/agent_commands.go`, `pkg/controllerstorage/policy_deliveries.go`, `pkg/controllerstorage/node_control_states.go`, `pkg/controllerstorage/policy_sync.go`: command status, delivery status, desired/applied state, and policy dispatch.
- `pkg/controllerstorage/monitoring_queries.go`: monitoring aggregates and topology source data.
- `frontend/src/stores/user.js`, `frontend/src/router/index.js`: frontend permission refresh and fail-closed routing behavior.
- `frontend/src/views/Nodes.vue`, `frontend/src/stores/node.js`, `frontend/src/composables/useRouteApi.js`: node edit form and route mutation path.
- `frontend/src/utils/controlLoopStatus.js`: frontend command/policy status normalization.

Deferred Hermes files, not modified by this plan:
- `internal/agent/brain/agent.go`
- `internal/service/ai_service.go`
- `internal/agent/tools/route_management.go`
- `frontend/src/views/AIAssistant.vue`

## Batch B1: Tenant, Permission, And Node Lifecycle Fail-Closed

**Bugs:** BUG-28, BUG-29, BUG-30, BUG-33, BUG-34

**Files:**
- Modify: `internal/cli/controller_serve.go`
- Modify: `internal/token/validator.go`
- Modify: `internal/api/handlers/auth.go`
- Modify: `internal/api/v2/setup.go`
- Modify: `internal/api/v2/platform.go`
- Modify: `pkg/controllerstorage/postgres.go`
- Modify: `frontend/src/stores/user.js`
- Modify: `frontend/src/router/index.js`
- Test: `internal/cli/controller_registration_test.go`
- Test: `internal/cli/controller_southbound_auth_test.go`
- Test: `internal/api/handlers/auth_refresh_test.go`
- Test: `internal/api/handlers/auth_permissions_test.go`
- Test: `frontend/tests/unit/userSession.test.js`
- Test: `frontend/tests/unit/routerPermissions.test.js`
- Test: `frontend/tests/unit/usePermission.test.js`

- [ ] **Step 1: Add failing tenant lifecycle tests**

  Add tests that prove these requests are rejected:
  - registration with an active enrollment token whose tenant is `suspended` or `deleted`;
  - gRPC runtime request for a node whose tenant is inactive;
  - CLI/API token creation for suspended tenant;
  - login, refresh, and permissions for a non-super-admin user under inactive tenant.

  Run:
  ```bash
  go test ./internal/cli ./internal/controller/grpc ./internal/api/handlers -run 'Tenant|Suspended|Deleted|Permissions|Refresh' -count=1
  ```
  Expected before implementation: at least one new test fails because inactive tenant paths still pass.

- [ ] **Step 2: Enforce tenant active checks at token and runtime boundaries**

  Implement a storage/helper path that verifies `tenants.status = 'active'` for enrollment token tenant lookup and runtime node tenant lookup. Apply it in:
  - `validateEnrollmentTokenTenant`;
  - runtime gRPC node resolution;
  - southbound certificate/registration paths that use runtime tokens;
  - tenant token creation APIs and CLI token creation paths.

  Run:
  ```bash
  go test ./internal/cli ./internal/controller/grpc ./internal/api/v2 -run 'Tenant|Runtime|Token|Register|Certificate' -count=1
  ```
  Expected: inactive tenant tests pass, active tenant happy paths remain green.

- [ ] **Step 3: Close auth and frontend permission fallback**

  Backend behavior:
  - login rejects inactive tenant users except super-admin platform access;
  - refresh rejects inactive tenant users;
  - permissions returns an auth/tenant error for inactive tenant users.

  Frontend behavior:
  - production authenticated permission refresh failure clears privileged permissions;
  - router does not repopulate write permissions from local role defaults after backend failure;
  - cached permissions are scoped by user, tenant, and role if retained.

  Run:
  ```bash
  go test ./internal/api/handlers -run 'Login|Refresh|Permissions|Suspended|Deleted' -count=1
  cd frontend && npm test -- --run
  ```
  Expected: backend rejects inactive tenant sessions; frontend tests show fail-closed UI permissions.

- [ ] **Step 4: Fix hostname reuse and unregister inactive-node bypass**

  Implement:
  - `ReuseHostnameIP` must not soft-delete `suspended` or `banned` hostname matches;
  - registration with same hostname and new public key must return conflict when the old hostname belongs to suspended/banned node;
  - `/api/v2/agents/unregister` must reject suspended/banned runtime token before applying `deleted`.

  Run:
  ```bash
  go test ./internal/cli ./pkg/controllerstorage -run 'ReuseHostname|Unregister|Suspended|Banned|Lifecycle' -count=1
  ```
  Expected: suspended/banned nodes keep their lifecycle state and cannot self-delete.

- [ ] **Step 5: Commit B1**

  ```bash
  git add internal/cli internal/controller/grpc internal/api pkg/controllerstorage frontend/src docs/confirmed-bugs.md
  git commit -m "fix: harden tenant and node lifecycle boundaries"
  ```

## Batch B2: Non-AI Route Writes Must Enter Policy Delivery

**Bugs:** BUG-27, BUG-32

**Files:**
- Modify: `internal/api/v2/setup.go`
- Modify: `internal/cli/controller_serve.go`
- Modify: `pkg/controllerstorage/policy_sync.go`
- Modify: `frontend/src/views/Nodes.vue`
- Modify: `frontend/src/stores/node.js`
- Modify: `frontend/src/composables/useRouteApi.js`
- Test: `internal/api/v2/tenant_routes_real_path_test.go`
- Test: `internal/cli/controller_southbound_auth_test.go`
- Test: `frontend/tests/unit/nodesWorkbench.test.js`
- Test: `frontend/tests/unit/useRouteApi.test.js`

- [ ] **Step 1: Add failing route-write consistency tests**

  Cover three write sources:
  - legacy `/api/v2/agents/network`;
  - `PUT /api/v2/tenants/{tenant_id}/nodes/{node_id}` with `advertised_routes`.

  Each test must assert `routes:write` is required and that writes create desired state, command, and policy delivery through `MutatePolicyAndQueueSync`.

  Run:
  ```bash
  go test ./internal/api/v2 ./internal/cli -run 'Route|Network|Advertised|PolicyDelivery' -count=1
  ```
  Expected before implementation: legacy/generic paths fail because they update `nodes.advertised_routes` directly or use the wrong permission.

- [ ] **Step 2: Remove direct route mutation from generic node edit**

  Implement:
  - generic node update ignores/rejects `advertised_routes` unless it delegates to the standard route mutation service;
  - frontend node edit no longer submits `advertised_routes` through the generic node update API;
  - any future AI/Hermes route write must call the same route mutation service, but old AI route tools are not changed in this batch.

  Run:
  ```bash
  go test ./internal/api/v2 -run 'Route|Advertised' -count=1
  cd frontend && npm test -- --run
  ```
  Expected: generic node edits cannot mutate route policy without the route path.

- [ ] **Step 3: Convert legacy southbound network route to the standard route mutation path**

  Keep `/api/v2/agents/network` as a compatibility endpoint, but remove its direct `UpdateTenantNodeAdvertisedRoutes(...)` write. Map it into the standard route mutation flow with tenant, node, permission, desired-state, command, and delivery records.

  Run:
  ```bash
  go test ./internal/cli ./internal/api/v2 -run 'AgentsNetwork|Route|PolicyDelivery' -count=1
  ```
  Expected: no route write can bypass policy delivery.

- [ ] **Step 4: Commit B2**

  ```bash
  git add internal/api internal/cli pkg/controllerstorage frontend/src docs/confirmed-bugs.md
  git commit -m "fix: route writes use policy delivery pipeline"
  ```

## Batch B3: Agent Command Status State Machine

**Bug:** BUG-35

**Files:**
- Modify: `internal/controller/grpc/server.go`
- Modify: `pkg/controllerstorage/agent_commands.go`
- Modify: `pkg/controllerstorage/policy_deliveries.go`
- Modify: `pkg/controllerstorage/node_control_states.go`
- Modify: `frontend/src/utils/controlLoopStatus.js`
- Test: `pkg/controllerstorage/agent_commands_test.go`
- Test: `internal/controller/grpc/command_stream_test.go`
- Test: `internal/controller/grpc/server_command_stream_test.go`

- [ ] **Step 1: Add failing command status tests**

  Add tests for:
  - unknown status such as `ready` is rejected and not persisted;
  - terminal `completed` / `failed` cannot be changed back to `sent`, `acknowledged`, or arbitrary status;
  - valid `sent -> acknowledged -> completed` and `sent -> failed` flows still pass;
  - `policy_deliveries.command_status` changes only when command status is accepted.

  Run:
  ```bash
  go test ./pkg/controllerstorage ./internal/controller/grpc -run 'CommandStatus|CommandStream|PolicyDelivery' -count=1
  ```
  Expected before implementation: unknown status or terminal downgrade test fails.

- [ ] **Step 2: Add storage-level enum and transition validation**

  Implement a single validation function in `pkg/controllerstorage/agent_commands.go` and call it before any update. Supported persisted statuses should stay aligned with constants:
  - `pending`
  - `sent`
  - `acknowledged`
  - `completed`
  - `failed`
  - `stale`

  Enforce these transitions:
  - `pending -> sent | failed | stale`
  - `sent -> acknowledged | completed | failed | stale`
  - `acknowledged -> completed | failed | stale`
  - `completed`, `failed`, `stale` are terminal for Agent responses.

  Run:
  ```bash
  go test ./pkg/controllerstorage -run 'AgentCommandStatus' -count=1
  ```
  Expected: invalid transitions return explicit errors and do not update command or delivery rows.

- [ ] **Step 3: Make gRPC CommandStream reject invalid responses safely**

  `CommandStream` should reject invalid Agent command responses with `codes.InvalidArgument`, keep the command row unchanged, and avoid advancing the stream to the next command. It should log enough context for audit/debugging but not persist unknown status.

  Run:
  ```bash
  go test ./internal/controller/grpc -run 'CommandStream' -count=1
  ```
  Expected: invalid status does not poison command state.

- [ ] **Step 4: Commit B3**

  ```bash
  git add internal/controller/grpc pkg/controllerstorage frontend/src/utils/controlLoopStatus.js docs/confirmed-bugs.md
  git commit -m "fix: validate agent command status transitions"
  ```

## Batch B4: Monitoring Inactive Node Semantics

**Bug:** BUG-31

**Files:**
- Modify: `internal/api/v2/operations.go`
- Modify: `internal/api/v2/monitoring.go`
- Modify: `pkg/controllerstorage/monitoring_queries.go`
- Modify: `pkg/controllerstorage/postgres.go`
- Test: `internal/api/v2/nodes_monitoring_behavior_test.go`

- [ ] **Step 1: Add failing monitoring tests**

  Cover:
  - suspended/banned node with fresh `last_seen` is not `online`;
  - topology excludes inactive nodes from online/eligible links and surfaces their lifecycle state only in node detail/history views;
  - traffic/status cards do not count inactive nodes as eligible online nodes.

  Run:
  ```bash
  go test ./internal/api/v2 ./pkg/controllerstorage -run 'Monitoring|Topology|Traffic|Suspended|Banned' -count=1
  ```
  Expected before implementation: recent inactive node still appears online.

- [ ] **Step 2: Normalize inactive availability semantics**

  Implement a single helper used by node detail, monitoring stats, topology, and traffic filters:
  - `deleted` = deleted/hidden;
  - `suspended` = suspended, not online;
  - `banned` = banned, not online;
  - only active nodes can become online by `last_seen`.

  Run:
  ```bash
  go test ./internal/api/v2 ./pkg/controllerstorage -run 'Monitoring|Availability|Topology' -count=1
  ```
  Expected: all monitoring surfaces agree on inactive node status.

- [ ] **Step 3: Commit B4**

  ```bash
  git add internal/api/v2 pkg/controllerstorage docs/confirmed-bugs.md
  git commit -m "fix: exclude inactive nodes from online monitoring"
  ```

## Final Verification

- [ ] **Step 1: Run backend focused tests**

  ```bash
  go test ./internal/cli ./internal/controller/grpc ./internal/api/handlers ./internal/api/v2 ./internal/agent/... ./internal/service/... ./pkg/controllerstorage -count=1
  ```

- [ ] **Step 2: Run frontend tests and build**

  ```bash
  cd frontend
  npm test -- --run
  npm run build
  ```

- [ ] **Step 3: Update bug statuses**

  For each closed bug in `docs/confirmed-bugs.md`:
  - move it from `当前仍未闭合的 Bug` to `已重新验证为已修复的 Bug`;
  - include the exact verification command that passed;
  - keep any not-yet-fixed bug OPEN.

- [ ] **Step 4: Deployment decision**

  If the final patch only changes Controller/frontend and not Rust Agent runtime, use the low-bandwidth local artifact deployment path. If Rust Agent runtime behavior changes, use branch CI for the Agent artifact.

  Baseline:
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/aria-controller-linux-amd64 ./cmd/controller
  cd frontend && npm run build
  ```

- [ ] **Step 5: Smoke test closure loops**

  Validate:
  - inactive tenant cannot register/login/refresh;
  - active node can register and become online;
  - route update creates policy delivery and command;
  - Agent command completion updates delivery;
  - monitoring does not show suspended/banned as online.

## Deferred Hermes Agent Scope

The following issues remain real, but they are not part of the current implementation plan because the old AI Agent will be replaced by Hermes Agent:

- BUG-25: old AI chat can execute write tools without a backend confirmation gate.
- BUG-26: old AI route write tools bypass route policy delivery.

Hermes implementation requirements:
- chat phase must not execute write tools directly;
- write requests must become backend-owned pending actions with confirm/cancel;
- route writes must reuse the same route mutation and policy delivery service as UI/API writes;
- audit records must show who requested, confirmed, executed, and observed the write.
