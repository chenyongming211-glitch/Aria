# QoS/ACL Precision Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the current ACL/QoS implementation around the product model in `docs/qos-product-decision.md`, including IP Group semantics, generation atomic apply, UI stats/edit/status behavior, and real two-Agent QoS precision validation.

**Architecture:** Keep Aria SD-WAN's Controller/Agent/IP Group/generation model. Use a lock-free shared QoS token bucket plus egress EDT pacing as the current production design, borrowing the practical runtime tradeoff from `aria-firewall` while preserving Aria SD-WAN's ACL/QoS generation atomic switch. `bpf_spin_lock` remains deferred until the Aya/BTF verifier path is proven online.

**Tech Stack:** Go Controller and storage, Rust Agent and Aya eBPF, Vue 3 frontend, GitHub Actions for build/test. No local compile/build validation.

---

## Product Contract

- QoS is not the old service/peers/ip three-tier model.
- A QoS rule applies aggregate bandwidth on one Agent node. All `aria0..ariaN` WireGuard tunnels on that Agent share the same compiled rule bucket.
- Current production bucket design is lock-free shared `HashMap<QosKey, TokenBucket>` plus per-cpu `QOS_STATS`.
- Egress QoS must use EDT pacing with `fq` qdisc; ingress QoS remains policing.
- Minimum acceptable long-running QoS accuracy is 90% of configured rate under controlled VPN traffic tests.
- Target long-running QoS accuracy is 98% or better when traffic and kernel conditions allow it.
- ACL/QoS must apply by generation: a snapshot either becomes active together or not at all.
- ACL ingress is XDP, ACL egress is TC, and `both` expands to ingress plus egress.
- IP Group is the product model. Direct CIDR input is normalized to inline IP Groups before policy compilation.
- Controller must reject ambiguous equal-priority conflicts and must not use `created_at` as a runtime winner.

## Batch 0: Design And Gap Review

**Files:**

- Modify `docs/qos-product-decision.md`
- Create or update `docs/superpowers/plans/2026-06-15-qos-acl-precision-closure.md`
- Create `docs/code-review-findings.md` entries only if current code gaps need durable tracking

- [ ] Update the product decision doc with the precision SLO:
  - minimum 90% long-running throughput accuracy;
  - target 98% or better;
  - lock-free shared bucket is current production design;
  - `bpf_spin_lock` remains a toolchain/BTF follow-up.
- [ ] Review current Controller, Agent, frontend, and docs against the product decision.
- [ ] Record concrete gaps before changing product behavior.
- [ ] Commit the doc/review baseline.
- [ ] Push the branch and wait for GitHub Actions.

**Verification:**

- GitHub Actions must pass on the doc/review baseline branch before Batch 1 begins.

## Batch 1: Remove Old Three-Tier QoS Residue

**Files to inspect and modify as needed:**

- `README.md`
- `docs/README.md`
- `docs/qos-product-decision.md`
- `docs/v0.1.0-product-blueprint.md`
- `frontend/src/views/BandwidthControl.vue`
- `frontend/src/composables/useQosApi.js`
- `frontend/tests/unit/useQosApi.test.js`
- `internal/api/v2/security.go`
- `internal/api/v2/rbac_handler_matrix_test.go`
- `pkg/controllerstorage/network_policy.go`
- `agent-rust/agent/src/acl_qos_maps.rs`
- `agent-rust/agent/src/acl_qos_manager.rs`
- `agent-rust/ebpf/src/qos.rs`

- [ ] Search for stale terms: `QoSCategoryService`, `QoSCategoryPeers`, `QoSCategoryIP`, `/qos/{category}`, `service QoS`, `peer QoS`, `IP QoS`, `三级`.
- [ ] Delete unused production code paths that still implement category-style QoS.
- [ ] Keep compatibility-only fields only when protobuf/database compatibility requires them, and mark them as non-product behavior.
- [ ] Update frontend labels/help text so QoS is described as rule-level aggregate bandwidth, not category QoS.
- [ ] Add or update tests proving frontend API payloads do not send legacy category/protocol/port QoS fields.
- [ ] Commit Batch 1.
- [ ] Push and wait for GitHub Actions before Batch 2.

**Verification:**

- GitHub Actions must pass.
- `rg "QoSCategory|/qos/\\{category\\}|service QoS|peer QoS|IP QoS|三级"` should only return historical docs explicitly marked stale, or no results.

## Batch 2: Align QoS Shared Bucket Runtime

**Files to inspect and modify as needed:**

- `agent-rust/ebpf/src/qos.rs`
- `agent-rust/shared/src/lib.rs`
- `agent-rust/agent/src/acl_qos_maps.rs`
- `agent-rust/agent/src/acl_qos_manager.rs`
- `agent-rust/agent/src/agent_runtime.rs`
- `agent-rust/agent/src/metrics.rs`
- `internal/controller/grpc/policy_snapshot.go`
- `internal/api/v2/security.go`

