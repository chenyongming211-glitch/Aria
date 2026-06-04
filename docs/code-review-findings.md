# Code Review Findings

> Living document from pre-landing reviews.
> Last updated: 2026-06-04.
> Reviewed tree: `codex/close-review-followups` on top of `master` `ae16de2`.

Status legend: `open` | `fixed` | `partial` | `wontfix` | `deferred`

---

## Current Summary

The original review tracked 62 findings. The old 2026-06-01 snapshot is stale:
most items that were listed as `open` there were fixed by the 2026-06-02
bugfix batches and the 2026-06-03 super-admin bootstrap hardening.

| Outcome | Count |
|---------|-------|
| fixed | 61 |
| partial | 0 |
| open | 0 |
| wontfix | 1 |
| total | 62 |

## Still Open

No tracked open findings remain from this review set.

## Partial

No tracked partial findings remain from this review set.

## Recently Fixed Stale Entries

These were previously listed as open or partial but are fixed in the current
reviewed tree.

| ID | Previous stale status | Current status | Evidence |
|----|-----------------------|----------------|----------|
| GRPC-003 | open | fixed | gRPC Register now accepts a typed `RegistrationRequest` and returns handler-issued `RegistrationResult` values; the gRPC server no longer adapts through a map or independently reloads nodes/signs runtime tokens. |
| GRPC-001 / GRPC-002 | open | fixed | `resolveLegacyAgentIdentity` rejects deleted, suspended, and banned nodes; command stream and metrics bind runtime-token claims to node/tenant identity. |
| ENROLL-002 | open | fixed | Fresh registration saves the node before consuming the enrollment token, so a failed `SaveNode` does not burn the token. |
| POLICY-003 | open | fixed | ACL/QoS PUT handlers load the existing row and preserve omitted fields. |
| AUTH-011 | open | fixed | `GetRolePermissions` does case-insensitive role-name lookup while preferring exact-case matches. |
| AUTH-012 | open | fixed | `loadSession()` seeds `aria_last_activity` when missing. |
| AUTH-014 | open | fixed | Router refreshes permissions through Pinia before falling back to cached permissions. |
| AUTH-015 | open | fixed | Existing `super_admin` passwords are not overwritten by `ARIA_SUPER_ADMIN_PASSWORD` on normal restart. `ARIA_SUPER_ADMIN_SYNC=true` is required for intentional reset. |
| AUTH-020 | open | fixed | Existing `super_admin` username/password migration now requires explicit `ARIA_SUPER_ADMIN_SYNC=true`; normal restarts keep the database identity. |
| ACL-001 | partial | fixed | ACL create/update writes legacy sync columns and sync queries use `COALESCE(src_net, src_cidr)` / `COALESCE(dst_net, dst_cidr)`. |
| ACL-002 | open | fixed | ACL region filtering uses tenant-scoped node lists rather than global `GetAllNodes()`. |
| MON-002 | partial | fixed | Monitoring query failures return errors, and tenant/node metric endpoints return service-unavailable when VictoriaMetrics is required but unavailable. |
| AUTH-019 | open | fixed | Removed the unused `RequirePermission` middleware duplicate; v2 direct `authorizeTenantPermission` remains the canonical RBAC enforcement path. |
| AUTH-002 | partial | fixed | Forced-password sessions now route to a dedicated `/change-password` page instead of being sent back to `/login`. |
| HOST-001 | partial | fixed | Agent hostname tools and `aria admin ban --hostname` accept tenant scope while preserving fail-closed behavior for ambiguous hostnames. |

## Phase 1 Control-Plane Preconditions

Closed before starting the Phase 1 control-plane work:

1. `ENROLL-002`: fixed; enrollment token consumption no longer happens before
   successful node persistence.
2. `GRPC-001` / `GRPC-002`: fixed; lifecycle gates now apply to legacy identity
   fallback and runtime-token node binding.
3. `GRPC-003`: fixed; gRPC Register now shares the typed registration result
   contract with the REST registration path.

The follow-up cleanup items `AUTH-019`, `AUTH-002`, and `HOST-001` were closed
after Phase 1, leaving only the separately accepted `wontfix` item in this
review set.
