# Agent ACL/QoS Dataplane Replacement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current Agent ACL/QoS implementation with the logic from `agent-acl-qos-implementation.rs`, then adapt Controller and frontend to the new group/direction/stats model.

**Architecture:** Controller remains responsible for SaaS tenant isolation and compiles tenant/node policy records into an Agent-local snapshot. Agent has no tenant dimension; it receives only its node snapshot and applies it through group ids, tap-aware eBPF maps, port bitmap pools, runtime enable flags, and ACL/QoS stats maps. The old `POLICY_MAP`, `BLOCK_*`, `SRC_ID_QOS_MAP`, `PAIR_ID_QOS_MAP`, and `SERVICE_QOS_MAP` execution model is removed rather than kept as compatibility.

**Tech Stack:** Go Controller, Vue 3 frontend, Rust Agent, aya/aya-ebpf, PostgreSQL, GitHub Actions for all compile/build/test verification.

---

## Constraints

- Do not locally compile or build. Use GitHub Actions for Go/Rust/frontend test and build verification.
- Start from a clean `origin/master` worktree. Do not continue from `/Users/chen/code/aria-sdwan` while it contains local dirty files.
- Agent does not store or evaluate tenant ids.
- Controller must keep tenant isolation by querying policies with `tenant_id + node_id`.
- The reference file is a logic source, not a file to paste verbatim.
- Online deploy happens only after Actions are green for the final replacement branch.

## Current Implementation To Remove

- `agent-rust/agent/src/acl.rs`
  - Remove old `AclManager` logic based on `POLICY_MAP`, `BLOCK_SRC_ID_MAP`, `BLOCK_DST_ID_MAP`, `BLOCK_PORT_MAP`.
- `agent-rust/agent/src/qos.rs`
  - Remove old `QoSManager` logic based on `SRC_ID_QOS_MAP`, `PAIR_ID_QOS_MAP`, `SERVICE_QOS_MAP`.
- `agent-rust/agent/src/sync_apply.rs`
  - Remove adapter that expands new ACL/QoS contract into old map operations.
- `agent-rust/ebpf/src/acl.rs`
  - Replace old ingress-only XDP ACL lookup with `POLICY_TABLE`, `PORT_BITMAP_POOL`, `RULE_STATS`, runtime ACL gate, direction-aware lookup.
- `agent-rust/ebpf/src/qos.rs`
  - Replace old service/pair/src QoS maps with `QOS_CONFIG`, `QOS_TOKEN_BUCKET`, `QOS_STATS`, runtime QoS gate, ingress/egress direction semantics, policing/shaping mode.
- Old Controller sync conversions may remain only during an intermediate branch batch; final branch must not require old Agent map names.

## Target File Structure

- Modify `agent-rust/shared/src/lib.rs`
  - Add shared ABI structs: `PolicyKey`, `PolicyValue`, `PortKey`, `QosKey`, `QosConfig`, `TokenBucket`, `RuleStatsValue`, `QosStatsValue`, `FirewallConfig`, `TapConfig`.
  - Keep existing constants only if their values match the target implementation.
- Replace `agent-rust/ebpf/src/acl.rs`
  - New ACL datapath with `POLICY_TABLE`, `PORT_BITMAP_POOL`, `RULE_STATS`, `FIREWALL_CONFIG`, `TAP_CONFIG_MAP`.
- Replace `agent-rust/ebpf/src/qos.rs`
  - New QoS datapath with `QOS_CONFIG`, `QOS_TOKEN_BUCKET`, `QOS_STATS`, runtime config maps, policing and shaping behavior.
- Create `agent-rust/agent/src/acl_qos_state.rs`
  - `FirewallState`, `GroupInfo`, `RuleInfo`, `QosRuleInfo`, `PortSetInfo`, and snapshot-oriented state mutation helpers.
- Create `agent-rust/agent/src/acl_qos_maps.rs`
  - Userspace map operations: add/delete ACL policy, add/delete port bitmap, add/delete QoS, runtime config read/update, stats read/clear, FQ qdisc helpers.
- Create `agent-rust/agent/src/acl_qos_manager.rs`
  - Agent-facing manager that owns state, identity/group ids, map runtime, snapshot apply, rollback, and observability.