- [ ] Confirm `TokenBucket` stays lock-free and ABI-compatible with the deployed map value size.
- [ ] Ensure each compiled QoS rule uses one shared bucket across all `aria*` interfaces.
- [ ] Ensure QoS edit/delete clears stale bucket state for the affected current-generation key.
- [ ] Ensure egress QoS writes EDT timestamps and installs `fq` qdisc for any egress rule.
- [ ] Ensure ingress QoS remains policing-only.
- [ ] Ensure stats stay per-cpu, are created even when user-space insertion misses a key, and are aggregated by product rule id before reporting to Controller/UI.
- [ ] Ensure human-facing errors mention policy names or IP Group names, never raw runtime group ids.
- [ ] Add or update Agent unit tests for rate edit bucket reset and stats ownership.
- [ ] Commit Batch 2.
- [ ] Push and wait for GitHub Actions before Batch 3.

**Verification:**

- GitHub Actions must pass.
- Online gray validation for this batch must show QoS map entries, `fq` qdisc, stats for one created rule, and a 5 Mbps egress TCP test above the 90% floor before continuing if the code changes runtime behavior.

## Batch 3: ACL/QoS Generation Atomic Apply Hardening

**Files to inspect and modify as needed:**

- `agent-rust/shared/src/lib.rs`
- `agent-rust/ebpf/src/acl.rs`
- `agent-rust/ebpf/src/qos.rs`
- `agent-rust/agent/src/acl_qos_state.rs`
- `agent-rust/agent/src/acl_qos_maps.rs`
- `agent-rust/agent/src/acl_qos_manager.rs`
- `agent-rust/agent/src/agent_runtime.rs`
- `internal/controller/grpc/policy_snapshot.go`
- `pkg/controllerstorage/network_policy.go`

- [ ] Verify ACL and QoS both read the same active policy generation from runtime config.
- [ ] Verify new snapshots write all next-generation ACL, QoS, IP identity, bucket, stats, and port-map state before flipping active generation.
- [ ] Verify failure before the generation flip leaves the previous generation active.
- [ ] Verify garbage collection only removes generations older than the previous active generation after the new generation is live.
- [ ] Add or update tests for failed apply rollback, active generation stats, and `both` ACL expansion.
- [ ] Commit Batch 3.
- [ ] Push and wait for GitHub Actions before Batch 4.

**Verification:**

- GitHub Actions must pass.
- Gray validation must show `POLICY_TABLE`, `QOS_CONFIG`, and runtime config generation aligned after a policy update.

## Batch 4: Frontend Stats, Edit, And Status Closure

**Files to inspect and modify as needed:**

- `frontend/src/views/BandwidthControl.vue`
- `frontend/src/views/ACLRules.vue`
- `frontend/src/views/IPGroups.vue`
- `frontend/src/composables/useQosApi.js`
- `frontend/src/composables/useAclApi.js`
- `frontend/src/composables/useIpGroupApi.js`
- `frontend/tests/unit/bandwidthControlEdit.test.js`
- `frontend/tests/unit/useQosApi.test.js`
- `frontend/tests/unit/useAclApi.test.js`
- `frontend/tests/unit/pagePermissionVisibility.test.js`

- [ ] Ensure QoS rule editing updates an existing rule rather than creating a duplicate.
- [ ] Ensure QoS and ACL status names distinguish `pending`, `sent`, `applied`, `failed`, and no-data states.
- [ ] Ensure stats display pass/drop/shape bytes and packets from the API without collapsing load errors into zero.
- [ ] Ensure UI text says aggregate rule bandwidth and 90% minimum / 98% target gray validation where operator help text is appropriate.
- [ ] Ensure inline CIDR creation displays as an inline IP Group, not as an opaque runtime group id.
- [ ] Commit Batch 4.
- [ ] Push and wait for GitHub Actions before Batch 5.

**Verification:**

- GitHub Actions must pass.
- Frontend unit tests must cover editing QoS rates, API normalization, and stats display.

## Batch 5: Two-Agent Gray Traffic Validation

**Environment:**

- Controller: `8.152.163.101`
- Agent 1: `82.156.48.111`
- Agent 2: `43.143.245.123`
- Validate through GitHub Actions artifacts only. Do not compile locally.

- [ ] Deploy the branch Controller, frontend, and Agent artifacts to gray validation.
- [ ] Confirm both Agents are online, have VPN IPs, and can route overlay traffic.
- [ ] Create ACL allow and deny rules through API or UI and prove traffic pass/drop with stats.
- [ ] Create QoS rules at representative rates: 1 Mbps, 5 Mbps, and 10 Mbps when the environment can generate enough traffic.
- [ ] For each QoS test, run sustained traffic for at least 60 seconds and record configured rate, measured throughput, duration, pass/drop/shape stats, and direction.
- [ ] Mark the batch passing only if long-running measured rate is at least 90% accurate. Record whether the target 98% was reached.
- [ ] If a test misses 90%, stop and fix the runtime behavior before merging.
- [ ] If tests pass, report gray validation evidence and wait for merge confirmation if the user has not already authorized merge.

**Verification:**

- Both Agents remain `online` and `observed_state=applied`.
- `bpftool net` shows expected XDP/TC attachments.
- `QOS_STATS` and ACL stats increase under real traffic.
- UI displays the same rule status and stats after refresh.

## Merge And Release Gate

- Do not merge this branch into `master` until the gray validation passes.
- After merge, run `master` GitHub Actions and `workflow_dispatch`.
- Deploy only `master` artifacts after `master` Actions pass.
- Record run id, commit, deployed image/artifact, and gray validation evidence in deployment docs.
