# B4 Controller Correctness Bugfix

**Date:** 2026-07-04
**Branch:** `codex/bugfix-b4-controller-correctness`
**Scope:** Controller/API/storage correctness only; no frontend redesign, no Rust Agent runtime changes, no deployment in this batch.

## Bugs

- BUG-70: IP Group delete reference check and DELETE were not in one transaction.
- BUG-73: expired-but-issued certificates were excluded from renewal candidates.
- BUG-88: tenant token tag had no length/control-character validation.
- BUG-96: node update accepted overlong `hostname`, `region`, and `vpc_id`.
- BUG-102: empty route PUT body could silently delete the existing route.
- BUG-103: bare IPv6 route normalized to `/32` instead of `/128`.
- BUG-104: force-change-password parsed `Bearer` differently from refresh/middleware.
- BUG-105: Controller allowed `restart` even though Agent returns not implemented.

## Fix Summary

- Reject empty route mutation bodies before policy mutation.
- Normalize bare IPv4 host routes to `/32` and bare IPv6 host routes to `/128`.
- Reuse the existing case-insensitive Bearer parser in force-change-password.
- Disable `restart` in the Controller command allowlist until Agent support exists.
- Validate token tag length/content before tenant lookup or DB writes.
- Validate node update identity/location fields before DB writes.
- Include expired-but-issued certificates in renewal candidate queries.
- Run IP Group delete reference check and delete in one transaction.

## Validation

```bash
go test ./internal/api/v2 ./internal/api/handlers ./pkg/controllerstorage -count=1
```

The full Go test suite should be run before the branch is pushed or merged:

```bash
go test ./... -count=1
```
