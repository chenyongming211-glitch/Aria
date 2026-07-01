# QoS Product Decision

Last updated: 2026-06-28

Aria SD-WAN no longer uses the old "three-tier QoS" model. Do not describe QoS
as `service / peers / ip`, and do not introduce new UI, API, Controller, or
Agent behavior around those categories.

## Current Model

QoS is a unified node-scoped rule model. A rule matches traffic by fields such as:

- direction: `ingress`, `egress`, or `both`
- source CIDR
- destination CIDR
- bandwidth
- burst
- mode: `auto`, `policing`, or `shaping`

The current northbound API intentionally rejects protocol matching and source or
destination port matching. QoS mode is a current product capability:

- `auto` is the default. It prefers egress shaping when the Agent can install
  the required `fq` qdisc/EDT path and otherwise falls back to policing.
- `shaping` is an explicit operator request for egress shaping. If the Agent
  cannot enable the required kernel/qdisc support, policy apply fails and the
  delivery error must explain the reason.
- `policing` explicitly uses drop-based policing and does not require `fq`.

Runtime enforcement is direction-aware:

- `egress` QoS can use shaping or policing. Shaping sets EDT on TC egress and
  requires `fq` qdisc; policing drops packets when the rule bucket is empty.
- `ingress` QoS uses policing. Ingress packets cannot be reliably delayed at
  the receiving interface, so ingress enforcement may drop packets when the
  bucket is empty.
- `both` is expanded by the Agent into ingress and egress runtime rules. In
  `auto`, the egress side can shape while the ingress side remains policing.
- EDT must stay bounded so packets are not scheduled beyond the qdisc horizon.

The Controller remains responsible for SaaS tenant isolation and compiles
tenant/node QoS policy records into an Agent-local snapshot. The Agent does not
evaluate tenant ids; it only applies the snapshot for its own node.

ACL uses the same node-scoped snapshot boundary. Its data-plane direction is
explicit:

- `ingress` rules are enforced by `xdp_ingress_acl`.
- `egress` rules are enforced by `tc_egress_acl`.
- `both` rules are expanded into one ingress rule and one egress rule by the
  Agent runtime before writing eBPF maps.

QoS is also enforced at node scope. All attached aria interfaces consume the
same `QOS_TOKEN_BUCKET` entry for a compiled QoS rule. This means bandwidth is
interpreted as rule-level aggregate bandwidth on the Agent node, not as
independent per-interface bandwidth. If a node uses `aria0..ariaN` to spread
traffic across multiple WireGuard tunnels, those tunnels share the same rule
bucket. Runtime packet counters stay in `QOS_STATS` as per-cpu stats and are
aggregated by the Agent when reported to the Controller/UI.

The current production QoS bucket design is a lock-free shared bucket, following
the same practical tradeoff as `aria-firewall`: one shared `HashMap` bucket per
compiled QoS rule and per-cpu stats for counters. This avoids the much larger
long-term error of per-cpu full-rate buckets while staying compatible with the
current Aya 0.13.1 / aya-ebpf 0.1 toolchain.

QoS precision is a product SLO, not an absolute mathematical guarantee:

- Minimum acceptable long-running throughput accuracy is 90% of the configured
  rate under controlled VPN traffic tests. A release candidate that misses 90%
  for representative egress TCP tests must not be merged.
- Target long-running throughput accuracy is 98% or better when traffic,
  kernel scheduling, MTU, and WireGuard overhead allow it.
- Short bursts within `burst_bytes` are expected and must not be treated as
  precision failures.
- Test results must report the configured rate, measured goodput, measured
  wire bytes when available, duration, packet size, direction, and whether the
  rule used policing or shaping semantics.

