# QoS Product Decision

Last updated: 2026-06-10

Aria SD-WAN no longer uses the old "three-tier QoS" model. Do not describe QoS
as `service / peers / ip`, and do not introduce new UI, API, Controller, or
Agent behavior around those categories.

## Current Model

QoS is a unified node-scoped rule model. A rule matches traffic by fields such as:

- direction: `ingress`, `egress`, or `both`
- source CIDR
- destination CIDR
- protocol
- source or destination port
- bandwidth
- burst
- mode: `policing` or `shaping`
- priority

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
