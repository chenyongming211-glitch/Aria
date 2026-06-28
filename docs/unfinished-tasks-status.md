# Unfinished Tasks Status

**Last verified**: 2026-06-28 21:23 +0800

This document records the current unfinished work from the active checkout. It
does not count stale unchecked items from old plans when the linked feature has
already shipped or has online validation evidence.

## Current Facts

| Item | Current value |
| --- | --- |
| Current branch | `codex/ip-group-reference-closure` |
| Current version | `0.2.85` |
| `origin/master` head | `739c222 docs: record platform backup certificate deployment` |
| Current branch delta | `10` commits ahead of `origin/master` |
| Branch CI | `28317366377` passed for `95a435f` |
| Online gray deploy | `0.2.85` from `95a435f` |
| Open tracked bugs | `0` |
| BUG-25 to BUG-37 | all `FIXED` in `docs/confirmed-bugs.md` |
| Tracked local modification | `docs/superpowers/plans/2026-06-28-i18n-hardcoded-text-migration.md` |

## Must Close

These are the active delivery closure tasks.

1. Merge `codex/ip-group-reference-closure` into `master`.
2. Run and verify `master` GitHub Actions after the merge.
3. Deploy the `master` `0.2.85` Controller/frontend artifacts online.
4. Add missing deployment records in `docs/deployment.md` for `0.2.84` and
   `0.2.85`.
5. Run IP Group reference online smoke validation:
   - references endpoint returns ACL/QoS references;
   - delete is blocked when a group is referenced;
   - ACL/QoS click-through opens the referenced node/rule context;
   - latest delivery status is shown after retry.
6. Commit the tracked i18n plan update, including the dead-code cleanup plan.
7. Delete the stale local branch `codex/i18n-hardcoded-text-migration` after the
   current branch is merged, because it is already an ancestor of
   `codex/ip-group-reference-closure`.

## Active Follow-Up Work

These are real follow-up tracks, but they should not block the 7 delivery
closure tasks above.

1. Strategy workbench experience unification across IP Group, ACL, Bandwidth,
   Route, Policy Center, Nodes, and Monitoring.
2. Hermes Agent design and integration. The old AI write path remains
   fail-closed.
3. IM alert integration for Feishu/DingTalk, confirmation cards, and result
   writeback.
4. Full self-healing loop for low-risk actions and multi-step action plans.
5. Backup operations hardening: restore runbook, retention policy, offline
   encrypted archive, and finer-grained restore audit.
6. Test hardening: gRPC end-to-end tests, VictoriaMetrics integration tests,
   performance baseline, and coverage threshold gate.
7. Frontend TypeScript SFC migration. `useAiApi.js` remains deferred until
   Hermes work starts.
8. Technical-debt cleanup, including the 24 dead-code cleanup items recorded in
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
