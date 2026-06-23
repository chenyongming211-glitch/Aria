# QoS/ACL Precision Gap Review

Last updated: 2026-06-23

This review compares the current `master` implementation with
`docs/qos-product-decision.md`.

## Summary

The old three-tier QoS product model is removed from production code. The
current product model is unified node-scoped ACL/QoS rules backed by IP Groups,
policy generation, and Agent runtime group ids.

No confirmed QoS/ACL precision gaps remain open in this review set. The items
below are retained as closure evidence so stale findings do not keep
reappearing in future review passes.

## Closure Status

| ID | Status | Current evidence |
| --- | --- | --- |
| G1 | fixed | `agent-rust/ebpf/src/qos.rs` uses `QOS_TOKEN_BUCKET.get_ptr_mut()` for existing buckets and only inserts on first initialization. The bucket remains lock-free and does not use `bpf_spin_lock`. |
| G2 | fixed for current gray environment | Two-Agent UDP gray validation reached the 90% hard floor and the 98% target at 5 Mbps and 10 Mbps with egress EDT pacing and `fq`. |
| G3 | fixed | `frontend/src/views/BandwidthControl.vue` now reports `更新失败` for edit failures and `创建失败` for create failures. |
| G4 | fixed | `frontend/src/composables/useQosApi.js` and `BandwidthControl.vue` prefer group names/CIDRs and fall back to `未知 IP Group`, not bare opaque ids. |

## Fixed Items

### G1: QoS bucket update uses in-place mutation for existing buckets

Current status: fixed.

The accepted production design is still lock-free:

- `QOS_TOKEN_BUCKET` remains a shared non-per-cpu `HashMap`.
- eBPF updates use `get_ptr_mut()` and mutate the map value in place when the
  bucket exists.
- First packet may initialize the bucket and insert it.
- The design still does not use `bpf_spin_lock`.

This keeps the hot path close to the `aria-firewall` shape while avoiding the
current Aya/BTF verifier risk around `bpf_spin_lock`.

### G2: QoS precision meets the current gray-validation floor

Drop-only egress policing did not meet the product SLO in gray validation. A
5 Mbps TCP test delivered roughly 71.6% of the configured rate, below the 90%
minimum floor. The runtime now uses egress EDT pacing with `fq` qdisc for
shaping-capable egress paths and falls back to policing when shaping is not
available.

The gray-validation gate is:

- minimum long-running accuracy: 90%;
- target long-running accuracy: 98% or better when environment allows;
- representative rates: 1 Mbps, 5 Mbps, 10 Mbps;
- real traffic over at least two Agents connected through the WireGuard overlay.

#### Gray Validation Evidence: egress EDT pacing

Run context:

- branch: `codex/qos-acl-precision-plan`
- commit: `617a8a9`
- GitHub Actions run: `27561311535`
- Controller: `8.152.163.101`
- Agent 1: `82.156.48.111`, VPN IP `100.64.0.2`
- Agent 2: `43.143.245.123`, VPN IP `100.64.0.27`
- QoS rule: `a61cd5af-665b-44d8-80ce-76f94e3795f6`
- direction: Agent 2 egress to Agent 1
- datapath: `tc_egress_qos` with `fq` qdisc on `aria0..aria3`

TCP was not used as the final precision gate for this gray pass because the
same two-node path showed heavy retransmits even with a high 100 Mbps QoS rate.
The precision gate therefore used UDP saturation traffic and compared receiver
throughput plus Controller-reported `QOS_STATS` deltas.

| Configured rate | iperf UDP receive | Receiver accuracy | Controller stats delta | Stats accuracy |
| --- | ---: | ---: | ---: | ---: |
| 1 Mbps | 1.057 Mbps | 105.7% | 8,102,391 B over 64.806s | 100.0% |
| 5 Mbps | 4.938 Mbps | 98.8% | 37,827,587 B over 60.525s | 100.0% |
| 10 Mbps | 9.861 Mbps | 98.6% | 75,540,194 B over 60.434s | 100.0% |

Result: the egress EDT pacing implementation meets the 90% hard floor in this
gray environment and reaches the 98% target at 5 Mbps and 10 Mbps. The 1 Mbps
receiver result is slightly above the configured rate, while the Agent-side
stats are effectively on target.

### G3: Frontend QoS failure text is wrong for edits

Current status: fixed.

`frontend/src/views/BandwidthControl.vue` reports:

- `更新失败` when editing an existing QoS rule fails.
- `创建失败` when creating a new QoS rule fails.

### G4: QoS group display can fall back to raw ids

Current status: fixed.

`frontend/src/composables/useQosApi.js` and `BandwidthControl.vue` now prefer:

1. IP Group name.
2. Inline/direct CIDR.
3. Runtime group CIDR when provided by the API.
4. `未知 IP Group` as the fallback label.

The UI no longer uses bare `group_id` as the operator-facing label.

## Already Aligned

- ACL ingress is handled by `xdp_ingress_acl`.
- ACL egress is handled by `tc_egress_acl`.
- `both` ACL rules are expanded into ingress and egress runtime rules.
- ACL and QoS keys include policy generation.
- Agent state tracks active policy generation.
- Controller supports IP Group-first ACL/QoS payloads and inline group fallback.
- Production code no longer exposes `/qos/{category}` or `QoSCategory*` paths.

## Batch Mapping

- Batch 1: keep old three-tier QoS residue out of production and docs. Fixed.
- Batch 2: align bucket updates with in-place mutation where possible. Fixed.
- Batch 3: confirm generation rollback and active-generation behavior. Fixed in
  the ACL/QoS generation switch work.
- Batch 4: fix QoS frontend edit errors and group labels. Fixed.
- Batch 5: close precision through two-Agent gray validation evidence. Fixed for
  the current gray environment.

## Next Review Trigger

Reopen this document only when one of these changes happens:

- QoS eBPF map ABI changes.
- `bpf_spin_lock` or another locking strategy is reintroduced.
- shaping fallback behavior changes.
- the two-Agent gray environment is replaced and the precision SLO needs to be
  re-measured.