- Modify `agent-rust/agent/src/grpc_client.rs`
  - Keep parsing `SyncResponse.acl_rules` and `SyncResponse.qos_rules`, but convert to target group/direction structures rather than old source/pair/service targets.
- Modify `agent-rust/agent/src/unified_agent.rs`
  - Replace `AclManager` and `QoSManager` fields with one `AclQosManager`.
  - Replace `sync_acl_rules()` and `sync_qos_rules()` with a single authoritative `sync_acl_qos_snapshot()`.
- Modify `agent-rust/agent/src/main.rs` and `agent-rust/agent/src/lib.rs`
  - Register new modules, remove old module exports.
- Modify `pkg/grpc/agentpb/aria-agent.proto` and generated files if group ids/names are added to the sync contract.
  - Prefer adding optional fields while preserving existing `src_net/dst_net` and `src_ip/dst_ip` until frontend/backend are migrated.
- Modify `internal/controller/grpc/server.go`
  - Build Agent snapshot from tenant/node ACL/QoS records into group-based sync objects.
- Modify `pkg/controllerstorage/network_policy.go`
  - Keep DB tenant/node storage, add compiler-friendly query helpers if needed.
- Modify `internal/api/v2/security.go`
  - Keep node-scoped CRUD; normalize validation to the target model.
- Modify `frontend/src/composables/useAclApi.js`, `frontend/src/composables/useQosApi.js`
  - Preserve API compatibility but expose target fields consistently.
- Modify `frontend/src/views/ACLRules.vue`, `frontend/src/views/BandwidthControl.vue`, `frontend/src/views/Nodes.vue`
  - Display direction, group, ports, mode, delivery status, and stats without relying on old service/pair/source wording where it no longer maps cleanly.

## Batch 0: Clean Worktree And Baseline

**Files:** no source changes

- [ ] **Step 1: Create a clean implementation worktree**

```bash
git fetch origin master
git worktree add /Users/chen/.config/superpowers/worktrees/aria-sdwan/codex-agent-acl-qos-replacement -b codex/agent-acl-qos-replacement origin/master
```

Expected: worktree branch `codex/agent-acl-qos-replacement` starts at current `origin/master`.

- [ ] **Step 2: Copy the reference file into the worktree for local reading only**

```bash
cp /Users/chen/code/aria-sdwan/agent-acl-qos-implementation.rs /Users/chen/.config/superpowers/worktrees/aria-sdwan/codex-agent-acl-qos-replacement/agent-acl-qos-implementation.rs
```

Expected: reference file is available but not wired into crates.

- [ ] **Step 3: Confirm old map references before removal**

```bash
rg -n "POLICY_MAP|BLOCK_SRC_ID_MAP|BLOCK_DST_ID_MAP|BLOCK_PORT_MAP|SRC_ID_QOS_MAP|PAIR_ID_QOS_MAP|SERVICE_QOS_MAP" agent-rust
```

Expected: output shows current old Agent/eBPF paths that later batches will eliminate.

## Batch 1: Shared ABI And Pure Logic

**Files:**
- Modify: `agent-rust/shared/src/lib.rs`
- Create: `agent-rust/agent/src/acl_qos_state.rs`
- Test: Rust unit tests inside the two files

- [ ] **Step 1: Add target ABI structs to shared crate**

Add structs matching the reference logic:

```rust
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq, Hash)]
pub struct PolicyKey {
    pub tap_id: u32,
    pub src_id: u32,
    pub dst_id: u32,
    pub proto: u8,
    pub direction: u8,
    pub pad: [u8; 2],
}

#[repr(C)]
#[derive(Copy, Clone, Debug, Default)]
pub struct PolicyValue {
    pub action: u8,
    pub has_port_filter: u8,
    pub pad1: [u8; 2],
    pub bitmap_idx: u32,
}

#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq, Hash)]
pub struct PortKey {
    pub tap_id: u32,
    pub idx: u32,
    pub port: u16,
    pub pad: u16,
}

#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq, Hash)]
pub struct QosKey {
    pub tap_id: u32,
    pub group_id: u32,
    pub direction: u8,
    pub pad: [u8; 3],
}
```

