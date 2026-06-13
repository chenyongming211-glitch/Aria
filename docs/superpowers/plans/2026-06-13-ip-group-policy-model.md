# IP Group Policy Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement IP Group as the primary product model for ACL and QoS while preserving direct CIDR input as an inline-group convenience path.

**Architecture:** The Controller owns tenant-scoped IP Group resources, policy references, validation, overlap warnings, and snapshot compilation. The Agent receives group definitions plus policy references, assigns local runtime group ids, writes all group CIDRs to IPv4/IPv6 LPM maps, and the eBPF datapath continues to match `PolicyKey` / `QosKey` by runtime ids. Runtime group ids are local implementation details and must not be exposed as product identifiers.

**Tech Stack:** Go Controller, PostgreSQL schema bootstrap in `pkg/controllerstorage/postgres.go`, gRPC protobuf in `pkg/grpc/agentpb/aria_agent.proto` and `agent-rust/proto/aria_agent.proto`, Rust Agent with Aya eBPF maps, Vue 3 + Element Plus frontend, GitHub Actions for build/test verification.

---

## Product Contract

- IP Group is tenant-scoped.
- IP Group members are IPv4 or IPv6 CIDR values.
- ACL stores and exposes `src_group_id` and `dst_group_id`.
- QoS stores and exposes `group_id`, `direction`, `rate_bps`, `burst_bytes`, `mode`, and `priority`.
- Direct CIDR input remains allowed in the UI and API, but it is normalized into a system-managed inline IP Group before policy storage.
- `any` is a system group and compiles to runtime id `0`.
- Overlapping CIDRs across groups are allowed. Effective datapath matching follows LPM longest-prefix semantics.
- Exact duplicate CIDR members across two non-inline groups are rejected because an eBPF LPM key can store only one runtime id for an exact prefix.
- The Controller and frontend surface overlap warnings so operators understand that the more specific group will match first.
- ACL and QoS both use explicit priority. A smaller number has higher priority.
- New ACL and QoS rules default to priority `100` unless a migration deliberately preserves an existing value.
- Rule conflict resolution is: direction match, protocol/port match where applicable, IP Group LPM match, smaller priority number, then more specific group/CIDR.
- `created_at` is not a policy winner. If two enabled rules can match the same traffic with the same priority and no rule is strictly more specific, the Controller rejects create/update with `409 Conflict`.
- The Agent has no tenant dimension. It applies only the snapshot for its own node.
- The eBPF datapath is not redesigned in this plan. It continues to use `SRC_IPV4_ID_MAP`, `DST_IPV4_ID_MAP`, `SRC_IPV6_ID_MAP`, `DST_IPV6_ID_MAP`, `POLICY_TABLE`, `QOS_CONFIG`, `RULE_STATS`, and `QOS_STATS`.

## Policy Priority Contract

ACL priority is a first-class product control because allow/deny conflicts must
be explicit. QoS priority is also part of the product model, but the UI may place
it in advanced settings so the default path stays simple.

Use this contract across Controller, Agent, frontend, docs, and tests:

```text
priority: integer
default: 100
valid ACL range: 1..10000
valid QoS range: 0..255 until the eBPF ABI is widened
sort: ascending
meaning: smaller number wins
```

Effective rule selection:

```text
1. Only enabled rules participate.
2. Direction must match the packet path.
3. Protocol and port must match when the rule type supports them.
4. IP Group membership is resolved by LPM; more specific CIDRs map to their runtime group first.
5. If multiple rules still match, the smaller priority number wins.
6. If priority is equal, the more specific group/CIDR wins.
7. If priority and specificity are still tied, Controller rejects the write as an ambiguous conflict.
```

Examples:

```text
ACL: deny any -> any priority 10 beats allow office -> any priority 100.
ACL: allow 10.10.1.10/32 priority 5 beats deny 10.10.0.0/16 priority 10.
QoS: group vip priority 20 beats group any priority 100 for the same direction.
QoS: group office priority 100 and group any priority 100 resolves by LPM specificity.
Reject: two enabled ACL rules with overlapping source/destination/protocol/ports, same priority, and no strictly more specific winner.
Reject: two enabled QoS rules with overlapping groups, same direction, same priority, and no strictly more specific winner.
```

Controller conflict detection owns the ambiguous case. `created_at` is only for
list ordering, audit, and stable API output. The Agent should not receive a
snapshot that requires `created_at` to decide a policy winner. If the Agent sees
an ambiguous same-priority/same-specificity candidate set because of a Controller
bug or stale state, it should reject the snapshot and keep the previous applied
maps instead of silently choosing a winner.

## Non-Goals

- Do not reintroduce old QoS categories such as `service / peers / ip`.
- Do not implement per-interface QoS accounting; QoS remains node-scoped total bandwidth.
- Do not add `bpf_spin_lock` in this plan.
- Do not remove existing `src_cidr` / `dst_cidr` columns in this phase; they remain transitional compatibility fields until online data is migrated.
- Do not build locally. Use GitHub Actions for Go, frontend, Rust, and eBPF verification.

## Required Worktree Rule

Before implementation, start from a clean branch. The current working tree may contain unrelated frontend `any`/status-display changes; finish, commit, or stash those before starting this plan.

```bash
git status --short --branch
git checkout master
git pull --ff-only origin master
git checkout -b codex/ip-group-policy-model
```

Expected:

```text
## codex/ip-group-policy-model
```

---

## File Structure

### Controller Storage

- Modify `pkg/controllerstorage/postgres.go`
  - Create `ip_groups` and `ip_group_members`.
  - Add `src_group_id`, `dst_group_id`, and `group_id` foreign-key columns to policy tables.
  - Add tenant and group indexes.
- Create `pkg/controllerstorage/ip_groups.go`
  - Storage records and CRUD for IP Groups.
  - Inline-group creation and lookup helpers.
  - Overlap and exact-duplicate detection.
- Create `pkg/controllerstorage/ip_groups_test.go`
  - SQL contract tests using `sqlmock`.
- Modify `pkg/controllerstorage/network_policy.go`
  - Policy records include group references.
  - Create/update paths resolve direct CIDR into inline groups.
  - Reject ambiguous ACL/QoS conflicts before persisting mutations.
  - List/get paths include group fields and expanded display members.

### Controller API

- Create `internal/api/v2/ip_groups.go`
  - REST handlers for tenant IP Groups.
  - Validation and overlap warning response shape.
- Modify `internal/api/v2/setup.go`
  - Route `/api/v2/tenants/{tenant_id}/ip-groups`.
  - Route `/api/v2/tenants/{tenant_id}/ip-groups/{group_id}`.
- Modify `internal/api/v2/security.go`
  - ACL create/update accepts `src_group_id`, `dst_group_id`, direct `src_cidr`, and direct `dst_cidr`.
  - QoS create/update accepts `group_id` and direct group/CIDR input.
  - Conflict validation uses resolved group ids, direction, protocol, ports, priority, and specificity.
- Modify `pkg/controllerstorage/rbac.go`, `internal/api/middleware/permissions.go`, and `frontend/src/utils/permissions.js`
  - Add `ip-groups:read` and `ip-groups:write`.

### Sync And Proto

- Modify `pkg/grpc/agentpb/aria_agent.proto`
- Modify `agent-rust/proto/aria_agent.proto`
  - Add `IPGroup` message.
  - Add `repeated IPGroup ip_groups` to `SyncResponse` and `SyncConfigRequest`.
  - Add group reference fields to `ACLRule` and `QoSRule`.
- Regenerate generated Go/Rust protobuf artifacts using the repository’s CI-supported generation path.
- Modify `internal/controller/grpc/policy_snapshot.go`
  - Compile group definitions into snapshots.
  - Prefer group references over legacy CIDR fields.
  - Preserve CIDR fallback for existing rows.
- Modify `internal/controller/grpc/policy_snapshot_test.go`
  - Cover multi-member groups, inline groups, `any`, overlap, and duplicate rejection.

### Agent Runtime

- Modify `agent-rust/agent/src/grpc_client.rs`
  - Parse `ip_groups`.
  - Prefer `src_group_id` / `dst_group_id` / `group_id` over CIDR fields.
  - Preserve legacy CIDR fallback.
- Modify `agent-rust/agent/src/acl_qos_manager.rs`
  - Add explicit group definitions to `AclQosSnapshot`.
  - Compile ACL/QoS against product group ids.
