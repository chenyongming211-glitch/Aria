# Focused Status Polling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace stale control-loop status screens with focused polling that tracks only unfinished policy deliveries and node convergence state.

**Architecture:** Keep the existing full-list APIs for initial rendering and CRUD refreshes. Add lightweight status endpoints that return only latest delivery or node control fields for caller-provided refs, then add frontend composables that poll only non-terminal items and patch rows locally. This avoids broad table reloads while making `pending/in_progress/syncing` state converge automatically.

**Tech Stack:** Go Controller API + Postgres storage, Vue 3 + Pinia + Vitest frontend.

---

## Design Decisions

- Policy pages must not poll whole ACL/QoS/Route/Policies datasets after initial load.
- Policy polling key is `{node_id, policy_domain, policy_ref}`; `policy_ref` alone is not globally meaningful.
- Policy status request uses `POST` with JSON body instead of `GET ?refs=` so route CIDRs and mixed domains do not need fragile URL encoding.
- Node list/detail polling is separate from policy polling because node convergence fields come from node status/control state, not only `policy_deliveries`.
- Polling pauses when the tab is hidden, resumes immediately when visible, prevents overlapping requests, and stops once all watched items reach terminal state.
- First implementation targets HTTP polling. SSE/WebSocket remains a later optimization after the control-loop API shape is stable.

## Status Semantics

Terminal policy states:

- `applied`
- `completed`
- `failed`
- `error`
- `stale`
- `idle`

Active policy states:

- `pending`
- `queued`
- `sent`
- `ack`
- `acked`
- `in_progress`
- `syncing`

Terminal node convergence states:

- `converged`
- `applied`
- `failed`
- `error`
- `idle`

Active node convergence states:

- `pending`
- `syncing`
- `queued`
- `sent`
- `ack`
- `in_progress`
- `diverged`

## Files

- Create: `frontend/src/composables/useFocusedPolling.ts`
- Create: `frontend/tests/unit/useFocusedPolling.test.ts`
- Create: `internal/api/v2/status_polling_test.go`
- Modify: `pkg/controllerstorage/policy_deliveries.go`
- Modify: `internal/api/v2/setup.go`
- Modify: `frontend/src/config/api.ts`
- Modify: `frontend/src/composables/usePolicyStatusApi.ts`
- Modify: `frontend/src/views/ACLRules.vue`
- Modify: `frontend/src/views/BandwidthControl.vue`
- Modify: `frontend/src/views/Routing.vue`
- Modify: `frontend/src/views/Policies.vue`
- Modify: `frontend/src/views/Nodes.vue`
- Modify: `frontend/src/views/NodeMonitorDetail.vue`
- Modify: `VERSION`
- Modify: `docs/deployment.md`

## Task 1: Backend Policy Delivery Status Endpoint

**Files:**
- Create test: `internal/api/v2/status_polling_test.go`
- Modify: `pkg/controllerstorage/policy_deliveries.go`
- Modify: `internal/api/v2/setup.go`

- [x] **Step 1: Write failing test**

Add a test that sends:

```http
POST /api/v2/tenants/{tenantID}/policy-deliveries/status
```

with:

```json
{
  "items": [
    {"node_id":"11111111-1111-1111-1111-111111111111","policy_domain":"acl","policy_ref":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
    {"node_id":"11111111-1111-1111-1111-111111111111","policy_domain":"route","policy_ref":"10.20.0.0/16"}
  ]
}
```

Expect HTTP 200 and an `items` array where each item contains `node_id`, `policy_domain`, `policy_ref`, `policy_status`, `pending_cmds`, `last_delivery`, and `delivery_history`.

- [x] **Step 2: Run test to verify RED**

Run:

```bash
go test ./internal/api/v2 -run TestPolicyDeliveryStatusEndpoint -count=1
```

Expected: FAIL because the endpoint does not exist.

- [x] **Step 3: Add storage query**

Add a storage method that loads latest delivery rows for an explicit item set:

```go
func (s *Storage) ListLatestPolicyDeliveriesByRefs(
    tenantID uuid.UUID,
    items []PolicyDeliveryRef,
) (map[PolicyDeliveryRef]*PolicyDelivery, error)
```

Use `DISTINCT ON (node_id, policy_domain, policy_ref)` ordered by `created_at DESC` and scoped by `tenant_id`.

- [x] **Step 4: Add API handler**

Add `handlePolicyDeliveryStatus()` under tenant-scoped routing. Validate:

- body has 1-100 items
- each `node_id` is a UUID
- `policy_domain` is one of `acl`, `qos`, `route`
- `policy_ref` is non-empty and <= 255 chars
- caller has read permission for the domain (`acls:read`, `qos:read`, `routes:read`, or `policies:read`)

- [x] **Step 5: Run test to verify GREEN**

Run:

```bash
go test ./internal/api/v2 -run TestPolicyDeliveryStatusEndpoint -count=1
```

Expected: PASS.

## Task 2: Backend Node Status Endpoint

**Files:**
- Modify test: `internal/api/v2/status_polling_test.go`
- Modify: `internal/api/v2/setup.go`

- [x] **Step 1: Write failing test**

Add a test for:

```http
POST /api/v2/tenants/{tenantID}/nodes/status
```

