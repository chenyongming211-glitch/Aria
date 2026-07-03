# Open Bugfix Batches Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the currently confirmed open bugs from `docs/confirmed-bugs.md` in priority order without mixing security, Agent runtime, frontend, CI/CD, and performance work into one large change.

**Architecture:** Treat `docs/confirmed-bugs.md` as the source of truth. Exclude `BUG-62` and `BUG-83` because they were rechecked as not confirmed. Keep performance-only items separate from correctness/security fixes unless the same file is already being touched in that batch.

**Tech Stack:** Go Controller, Rust Agent, Vue 3 + TypeScript frontend, GitHub Actions, Ansible deployment assets, Vitest, Go test, Cargo test/CI.

---

## Source Bug Set

### Do Not Fix

| Bug | Reason |
| --- | --- |
| BUG-62 | Not confirmed; `fetchVersion()` catches internally. |
| BUG-83 | Not confirmed; no CORS header is not an API security boundary by itself. |

### Current Open Bugs To Fix

| Priority | Bugs | Theme |
| --- | --- | --- |
| P0 | BUG-68, BUG-69, BUG-72, BUG-80, BUG-81, BUG-82, BUG-87 | tenant/session/webhook/backup security |
| P1 | BUG-64, BUG-65, BUG-66, BUG-67, BUG-75, BUG-76, BUG-100, BUG-101 | Agent/runtime and destructive restore safety |
| P2 | BUG-58, BUG-59, BUG-60, BUG-61, BUG-70, BUG-71, BUG-73, BUG-77, BUG-78, BUG-79, BUG-102, BUG-103, BUG-104, BUG-105, BUG-106, BUG-107, BUG-108 | correctness, UI stability, delivery integrity |
| P3 | BUG-63, BUG-84, BUG-85, BUG-86, BUG-88, BUG-89, BUG-90, BUG-91, BUG-92, BUG-93, BUG-95, BUG-96, BUG-98, BUG-109, BUG-110 | operational hardening and scale |
| P4 | BUG-94, BUG-97, BUG-99 | Rust allocation optimization debt |

---

## Batch Order

### B1: Tenant, Runtime Token, and User Session Revocation

**Priority:** P0
**Status:** ✅ Implemented on `codex/bugfix-b1-security`; local Go validation passed, CI/merge/deploy still pending.

**Bugs:** BUG-68, BUG-69, BUG-72, BUG-81, BUG-95

**Why first:** These are access-control and fail-closed issues. They affect whether a suspended tenant, stolen JWT, or stale runtime token can continue operating.

**Files:**
- Modify: `internal/api/v2/setup.go`
- Modify: `internal/auth/jwt.go`
- Modify: `internal/auth/runtime_token.go`
- Modify: `internal/api/handlers/auth.go`
- Modify: `internal/api/middleware/jwt_auth.go`
- Modify: `internal/controller/grpc/auth_interceptor.go`
- Modify: `internal/controller/grpc/server.go`
- Modify: `pkg/controllerstorage/node_lifecycle.go`
- Modify: `pkg/controllerstorage/agent_commands.go`
- Test: `internal/api/v2/*tenant*test.go`
- Test: `internal/api/handlers/auth_*_test.go`
- Test: `internal/controller/grpc/*runtime*_test.go`
- Test: `pkg/controllerstorage/*lifecycle*_test.go`

**Implementation boundary:**
- Add one storage-level tenant suspension operation that updates tenant status, marks tenant nodes suspended, fails incomplete commands, and records enough audit context.
- Add token/session invalidation that is actually checked during HTTP auth and refresh.
- Add runtime token revocation/versioning so a tenant or node suspension invalidates future runtime auth.
- Ensure established command streams fail closed when tenant status changes, not just when node status changes.
- Add JWT issuer validation for user tokens.

**Validation:**

```bash
go test ./internal/api/v2 ./internal/api/handlers ./internal/api/middleware ./internal/controller/grpc ./pkg/controllerstorage -count=1
```