- Modify `agent-rust/agent/src/identity.rs`
  - Add one-runtime-id-per-product-group mapping.
  - Insert all group CIDR members into source and destination IPv4/IPv6 LPM maps with the same runtime id.
  - Reconcile group member changes when a new snapshot arrives.
- Modify `agent-rust/agent/src/acl_qos_state.rs`
  - Persist product group id/name to runtime id mapping.
  - Persist group CIDR members for cleanup and reconciliation.
- Modify `agent-rust/agent/src/acl_qos_maps.rs`
  - No ABI change required, but tests should prove policy/QoS map writes use runtime ids from the new group map.

### Frontend

- Create `frontend/src/composables/useIpGroupApi.js`
  - Tenant-scoped IP Group list/create/update/delete.
- Create `frontend/src/views/IPGroups.vue`
  - IP Group management page.
  - Member list editor.
  - Overlap warnings.
- Modify `frontend/src/router/index.js`
  - Add `IPGroups` route under Policy Center.
- Modify `frontend/src/components/layout/Layout.vue`
  - Add menu item.
- Modify `frontend/src/views/ACLRules.vue`
  - Source and destination selectors use IP Groups.
  - Direct CIDR input creates inline groups.
- Modify `frontend/src/views/BandwidthControl.vue`
  - QoS Group selector uses IP Groups.
  - Direct CIDR input creates inline group.
- Modify `frontend/tests/unit/pagePermissionVisibility.test.js`
  - Cover `ip-groups:read` and `ip-groups:write`.
- Create `frontend/tests/unit/useIpGroupApi.test.js`
  - API response normalization tests.

### Docs

- Modify `docs/qos-product-decision.md`
  - Keep product decision in sync with implementation details discovered during development.
- Modify `docs/api-v2-whitepaper.md`
  - Add IP Group endpoints and policy payload examples.
- Modify `docs/known-issues-status.md`
  - Mark direct CIDR primary model as superseded after implementation lands.

---

## Batch 1: Schema And Storage Foundation

**Outcome:** Controller can persist tenant IP Groups, members, inline groups, and policy references without changing the Agent or frontend.

**Files:**

- Modify: `pkg/controllerstorage/postgres.go`
- Create: `pkg/controllerstorage/ip_groups.go`
- Create: `pkg/controllerstorage/ip_groups_test.go`
- Modify: `pkg/controllerstorage/network_policy.go`
- Modify: `pkg/controllerstorage/network_policy_test.go`

- [ ] **Step 1: Add failing storage tests for IP Group CRUD**

Create `pkg/controllerstorage/ip_groups_test.go` with tests shaped like existing `network_policy_test.go` tests.

```go
package controllerstorage

import (
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCreateIPGroupStoresTenantScopedMembers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	store := &Storage{db: db}
	tenantID := uuid.New()
	groupID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO ip_groups (tenant_id, name, description, kind, created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, name, description, kind, created_by, created_at, updated_at`)).
		WithArgs(tenantID, "office", "office networks", "custom", sql.NullString{}).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "kind", "created_by", "created_at", "updated_at"}).
			AddRow(groupID, tenantID, "office", "office networks", "custom", sql.NullString{}, "2026-06-13T00:00:00Z", "2026-06-13T00:00:00Z"))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO ip_group_members (tenant_id, group_id, cidr, note)
		 VALUES ($1, $2, $3::cidr, $4)`)).
		WithArgs(tenantID, groupID, "10.10.0.0/16", "").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	created, err := store.CreateIPGroup(&IPGroupRecord{
		TenantID:    tenantID,
		Name:        "office",
		Description: "office networks",
		Kind:        IPGroupKindCustom,
		Members:     []IPGroupMemberRecord{{CIDR: "10.10.0.0/16"}},
	})
	if err != nil {
		t.Fatalf("CreateIPGroup failed: %v", err)
	}
	if created.ID != groupID || created.Name != "office" || len(created.Members) != 1 {
		t.Fatalf("unexpected IP group: %#v", created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
```

- [ ] **Step 2: Add schema bootstrap SQL**

In `pkg/controllerstorage/postgres.go`, add these statements near the existing policy table bootstrap.

```go
`CREATE TABLE IF NOT EXISTS ip_groups (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
	name VARCHAR(128) NOT NULL,
	description TEXT DEFAULT '',
	kind VARCHAR(16) NOT NULL DEFAULT 'custom',
	created_by UUID REFERENCES users(id),
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	CHECK (kind IN ('custom', 'inline', 'system')),
	UNIQUE (tenant_id, name)
)`,
`CREATE TABLE IF NOT EXISTS ip_group_members (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
	group_id UUID NOT NULL REFERENCES ip_groups(id) ON DELETE CASCADE,
	cidr CIDR NOT NULL,
	note TEXT DEFAULT '',
	created_at TIMESTAMPTZ DEFAULT NOW(),
	UNIQUE (group_id, cidr)
)`,
`CREATE INDEX IF NOT EXISTS idx_ip_groups_tenant_kind ON ip_groups(tenant_id, kind)`,
`CREATE INDEX IF NOT EXISTS idx_ip_group_members_tenant_group ON ip_group_members(tenant_id, group_id)`,
`CREATE INDEX IF NOT EXISTS idx_ip_group_members_cidr ON ip_group_members USING gist(cidr inet_ops)`,
`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS src_group_id UUID REFERENCES ip_groups(id)`,
`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS dst_group_id UUID REFERENCES ip_groups(id)`,
`ALTER TABLE qos_rules ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES ip_groups(id)`,
`CREATE INDEX IF NOT EXISTS idx_acl_rules_src_group ON acl_rules(tenant_id, src_group_id)`,
`CREATE INDEX IF NOT EXISTS idx_acl_rules_dst_group ON acl_rules(tenant_id, dst_group_id)`,
`CREATE INDEX IF NOT EXISTS idx_qos_rules_group ON qos_rules(tenant_id, group_id)`,
```

- [ ] **Step 3: Implement IP Group storage records and constants**

Create `pkg/controllerstorage/ip_groups.go`.

```go
package controllerstorage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	IPGroupKindCustom = "custom"
	IPGroupKindInline = "inline"
	IPGroupKindSystem = "system"
	IPGroupAnyName    = "any"
)

type IPGroupRecord struct {
	ID          uuid.UUID             `json:"id"`
	TenantID    uuid.UUID             `json:"tenant_id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Kind        string                `json:"kind"`
	CreatedBy   sql.NullString        `json:"created_by"`
	Members     []IPGroupMemberRecord `json:"members"`
	Warnings    []IPGroupWarning      `json:"warnings,omitempty"`
	CreatedAt   string                `json:"created_at"`
	UpdatedAt   string                `json:"updated_at"`
}

type IPGroupMemberRecord struct {
	ID      uuid.UUID `json:"id"`
	GroupID uuid.UUID `json:"group_id"`
	CIDR    string    `json:"cidr"`
	Note    string    `json:"note"`
}

type IPGroupWarning struct {
	Type              string `json:"type"`
	CIDR              string `json:"cidr"`
	OverlapsGroupID   string `json:"overlaps_group_id"`
	OverlapsGroupName string `json:"overlaps_group_name"`
	OverlapsCIDR      string `json:"overlaps_cidr"`
	Resolution        string `json:"resolution"`
}

func normalizeIPGroupName(name string) string {
	return strings.TrimSpace(name)
}

func normalizeIPGroupMembers(members []IPGroupMemberRecord) ([]IPGroupMemberRecord, error) {
	seen := map[string]struct{}{}
	normalized := make([]IPGroupMemberRecord, 0, len(members))
	for _, member := range members {
		cidr, err := normalizeCIDR(member.CIDR)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[cidr]; ok {
			continue
		}
		seen[cidr] = struct{}{}
		member.CIDR = cidr
		member.Note = strings.TrimSpace(member.Note)
		normalized = append(normalized, member)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].CIDR < normalized[j].CIDR
	})
	return normalized, nil
}

func normalizeCIDR(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "any") || trimmed == "0" {
		return "0.0.0.0/0", nil
	}
	prefix, err := netip.ParsePrefix(trimmed)
	if err == nil {
		return prefix.Masked().String(), nil
	}
	addr, err := netip.ParseAddr(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q", value)
	}
	if addr.Is4() {
		return netip.PrefixFrom(addr, 32).String(), nil
	}
	return netip.PrefixFrom(addr, 128).String(), nil
}