with:

```json
{"node_ids":["11111111-1111-1111-1111-111111111111"]}
```

Expect HTTP 200 and each item includes `node_id`, `status`, `pending_cmds`, `configuration_status`, `desired_state_version`, `applied_state_version`, `observed_state`, `convergence_status`, `last_sync_at`, `last_command_status`, and `last_command_error`.

- [x] **Step 2: Run test to verify RED**

Run:

```bash
go test ./internal/api/v2 -run TestNodeStatusEndpoint -count=1
```

Expected: FAIL because the endpoint does not exist.

- [x] **Step 3: Add API handler**

Add `handleTenantNodeStatus()` scoped by tenant. Validate 1-100 node ids and require `nodes:read`. Query only nodes inside the tenant.

- [x] **Step 4: Run test to verify GREEN**

Run:

```bash
go test ./internal/api/v2 -run TestNodeStatusEndpoint -count=1
```

Expected: PASS.

## Task 3: Frontend Focused Polling Composable

**Files:**
- Create: `frontend/src/composables/useFocusedPolling.ts`
- Create test: `frontend/tests/unit/useFocusedPolling.test.ts`

- [x] **Step 1: Write failing tests**

Cover:

- starts when active items exist
- does not start when no active items exist
- prevents overlapping requests
- pauses when `document.hidden`
- resumes and immediately polls when visible
- stops when `hasActiveItems()` becomes false

- [x] **Step 2: Run test to verify RED**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/useFocusedPolling.test.ts
```

Expected: FAIL because `useFocusedPolling` does not exist.

- [x] **Step 3: Implement composable**

Expose:

```ts
export function useFocusedPolling(options: {
  poll: () => Promise<void>
  hasActiveItems: () => boolean
  intervalMs?: number
  enabled?: () => boolean
})
```

Return:

```ts
{ start, stop, trigger, isPolling }
```

- [x] **Step 4: Run test to verify GREEN**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/useFocusedPolling.test.ts
```

Expected: PASS.

## Task 4: Frontend Policy Status API and Page Integration

**Files:**
- Modify: `frontend/src/config/api.ts`
- Create or modify: `frontend/src/composables/usePolicyStatusApi.ts`
- Modify: `frontend/src/views/ACLRules.vue`
- Modify: `frontend/src/views/BandwidthControl.vue`
- Modify: `frontend/src/views/Routing.vue`
- Modify: `frontend/src/views/Policies.vue`

- [x] **Step 1: Add API helper tests**

Verify the helper posts policy refs to `/policy-deliveries/status` and normalizes response items.

- [x] **Step 2: Implement helper**

Add:

```ts
getPolicyDeliveryStatuses(items: PolicyDeliveryStatusRequestItem[])
```

- [x] **Step 3: Patch page rows locally**

Each page computes active refs from current rows, polls only those refs, and patches only status fields:

- `policy_status`
- `pending_cmds`
- `last_delivery`
- `delivery_history`
- `last_delivery_error`
- `last_delivery_command_id`
- `last_delivery_action`

- [x] **Step 4: Verify page tests**

Run targeted frontend tests for ACL/QoS/Route/Policy pages.

## Task 5: Frontend Node Status API and Page Integration

**Files:**
- Modify: `frontend/src/config/api.ts`
- Modify: `frontend/src/stores/node.ts`
- Modify: `frontend/src/views/Nodes.vue`
- Modify: `frontend/src/views/NodeMonitorDetail.vue`

- [x] **Step 1: Add node status API helper**

Expose:

```ts
getNodeStatuses(nodeIds: string[])
```

- [x] **Step 2: Patch node list rows locally**

`Nodes.vue` watches only nodes with active convergence or pending commands. It patches `status`, `pendingCmds`, `configurationStatus`, `desiredStateVersion`, `appliedStateVersion`, `observedState`, `stateConvergence`, `lastSyncAt`, `lastCommandStatus`, and `lastCommandError`.

- [x] **Step 3: Patch node detail locally**

`NodeMonitorDetail.vue` polls while the viewed node has active command or policy state.

## Task 6: Verification, Version, Deployment

**Files:**
- Modify: `VERSION`
- Modify: `docs/deployment.md`

- [x] **Step 1: Run backend verification**

```bash
go test ./internal/api/v2 ./pkg/controllerstorage
```

- [x] **Step 2: Run frontend verification**

```bash
cd frontend
npm run type-check
npm run test:run
npm run build -- --outDir ../dist/frontend --emptyOutDir
```

- [x] **Step 3: Bump version**

Advance `VERSION` from `0.2.87` to `0.2.88` before deployment.

- [ ] **Step 4: Push branch and verify CI**

Push `codex/focused-status-polling` and wait for GitHub Actions to pass.

- [ ] **Step 5: Gray deploy**

Use the Controller/frontend low-bandwidth path because this changes Controller API and frontend only, not Rust Agent/eBPF/southbound contracts.

- [ ] **Step 6: Online smoke**

Verify:

- `https://aria.yun/api/version` returns `0.2.88`
- nodes list status changes without manual refresh
- ACL/QoS/Route/Policies pending rows converge without full page refresh
- browser network shows focused status requests instead of whole-table polling
