# Nodes Workbench Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the Nodes workbench path so one node detail view shows control state, command lifecycle, policy deliveries, alerts, and certificate status.

**Architecture:** Keep backend contracts stable and make the frontend node store the normalization layer. `Nodes.vue` renders normalized workbench data and uses existing Monitoring and Policy Center routes for deeper context.

**Tech Stack:** Vue 3, Pinia, Element Plus, Vitest, Go Controller API, GitHub Actions for compile/test verification.

---

### Task 1: Node Store Workbench Normalization

**Files:**
- Modify: `frontend/src/stores/node.js`
- Modify: `frontend/tests/unit/nodeStore.test.js`

- [ ] Add failing tests that call `loadNodeDetail()` with monitoring detail containing `certificate`, `certificate_activity`, `recent_policy_deliveries`, and `active_alerts`.
- [ ] Add a failing test where agent status and command history fail but monitoring detail succeeds.
- [ ] Implement `Promise.allSettled()` based detail loading and normalize certificate fields, certificate activity, policy deliveries, alerts, and recent commands.
- [ ] Verify with GitHub Actions because local compile/test is not allowed.

### Task 2: Nodes Detail UI Completion

**Files:**
- Modify: `frontend/src/views/Nodes.vue`
- Create or modify: `frontend/tests/unit/nodesWorkbench.test.js`

- [ ] Add failing UI tests for certificate status text and quick command optimistic insertion.
- [ ] Add operations summary and certificate status sections to the node detail dialog.
- [ ] Improve command and policy delivery tables with command id, lifecycle statuses, errors, and updated/completed timestamps.
- [ ] Add status helpers for `sent`, `acknowledged`, `completed`, and `failed`.
- [ ] Verify with GitHub Actions because local compile/test is not allowed.

### Task 3: Monitoring And Policy Context Links

**Files:**
- Modify: `frontend/src/views/Nodes.vue`
- Modify: `frontend/tests/unit/nodesWorkbench.test.js`

- [ ] Add failing tests for routing to Monitoring detail with `focus=alerts|commands|certificate` and Policy Center with `nodeId`, `policyRef`, and `kind`.
- [ ] Preserve selected node context when opening deeper pages.
- [ ] Verify with GitHub Actions because local compile/test is not allowed.

### Task 4: Documentation And CI

**Files:**
- Modify: `docs/known-issues-status.md`
- Modify: `docs/v0.1.0-product-blueprint.md`

- [ ] Update docs to state that Nodes workbench has first-stage closure when tests pass.
- [ ] Push branch and trigger GitHub Actions.
- [ ] Inspect GitHub Actions logs and fix failures without local compilation.
