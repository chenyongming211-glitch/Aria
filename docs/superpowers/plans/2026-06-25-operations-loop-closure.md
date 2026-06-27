# Operations Loop Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the v0.1.0 non-AI operations loop so an alert can lead to a confirmed sync or health_check command, visible command state, and preserved alert/event context.

**Architecture:** Reuse the existing tenant-scoped Agent command API instead of adding a new backend endpoint. Frontend Monitoring and Node Detail pass alert/event/policy context into `agent_commands.params`, immediately refresh command/alert state, and keep AI as an optional later layer rather than a hard dependency.

**Tech Stack:** Vue 3, Element Plus, Vitest, Go Controller API, existing `agent_commands`, `alerts`, `audit_events`, `policy_deliveries`.

---

### Task 1: Add Monitoring Alert Action Tests

**Files:**
- Modify: `frontend/tests/unit/monitoringWorkflow.test.js`
- Modify: `frontend/src/views/Monitoring.vue`

- [x] **Step 1: Write failing tests**

Add assertions that `Monitoring.vue` can queue a `sync` command from a `sync_failed` alert and passes `alert_id`, `event_type`, `policy_ref`, `policy_domain`, and `source: "monitoring"` in command params.

- [x] **Step 2: Run test to verify it fails**

Run: `npm run test:run -- monitoringWorkflow`

Expected: FAIL because Monitoring does not import `useAgentProxyApi` or expose `handleAlertCommand`.

- [x] **Step 3: Implement Monitoring action buttons**

Add `Run Sync` and `Health Check` buttons for actionable node alerts. Queue commands through `useAgentProxyApi.sendAgentCommand(node_id, { command, params, timeout: 30 })`, then refresh stats/events/alerts.

- [x] **Step 4: Run test to verify it passes**

Run: `npm run test:run -- monitoringWorkflow`

Expected: PASS.

### Task 2: Add Node Detail Context Action Tests

**Files:**
- Modify: `frontend/tests/unit/monitoringWorkflow.test.js`
- Modify: `frontend/src/views/NodeMonitorDetail.vue`

- [x] **Step 1: Write failing tests**

Add assertions that `NodeMonitorDetail.vue` exposes context actions when opened with `alertId` and `eventType=sync_failed`, queues `sync`, prepends the queued command, and can resolve the active alert.

- [x] **Step 2: Run test to verify it fails**

Run: `npm run test:run -- monitoringWorkflow`

Expected: FAIL because Node Detail has no context command or alert resolve actions.

- [x] **Step 3: Implement Node Detail actions**

Import `useAgentProxyApi`, `usePermission`, and `ElMessage`. Add context action buttons and handlers for `sync`, `health_check`, and alert resolve. Command params must include `source: "node_monitor_detail"` plus alert/policy/event context.

- [x] **Step 4: Run test to verify it passes**

Run: `npm run test:run -- monitoringWorkflow`

Expected: PASS.

### Task 3: Verify Full Frontend and Diff

**Files:**
- No new files.

- [x] **Step 1: Run full frontend unit tests**

Run: `npm run test:run`

Expected: all tests pass.

- [x] **Step 2: Run diff whitespace check**

Run: `git diff --check`

Expected: no output, exit 0.

- [x] **Step 3: Commit and push**

Commit message: `frontend: close non-ai operations loop`

Push branch: `codex/operations-loop-closure`

### Task 4: CI and Merge

**Files:**
- No code files.

- [x] **Step 1: Wait for branch GitHub Actions**

Run: `gh run watch <branch-run-id> --exit-status`

Expected: branch Actions success.

- [x] **Step 2: Fast-forward merge to master**

Run: `git switch master && git merge --ff-only origin/master && git merge --ff-only codex/operations-loop-closure && git push origin master`

Expected: master push succeeds.

- [x] **Step 3: Wait for master GitHub Actions**

Run: `gh run watch <master-run-id> --exit-status`

Expected: master Actions success.

- [x] **Step 4: Clean branch**

Delete local and remote `codex/operations-loop-closure`.

### 2026-06-28 Follow-Up Verification

- `0.2.82` master deployment kept active alerts at `0`.
- A live `health_check` from the tenant-scoped node command API queued command
  `a71d279a-a7fb-4457-8c8b-f45f10161e8d` for
  `node-82-156-48-111` and converged from `pending` to `completed` with
  message `agent healthy`.
- Monitoring event feed still exposes historical `alert_resolved` events.
