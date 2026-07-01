# Unfinished Tasks Status

**Last verified**: 2026-07-01

This document records the current unfinished work from the active checkout. It
does not count stale unchecked items from old plans when the linked feature has
already shipped or has online validation evidence.

## Current Facts

| Item | Current value |
| --- | --- |
| Current branch | `codex/policy-workbench-unification` |
| Current version | `0.2.90` |
| `origin/master` head | `2887daf fix: make agent endpoint port adjustment fallible` |
| Current branch delta | documentation and policy workbench changes in progress |
| Latest master CI | `28524622363` passed |
| Latest master workflow_dispatch | `28525088215` passed |
| Latest online deploy | `0.2.89` from `2887daf` |
| Open tracked bugs | `0` |
| BUG-25 to BUG-57 | all `FIXED` in `docs/confirmed-bugs.md` |
| Tracked local modification | none at branch creation |

## Must Close

These are the active delivery closure tasks for the current branch.

1. Record the `0.2.89` deployment in `docs/deployment.md`.
2. Unify the policy workbench experience across IP Group, ACL, Bandwidth,
   Route, Policy Center, Nodes, and Monitoring:
   - IP Group references show ACL/QoS usage and latest delivery status;
   - ACL/QoS/Route rows deep-link back to the referenced node and policy;
   - Nodes and Monitoring surface the same policy failure reason and context;
   - focused status polling keeps pending policy and node states current.
3. Add regression coverage for the workflow paths that have regressed recently:
   - Nodes advertised route edit;
   - IP Group references and delete preflight;
   - ACL/QoS create, edit, delete, and delivery status;
   - focused policy/node status polling;
   - Settings Backup safety paths;
   - Agent command status transitions.
4. Push the feature branch and run GitHub Actions.
5. Gray-validate the policy workbench workflow online.
6. Merge to `master`, run master Actions, deploy master artifacts, and smoke
   test if the branch changes deployed behavior.

## Active Follow-Up Work

These are real follow-up tracks, but they should not block the delivery closure
tasks above.

1. Hermes Agent design and integration. The old AI write path remains
   fail-closed.
2. IM alert integration for Feishu/DingTalk, confirmation cards, and result
   writeback.
3. Full self-healing loop for low-risk actions and multi-step action plans.
4. Backup operations hardening: restore runbook, retention policy, offline
   encrypted archive, and finer-grained restore audit.
5. Test hardening: gRPC end-to-end tests, VictoriaMetrics integration tests,
   performance baseline, and coverage threshold gate.
6. Frontend TypeScript SFC migration. `useAiApi.js` remains deferred until
   Hermes work starts.
7. Technical-debt cleanup, including the 24 dead-code cleanup items recorded in
   the i18n plan.

## Not Counted As Active Unfinished Work

- `docs/superpowers/plans/2026-06-28-platform-backup-certificate-closure.md`
  has `0` unchecked executable items and `39` checked executable items.
- `docs/superpowers/plans/2026-06-28-ip-group-reference-closure.md` still has
  unchecked boxes, but the implementation exists in code and has branch CI plus
  gray deploy evidence. The remaining work is delivery closure and checklist
  synchronization, not reimplementation.
- Old control-loop and operations-loop unchecked checklist entries are not
  counted here because those v0.1.0 paths already have online validation records.
- `0.2.89` is already deployed from `master`; this branch is preparing
  `0.2.90` and must not reuse `0.2.89` for deployment.