`bpf_spin_lock` is not a current release requirement. Linux requires BTF-style
maps for `bpf_spin_lock`, while the current Aya map macro emits a map layout
that online Linux 6.8 verifier validation rejects for `QOS_TOKEN_BUCKET` with
`has to have BTF in order to use bpf_spin_lock`. Do not reintroduce
`bpf_spin_lock` until the Agent eBPF toolchain supports BTF-style map value
metadata or a verified equivalent. If real gray tests cannot maintain the 90%
minimum SLO with the lock-free shared bucket, the next project is the eBPF
toolchain/BTF path, not another product-level QoS model rewrite.

## IP Group Product Model

The product model should be IP Group first, with direct CIDR input treated as a
convenience and migration path. Users and the Controller reason about IP
Groups; the Agent and eBPF datapath reason about local runtime group ids.

The implemented northbound model is:

- IP Group is a tenant-scoped resource with a stable product id, name, and one
  or more IPv4/IPv6 CIDR members.
- ACL references `src_group_id` and `dst_group_id`.
- QoS references `group_id`, `direction`, `rate_bps`, `burst_bytes`, `mode`, and
  `priority`.
- The UI still allows users to type a direct CIDR. Saving a direct CIDR creates
  or reuses a system-managed inline group, so the stored product
  model remains group-based.
- The Controller expands IP Groups into CIDR members when compiling the
  node-scoped Agent snapshot.
- The Agent assigns local runtime ids for those CIDR members and writes them to
  source/destination IPv4/IPv6 LPM maps. The eBPF programs then match packets by
  `PolicyKey` or `QosKey` using runtime ids, not raw CIDR strings.

Runtime group ids are local implementation details. They are not stable product
ids, must not be exposed as northbound API identifiers, and may differ across
Agents or restarts.

The `any` group is represented as runtime id `0`. Multiple CIDR members can
map to the same product IP Group. In the eBPF datapath, those members are stored
as multiple LPM entries that point to the same runtime group id.

Overlapping IP Group members are allowed. The datapath semantics are longest
prefix match: a more specific CIDR wins over a broader CIDR. The Controller
returns overlap warnings on IP Group detail/create/update responses, and the UI
shows those warnings so operators understand which group will match a packet
first. Overlap is not a hard validation failure.

## IP Group Reference Contract

IP Group references are first-class control-loop evidence. They are not only a
delete guard. Operators must be able to answer these questions before changing
or deleting a group:

- Which ACL/QoS rules currently reference this group?
- Which nodes are affected?
- What is the latest delivery status for each referencing rule?
- Where can I jump in the UI to inspect or edit that rule?

The reference model should expose product identifiers and names only:

- `domain`: `acl` or `qos`
- `rule_id` and `rule_name`
- `node_id` and `node_name`
- `direction`
- `enabled`
- `latest_delivery`, selected from the newest matching `policy_deliveries`
  record by `created_at DESC`
- a frontend route target for the owning rule

Runtime group ids must not appear in northbound reference responses or operator
errors. If a CIDR is already used by another group, the error should identify
the product group or policy name, not an internal runtime group number.

The latest delivery status for a reference is defined as:

```text
tenant_id = current tenant
node_id = referencing rule node
policy_domain = acl|qos
policy_ref = rule_id
ORDER BY created_at DESC
LIMIT 1
```

Implementations should avoid one query per reference. Prefer a `LEFT JOIN
LATERAL` or window-function query that fetches the newest delivery row while
listing references. This matters because a rule may have old failed deliveries
followed by a successful retry, and the UI must not show stale failure state.

References should be queried lazily through a dedicated endpoint:

```text
GET /api/v2/tenants/{tenant_id}/ip-groups/{group_id}/references?limit=20&offset=0
```

The response shape is:

```json
{
  "items": [],
  "total": 0,
  "limit": 20,
  "offset": 0,
  "has_more": false
}
```

Group detail responses should stay lightweight and should not embed an
unbounded reference list.

Deleting a referenced group remains forbidden. The UI should preflight the
references endpoint before delete, show the first page of references, and offer
links to the affected ACL/QoS rules. Do not add force-delete or auto-clear
semantics for referenced groups.