func inlineIPGroupName(cidrs []string) string {
	copyCIDRs := append([]string(nil), cidrs...)
	sort.Strings(copyCIDRs)
	sum := sha256.Sum256([]byte(strings.Join(copyCIDRs, ",")))
	return "inline:" + hex.EncodeToString(sum[:8])
}
```

- [ ] **Step 4: Implement IP Group CRUD storage methods**

Add these method signatures to `pkg/controllerstorage/ip_groups.go` and implement with `db.Begin()`, `QueryRow`, `Exec`, and `Commit`.

```go
func (s *Storage) CreateIPGroup(group *IPGroupRecord) (*IPGroupRecord, error)
func (s *Storage) ListIPGroups(tenantID uuid.UUID) ([]*IPGroupRecord, error)
func (s *Storage) GetIPGroup(tenantID, groupID uuid.UUID) (*IPGroupRecord, error)
func (s *Storage) UpdateIPGroup(group *IPGroupRecord) (*IPGroupRecord, error)
func (s *Storage) DeleteIPGroup(tenantID, groupID uuid.UUID) error
func (s *Storage) EnsureInlineIPGroup(tenantID uuid.UUID, cidrs []string) (*IPGroupRecord, error)
func (s *Storage) FindIPGroupOverlapWarnings(tenantID uuid.UUID, groupID uuid.UUID, members []IPGroupMemberRecord) ([]IPGroupWarning, error)
```

`DeleteIPGroup` must reject referenced groups with an error message containing:

```text
ip group is referenced by policy rules
```

`EnsureInlineIPGroup` must use `inlineIPGroupName()` and kind `inline`, so repeated direct-CIDR policy saves reuse the same group.

- [ ] **Step 5: Add exact duplicate and overlap tests**

In `pkg/controllerstorage/ip_groups_test.go`, add:

```go
func TestFindIPGroupOverlapWarningsAllowsMoreSpecificCIDR(t *testing.T) {
	tenantID := uuid.New()
	groupID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()
	store := &Storage{db: db}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT g.id, g.name, m.cidr::text
		FROM ip_group_members m
		JOIN ip_groups g ON g.id = m.group_id
		WHERE m.tenant_id = $1 AND g.id <> $2 AND m.cidr && $3::cidr`)).
		WithArgs(tenantID, groupID, "10.10.0.0/16").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "cidr"}).
			AddRow(uuid.New(), "office-wide", "10.0.0.0/8"))

	warnings, err := store.FindIPGroupOverlapWarnings(tenantID, groupID, []IPGroupMemberRecord{{CIDR: "10.10.0.0/16"}})
	if err != nil {
		t.Fatalf("FindIPGroupOverlapWarnings failed: %v", err)
	}
	if len(warnings) != 1 || warnings[0].Resolution != "longest_prefix_wins" {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}
```

- [ ] **Step 6: Extend policy records with group references**

In `pkg/controllerstorage/network_policy.go`, extend records:

```go
type QoSRuleRecord struct {
	// existing fields
	GroupID uuid.NullUUID `json:"group_id"`
	Group   *IPGroupRecord `json:"group,omitempty"`
}

type ACLRuleRecord struct {
	// existing fields
	SrcGroupID uuid.NullUUID `json:"src_group_id"`
	DstGroupID uuid.NullUUID `json:"dst_group_id"`
	SrcGroup   *IPGroupRecord `json:"src_group,omitempty"`
	DstGroup   *IPGroupRecord `json:"dst_group,omitempty"`
}
```

If `uuid.NullUUID` is not already available in the current module version, use `sql.NullString` for scanning and convert to/from `uuid.UUID` at the API boundary.

- [ ] **Step 7: Verify Batch 1 through GitHub Actions**

Commit and push this batch.

```bash
git add pkg/controllerstorage/postgres.go pkg/controllerstorage/ip_groups.go pkg/controllerstorage/ip_groups_test.go pkg/controllerstorage/network_policy.go pkg/controllerstorage/network_policy_test.go
git commit -m "feat(controller): add IP group storage model"
git push -u origin codex/ip-group-policy-model
gh workflow run build.yml --repo chenyongming211-glitch/Aria --ref codex/ip-group-policy-model
gh run list --repo chenyongming211-glitch/Aria --branch codex/ip-group-policy-model --limit 3
```

Expected: latest branch run is `completed success`.

---

## Batch 2: IP Group API And RBAC

**Outcome:** Tenants can manage IP Groups through API v2, with RBAC and overlap warnings.

**Files:**

- Create: `internal/api/v2/ip_groups.go`
- Create: `internal/api/v2/ip_groups_test.go`
- Modify: `internal/api/v2/setup.go`
- Modify: `internal/api/middleware/permissions.go`
- Modify: `pkg/controllerstorage/rbac.go`
- Modify: `internal/api/v2/roles.go`
- Modify: `docs/api-v2-whitepaper.md`

- [ ] **Step 1: Add permission constants**

In `internal/api/middleware/permissions.go` add:

```go
PermIPGroupsRead  = "ip-groups:read"
PermIPGroupsWrite = "ip-groups:write"
```

In `pkg/controllerstorage/rbac.go`, add:

```go
"ip-groups:read", "ip-groups:write",
```

to admin/operator roles, and:

```go
"ip-groups:read",
```

to viewer roles.

- [ ] **Step 2: Add API tests for create/list/delete protection**

Create `internal/api/v2/ip_groups_test.go`.

```go
package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestIPGroupCreateRejectsInvalidCIDR(t *testing.T) {
	tenantID := uuid.New()
	req := withAuthContext(
		httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/ip-groups", strings.NewReader(`{"name":"bad","members":[{"cidr":"not-a-cidr"}]}`)),
		"admin",
		tenantID,
	)
	rec := httptest.NewRecorder()

	router := &Router{}
	router.createTenantIPGroup(rec, req, tenantID)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
```

Use the existing auth/test helpers in `internal/api/v2/rbac_handler_matrix_test.go` rather than adding a second auth test framework.

- [ ] **Step 3: Implement handler request and response shapes**

Create `internal/api/v2/ip_groups.go`.

```go
type ipGroupMemberPayload struct {
	CIDR string `json:"cidr"`
	Note string `json:"note"`
}

type ipGroupPayload struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Kind        string                 `json:"kind"`
	Members     []ipGroupMemberPayload `json:"members"`
}
```

Response shape:

```json
{
  "id": "uuid",
  "tenant_id": "uuid",
  "name": "office",
  "description": "office networks",
  "kind": "custom",
  "members": [{"cidr": "10.10.0.0/16", "note": ""}],
  "warnings": [
    {
      "type": "overlap",
      "cidr": "10.10.0.0/16",
      "overlaps_group_id": "uuid",
      "overlaps_group_name": "office-wide",
      "overlaps_cidr": "10.0.0.0/8",
      "resolution": "longest_prefix_wins"
    }
  ]
}
```

- [ ] **Step 4: Route IP Group endpoints**

In `internal/api/v2/setup.go`, route:

```text
GET    /api/v2/tenants/{tenant_id}/ip-groups
POST   /api/v2/tenants/{tenant_id}/ip-groups
GET    /api/v2/tenants/{tenant_id}/ip-groups/{group_id}
PUT    /api/v2/tenants/{tenant_id}/ip-groups/{group_id}
DELETE /api/v2/tenants/{tenant_id}/ip-groups/{group_id}
```

Use `ip-groups:read` for GET and `ip-groups:write` for POST/PUT/DELETE.

- [ ] **Step 5: Document API examples**

In `docs/api-v2-whitepaper.md`, add:

````markdown
### IP Groups

`POST /api/v2/tenants/{tenant_id}/ip-groups`

```json
{
  "name": "office",
  "description": "office IPv4 and IPv6 ranges",
  "members": [
    {"cidr": "10.10.0.0/16"},
    {"cidr": "2001:db8:10::/48"}
  ]
}
```

Overlaps are allowed and returned as warnings. Exact duplicate CIDRs across two
non-inline groups are rejected.
````

- [ ] **Step 6: Verify Batch 2 through GitHub Actions**

```bash
git add internal/api/v2/ip_groups.go internal/api/v2/ip_groups_test.go internal/api/v2/setup.go internal/api/middleware/permissions.go pkg/controllerstorage/rbac.go internal/api/v2/roles.go docs/api-v2-whitepaper.md
git commit -m "feat(api): add tenant IP group endpoints"
git push
gh workflow run build.yml --repo chenyongming211-glitch/Aria --ref codex/ip-group-policy-model
gh run list --repo chenyongming211-glitch/Aria --branch codex/ip-group-policy-model --limit 3
```

Expected: latest branch run is `completed success`.

---

## Batch 3: Policy Write Path Uses IP Groups

**Outcome:** ACL/QoS create and update store group references. Direct CIDR input creates inline groups. Ambiguous same-priority conflicts are rejected by the Controller before storage changes are committed.

**Files:**

- Modify: `internal/api/v2/security.go`
- Modify: `internal/api/v2/rbac_handler_matrix_test.go`
- Modify: `internal/api/v2/security_delivery_status_test.go`
- Modify: `pkg/controllerstorage/network_policy.go`
- Modify: `pkg/controllerstorage/network_policy_test.go`

- [ ] **Step 1: Add tests for ACL direct CIDR inline groups**

In `internal/api/v2/rbac_handler_matrix_test.go`, add a test that posts:

```json
{
  "name": "deny-office",
  "src_cidr": "10.10.0.0/16",
  "dst_cidr": "any",
  "protocol": 1,
  "direction": "ingress",
  "action": "deny",
  "enabled": true
}
```

Expected storage behavior:

```text
EnsureInlineIPGroup(tenant_id, ["10.10.0.0/16"])
src_group_id = returned inline group id
dst_group_id = null
src_cidr = ""
dst_cidr = ""
```

- [ ] **Step 2: Add tests for ACL explicit group ids**

Request:

```json
{
  "name": "deny-office-to-prod",
  "src_group_id": "11111111-1111-1111-1111-111111111111",
  "dst_group_id": "22222222-2222-2222-2222-222222222222",
  "protocol": 6,
  "ports": "443",
  "direction": "egress",
  "action": "deny",
  "priority": 100
}
```

Expected:

```text
Controller verifies both groups belong to tenant_id.
Controller stores src_group_id and dst_group_id.
Controller stores priority = 100.
Controller does not create inline groups.
```

- [ ] **Step 3: Add tests for QoS direct CIDR inline group**

Request:

```json
{
  "group": "10.10.0.0/16",
  "direction": "egress",
  "bandwidth_mbps": 10,
  "mode": "policing",
  "priority": 100
}
```

Expected:

```text
EnsureInlineIPGroup(tenant_id, ["10.10.0.0/16"])
qos_rules.group_id = returned inline group id
qos_rules.priority = 100
src_cidr = ""
dst_cidr = ""
```

- [ ] **Step 4: Implement request parsing**

In `internal/api/v2/security.go`, extend create/update bodies.

```go
SrcGroupID *uuid.UUID `json:"src_group_id"`
DstGroupID *uuid.UUID `json:"dst_group_id"`
GroupID    *uuid.UUID `json:"group_id"`
Group      string     `json:"group"`
Priority   int        `json:"priority"`
```

Resolve order:

```text
ACL src: src_group_id > src_cidr/src_net > any
ACL dst: dst_group_id > dst_cidr/dst_net > any
QoS group: group_id > group > src_cidr/dst_cidr by direction > any
```

Priority handling:

```text
ACL create/update keeps current ACL validation range and defaults missing priority to 100.
QoS create/update validates priority fits uint8 and defaults missing priority to 100.
Existing rows keep their stored priority during migration.
```

- [ ] **Step 5: Implement storage resolution helpers**

In `pkg/controllerstorage/network_policy.go`, add helpers:

```go
func (s *Storage) ResolvePolicyGroupRef(tenantID uuid.UUID, explicit uuid.NullUUID, directCIDR string) (uuid.NullUUID, error) {
	if explicit.Valid {
		if _, err := s.GetIPGroup(tenantID, explicit.UUID); err != nil {
			return uuid.NullUUID{}, err
		}
		return explicit, nil
	}
	normalized := strings.TrimSpace(directCIDR)
	if normalized == "" || strings.EqualFold(normalized, "any") || normalized == "0" || normalized == "0.0.0.0/0" || normalized == "::/0" {
		return uuid.NullUUID{}, nil
	}
	group, err := s.EnsureInlineIPGroup(tenantID, []string{normalized})
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: group.ID, Valid: true}, nil
}
```

If the project does not use `uuid.NullUUID`, implement the same behavior with `sql.NullString` and parse UUIDs at handler boundaries.

- [ ] **Step 6: Add Controller conflict tests**

In `pkg/controllerstorage/network_policy_test.go`, add tests for the conflict helper before wiring it into handlers:

```go
func TestDetectACLPolicyConflictRejectsSamePriorityAmbiguousOverlap(t *testing.T) {
	existing := []*ACLRuleRecord{
		{
			ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			SrcCIDR:   "10.0.0.0/8",
			DstCIDR:   "172.16.10.0/24",
			Protocol:  6,
			Ports:     "443",
			Direction: "egress",
			Action:    "deny",
			Priority:  100,
			Enabled:   true,
		},
	}
	candidate := &ACLRuleRecord{
		SrcCIDR:   "10.10.0.0/16",
		DstCIDR:   "172.16.0.0/16",
		Protocol:  6,
		Ports:     "443",
		Direction: "egress",
		Action:    "allow",
		Priority:  100,
		Enabled:   true,
	}
	err := DetectACLPolicyConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected ambiguous ACL conflict")
	}
	if !strings.Contains(err.Error(), "ambiguous ACL conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectACLPolicyConflictAllowsStrictlyMoreSpecificSamePriority(t *testing.T) {
	existing := []*ACLRuleRecord{
		{
			SrcCIDR:   "10.0.0.0/8",
			DstCIDR:   "172.16.0.0/16",
			Protocol:  1,
			Direction: "ingress",
			Action:    "deny",
			Priority:  100,
			Enabled:   true,
		},
	}
	candidate := &ACLRuleRecord{
		SrcCIDR:   "10.10.1.10/32",
		DstCIDR:   "172.16.1.10/32",
		Protocol:  1,
		Direction: "ingress",
		Action:    "allow",
		Priority:  100,
		Enabled:   true,
	}
	if err := DetectACLPolicyConflict(existing, candidate); err != nil {
		t.Fatalf("strictly more specific rule should be allowed: %v", err)
	}
}

func TestDetectQoSPolicyConflictRejectsSamePriorityAmbiguousOverlap(t *testing.T) {
	existing := []*QoSRuleRecord{
		{
			DstCIDR:    "100.64.0.0/24",
			Direction:  "egress",
			RateBps:    1_000_000,
			BurstBytes: 1500,
			Priority:   100,
			Enabled:    true,
		},
	}
	candidate := &QoSRuleRecord{
		SrcCIDR:    "100.64.0.0/24",
		Direction:  "egress",
		RateBps:    10_000_000,
		BurstBytes: 1500,
		Priority:   100,
		Enabled:    true,
	}
	err := DetectQoSPolicyConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected ambiguous QoS conflict")
	}
	if !strings.Contains(err.Error(), "ambiguous QoS conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

Expected failure before implementation:

```text
undefined: DetectACLPolicyConflict
undefined: DetectQoSPolicyConflict
```

- [ ] **Step 7: Implement conflict detection helpers**

In `pkg/controllerstorage/network_policy.go`, add helpers with this behavior:

```go
var ErrAmbiguousPolicyConflict = errors.New("ambiguous policy conflict")

func DetectACLPolicyConflict(existing []*ACLRuleRecord, candidate *ACLRuleRecord) error {
	if candidate == nil || !candidate.Enabled {
		return nil
	}
	for _, rule := range existing {
		if rule == nil || !rule.Enabled || rule.ID == candidate.ID {
			continue
		}
		if !expandedDirectionsOverlap(rule.Direction, candidate.Direction) {
			continue
		}
		if !protocolsOverlap(rule.Protocol, candidate.Protocol) {
			continue
		}
		if !portsOverlap(rule.Ports, rule.DstPort, candidate.Ports, candidate.DstPort) {
			continue
		}
		srcRelation := cidrSpecificityRelation(rule.SrcCIDR, candidate.SrcCIDR)
		dstRelation := cidrSpecificityRelation(rule.DstCIDR, candidate.DstCIDR)
		if !srcRelation.overlaps || !dstRelation.overlaps {
			continue
		}
		if rule.Priority != candidate.Priority {
			continue
		}
		if candidateStrictlyMoreSpecific(srcRelation, dstRelation) || existingStrictlyMoreSpecific(srcRelation, dstRelation) {
			continue
		}
		return fmt.Errorf("%w: ambiguous ACL conflict with rule %s", ErrAmbiguousPolicyConflict, rule.ID)
	}
	return nil
}

func DetectQoSPolicyConflict(existing []*QoSRuleRecord, candidate *QoSRuleRecord) error {
	if candidate == nil || !candidate.Enabled {
		return nil
	}
	for _, rule := range existing {
		if rule == nil || !rule.Enabled || rule.ID == candidate.ID {
			continue
		}
		if !expandedDirectionsOverlap(rule.Direction, candidate.Direction) {
			continue
		}
		relation := cidrSpecificityRelation(qosGroupCIDR(rule), qosGroupCIDR(candidate))
		if !relation.overlaps {
			continue
		}
		if rule.Priority != candidate.Priority {
			continue
		}
		if relation.leftContainsRight && !relation.rightContainsLeft {
			continue
		}
		if relation.rightContainsLeft && !relation.leftContainsRight {
			continue
		}
		return fmt.Errorf("%w: ambiguous QoS conflict with rule %s", ErrAmbiguousPolicyConflict, rule.ID)
	}
	return nil
}
```

Implementation requirements:

```text
expandedDirectionsOverlap expands both into ingress/egress sets; both overlaps either direction.
protocolsOverlap treats protocol 0 as any.
portsOverlap treats empty/0 as any and handles single port or ranges.
cidrSpecificityRelation treats empty/any/0.0.0.0/0/::/0 as wildcard.
candidateStrictlyMoreSpecific returns true only when candidate is subset-or-equal in every compared dimension and strictly subset in at least one.
existingStrictlyMoreSpecific is the inverse.
```

- [ ] **Step 8: Wire conflict checks before policy mutation commits**

In `internal/api/v2/security.go`, after resolving group references and before calling create/update storage:

```go
existing, err := h.storage.ListTenantNodeACLRules(tenantID, nodeID)
if err != nil {
	apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternal, "Failed to load ACL rules", nil)
	return
}
if err := controllerstorage.DetectACLPolicyConflict(existing, rule); err != nil {
	if errors.Is(err, controllerstorage.ErrAmbiguousPolicyConflict) {
		apibase.WriteError(w, http.StatusConflict, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}
	apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternal, "Failed to validate ACL conflict", nil)
	return
}
```

For QoS, use:

```go
existing, err := h.storage.ListTenantNodeQoSRules(tenantID, nodeID)
if err != nil {
	apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternal, "Failed to load QoS rules", nil)
	return
}
if err := controllerstorage.DetectQoSPolicyConflict(existing, rule); err != nil {
	if errors.Is(err, controllerstorage.ErrAmbiguousPolicyConflict) {
		apibase.WriteError(w, http.StatusConflict, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}
	apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternal, "Failed to validate QoS conflict", nil)
	return
}
```

Update handler tests to expect HTTP `409` for ambiguous same-priority conflicts.

- [ ] **Step 9: Preserve compatibility fields on list responses**

For list/get responses, include both group fields and transitional display fields.

```json
{
  "src_group_id": "uuid",
  "src_group": {"id": "uuid", "name": "office", "members": [{"cidr": "10.10.0.0/16"}]},
  "src_cidr": "10.10.0.0/16",
  "runtime_src_group": "office"
}
```

For multi-member groups, `src_cidr` should be empty and `src_group.members` should be used by the frontend.

- [ ] **Step 10: Verify Batch 3 through GitHub Actions**

```bash
git add internal/api/v2/security.go internal/api/v2/rbac_handler_matrix_test.go internal/api/v2/security_delivery_status_test.go pkg/controllerstorage/network_policy.go pkg/controllerstorage/network_policy_test.go
git commit -m "feat(policy): store ACL QoS rules by IP group"
git push
gh workflow run build.yml --repo chenyongming211-glitch/Aria --ref codex/ip-group-policy-model
gh run list --repo chenyongming211-glitch/Aria --branch codex/ip-group-policy-model --limit 3
```

Expected: latest branch run is `completed success`.

---

## Batch 4: Snapshot Contract And Controller Compiler

**Outcome:** Controller sends explicit IP Group definitions plus policy group references to Agent. Legacy CIDR fields remain as fallback.

**Files:**

- Modify: `pkg/grpc/agentpb/aria_agent.proto`
- Modify: `agent-rust/proto/aria_agent.proto`
- Modify generated protobuf outputs through the repository-supported generation command in CI.
- Modify: `internal/controller/grpc/policy_snapshot.go`
- Modify: `internal/controller/grpc/policy_snapshot_test.go`
- Modify: `internal/controller/grpc/server.go`

- [ ] **Step 1: Add proto fields**

In both proto files, add:

```proto
message IPGroup {
  string id = 1;
  string name = 2;
  repeated string cidrs = 3;
  string kind = 4;
}
```

Add to `SyncResponse` and `SyncConfigRequest`:

```proto
repeated IPGroup ip_groups = 20;
```

Extend `ACLRule`:

```proto
string src_group_id = 10;
string dst_group_id = 11;
```

Extend `QoSRule`:

```proto
string group_id = 13;
```

Use field numbers that do not collide with existing fields.

- [ ] **Step 2: Add compiler tests**

In `internal/controller/grpc/policy_snapshot_test.go`, add:

```go
func TestCompilePolicySnapshotIncludesIPGroups(t *testing.T) {
	officeID := "11111111-1111-1111-1111-111111111111"
	prodID := "22222222-2222-2222-2222-222222222222"
	snapshot, err := compileAgentPolicySnapshotWithGroups(
		[]*controllerstorage.IPGroupRecord{
			{ID: uuid.MustParse(officeID), Name: "office", Kind: "custom", Members: []controllerstorage.IPGroupMemberRecord{{CIDR: "10.10.0.0/16"}, {CIDR: "2001:db8:10::/48"}}},
			{ID: uuid.MustParse(prodID), Name: "prod", Kind: "custom", Members: []controllerstorage.IPGroupMemberRecord{{CIDR: "172.16.0.0/16"}}},
		},
		[]*controllerstorage.ACLRuleRecord{
			{ID: uuid.New(), SrcGroupID: uuid.NullUUID{UUID: uuid.MustParse(officeID), Valid: true}, DstGroupID: uuid.NullUUID{UUID: uuid.MustParse(prodID), Valid: true}, Protocol: 6, Action: "deny", Direction: "egress", Enabled: true},
		},
		[]*controllerstorage.QoSRuleRecord{
			{ID: uuid.New(), GroupID: uuid.NullUUID{UUID: uuid.MustParse(officeID), Valid: true}, Direction: "egress", BandwidthMbps: 10, RateBps: 10000000, BurstBytes: 1500, Mode: "policing", Enabled: true},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("compileAgentPolicySnapshotWithGroups failed: %v", err)
	}
	if len(snapshot.IPGroups) != 2 {
		t.Fatalf("expected 2 IP groups, got %#v", snapshot.IPGroups)
	}
	if snapshot.ACLRules[0].GetSrcGroupId() != officeID || snapshot.ACLRules[0].GetDstGroupId() != prodID {
		t.Fatalf("ACL group ids not preserved: %#v", snapshot.ACLRules[0])
	}
	if snapshot.QoSRules[0].GetGroupId() != officeID {
		t.Fatalf("QoS group id not preserved: %#v", snapshot.QoSRules[0])
	}
}
```

- [ ] **Step 3: Implement compiler behavior**

In `internal/controller/grpc/policy_snapshot.go`, add a compiler entry point:

```go
func compileAgentPolicySnapshotWithGroups(
	groups []*controllerstorage.IPGroupRecord,
	aclRules []*controllerstorage.ACLRuleRecord,
	qosRules []*controllerstorage.QoSRuleRecord,
	blacklistRules []*agentpb.BlacklistRule,
) (*agentPolicySnapshot, error)
```

Rules:

```text
If ACL has src_group_id, set ACLRule.src_group_id and leave src_net as fallback display only.
If ACL has no src_group_id, compile src_cidr through legacy src_net.
If QoS has group_id, set QoSRule.group_id.
If QoS has no group_id, compile src_cidr/dst_cidr through legacy src_ip/dst_ip.
Always include all IP Groups referenced by enabled ACL/QoS rules.
Include any inline group referenced by policy rules.
Sort ACL and QoS snapshot rules by priority ASC, created_at ASC before sending to Agent.
This sort is for stable output only. It is not a policy winner. Ambiguous equal-priority conflicts must already have been rejected by Controller conflict validation.
```

- [ ] **Step 4: Ensure SyncResponse uses group compiler**

In `internal/controller/grpc/server.go`, when building Sync response:

```go
groups, err := s.storage.ListIPGroupsForNodePolicySnapshot(node.TenantID, node.ID)
if err != nil {
	return nil, fmt.Errorf("load policy ip groups: %w", err)
}
snapshot, err := compileAgentPolicySnapshotWithGroups(groups, aclRules, qosRules, blacklistRules)
if err != nil {
	return nil, err
}
resp.IpGroups = snapshot.IPGroups
resp.AclRules = snapshot.ACLRules
resp.QosRules = snapshot.QoSRules
```

- [ ] **Step 5: Verify Batch 4 through GitHub Actions**

```bash
git add pkg/grpc/agentpb/aria_agent.proto agent-rust/proto/aria_agent.proto pkg/grpc/agentpb/aria_agent.pb.go pkg/grpc/agentpb/aria_agent_grpc.pb.go internal/controller/grpc/policy_snapshot.go internal/controller/grpc/policy_snapshot_test.go internal/controller/grpc/server.go
git commit -m "feat(sync): include IP groups in agent policy snapshot"
git push
gh workflow run build.yml --repo chenyongming211-glitch/Aria --ref codex/ip-group-policy-model
gh run list --repo chenyongming211-glitch/Aria --branch codex/ip-group-policy-model --limit 3
```

Expected: latest branch run is `completed success`.

---

## Batch 5: Agent Runtime Group Mapping

**Outcome:** Agent maps one product IP Group to one local runtime id and inserts all group CIDRs into LPM maps with that same runtime id.

**Files:**

- Modify: `agent-rust/agent/src/grpc_client.rs`
- Modify: `agent-rust/agent/src/acl_qos_manager.rs`
- Modify: `agent-rust/agent/src/identity.rs`
- Modify: `agent-rust/agent/src/acl_qos_state.rs`
- Modify: `agent-rust/agent/src/acl_qos_maps.rs`
- Modify or create: `agent-rust/tests/test_grpc_sync.rs`

- [ ] **Step 1: Add Agent unit tests for multi-member group mapping**

In `agent-rust/agent/src/acl_qos_manager.rs` test module, add:

```rust
#[test]
fn qos_group_with_multiple_cidrs_compiles_to_one_group_id() {
    let mut groups = std::collections::HashMap::new();
    groups.insert(
        "office".to_string(),
        GroupInfo {
            id: 7,
            name: "office".to_string(),
            cidrs: vec!["10.10.0.0/16".to_string(), "192.168.1.0/24".to_string()],
        },
    );

    let rules = vec![QosRuleSpec {
        id: "qos-1".to_string(),
        group: "office".to_string(),
        direction: DIRECTION_EGRESS,
        rate_bps: 10_000_000,
        burst_bytes: 1500,
        priority: 10,
        mode: 0,
    }];

    let compiled = compile_qos_rules_for_groups(&rules, &groups).expect("compile qos");
    assert_eq!(compiled.len(), 1);
    assert_eq!(compiled[0].group_id, 7);
}
```

- [ ] **Step 2: Extend Agent snapshot structs**

In `agent-rust/agent/src/acl_qos_manager.rs`:

```rust
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IPGroupSpec {
    pub id: String,
    pub name: String,
    pub cidrs: Vec<String>,
    pub kind: String,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct AclQosSnapshot {
    pub ip_groups: Vec<IPGroupSpec>,
    pub acl_rules: Vec<AclRuleSpec>,
    pub qos_rules: Vec<QosRuleSpec>,
    pub acl_enabled: bool,
    pub qos_enabled: bool,
}
```

- [ ] **Step 3: Extend Agent policy specs to use product group refs**

```rust
pub struct AclRuleSpec {
    pub id: String,
    pub src_group: String,
    pub dst_group: String,
    pub src_group_id: String,
    pub dst_group_id: String,
    pub proto: u8,
    pub action: u8,
    pub priority: u16,
    pub direction: u8,
    pub ports: Option<String>,
}

pub struct QosRuleSpec {
    pub id: String,
    pub group: String,
    pub group_id: String,
    pub direction: u8,
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    pub mode: u8,
}
```

Resolution rule:

```text
If group_id is present, match by product group id.
If group_id is empty, fall back to legacy CIDR string.
```

Compilation rule:

```text
Compiled ACL and QoS candidates carry a specificity score derived from the matched CIDR prefix length.
For ACL, specificity is src_prefix_len + dst_prefix_len after group expansion.
For QoS, specificity is group_prefix_len after group expansion.
Candidate winner order is priority ASC, specificity DESC.
If priority and specificity are both equal for overlapping candidates, return a validation error and keep the previously applied maps.
```

- [ ] **Step 4: Implement one-runtime-id-per-product-group**

In `agent-rust/agent/src/identity.rs`, add:

```rust
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct RuntimeIPGroup {
    pub key: String,
    pub cidrs: Vec<String>,
}

pub fn replace_groups(&mut self, groups: &[RuntimeIPGroup]) -> Result<HashMap<String, u32>, IdentityError> {
    let mut result = HashMap::new();
    for group in groups {
        let id = self.assign_group_id(&group.key)?;
        self.replace_group_cidrs(&group.key, id, &group.cidrs)?;
        result.insert(group.key.clone(), id);
    }
    self.remove_groups_not_in(groups)?;
    Ok(result)
}
```

`replace_group_cidrs` must insert every CIDR into all source/destination LPM maps with the same runtime id. Exact duplicate CIDRs inside the same group are ignored after normalization.

- [ ] **Step 5: Compile policy rules by resolved runtime group id**

In `agent-rust/agent/src/acl_qos_manager.rs`:

```text
Build a map product_group_id -> runtime_group_id from snapshot.ip_groups.
For legacy direct CIDR rules, create a synthetic inline group key `legacy:<cidr>` and map it through IdentityManager.
For ACL `any`, use ID_WILDCARD.
For QoS `any`, use ID_WILDCARD.
```

- [ ] **Step 6: Preserve LPM overlap semantics**

Add Agent tests:

```rust
#[test]
fn overlapping_groups_are_allowed_and_exact_duplicate_is_rejected_before_agent() {
    let broad = IPGroupSpec {
        id: "broad".to_string(),
        name: "broad".to_string(),
        cidrs: vec!["10.0.0.0/8".to_string()],
        kind: "custom".to_string(),
    };
    let narrow = IPGroupSpec {
        id: "narrow".to_string(),
        name: "narrow".to_string(),
        cidrs: vec!["10.10.0.0/16".to_string()],
        kind: "custom".to_string(),
    };
    assert_ne!(broad.id, narrow.id);
}
```

The Agent should accept overlaps. Exact duplicate validation belongs in Controller because the Agent has no tenant context and should receive an already-valid snapshot.

- [ ] **Step 7: Add priority conflict tests**

In `agent-rust/agent/src/acl_qos_manager.rs`, keep or add tests that prove smaller priority numbers win after group expansion:

```rust
#[test]
fn acl_priority_overrides_more_specific_group_when_priority_is_higher() {
    let groups = test_groups(&[("10.0.0.0/8", 1), ("10.10.1.10/32", 2)]);
    let rules = vec![
        AclRuleSpec {
            id: "deny-wide".to_string(),
            src_group: "10.0.0.0/8".to_string(),
            dst_group: "any".to_string(),
            proto: 1,
            action: 1,
            priority: 10,
            direction: DIRECTION_INGRESS,
            ports: None,
        },
        AclRuleSpec {
            id: "allow-specific".to_string(),
            src_group: "10.10.1.10/32".to_string(),
            dst_group: "any".to_string(),
            proto: 1,
            action: 0,
            priority: 100,
            direction: DIRECTION_INGRESS,
            ports: None,
        },
    ];
    let compiled = compile_acl_rules_for_groups(&rules, &groups).expect("compile acl");
    let child = compiled
        .iter()
        .find(|rule| rule.src_group_id == 2 && rule.dst_group_id == ID_WILDCARD)
        .expect("specific runtime ACL entry");
    assert_eq!(child.rule_id, "deny-wide");
    assert_eq!(child.action, 1);
}

#[test]
fn qos_priority_overrides_more_specific_group_when_priority_is_higher() {
    let groups = test_groups(&[("100.64.0.0/24", 1), ("100.64.0.3/32", 2)]);
    let rules = vec![
        QosRuleSpec {
            id: "wide-limit".to_string(),
            group: "100.64.0.0/24".to_string(),
            direction: DIRECTION_EGRESS,
            rate_bps: 1_000_000,
            burst_bytes: 1500,
            priority: 10,
            mode: 0,
        },
        QosRuleSpec {
            id: "specific-limit".to_string(),
            group: "100.64.0.3/32".to_string(),
            direction: DIRECTION_EGRESS,
            rate_bps: 10_000_000,
            burst_bytes: 1500,
            priority: 100,
            mode: 0,
        },
    ];
    let compiled = compile_qos_rules_for_groups(&rules, &groups).expect("compile qos");
    let child = compiled
        .iter()
        .find(|rule| rule.group_id == 2 && rule.direction == DIRECTION_EGRESS)
        .expect("specific runtime QoS entry");
    assert_eq!(child.rule_id, "wide-limit");
    assert_eq!(child.rate_bps, 1_000_000);
}
```

Expected behavior:

```text
Priority is evaluated after group expansion.
Smaller priority number wins.
When priority is equal, the more specific matched CIDR wins.
When priority and specificity are both equal, the Agent rejects the ambiguous snapshot instead of using snapshot order.
Controller should normally prevent this by rejecting ambiguous writes with HTTP 409.
```

Also add equal-priority specificity tests:

```rust
#[test]
fn acl_equal_priority_prefers_more_specific_group() {
    let groups = test_groups(&[("10.0.0.0/8", 1), ("10.10.1.10/32", 2)]);
    let rules = vec![
        AclRuleSpec {
            id: "deny-wide".to_string(),
            src_group: "10.0.0.0/8".to_string(),
            dst_group: "any".to_string(),
            proto: 1,
            action: 1,
            priority: 100,
            direction: DIRECTION_INGRESS,
            ports: None,
        },
        AclRuleSpec {
            id: "allow-specific".to_string(),
            src_group: "10.10.1.10/32".to_string(),
            dst_group: "any".to_string(),
            proto: 1,
            action: 0,
            priority: 100,
            direction: DIRECTION_INGRESS,
            ports: None,
        },
    ];
    let compiled = compile_acl_rules_for_groups(&rules, &groups).expect("compile acl");
    let child = compiled
        .iter()
        .find(|rule| rule.src_group_id == 2 && rule.dst_group_id == ID_WILDCARD)
        .expect("specific runtime ACL entry");
    assert_eq!(child.rule_id, "allow-specific");
    assert_eq!(child.action, 0);
}

#[test]
fn qos_equal_priority_prefers_more_specific_group() {
    let groups = test_groups(&[("100.64.0.0/24", 1), ("100.64.0.3/32", 2)]);
    let rules = vec![
        QosRuleSpec {
            id: "wide-limit".to_string(),
            group: "100.64.0.0/24".to_string(),
            direction: DIRECTION_EGRESS,
            rate_bps: 1_000_000,
            burst_bytes: 1500,
            priority: 100,
            mode: 0,
        },
        QosRuleSpec {
            id: "specific-limit".to_string(),
            group: "100.64.0.3/32".to_string(),
            direction: DIRECTION_EGRESS,
            rate_bps: 10_000_000,
            burst_bytes: 1500,
            priority: 100,
            mode: 0,
        },
    ];
    let compiled = compile_qos_rules_for_groups(&rules, &groups).expect("compile qos");
    let child = compiled
        .iter()
        .find(|rule| rule.group_id == 2 && rule.direction == DIRECTION_EGRESS)
        .expect("specific runtime QoS entry");
    assert_eq!(child.rule_id, "specific-limit");
    assert_eq!(child.rate_bps, 10_000_000);
}
```

- [ ] **Step 8: Verify Batch 5 through GitHub Actions**

```bash
git add agent-rust/agent/src/grpc_client.rs agent-rust/agent/src/acl_qos_manager.rs agent-rust/agent/src/identity.rs agent-rust/agent/src/acl_qos_state.rs agent-rust/agent/src/acl_qos_maps.rs agent-rust/tests/test_grpc_sync.rs
git commit -m "feat(agent): map IP groups to runtime policy ids"
git push
gh workflow run build.yml --repo chenyongming211-glitch/Aria --ref codex/ip-group-policy-model
gh run list --repo chenyongming211-glitch/Aria --branch codex/ip-group-policy-model --limit 3
```

Expected: latest branch run is `completed success`.

---

## Batch 6: Frontend IP Group Experience

**Outcome:** Operators manage IP Groups and use them from ACL/QoS pages. Direct CIDR entry remains available as inline group creation.

**Files:**

- Create: `frontend/src/composables/useIpGroupApi.js`
- Create: `frontend/src/views/IPGroups.vue`
- Modify: `frontend/src/router/index.js`
- Modify: `frontend/src/components/layout/Layout.vue`
- Modify: `frontend/src/views/ACLRules.vue`
- Modify: `frontend/src/views/BandwidthControl.vue`
- Create: `frontend/tests/unit/useIpGroupApi.test.js`
- Modify: `frontend/tests/unit/pagePermissionVisibility.test.js`

- [ ] **Step 1: Add API composable tests**

Create `frontend/tests/unit/useIpGroupApi.test.js`.

```js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useIpGroupApi } from '@/composables/useIpGroupApi'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn()
  }
}))

