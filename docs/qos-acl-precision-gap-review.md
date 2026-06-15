# QoS/ACL Precision Gap Review

Last updated: 2026-06-15

This review compares the current `origin/master` implementation with
`docs/qos-product-decision.md`.

## Summary

The old three-tier QoS product model is mostly removed from production code.
Current remaining work is not a wholesale rewrite; it is a closure pass around
runtime bucket behavior, generation verification, UI status/stats clarity, and
two-Agent gray validation.

## Confirmed Gaps

### G1: QoS bucket update is shared but not aria-firewall-style in-place

Current `agent-rust/ebpf/src/qos.rs` reads a `TokenBucket` value from
`QOS_TOKEN_BUCKET`, mutates a local copy, then writes it back with
`QOS_TOKEN_BUCKET.insert(...)`.

The accepted production design is still lock-free, but it should follow the
`aria-firewall` shape more closely:

- `QOS_TOKEN_BUCKET` remains a shared non-per-cpu `HashMap`.
- eBPF updates should prefer `get_ptr_mut()` and mutate the map value in place
  when the bucket exists.
- First packet may initialize the bucket and insert it.
- The design still does not use `bpf_spin_lock`.

Expected benefit: smaller race window, fewer helper calls in the hot path, and
clearer alignment with the production-proven reference design.

### G2: QoS precision missed the hard floor in gray validation

Drop-only egress policing did not meet the product SLO in gray validation. A
5 Mbps TCP test delivered roughly 71.6% of the configured rate, below the 90%
minimum floor. The runtime must use egress EDT pacing with `fq` qdisc before the
branch can be merged.

The follow-up gray validation must record:

- minimum long-running accuracy: 90%;
- target long-running accuracy: 98% or better when environment allows;
- representative rates: 1 Mbps, 5 Mbps, 10 Mbps;
- real traffic over at least two Agents connected through the WireGuard overlay.

This is a runtime correctness gap until the two-Agent tests meet the 90% floor.

### G3: Frontend QoS failure text is wrong for edits

`frontend/src/views/BandwidthControl.vue` shows `创建失败` for both create and
update failures. When an operator edits a bandwidth rule, failures should say
`更新失败` so the UI matches the action being performed.

### G4: QoS group display can fall back to raw ids

`frontend/src/composables/useQosApi.js` and `BandwidthControl.vue` may display
`group_id` as the fallback group label when the group object/name is missing.
The product model says runtime ids must not be user-facing. Product IP Group
UUIDs are less severe than runtime group ids, but the UI should still prefer
group names or inline CIDRs and show a clear unknown-group label rather than a
bare opaque id.

## Already Aligned

- ACL ingress is handled by `xdp_ingress_acl`.
- ACL egress is handled by `tc_egress_acl`.
- `both` ACL rules are expanded into ingress and egress runtime rules.
- ACL and QoS keys include policy generation.
- Agent state tracks active policy generation.
- Controller supports IP Group-first ACL/QoS payloads and inline group fallback.
- Production code no longer exposes `/qos/{category}` or `QoSCategory*` paths.

## Batch Mapping

- Batch 1: keep old three-tier QoS residue out of production and docs.
- Batch 2: fix G1 and add focused Agent tests where possible.
- Batch 3: add/confirm generation rollback and active-generation tests.
- Batch 4: fix G3 and G4 with frontend tests.
- Batch 5: close G2 through two-Agent gray validation evidence.
