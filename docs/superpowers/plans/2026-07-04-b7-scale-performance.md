# B7 Scale and Performance Hardening

**Date:** 2026-07-04
**Branch:** `codex/bugfix-b7-scale-performance`
**Scope:** Controller storage/API scale paths, frontend node-list compatibility, CommandStream idle polling, and low-risk Rust Agent allocation hot spots. No online deployment in this batch.

## Bugs

- BUG-84: Node detail certificate activity used repeated audit queries.
- BUG-85: batch Agent commands wrote one command and audit event per node.
- BUG-86: CommandStream used fixed idle polling.
- BUG-89: node list query helper accepted raw SQL fragments.
- BUG-90: ACL tenant/node lookup lacked a composite index.
- BUG-91: hostname lookup lacked an index.
- BUG-92: API node list paths lacked bounded pagination.
- BUG-93: topology generated unbounded mesh links.
- BUG-94: Rust policy metric labels used repeated `format!()` calls.
- BUG-97: Rust peer diff cloned desired peers during sync.
- BUG-98: agent command timeout scans used non-index-friendly deadline expressions.
- BUG-99: Rust sync response conversion used chained `map().collect()` conversions.

## Fix Summary

- Added typed `NodeListOptions` plus `GetNodesByTenantPage` and moved API-facing node list paths to bounded reads.
- Added `idx_nodes_hostname`, `idx_acl_rules_tenant_node`, `agent_commands.deadline_at`, and `idx_agent_commands_node_status_deadline`.
- Replaced command timeout expression scans with indexed `deadline_at` checks.
- Added `QueueAgentCommands` and `CreateAuditEvents` bulk insert helpers for batch Agent commands.
- Changed CommandStream idle polling to exponential backoff with a cap and reset on command delivery.
- Consolidated node certificate activity into one `DISTINCT ON (event_type)` audit query.
- Bounded monitoring topology nodes/links and returned `links_truncated` when generated mesh links are capped.
- Updated frontend node-list consumers to unwrap both legacy arrays and paginated `{ items }` payloads.
- Reduced Rust allocation pressure by pre-sizing policy metric maps, replacing hot-path policy metric `format!()` calls, borrowing desired peers during diff, and preallocating sync response conversion vectors.

## Residual Notes

- BUG-84 is partially mitigated, not fully collapsed into one node-detail query. The remaining independent detail sections are still separate business queries.
- BUG-97 is partially mitigated. The `sync_peers` entry still clones the peer slice once because `spawn_blocking` requires owned `'static` data; removing that safely requires a separate blocking-boundary refactor.
- Rust local validation is blocked on this macOS workstation because `cargo` and `rustfmt` are not installed. Branch CI must pass the Rust Agent build/test job before merge.

## Validation

```bash
go test ./internal/api/v2 ./internal/controller/grpc ./pkg/controllerstorage -count=1
go test ./... -count=1
cd frontend && npm run type-check
cd frontend && npm run test:run -- tests/unit/apiResponse.test.ts tests/unit/useTenantApi.test.ts tests/unit/nodeStore.test.js tests/unit/useRouteApi.test.js
cd frontend && npm run test:run
cd frontend && npm run build
git diff --check
```

Local Rust validation attempted but blocked:

```bash
cargo --version
# zsh:1: command not found: cargo

rustfmt --version
# zsh:1: command not found: rustfmt
```

CI requirement:

```bash
cd agent-rust && cargo test -p aria-agent --lib
```

Expected: GitHub Actions Rust Agent validation must pass before this branch is merged or deployed.
