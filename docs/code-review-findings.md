# Code Review Findings

> Living document from pre-landing reviews.  
> Last updated: 2026-06-03.  
> Reviewed tree: `codex/control-plane-phase1-prep` at `95b3a1a` plus prior `master` fixes through `7736374`.

Status legend: `open` | `fixed` | `partial` | `wontfix` | `deferred`

---

## Current Summary

The original review tracked 62 findings. The old 2026-06-01 snapshot is stale:
most items that were listed as `open` there were fixed by the 2026-06-02
bugfix batches and the 2026-06-03 super-admin bootstrap hardening.

| Outcome | Count |
|---------|-------|
| fixed | 57 |
| partial | 2 |
| open | 2 |
| wontfix | 1 |
| total | 62 |

## Still Open

| ID | Severity | Status | Current finding | Recommended next step |
|----|----------|--------|-----------------|-----------------------|
| GRPC-003 | P1 | open | gRPC registration still adapts proto requests through a map-shaped REST adapter and then separately loads the node and issues the runtime token. It now reuses `processRegistration`, but the typed REST/gRPC registration contract is still split. | Replace the map adapter with a shared typed registration service/result used by both HTTP and gRPC. |
| AUTH-019 | P3 | open | `RequirePermission` middleware exists, but v2 routes use `authorizeTenantPermission` directly. This is not currently a security bypass, but it is duplicated RBAC plumbing. | Either remove/deprecate `RequirePermission` or wire routes through it consistently. |

## Partial

| ID | Severity | Status | Current finding | Recommended next step |
|----|----------|--------|-----------------|-----------------------|
| AUTH-002 | P0 | partial | Forced password change is enforced by the router and persisted `aria_must_change_password`, but the UX still redirects to `/login` rather than a dedicated change-password route. | Add a dedicated first-login change-password screen or explicitly document login-page handling as accepted. |
| HOST-001 | P3 | partial | Tenant-scoped hostname lookup exists for core API paths. Agent tools and `admin ban --hostname` now refuse ambiguous global hostname matches instead of picking the first row, but they are not tenant-scoped workflows. | Keep fail-on-ambiguous behavior or add tenant selector/tenant argument for admin tools. |

## Recently Fixed Stale Entries

These were previously listed as open or partial but are fixed in the current
reviewed tree.

| ID | Previous stale status | Current status | Evidence |
|----|-----------------------|----------------|----------|
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

## Phase 1 Control-Plane Preconditions

Before starting the Phase 1 control-plane work, close or explicitly defer:

1. `GRPC-003`: required before capability/audit/sync contracts can be trusted to
   behave the same for HTTP and gRPC registration.
2. `AUTH-019`: not blocking Phase 1 if treated as an RBAC architecture cleanup,
   but it should be marked `deferred` if not implemented.

`AUTH-002` and `HOST-001` are useful follow-ups, but they do not block Phase 1
if their current behavior is accepted and documented.
