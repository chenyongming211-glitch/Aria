# B3 Agent Runtime Durability

**Date:** 2026-07-04
**Branch:** `codex/bugfix-b3-agent-runtime`
**Scope:** Rust Agent runtime durability only.

## Bugs

- BUG-64: multi-interface `sync_peers` can partially apply peer changes if a later interface fails.
- BUG-65: corrupt runtime state YAML aborts Agent startup instead of falling back to legacy config or fresh state.
- BUG-66: certificate renewal writes new files before proving gRPC reconnect works, leaving no automatic rollback path.

## Implementation Boundary

- Do not change Controller APIs, frontend, or policy model.
- Do not change the formal command status state machine already closed by the older B3 batch.
- Add regression tests around the new Rust-only safety boundaries.
- Validate with GitHub Actions Rust Agent Build because this machine does not currently have `cargo`.

## Result

- `sync_peers` now snapshots all active WireGuard interfaces, builds all per-interface peer plans before mutation, then applies plans. If execution fails, it attempts to restore every interface to its pre-sync snapshot.
- Runtime state loading now preserves strict `load_state_opt()` diagnostics, but runtime startup through `load_or_migrate_state()` falls back to legacy config or fresh state if the state YAML is unreadable or corrupt.
- Certificate renewal now backs up existing CA/client cert/key files before writing renewed files. Write failure or reconnect failure restores the previous files.

## Validation

Local Rust validation was blocked because `cargo` is not installed on this macOS environment:

```bash
cargo test -p aria-agent
# zsh:1: command not found: cargo
```

Required validation after push:

```bash
gh run watch <run-id> --exit-status
```

Expected CI coverage:

- Rust Agent tests
- Rust Agent build
- Existing Go and frontend CI jobs
