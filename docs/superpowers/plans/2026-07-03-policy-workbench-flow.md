# Policy Workbench Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make IP Group, ACL, QoS, Route, Policy Center, Nodes, and Monitoring behave like one traceable policy workbench.

**Architecture:** Keep the first batch frontend-centered and low risk. Introduce shared policy-context utilities for query parsing, page routing, node-detail routing, delivery evidence normalization, and row matching; then use those utilities from the existing pages instead of adding a new backend aggregate endpoint.

**Tech Stack:** Vue 3, Vue Router, Vitest, existing Controller APIs for policies, IP Group references, policy delivery status, node detail, and monitoring.

---

### Task 1: Shared Policy Context Utilities

**Files:**
- Create: `frontend/src/utils/policyContext.ts`
- Test: `frontend/tests/unit/policyContextUtils.test.ts`

- [x] **Step 1: Write the failing tests**

Cover:
- snake_case and camelCase query aliases normalize into one context.
- ACL/QoS/Route route targets carry `nodeId`, `policyRef`, and `commandId`.
- Node detail queries choose `focus=commands` when `commandId` exists and `focus=policies` when only `policyRef` exists.
- Row matching prefers `policyRef`, then falls back to `commandId`.
- Latest delivery evidence uses `command_status` when `status` is missing.

- [x] **Step 2: Run tests and confirm RED**

Run: `npm --prefix frontend run test:run -- policyContextUtils.test.ts`

Expected: FAIL because `@/utils/policyContext` does not exist yet.

- [x] **Step 3: Implement minimal utility module**

Add pure functions only. No Vue runtime dependency except plain route-query shapes:
- `policyContextFromQuery`
- `policyCenterQueryFromContext`
- `policyPageRouteForDomain`
- `nodeDetailRouteFromContext`
- `policyRowMatchesContext`
- `normalizeDeliveryEvidence`

- [x] **Step 4: Run tests and confirm GREEN**

Run: `npm --prefix frontend run test:run -- policyContextUtils.test.ts`

Expected: PASS.

### Task 2: Use Shared Context in Policy Pages

**Files:**
- Modify: `frontend/src/views/Policies.vue`
- Modify: `frontend/src/views/ACLRules.vue`
- Modify: `frontend/src/views/BandwidthControl.vue`
- Modify: `frontend/src/views/Routing.vue`
- Modify: `frontend/src/views/IPGroups.vue`
- Test: `frontend/tests/unit/policyPageContext.test.js`
- Test: `frontend/tests/unit/policySpecialPageContext.test.js`

- [x] **Step 1: Add failing tests**

Cover:
- Policy Center row can open IP Group management while preserving selected policy context.
- Policy Center kind buttons and IP Group button use the same context shape.
- ACL/QoS/Route/IP Group clear/open-node/open-policy-center behavior remains stable.

- [x] **Step 2: Run targeted tests and confirm RED**

Run: `npm --prefix frontend run test:run -- policyPageContext.test.js policySpecialPageContext.test.js`

Expected: FAIL on the new selected-policy-to-IP-Group behavior.

- [x] **Step 3: Wire pages to `policyContext.ts`**

Replace repeated query parsing and route construction where it is already localized. Keep page structure intact and avoid broad TS migration of `Routing.vue` or `IPGroups.vue`.

- [x] **Step 4: Run targeted tests and confirm GREEN**

Run: `npm --prefix frontend run test:run -- policyPageContext.test.js policySpecialPageContext.test.js policyContextUtils.test.ts`

Expected: PASS.

### Task 3: Verify Focused Polling and Delivery Evidence

**Files:**
- Modify only if tests expose gaps: `frontend/src/composables/usePolicyStatusApi.ts`, `frontend/src/composables/useIpGroupApi.ts`
- Test: `frontend/tests/unit/usePolicyStatusApi.test.ts`
- Test: `frontend/tests/unit/useIpGroupApi.test.js`

- [x] **Step 1: Add/adjust tests for delivery status aliases**

Cover:
- IP Group reference delivery accepts `command_status` and exposes a visible `status`.
- Policy status polling keeps latest delivery command id, action, and failure reason.

- [x] **Step 2: Run tests and confirm RED if behavior is missing**

Run: `npm --prefix frontend run test:run -- useIpGroupApi.test.js usePolicyStatusApi.test.ts`

- [x] **Step 3: Implement minimal normalization fixes**

Do not change API paths. Normalize shape only.

- [x] **Step 4: Run tests and confirm GREEN**

Run: `npm --prefix frontend run test:run -- useIpGroupApi.test.js usePolicyStatusApi.test.ts`

### Task 4: Final Verification and Delivery

**Files:**
- Modify: `VERSION` only if this branch is deployed.
- Modify: `docs/deployment.md` only after deployment evidence exists.

- [x] **Step 1: Run targeted frontend regression suite**

Run:
- `npm --prefix frontend run test:run -- policyPageContext.test.js policySpecialPageContext.test.js useIpGroupApi.test.js usePolicyStatusApi.test.ts`
- `npm --prefix frontend run test:run -- monitoringWorkflow.test.js nodesWorkbench.test.js`

- [x] **Step 2: Run repository whitespace check**

Run: `git diff --check`

- [ ] **Step 3: Commit and push feature branch**

Commit message: `feat: unify policy workbench context flow`

- [ ] **Step 4: Follow project release flow**

Push branch, wait for GitHub Actions, do gray validation, then wait for user approval before merging to `master`.