vi.mock('@/config/api', async () => {
  const actual = await vi.importActual('@/config/api')
  return {
    ...actual,
    requireCurrentTenantId: vi.fn(() => 'tenant-1')
  }
})

import api from '@/composables/useApi'

describe('useIpGroupApi', () => {
  beforeEach(() => vi.clearAllMocks())

  it('normalizes list responses', async () => {
    api.get.mockResolvedValue({ data: { success: true, data: [{ id: 'g1', name: 'office', members: [{ cidr: '10.10.0.0/16' }] }] } })
    const groups = await useIpGroupApi.list()
    expect(api.get).toHaveBeenCalledWith('/v2/tenants/tenant-1/ip-groups')
    expect(groups[0]).toMatchObject({ id: 'g1', name: 'office', member_count: 1 })
  })

  it('sends member CIDRs on create', async () => {
    api.post.mockResolvedValue({ data: { success: true, data: { id: 'g1' } } })
    await useIpGroupApi.create({ name: 'office', members: [{ cidr: '10.10.0.0/16' }] })
    expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/ip-groups', {
      name: 'office',
      description: '',
      members: [{ cidr: '10.10.0.0/16', note: '' }]
    })
  })
})
```

- [ ] **Step 2: Implement `useIpGroupApi.js`**

```js
import api from './useApi'
import { requireCurrentTenantId } from '@/config/api'