- [ ] **Step 2: Add config/stats structs to shared crate**

Add `QosConfig`, `TokenBucket`, `RuleStatsValue`, `QosStatsValue`, `FirewallConfig`, and `TapConfig` with the exact field order from `agent-acl-qos-implementation.rs`.

- [ ] **Step 3: Add pure state module**

Create `agent-rust/agent/src/acl_qos_state.rs` with:

```rust
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct GroupInfo {
    pub id: u32,
    pub name: String,
    pub cidrs: Vec<String>,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct RuleInfo {
    pub name: Option<String>,
    pub src_group_id: u32,
    pub dst_group_id: u32,
    pub proto: u8,
    pub action: u8,
    pub ports: Option<String>,
    pub bitmap_idx: Option<u32>,
    #[serde(default)]
    pub direction: u8,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct QosRuleInfo {
    pub group_name: String,
    pub group_id: u32,
    pub direction: u8,
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    #[serde(default)]
    pub mode: u8,
}
```

- [ ] **Step 4: Add state mutation tests**

Test cases:

```rust
#[test]
fn port_sets_are_reused_and_ref_counted() {
    let mut state = FirewallState::default();
    let first = state.apply_add_rule(1, 2, 6, 1, Some("80-82,443:0"), 0).unwrap();
    let second = state.apply_add_rule(3, 4, 6, 1, Some("80-82,443:0"), 0).unwrap();
    assert_eq!(first.bitmap_idx, second.bitmap_idx);
    assert!(!second.is_new_port_set);
}

#[test]
fn both_direction_is_expanded_by_manager_not_state() {
    assert_eq!(direction_from_string("both").unwrap(), 2);
}
```

- [ ] **Step 5: Push branch and wait for GitHub Actions**

```bash
git add agent-rust/shared/src/lib.rs agent-rust/agent/src/acl_qos_state.rs
git commit -m "feat(agent): add ACL QoS shared ABI and state model"
git push -u origin codex/agent-acl-qos-replacement
```

Expected: GitHub Actions green before Batch 2.

## Batch 2: Replace eBPF Datapath ABI

**Files:**
- Replace: `agent-rust/ebpf/src/acl.rs`
- Replace: `agent-rust/ebpf/src/qos.rs`
- Modify: `agent-rust/ebpf/Cargo.toml` only if shared no-std import wiring is required

- [ ] **Step 1: Replace ACL maps**

Remove these old maps:

```rust
POLICY_MAP
BLOCK_SRC_ID_MAP
BLOCK_DST_ID_MAP
BLOCK_PORT_MAP
```

Add target maps:

```rust
#[map(name = "POLICY_TABLE", pin)]
static POLICY_TABLE: HashMap<PolicyKey, PolicyValue> = HashMap::with_max_entries(65536, 0);

#[map(name = "PORT_BITMAP_POOL", pin)]
static PORT_BITMAP_POOL: HashMap<PortKey, u8> = HashMap::with_max_entries(262144, 0);

#[map(name = "RULE_STATS", pin)]
static RULE_STATS: PerCpuHashMap<PolicyKey, RuleStatsValue> = PerCpuHashMap::with_max_entries(65536, 0);
```

- [ ] **Step 2: Implement ACL lookup order**

Implement this lookup contract:

```text
exact src/dst/proto/direction
wildcard src
wildcard dst
full wildcard
proto wildcard
```

When `has_port_filter = 1`, read `PORT_BITMAP_POOL`; missing port uses `PolicyValue.action` as fallback.

- [ ] **Step 3: Replace QoS maps**

Remove these old maps:

```rust
SRC_ID_QOS_MAP
PAIR_ID_QOS_MAP
SERVICE_QOS_MAP
```

Add target maps:

```rust
#[map(name = "QOS_CONFIG", pin)]
static QOS_CONFIG: HashMap<QosKey, QosConfig> = HashMap::with_max_entries(65536, 0);

#[map(name = "QOS_TOKEN_BUCKET", pin)]
static QOS_TOKEN_BUCKET: HashMap<QosKey, TokenBucket> = HashMap::with_max_entries(65536, 0);

#[map(name = "QOS_STATS", pin)]
static QOS_STATS: PerCpuHashMap<QosKey, QosStatsValue> = PerCpuHashMap::with_max_entries(65536, 0);
```