Updating group name or members should keep the current control-loop contract:
the Controller updates the group and queues policy sync for all affected nodes
in one storage transaction. The update response should continue to expose
dispatch evidence, and the reference list should show the latest delivery state
after Agent sync.

Inline groups created from direct CIDR input are system-managed artifacts. They
should remain hidden or collapsed by default in the IP Group UI, and should not
be manually edited or deleted while referenced by a rule. A future cleanup job
may remove unreferenced inline groups.

Reference navigation is part of the product contract:

```text
ACL reference -> /policy-center/acl-rules?node_id={node_id}&rule_id={rule_id}
QoS reference -> /policy-center/bandwidth-control?node_id={node_id}&rule_id={rule_id}
```

The destination pages must select the node, load rules, locate the row by
`rule_id`, and highlight or open the matching rule. Existing `policyRef` and
`commandId` query parameters remain valid context links, but `rule_id` is the
canonical direct rule target.

## Policy Priority And Conflict Resolution

ACL and QoS rules both have explicit priority. Priority is part of the product
model, not only an implementation detail.

The priority contract is:

- A smaller number has higher priority.
- ACL should expose priority as a normal rule field because allow/deny conflicts
  must be operator-controlled.
- QoS should keep priority too, but it may be shown under advanced settings.
  This is required for conflicts such as `any` total bandwidth plus a narrower
  IP Group bandwidth override.
- The default priority should be consistent across ACL and QoS. Use `100` for
  new rules unless a migration deliberately preserves older values.

When more than one enabled rule can match the same packet or runtime group, the
effective rule is resolved in this order:

```text
1. direction match
2. protocol and port match when the rule type supports them
3. IP Group LPM match; more specific CIDR maps to the more specific runtime group
4. smaller priority number wins
5. if priority is equal, the more specific group/CIDR wins
6. if priority and specificity are still tied, the Controller rejects the write
   as an ambiguous policy conflict
```

This means overlapping IP Groups are allowed, but a broad deny can still beat a
specific allow if the deny has a smaller priority number. The UI should warn
when overlapping groups and equal priority make rule behavior hard to reason
about.

`created_at` must not be used as a product-level rule winner. It is only useful
for list ordering, audit, and deterministic storage output. If two enabled rules
could match the same traffic with the same priority and neither rule is strictly
more specific than the other, the Controller must reject create/update with a
conflict error and tell the operator to change the priority or narrow the match.

Controller conflict detection should run before persisting a policy mutation and
before queueing Agent delivery:

- ACL conflict key: tenant, node, expanded direction, protocol, overlapping
  ports, overlapping source IP Groups, and overlapping destination IP Groups.
- Because the current eBPF `PolicyKey` does not include ports, two enabled ACL
  rules with the same runtime direction/protocol/source/destination key are
  rejected even if their port filters or priorities differ. Supporting multiple
  independent port policies for the same runtime key requires a data-plane ABI
  change first.
- QoS conflict key: tenant, node, expanded direction, overlapping IP Group, and
  same priority.
- Exact duplicate enabled ACL/QoS match scopes at the same priority are rejected,
  even when the action or rate is identical, because stats and delivery ownership
  would be ambiguous.
- Same-priority overlaps are allowed only when one rule is strictly more specific
  than the other. Examples: `/32` inside `/24`, or a narrower source group with
  the same destination group.
- Same-priority cross-overlaps are rejected. Example: rule A has narrower source
  but broader destination, while rule B has broader source but narrower
  destination; neither rule is globally more specific.

Existing `src_cidr` / `dst_cidr` database columns and API payload fields are
transitional compatibility fields. New product design and bug fixes should not
build new behavior around direct CIDR as the primary model; they should either
use IP Groups directly or normalize direct CIDR input into inline groups before
policy compilation.

## Implementation Guidance

