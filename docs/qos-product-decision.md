# QoS Product Decision

Last updated: 2026-06-13

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
- mode: `policing`

The current northbound API intentionally rejects protocol matching, source or
destination port matching, and `shaping` mode. Those fields may remain in older
database rows, protobuf structs, or eBPF structs for compatibility and future
work, but they are not current product capabilities.

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
interpreted as node total bandwidth for the matching rule, not as independent
per-interface bandwidth. Runtime packet counters stay in `QOS_STATS` as per-cpu
stats and are aggregated by the Agent when reported to the Controller/UI.

The preferred long-term QoS bucket implementation is a shared locked bucket.
However, Linux requires BTF-style maps for `bpf_spin_lock`, while the current
Aya map macro emits legacy map definitions. Attempting to use `bpf_spin_lock`
with `QOS_TOKEN_BUCKET` is rejected by the kernel verifier with `has to have BTF
in order to use bpf_spin_lock`. Do not reintroduce `bpf_spin_lock` until the
Agent eBPF toolchain supports BTF-style maps or a verified equivalent.

## IP Group Product Model

The product model should be IP Group first, with direct CIDR input treated as a
convenience and migration path. Users and the Controller reason about IP
Groups; the Agent and eBPF datapath reason about local runtime group ids.

The target northbound model is:

- IP Group is a tenant-scoped resource with a stable product id, name, and one
  or more IPv4/IPv6 CIDR members.
- ACL references `src_group_id` and `dst_group_id`.
- QoS references `group_id`, `direction`, `rate_bps`, `burst_bytes`, `mode`, and
  `priority`.
- The UI may still allow users to type a direct CIDR. Saving a direct CIDR
  should create or reuse a system-managed inline group, so the stored product
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
prefix match: a more specific CIDR wins over a broader CIDR. The Controller or
frontend should surface an overlap warning so operators understand which group
will match a packet first, but overlap is not a hard validation failure.

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

Runtime verification should check both ACL and QoS attachment points:

```bash
bpftool net
tc filter show dev aria0 egress
```