**Expected result:** suspended tenant users cannot refresh/use sessions; suspended tenant Agents cannot keep CommandStream active after the next status check; old JWT/runtime tokens are rejected after revocation/version change.

**2026-07-03 validation:**

```bash
go test ./internal/auth ./internal/api/middleware ./internal/api/handlers ./internal/api/v2 ./internal/controller/grpc ./pkg/controllerstorage -count=1
go test ./internal/cli -count=1
go test ./... -count=1
```

All three commands passed locally.

---

### B2: Backup, Restore, and Webhook Security

**Priority:** P0/P1

**Status:** ✅ FIXED locally in `codex/bugfix-b1-security`

**Bugs:** BUG-80, BUG-82, BUG-87, BUG-67, BUG-71, BUG-63

**Why second:** Backup export and restore can leak or replace control-plane state. Webhook endpoints are externally callable and should fail closed when security settings are incomplete.

**Files:**
- Modify: `internal/api/v2/settings.go`
- Modify: `internal/im/dingtalk.go`
- Modify: `internal/im/feishu.go`
- Test: `internal/api/v2/settings_test.go`
- Test: `internal/im/*_test.go`

**Implementation boundary:**
- Add an explicit sensitive backup mode: either encrypt full backups or require a separate `include_sensitive=true` confirmation and return redacted data by default.
- Add restore preflight checks for active Agents, incomplete commands, and table dependency closure before destructive restore runs.
- Make DingTalk and Feishu webhook auth fail closed when a webhook endpoint is enabled without the configured signing secret or verify token.
- Normalize DingTalk JSON response writing to match Feishu's checked encoder path.

**Validation:**

```bash
go test ./internal/api/v2 ./internal/im -count=1
```

**Expected result:** normal backup no longer leaks reusable secrets by default; restore dry-run catches dependency and live-runtime hazards; webhook requests without required auth fail.

**2026-07-03 result:** implemented default redacted backups, explicit sensitive export confirmation, redacted-restore rejection, restore runtime preflight, selective restore dependency closure, DingTalk checked JSON response writing, and DingTalk/Feishu webhook fail-closed auth.

**2026-07-03 validation:**

```bash
go test ./internal/api/v2 ./internal/im -count=1
```

Passed locally.

---

### B3: Rust Agent Runtime Durability

**Priority:** P1

**Bugs:** BUG-64, BUG-65, BUG-66, BUG-74, BUG-75, BUG-76, BUG-100, BUG-101

**Why third:** These bugs can brick or desynchronize Agents, but they require Rust runtime changes and should be isolated from Controller security work.

**Files:**
- Modify: `agent-rust/agent/src/main.rs`
- Modify: `agent-rust/agent/src/config.rs`
- Modify: `agent-rust/agent/src/agent_runtime.rs`
- Modify: `agent-rust/agent/src/certificate_client.rs`
- Test: `agent-rust/agent/src/*`

**Implementation boundary:**
- Use runtime credential for re-registration when the Controller supports it, falling back to enrollment token only for first enrollment or fresh enrollment requirements.
- Persist refreshed runtime token immediately after Sync receives it, before peer/route/policy apply can fail.
- Write Agent state atomically with temp file plus rename.
- Treat damaged state as recoverable only when the bootstrap/legacy material can safely reconstruct runtime state; otherwise fail with a clear diagnostic and preserve the corrupt file.
- Make certificate renewal transactional: write renewed material to temp paths, verify gRPC reconnect, then atomically promote.
- Clean all pinned ACL/QoS maps on shutdown, matching startup cleanup.
- Requeue or durably report in-flight command results when the stream disconnects.
- Add rollback or staged apply for multi-interface `sync_peers`.

**Validation:**

```bash
cd agent-rust
cargo test -p aria-agent --lib
```

**CI requirement:** Because Rust Agent runtime and eBPF-adjacent code are modified, push the branch and require GitHub Actions Rust Agent build to pass before merge.