Legacy names such as `QoSCategoryService`, `QoSCategoryPeers`, `QoSCategoryIP`,
`/qos/{category}`, `service QoS`, `peer QoS`, or `IP QoS` are stale product
language. If they still exist in code, they are migration residue or
compatibility plumbing, not the product model.

The only valid northbound QoS route shape is node-scoped:

```text
/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos
/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/{rule_id}
```

Future work should keep Controller, Agent, frontend, docs, and tests on the
unified rule model above.

## Verification And Acceptance

Every ACL/QoS release candidate must be verified in this order:

1. GitHub Actions build/test passes. Local compilation is not required and must
   not be used as the authoritative build gate.
2. Deploy the branch artifact to gray validation before merging to `master`.
3. Verify ACL/QoS generation semantics: a new snapshot becomes active only
   after ACL maps, QoS maps, runtime config, and attachment health are all
   valid. If any write fails, the old generation remains active.
4. Verify ACL behavior with real traffic:
   - allow rule passes traffic and increments pass stats;
   - deny rule drops traffic and increments drop stats;
   - `both` expands into ingress XDP and egress TC behavior.
5. Verify QoS behavior with real traffic across at least two Agents connected
   by the WireGuard overlay:
   - create a QoS rule from the Controller/API or UI;
   - wait for Agent sync and map updates;
   - generate sustained traffic for at least 60 seconds;
   - confirm measured long-running egress TCP throughput is at least 90%
     accurate, preferably 98% or better, for representative 1 Mbps, 5 Mbps, and
     10 Mbps policies;
   - record ingress policing separately if tested, because ingress can only
     enforce by drop;
   - confirm `QOS_STATS` pass/drop/shape counters increase and the UI displays
     the aggregated values.
6. Verify rollback/edit behavior:
   - editing a QoS rate clears stale bucket state for that rule;
   - deleting ACL/QoS policies removes stale config, bucket, and stats ownership
     from the current generation;
   - failed delivery surfaces a human-readable rule or IP Group name rather than
     an internal runtime group id.
7. Verify IP Group reference behavior:
   - a group referenced by ACL/QoS cannot be deleted;
   - the references endpoint returns the newest delivery after a failed delivery
     is retried successfully;
   - ACL/QoS reference links open the correct node and rule;
   - updating a referenced group queues sync for affected nodes and exposes
     dispatch evidence.

## Current Implementation Status

As of 2026-06-13, the IP Group model is implemented across Controller storage,
v2 REST API, gRPC Sync snapshots, Agent runtime mapping, and Vue frontend:

- Controller stores tenant-scoped `ip_groups` / `ip_group_members`.
- ACL create/update accepts `src_group_id` / `dst_group_id` and direct
  `src_cidr` / `dst_cidr` fallback.
- QoS create/update accepts `group_id` and direct CIDR fallback.
- QoS create/update accepts `mode=auto|policing|shaping`; omitted mode defaults
  to `auto`.
- Direct CIDR fallback is normalized into inline IP Groups before policy
  compilation.
- Sync snapshots include referenced IP Groups and product group ids.
- Agent maps product group ids to local runtime group ids and writes all group
  CIDRs into the LPM maps.
- Agent keeps `auto` as a runtime management mode until apply time. Egress auto
  resolves to shaping when `fq` qdisc setup succeeds and to policing when it
  cannot be enabled; ingress auto resolves to policing.
- Frontend includes an IP Group management page, ACL source/destination group
  selectors, and QoS group selector.

Known remaining compatibility surface:

- Existing rows may still carry legacy `src_cidr` / `dst_cidr` fields; they are
  read as fallback and should be migrated out later.
- QoS `priority` remains limited to `0..255` until the eBPF ABI is widened.
- `bpf_spin_lock` remains intentionally out of scope for the current Aya map
  layout.
- The IP Group reference endpoint, delete preflight, and rule-level click-through
  are planned closure work and should be implemented before treating IP Group
  management as fully closed.

Runtime verification should check both ACL and QoS attachment points:

```bash
bpftool net
tc filter show dev aria0 egress
```
