# Unfinished Tasks Status

**Last verified**: 2026-07-02

This document records the current unfinished work from the active checkout. It
does not count stale unchecked items from old plans when the linked feature has
already shipped or has online validation evidence.

## Current Facts

| Item | Current value |
| --- | --- |
| Current branch | `master` |
| Current version | `0.2.90` |
| `origin/master` head | `edde6cc docs: record policy workbench master deployment` |
| Local/remote delta | none; local `master` matches `origin/master` |
| Latest master CI | `28530465332` passed |
| Latest runtime deploy | `0.2.90` from `38893ce` with master Actions run `28529945452` |
| Latest docs-only commit | `edde6cc`; CI passed and no runtime redeploy required |
| Latest online smoke | `https://aria.yun/api/version` returned `0.2.90`; Controller and frontend were healthy |
| Open tracked bugs | `0` |
| BUG-25 to BUG-57 | all `FIXED` in `docs/confirmed-bugs.md` |
| Tracked local modification | none at verification time |

## Recently Closed

The `codex/policy-workbench-unification` branch has been merged into `master`
and cleaned up. Its shipped scope is complete:

1. Recorded the `0.2.89` deployment in `docs/deployment.md`.
2. Bumped the product version to `0.2.90`.
3. Fixed IP Group reference routes:
   - ACL references link to `/policy-center/acl-rules`;
   - QoS references link to `/policy-center/bandwidth-control`;
   - frontend navigation prefers named routes with `rule_id` and `node_id`.
4. Added regression coverage for:
   - IP Group references;
   - policy context routing;
   - focused polling;
   - Settings Backup API paths;
   - Agent command status paths;
   - node advertised route edit paths.
5. Gray-deployed `0.2.90`, smoke tested it online, merged to `master`, ran
   master Actions, and redeployed the master artifacts.

## Must Close

There is no active feature branch delivery closure pending.

The next work should start from a fresh branch. If it changes deployed behavior,
it must follow the normal flow: bump `VERSION`, branch CI, gray validation, merge
to `master`, master CI, master deploy, smoke verification, and branch cleanup.

## Active Follow-Up Work

These are real follow-up tracks, but they are not currently blocking a release.

1. **Policy workbench product flow**
   Continue unifying IP Group, ACL, Bandwidth, Route, Policy Center, Nodes, and
   Monitoring around one operator workflow:
   - from IP Group to all referencing policies;
   - from a policy to delivery status and node context;
   - from Nodes and Monitoring back to the exact failed policy;
   - consistent failure reason, retry, and latest delivery display.
2. **Regression test hardening**
   Keep adding tests for paths that have regressed recently:
   - Nodes advertised route edit;
   - IP Group references and delete preflight;
   - ACL/QoS create, edit, delete, and delivery status;
   - focused policy/node status polling;
   - Settings Backup safety paths;
   - Agent command status transitions.
3. **Hermes Agent design and integration**
   The old AI write path remains fail-closed. Hermes should be designed against
   the policy delivery and audit model rather than the old AI tool executor.
4. **IM alert integration**
   Feishu/DingTalk confirmation cards, result writeback, and operator audit
   trail still need product work.
5. **Full self-healing loop**
   Low-risk automatic remediation and multi-step action plans remain future
   work.
6. **Backup operations hardening**
   Restore runbook, retention policy, offline encrypted archive, and
   finer-grained restore audit are still open.
7. **Test infrastructure**
   gRPC end-to-end tests, VictoriaMetrics integration tests, performance
   baseline, and coverage threshold gate are still open.
8. **Frontend TypeScript SFC migration**
   JavaScript modules are largely migrated, but `.vue` SFC migration remains
   incremental work. `useAiApi.js` remains deferred until Hermes starts.
9. **Technical-debt cleanup**
   Continue the dead-code cleanup items recorded in the i18n plan.

## Not Counted As Active Unfinished Work

- `docs/superpowers/plans/2026-06-28-platform-backup-certificate-closure.md`
  has `0` unchecked executable items and `39` checked executable items.
- `docs/superpowers/plans/2026-06-28-ip-group-reference-closure.md` still has
  unchecked boxes, but the implementation exists in code and has branch CI,
  gray deployment, master CI, and master deployment evidence.
- Old control-loop and operations-loop unchecked checklist entries are not
  counted here because those v0.1.0 paths already have online validation
  records.
- `0.2.89` has been superseded by the `0.2.90` policy workbench reference
  deployment.