**Expected result:** Agents recover from state-write interruption, token rotation survives partial Sync failure, certificate renewal does not brick Agents, and command/peer state converges after reconnect.

---

### B4: Controller Policy, Route, Certificate, and Command Correctness

**Priority:** P2

**Bugs:** BUG-70, BUG-73, BUG-102, BUG-103, BUG-104, BUG-105, BUG-88, BUG-96

**Why fourth:** These are user-visible correctness bugs in API behavior and validation, but they do not require changing Agent runtime internals.

**Files:**
- Modify: `internal/api/v2/setup.go`
- Modify: `internal/api/v2/platform.go`
- Modify: `internal/api/handlers/auth.go`
- Modify: `pkg/controllerstorage/ip_groups.go`
- Modify: `pkg/controllerstorage/certificate_lifecycle.go`
- Modify: `pkg/controllerstorage/agent_commands.go`
- Test: `internal/api/v2/*route*_test.go`
- Test: `internal/api/v2/ip_groups_test.go`
- Test: `internal/api/v2/platform_test.go`
- Test: `internal/api/handlers/auth_*_test.go`
- Test: `pkg/controllerstorage/*certificate*_test.go`
- Test: `pkg/controllerstorage/agent_commands_test.go`

**Implementation boundary:**
- Reject empty route update bodies.
- Normalize IPv4 bare IP to `/32` and IPv6 bare IP to `/128`.
- Make refresh and force-change-password use the same Bearer parser as middleware.
- Make IP Group delete reference check and delete atomic enough to return a friendly conflict under concurrent policy creation.
- Include expired-but-issued certificates in the renewal/reconciliation path or explicitly mark them expired first.
- Remove `restart` from allowed commands until implemented, or implement the Agent command in B3 and keep it enabled.
- Validate token tag length/content and node update fields before DB writes.

**Validation:**

```bash
go test ./internal/api/v2 ./internal/api/handlers ./pkg/controllerstorage -count=1
```

**Expected result:** route mutations cannot silently delete routes, IPv6 routes normalize correctly, auth header behavior is consistent, and invalid user inputs fail with clear 4xx errors.

---

### B5: Frontend Stability and Policy Navigation

**Priority:** P2

**Bugs:** BUG-58, BUG-59, BUG-60, BUG-61, BUG-77, BUG-78, BUG-79, BUG-106

**Why fifth:** These are visible UI bugs, but they should follow API correctness so frontend tests can target final API behavior.

**Files:**
- Modify: `frontend/src/main.ts`
- Modify: `frontend/src/views/Routing.vue`
- Modify: `frontend/src/views/IPGroups.vue`
- Modify: `frontend/src/views/Tokens.vue`
- Modify: `frontend/src/views/Nodes.vue`
- Modify: `frontend/src/views/Policies.vue`
- Modify: `frontend/src/composables/useFocusedPolling.ts`
- Test: `frontend/tests/unit/*`

**Implementation boundary:**
- Replace bare `error.message` access with typed error helpers.
- Catch `bootstrap()` rejection and render/log a deterministic startup failure state.
- Make node edit route synchronization either transactional from the user's perspective or explicitly report partial success and refresh baseline before retry.
- Add polling backoff and stop conditions after repeated 5xx failures.
- Replace CIDR regex validation with a real CIDR/IP parser helper shared by Routing and Nodes.
- Change `@click="goToIpGroups"` to a wrapper that passes no MouseEvent as policy context.

**Validation:**

```bash
cd frontend
npm run type-check
npm test -- --run
npm run build
```

**Expected result:** frontend error handling does not throw secondary exceptions, policy navigation keeps the intended context, and route input validation matches backend behavior.

---

### B6: CI/CD and Deployment Guardrails

**Priority:** P2/P3

**Bugs:** BUG-107, BUG-108, BUG-109, BUG-110

**Why sixth:** These protect delivery quality and prevent accidental stale deploys. They should be separate from runtime bugfixes so CI workflow changes are easy to review.