- [ ] **Step 4: Implement QoS direction behavior**

Implement this contract:

```text
egress: match dst_id, then group 0 fallback
ingress: match src_id, then group 0 fallback
mode 0: policing drop when bucket empty
mode 1: shaping via EDT/FQ qdisc on egress
```

- [ ] **Step 5: Push and wait for GitHub Actions**

```bash
git add agent-rust/ebpf/src/acl.rs agent-rust/ebpf/src/qos.rs agent-rust/ebpf/Cargo.toml
git commit -m "feat(agent): replace ACL QoS eBPF datapath ABI"
git push
```

Expected: GitHub Actions green before Batch 3.

## Batch 3: Replace Agent Userspace Manager

**Files:**
- Delete or fully replace old logic in: `agent-rust/agent/src/acl.rs`
- Delete or fully replace old logic in: `agent-rust/agent/src/qos.rs`
- Delete: `agent-rust/agent/src/sync_apply.rs`
- Create: `agent-rust/agent/src/acl_qos_maps.rs`
- Create: `agent-rust/agent/src/acl_qos_manager.rs`
- Modify: `agent-rust/agent/src/lib.rs`
- Modify: `agent-rust/agent/src/main.rs`
- Modify: `agent-rust/agent/src/unified_agent.rs`

- [ ] **Step 1: Add userspace map operations**

Create `acl_qos_maps.rs` with functions equivalent to the reference:

```rust
pub fn add_policy_to_maps(
    src_id: u32,
    dst_id: u32,
    proto: u8,
    action: u8,
    ports: Option<&str>,
    bitmap_idx: Option<u32>,
    is_new_port_set: bool,
    direction: u8,
    runtime: TapMapRuntime<'_>,
) -> Result<(), String>;

pub fn add_qos_rule_to_maps(
    group_id: u32,
    direction: u8,
    rate_bps: u64,
    burst_bytes: u64,
    priority: u8,
    mode: u8,
    runtime: TapMapRuntime<'_>,
    user_qos_enabled: bool,
) -> Result<(), String>;
```

- [ ] **Step 2: Add FQ qdisc helpers**

Implement:

```rust
pub fn ensure_fq_qdisc(iface: &str) -> Result<FqQdiscState, String>;
pub fn cleanup_root_qdisc(iface: &str) -> Result<(), String>;
```

Expected behavior: `mode=shaping` prepares FQ qdisc; removing last shaping rule cleans it up.

- [ ] **Step 3: Add manager snapshot apply**

Create `AclQosManager::apply_snapshot()`:

```rust
pub struct AclQosSnapshot {
    pub groups: Vec<GroupInfo>,
    pub acl_rules: Vec<RuleInfo>,
    pub qos_rules: Vec<QosRuleInfo>,
    pub acl_enabled: bool,
    pub qos_enabled: bool,
}

impl AclQosManager {
    pub fn apply_snapshot(&mut self, snapshot: AclQosSnapshot) -> Result<(), AclQosError> {
        self.replace_groups(snapshot.groups)?;
        self.replace_acl_rules(snapshot.acl_rules)?;
        self.replace_qos_rules(snapshot.qos_rules)?;
        self.update_runtime_config(Some(snapshot.acl_enabled), Some(snapshot.qos_enabled))?;
        self.persist_state()?;
        Ok(())
    }
}
```

- [ ] **Step 4: Modify UnifiedAgent**

Replace:

```rust
acl_mgr: Arc<Mutex<AclManager>>,
qos_mgr: Arc<Mutex<QoSManager>>,
```

with:

```rust
acl_qos_mgr: Arc<Mutex<AclQosManager>>,
```

Replace separate `sync_acl_rules()` and `sync_qos_rules()` calls with:

```rust
self.sync_acl_qos_snapshot(&sync_result.acl_rules, &sync_result.qos_rules).await?;
```

- [ ] **Step 5: Remove old adapter**

Delete `agent-rust/agent/src/sync_apply.rs` and remove `mod sync_apply`.

