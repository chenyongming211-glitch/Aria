# Policy Generation Atomic Apply and QoS Locked Bucket Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ACL and QoS snapshots apply as one generation so they either become active together or not at all, then make each QoS rule's aggregate token bucket accurate across CPUs and all `aria*` tunnels.

**Architecture:** Phase 1 adds policy generation to the shared Agent/eBPF ABI. The Agent writes a complete next generation into ACL and QoS maps, flips the active generation only after every map write and qdisc operation succeeds, then garbage-collects old generation entries. Phase 2 changes the QoS token bucket value to carry a kernel spin lock while keeping per-cpu stats, so one QoS rule has one aggregate bandwidth bucket across all tunnel interfaces.

**Tech Stack:** Rust Agent, Aya eBPF, shared `aria_shared` ABI structs, GitHub Actions for Rust/eBPF/Controller/frontend verification. No local compile/build validation.

---

## Product Contract

- ACL and QoS are one policy snapshot for an Agent node.
- ACL and QoS must become effective together.
- If validation, map writes, qdisc updates, or runtime config writes fail, the old active generation must remain active.
- QoS remains rule-level aggregate bandwidth on a node. It is not per interface and it does not restore old service/peers/ip QoS categories.
- A node can have multiple `aria0..ariaN` WireGuard tunnels, but one QoS rule's limit applies to their combined traffic.
- Stats stay aggregated by product rule id, not by tunnel interface.
- Phase 1 must not add `bpf_spin_lock`; Phase 2 owns that ABI risk.

## Batch 1: ACL/QoS Generation Atomic Apply

**Files:**

- Modify `agent-rust/shared/src/lib.rs`
  - Add `generation` to `PolicyKey`, `PortKey`, and `QosKey`.
  - Add active `policy_generation` fields to runtime config structs.
  - Update ABI size tests.
- Modify `agent-rust/ebpf/src/acl.rs`
  - Read active generation from runtime config.
  - Include generation in all policy and port bitmap lookups.
  - Update rule stats by generated key.
- Modify `agent-rust/ebpf/src/qos.rs`
  - Read active generation from runtime config.
  - Include generation in QoS config, bucket, and stats lookups.
- Modify `agent-rust/agent/src/acl_qos_maps.rs`
  - Include generation in all key construction.
  - Add runtime config generation flip.
  - Add generation cleanup helpers for ACL/QoS entries and port sets.
- Modify `agent-rust/agent/src/acl_qos_state.rs`
  - Persist active policy generation in in-memory state.
- Modify `agent-rust/agent/src/acl_qos_manager.rs`
  - Write failing tests for generation planning and rollback semantics.
  - Build next state and compiled rules before touching active maps.
  - Insert all next-generation ACL/QoS entries first.
  - Flip runtime config generation after all writes and qdisc operations succeed.
  - Keep old state on failure; only replace state and snapshot cache after flip.

**Verification:**

- CI must build the eBPF objects and Agent with the new ABI.
- CI must pass Agent unit tests covering:
  - next generation increments from `0` to `1`;
  - generation wraps away from `0`;
  - a failed candidate snapshot does not replace the active generation/state;
  - active stats only aggregate current-generation keys.

## Batch 2: QoS Shared Locked Bucket

**Files:**

- Modify `agent-rust/shared/src/lib.rs`
  - Add a lock field to `TokenBucket` using the Aya-supported BTF/spin-lock representation.
  - Update ABI size tests.
- Modify `agent-rust/ebpf/src/qos.rs`
  - Use the lock around token refill, consume, shaping, and drop accounting.
  - Keep `QOS_STATS` as per-cpu stats.
- Modify `agent-rust/agent/src/acl_qos_maps.rs`
  - Initialize locked bucket values for new generation entries.

**Verification:**

- CI must build the eBPF verifier target.
- Online gray validation must create one QoS rule, generate traffic across both Agents, and confirm stats increase while the limit is aggregate across all `aria*` tunnels.

## Online Gray Validation

- Deploy the branch artifact before merging to `master`.
- Create ACL allow and deny policies through the Controller API and UI.
- Create a QoS rule through the Controller API and UI.
- Wait for Agent sync.
- Verify:
  - `bpftool map dump name POLICY_TABLE` only uses current active generation for hits.
  - `bpftool map dump name QOS_CONFIG` and `QOS_STATS` include current active generation.
  - `tc filter show dev aria0 ingress/egress` and the other `aria*` interfaces still show expected ACL/QoS programs.
  - Allowed traffic passes and increments pass stats.
  - Denied traffic drops and increments drop stats.
  - QoS stats increase under real VPN traffic.

## Rollback

- If Batch 1 CI fails, do not start Batch 2.
- If a deployed Agent cannot load new pinned maps because ABI changed, stop Agent, remove only Aria ACL/QoS pinned maps under `/sys/fs/bpf/aria`, restart Agent, and repeat gray validation.
- If QoS locked bucket verifier fails, keep Batch 1 and split Phase 2 into a separate verifier-focused branch.