**Files:**
- Modify: `.github/workflows/build.yml`
- Modify: `deployments/ansible/playbooks/deploy-controller.yml`
- Modify: `deployments/ansible/group_vars/all.yml`
- Modify: `deployments/ansible/playbooks/deploy-agent.yml`
- Optionally modify: `docs/deployment.md`

**Implementation boundary:**
- Remove `continue-on-error: true` from required artifact uploads.
- Make Docker publish depend on Go, frontend, and Rust jobs, or split Controller image publish from full release artifact publish explicitly.
- Restrict `latest` push to `master` or release tags.
- Stop pinning old Controller image versions in Ansible defaults; require version input or read `VERSION`.
- Align Agent Ansible path, binary, and service names with `deployments/scripts/deploy-agent.sh`.

**Validation:**

```bash
git diff --check -- .github/workflows/build.yml deployments/ansible docs/deployment.md
```

**CI requirement:** Push branch and verify GitHub Actions workflow behavior on the branch before merging.

**Expected result:** missing artifacts fail CI, `latest` cannot be published from arbitrary branches, and stale Ansible scripts cannot silently roll back production.

---

### B7: Scale and Performance Hardening

**Priority:** P3/P4

**Bugs:** BUG-84, BUG-85, BUG-86, BUG-89, BUG-90, BUG-91, BUG-92, BUG-93, BUG-98, BUG-94, BUG-97, BUG-99

**Why last:** These are real but mostly scale/performance issues. They should not block correctness/security closure unless production load shows they are urgent.

**Files:**
- Modify: `internal/api/v2/monitoring.go`
- Modify: `internal/api/v2/operations.go`
- Modify: `internal/controller/grpc/server.go`
- Modify: `pkg/controllerstorage/postgres.go`
- Modify: `pkg/controllerstorage/agent_commands.go`
- Modify: `pkg/controllerstorage/network_policy.go`
- Modify: `agent-rust/agent/src/agent_runtime.rs`
- Modify: `agent-rust/agent/src/grpc_client.rs`
- Test: `internal/api/v2/nodes_monitoring_behavior_test.go`
- Test: `pkg/controllerstorage/*test.go`
- Test: `internal/controller/grpc/*test.go`

**Implementation boundary:**
- Add ACL `(tenant_id, node_id)` index and hostname lookup indexes.
- Add paginated node listing where API callers need bounded responses.
- Bound topology link generation or derive links from actual peer/traffic data.
- Replace command-stream DB polling with notification/backoff or at least idle exponential backoff.
- Batch command queueing/audit writes where practical.
- Remove fragile string-concatenation query helpers or constrain them to typed internal options.
- Add command deadline fields or indexes that make timeout sweeps index-friendly.
- Optimize Rust per-sync/per-metric allocation only after correctness batches pass.

**Validation:**

```bash
go test ./internal/api/v2 ./internal/controller/grpc ./pkg/controllerstorage -count=1
cd agent-rust && cargo test -p aria-agent --lib
```

**Expected result:** monitoring and command APIs stay bounded at larger tenant sizes, database plans use intended indexes, and Rust Agent allocation pressure is reduced without changing behavior.

---

## Recommended Execution Policy

1. Create one branch per batch: `codex/bugfix-b1-security`, `codex/bugfix-b2-backup-webhooks`, and so on.
2. For each batch, write regression tests first for every bug being fixed.
3. Keep P0/P1 batches small enough to merge independently.
4. Do not start B7 until B1-B6 are merged or explicitly deferred.
5. For Controller/frontend-only batches, use local build/test first and low-bandwidth deployment when deployment validation is needed.
6. For Rust Agent runtime batches, require GitHub Actions Rust build artifacts before deployment validation.

## Status After This Plan

- `BUG-62` and `BUG-83` are excluded.
- `BUG-94`, `BUG-97`, and `BUG-99` are optimization debt, not correctness blockers.
- Highest-value first batch is B1.
- Highest operational-risk batch is B3 because it touches Rust Agent runtime.
- Fastest visible product-quality batch is B5.