- [ ] **Step 6: Push and wait for GitHub Actions**

```bash
git add agent-rust/agent/src agent-rust/agent/Cargo.toml
git commit -m "feat(agent): replace ACL QoS userspace manager"
git push
```

Expected: GitHub Actions green before Batch 4.

## Batch 4: Controller Snapshot Compiler

**Files:**
- Modify: `pkg/grpc/agentpb/aria-agent.proto`
- Regenerate: `pkg/grpc/agentpb/aria-agent.pb.go`, `pkg/grpc/agentpb/aria-agent_grpc.pb.go`, `agent-rust/proto/aria-agent.proto` if workflow expects checked-in generated code
- Modify: `internal/controller/grpc/server.go`
- Modify: `pkg/controllerstorage/network_policy.go`
- Modify: `internal/api/v2/security.go`
- Test: `internal/controller/grpc/policy_sync_contract_test.go`

- [ ] **Step 1: Extend sync contract for group model**

Add fields without removing old fields in the same batch:

```proto
message PolicyGroup {
  string name = 1;
  uint32 group_id = 2;
  repeated string cidrs = 3;
}

message ACLRule {
  string src_net = 1;
  string dst_net = 2;
  uint32 protocol = 3;
  uint32 min_port = 4;
  uint32 max_port = 5;
  string action = 6;
  string direction = 7;
  string ports = 8;
  string src_group = 9;
  uint32 src_group_id = 10;
  string dst_group = 11;
  uint32 dst_group_id = 12;
}

message QoSRule {
  string src_ip = 1;
  string dst_ip = 2;
  uint32 src_port = 3;
  uint32 dst_port = 4;
  uint32 protocol = 5;
  uint64 bandwidth_mbps = 6;
  string direction = 7;
  uint64 rate_bps = 8;
  uint64 burst_bytes = 9;
  uint32 priority = 10;
  string mode = 11;
  string group = 12;
  uint32 group_id = 13;
}
```

- [ ] **Step 2: Compile Controller records into groups**

Rules:

```text
ACL src_cidr "" => group any id 0
ACL dst_cidr "" => group any id 0
ACL non-empty cidr => deterministic group name cidr:<cidr>, group id stable within snapshot
QoS egress => group from dst_cidr if present, else src_cidr
QoS ingress => group from src_cidr if present, else dst_cidr
```

The Agent may use snapshot-local group ids; they do not need to be global DB ids.

- [ ] **Step 3: Add contract tests**

Test that one ACL with ports and one QoS shaping rule produce:

```text
acl.src_group_id != 0
acl.dst_group_id != 0
acl.direction == "both" supported
acl.ports == "80-82,443:0"
qos.group_id != 0
qos.mode == "shaping"
qos.rate_bps preserved
qos.burst_bytes preserved
```

- [ ] **Step 4: Push and wait for GitHub Actions**

```bash
git add pkg/grpc agent-rust/proto internal/controller/grpc pkg/controllerstorage internal/api/v2
git commit -m "feat(controller): compile ACL QoS sync snapshots for agent dataplane"
git push
```

Expected: GitHub Actions green before Batch 5.

## Batch 5: Frontend Product Model And Stats

**Files:**
- Modify: `frontend/src/composables/useAclApi.js`
- Modify: `frontend/src/composables/useQosApi.js`
- Modify: `frontend/src/views/ACLRules.vue`
- Modify: `frontend/src/views/BandwidthControl.vue`
- Modify: `frontend/src/views/Nodes.vue`

- [ ] **Step 1: Update ACL UI model**

Expose:

```text
source group/cidr
destination group/cidr
protocol
direction: ingress/egress/both
action: allow/deny
ports
delivery status
stats: packets, bytes, dropped packets, dropped bytes
```

- [ ] **Step 2: Update QoS UI model**

Expose:

```text
group/cidr
direction: ingress/egress
rate_bps and human bandwidth
burst_bytes
priority
mode: policing/shaping
stats: passed/dropped/shaped packets and bytes
```

- [ ] **Step 3: Keep SaaS node scoping**

Every frontend call remains under:

```text
/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/acls
/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos
```