const basePath = () => `/v2/tenants/${requireCurrentTenantId()}/ip-groups`

function normalizeGroup(group) {
  const members = Array.isArray(group.members) ? group.members : []
  return {
    ...group,
    members,
    member_count: members.length,
    warnings: Array.isArray(group.warnings) ? group.warnings : []
  }
}

function normalizeList(response) {
  const body = response?.data
  const data = body?.data ?? body
  if (Array.isArray(data)) return data.map(normalizeGroup)
  if (Array.isArray(data?.items)) return data.items.map(normalizeGroup)
  return []
}

function normalizePayload(payload) {
  return {
    name: String(payload.name || '').trim(),
    description: String(payload.description || '').trim(),
    members: (payload.members || []).map(member => ({
      cidr: String(member.cidr || '').trim(),
      note: String(member.note || '').trim()
    }))
  }
}

export const useIpGroupApi = {
  async list() {
    return normalizeList(await api.get(basePath()))
  },
  async create(payload) {
    const response = await api.post(basePath(), normalizePayload(payload))
    return normalizeGroup(response.data?.data || response.data)
  },
  async update(id, payload) {
    const response = await api.put(`${basePath()}/${id}`, normalizePayload(payload))
    return normalizeGroup(response.data?.data || response.data)
  },
  async remove(id) {
    const response = await api.delete(`${basePath()}/${id}`)
    return response.data?.data || response.data
  }
}
```

- [ ] **Step 3: Build `IPGroups.vue`**

The view must include:

```text
Table columns: Name, Kind, Members, Warnings, Updated At, Actions
Create/Edit dialog: name, description, member CIDR rows, note rows
Warning display: overlap warnings with "longest prefix wins"
Delete action disabled for system group and referenced groups
```

Use existing Element Plus table/dialog patterns from `frontend/src/views/ACLRules.vue`.

- [ ] **Step 4: Update ACL page**

`ACLRules.vue` should show:

```text
Source: radio [IP Group] [Direct CIDR]
Destination: radio [IP Group] [Direct CIDR]
When IP Group: select tenant group.
When Direct CIDR: send src_cidr/dst_cidr and let Controller create inline group.
Priority: normal field, default 100, helper text "数字越小优先级越高".
```

Payload examples:

```js
// group mode
{
  src_group_id: form.src_group_id,
  dst_group_id: form.dst_group_id,
  protocol: form.protocol,
  ports: form.ports,
  direction: form.direction,
  action: form.action,
  priority: form.priority,
  enabled: form.enabled
}

// inline CIDR mode
{
  src_cidr: form.src_cidr,
  dst_cidr: form.dst_cidr,
  protocol: form.protocol,
  ports: form.ports,
  direction: form.direction,
  action: form.action,
  priority: form.priority,
  enabled: form.enabled
}
```

- [ ] **Step 5: Update QoS page**

`BandwidthControl.vue` should show:

```text
Group selector: IP Group select
Secondary option: Direct CIDR inline input
Direction: ingress, egress, both
Mode: policing only
Priority: advanced field, default 100, helper text "数字越小优先级越高"
```

Payload examples:

```js
// group mode
{
  group_id: form.group_id,
  direction: form.direction,
  bandwidth_mbps: form.bandwidth_mbps,
  rate_bps: form.rate_bps,
  burst_bytes: form.burst_bytes,
  priority: form.priority,
  mode: 'policing',
  enabled: true
}

// inline CIDR mode
{
  group: form.group_cidr,
  direction: form.direction,
  bandwidth_mbps: form.bandwidth_mbps,
  rate_bps: form.rate_bps,
  burst_bytes: form.burst_bytes,
  priority: form.priority,
  mode: 'policing',
  enabled: true
}
```

- [ ] **Step 6: Verify Batch 6 through GitHub Actions**

```bash
git add frontend/src/composables/useIpGroupApi.js frontend/src/views/IPGroups.vue frontend/src/router/index.js frontend/src/components/layout/Layout.vue frontend/src/views/ACLRules.vue frontend/src/views/BandwidthControl.vue frontend/tests/unit/useIpGroupApi.test.js frontend/tests/unit/pagePermissionVisibility.test.js
git commit -m "feat(frontend): add IP group policy workflow"
git push
gh workflow run build.yml --repo chenyongming211-glitch/Aria --ref codex/ip-group-policy-model
gh run list --repo chenyongming211-glitch/Aria --branch codex/ip-group-policy-model --limit 3
```

Expected: latest branch run is `completed success`.

---

## Batch 7: Online Validation And Migration Safety

**Outcome:** New IP Group model is deployed to gray environment and proven with two-Agent ACL/QoS traffic tests.

**Files:**

- Modify: `docs/deployment.md`
- Modify: `docs/known-issues-status.md`
- Modify: `docs/qos-product-decision.md`

- [ ] **Step 1: Add deployment note**

In `docs/deployment.md`, add:

````markdown
## IP Group Policy Deployment Checks

After deploying a build that includes IP Group policy support:

1. Create a custom IP Group with two CIDRs.
2. Create an ACL rule referencing that group.
3. Create a QoS rule referencing that group.
4. Wait for Agent sync.
5. On Agent host, verify eBPF attachment:

```bash
bpftool net
tc filter show dev aria0 ingress
tc filter show dev aria0 egress
```

6. Verify maps:

```bash
bpftool map dump name SRC_IPV4_ID_MAP
bpftool map dump name DST_IPV4_ID_MAP
bpftool map dump name POLICY_TABLE
bpftool map dump name QOS_CONFIG
```

7. Send allow and deny traffic between two Agent VPN IPs.
8. Confirm ACL and QoS stats are non-zero in Controller and UI.
````

- [ ] **Step 2: Run branch workflow dispatch**

```bash
gh workflow run build.yml --repo chenyongming211-glitch/Aria --ref codex/ip-group-policy-model
gh run watch --repo chenyongming211-glitch/Aria
```

Expected: workflow finishes with `success`.

- [ ] **Step 3: Deploy branch artifacts to gray/online canary**

Use the same deployment path as recent Controller/frontend/Agent rollouts:

```text
Controller: deploy branch GHCR image or action artifact
Frontend: deploy frontend-dist artifact
Agent: deploy branch Agent artifact on both Agent nodes
```

No workstation-local build is allowed.

- [ ] **Step 4: API validation**

Create one group:

```bash
curl -sS -X POST "https://aria.yun/api/v2/tenants/${TENANT_ID}/ip-groups" \
  -H "Authorization: Bearer ${ARIA_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"name":"office","members":[{"cidr":"100.64.0.2/32"},{"cidr":"100.64.0.3/32"}]}'