- [ ] **Step 4: Push and wait for GitHub Actions**

```bash
git add frontend/src
git commit -m "feat(frontend): align ACL QoS views with agent dataplane model"
git push
```

Expected: GitHub Actions green before Batch 6.

## Batch 6: Remove Legacy Paths And Deploy

**Files:**
- Remove old code references across `agent-rust`, `internal`, `pkg`, `frontend`
- Modify: `docs/DEPLOYMENT.md` if deployment artifact paths changed
- Modify: `docs/CONFIRMED-BUGS.md` if it references old ACL/QoS bugs

- [ ] **Step 1: Prove old Agent map names are gone**

```bash
rg -n "POLICY_MAP|BLOCK_SRC_ID_MAP|BLOCK_DST_ID_MAP|BLOCK_PORT_MAP|SRC_ID_QOS_MAP|PAIR_ID_QOS_MAP|SERVICE_QOS_MAP" agent-rust
```

Expected: no results.

- [ ] **Step 2: Prove old adapter is gone**

```bash
rg -n "sync_apply|AclApplyOperation|QosApplyTarget|egress ACL is not supported|ingress QoS is not supported|shaping mode is not supported" agent-rust
```

Expected: no results.

- [ ] **Step 3: Commit cleanup**

```bash
git add -A
git commit -m "chore(agent): remove legacy ACL QoS compatibility paths"
git push
```

Expected: GitHub Actions green.

- [ ] **Step 4: Merge to master**

```bash
git fetch origin master
git checkout master
git merge --ff-only codex/agent-acl-qos-replacement
git push origin master
```

Expected: master GitHub Actions green.

- [ ] **Step 5: Run workflow_dispatch**

Trigger the existing manual workflow to publish:

```text
Controller image
frontend-dist artifact
rust-agent-binary artifact
```

Expected: workflow_dispatch green.

- [ ] **Step 6: Deploy Controller, frontend, and Agent**

Deployment order:

```text
1. Backup Controller Postgres and /root/aria-controller/config, certs, frontend dist.
2. Deploy Controller image from GitHub Actions.
3. Deploy frontend-dist artifact.
4. Deploy Agent binary artifact to one canary Agent.
5. Restart canary Agent.
6. Verify command stream, sync, heartbeat.
7. Create one ACL rule and one QoS rule.
8. Verify policy delivery, Agent apply logs, eBPF stats, UI status.
9. Roll out remaining Agents only after canary passes.
```

## Acceptance Criteria

- `rg` finds no old Agent ACL/QoS map names.
- Agent supports ACL `ingress`, `egress`, and `both`.
- Agent supports ACL port bitmap semantics without expanding each port into separate policies.
- Agent supports QoS `ingress` and `egress`.
- Agent supports QoS `policing` and `shaping`; shaping prepares FQ qdisc.
- Runtime config can enable/disable ACL and QoS.
- Per-policy ACL stats and QoS stats are available from Agent and visible through Controller/frontend.
- Controller still enforces tenant and node scoping; Agent remains tenant-unaware.
- GitHub Actions are green for every batch before the next batch begins.
- Online canary proves one real ACL and one real QoS rule apply successfully.

## Risks And Mitigations

- **Risk:** eBPF map struct mismatch between userspace and datapath.
  - **Mitigation:** Define ABI once in `agent-rust/shared/src/lib.rs` or duplicate with exact size tests if no-std import is not possible.
- **Risk:** pinned old maps remain on already-running hosts.
  - **Mitigation:** Agent startup must unpin stale old map names before loading new programs.
- **Risk:** shaping mode changes host qdisc behavior.
  - **Mitigation:** canary one Agent first; cleanup FQ qdisc when last shaping rule is removed.
- **Risk:** Controller snapshot-local group ids shift between syncs.
  - **Mitigation:** deterministic group ordering by normalized group name.
- **Risk:** frontend exposes unsupported legacy category semantics.
  - **Mitigation:** do not keep category as a UI or API grouping; the Agent model is group + direction.

## Execution Rule

Each batch must be implemented, committed, pushed, and verified green in GitHub Actions before the next batch starts. Do not deploy partial replacement branches to production until Batch 6 is green.