```

Create ACL:

```bash
curl -sS -X POST "https://aria.yun/api/v2/tenants/${TENANT_ID}/nodes/${NODE_ID}/security/acls" \
  -H "Authorization: Bearer ${ARIA_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"name":"deny-office-icmp","src_group_id":"'"${OFFICE_GROUP_ID}"'","dst_group_id":null,"protocol":1,"direction":"ingress","action":"deny","priority":100,"enabled":true}'
```

Create QoS:

```bash
curl -sS -X POST "https://aria.yun/api/v2/tenants/${TENANT_ID}/nodes/${NODE_ID}/qos" \
  -H "Authorization: Bearer ${ARIA_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"group_id":"'"${OFFICE_GROUP_ID}"'","direction":"egress","bandwidth_mbps":1,"mode":"policing","priority":100,"enabled":true}'
```

Expected:

```text
HTTP 200/201
policy_status eventually applied or idle with pending_cmds=0
stats increment after traffic
priority appears in ACL/QoS GET responses
```

- [ ] **Step 5: Two-Agent datapath validation**

From Agent A:

```bash
ping -c 3 100.64.0.3
```

Expected when deny ACL is enabled:

```text
packet loss is 100%
ACL dropped packet stats increment
```

Disable ACL and retry:

```bash
ping -c 3 100.64.0.3
```

Expected:

```text
packet loss is 0%
ACL passed packet stats increment
```

QoS validation:

```bash
iperf3 -s
iperf3 -c 100.64.0.3 -t 10
```

Expected:

```text
throughput is constrained near configured rule rate
QoS passed/drop stats increment
```

- [ ] **Step 6: Merge only after gray validation**

After the user confirms gray validation:

```bash
git checkout master
git pull --ff-only origin master
git merge --ff-only codex/ip-group-policy-model
git push origin master
gh workflow run build.yml --repo chenyongming211-glitch/Aria --ref master
gh run watch --repo chenyongming211-glitch/Aria
```

Expected:

```text
master workflow finishes with success
master artifacts are deployed
```

---

## Review Checklist For Future Bug Fixes

When reviewing or fixing ACL/QoS bugs after this plan lands, classify issues against this product model:

- If a bug fix treats direct CIDR as the primary product model, reject it or rewrite it around IP Groups.
- If a UI creates ACL/QoS without group references or inline group normalization, it is incomplete.
- If Controller returns runtime group ids to the frontend, it is a product boundary bug.
- If Agent requires tenant id to apply ACL/QoS, it violates the Agent ownership boundary.
- If overlapping groups are hard rejected solely because they overlap, it violates the LPM product decision.
- If exact duplicate CIDRs across non-inline groups are allowed without deterministic ownership, it is a datapath ambiguity bug.
- If ACL/QoS conflict handling ignores priority, treats larger numbers as higher priority, or defaults new QoS rules to priority `0`, it violates the policy priority contract.
- If ambiguous same-priority ACL/QoS conflicts are resolved by `created_at`, snapshot order, or Agent map insertion order instead of being rejected by Controller with HTTP 409, it violates the conflict-detection contract.
- If tests only verify CRUD but not snapshot compilation and Agent map effects, they do not prove the feature.

## Self-Review

- Spec coverage: The plan covers tenant IP Group resources, ACL/QoS group references, direct CIDR inline groups, priority and conflict resolution, Controller snapshot compilation, Agent runtime group ids, LPM overlap semantics, frontend workflows, docs, and online validation.
- Placeholder scan: The plan avoids placeholder tasks and gives exact files, payloads, commands, and expected outcomes.
- Type consistency: Product ids are UUID strings in Controller/API/proto; runtime ids are local `u32` values in Agent/eBPF only.
- Risk boundary: The plan does not change eBPF map ABI except by feeding existing maps with group ids; the deeper QoS `bpf_spin_lock` work remains out of scope.
