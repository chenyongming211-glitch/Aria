# IP Group Reference Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make IP Group references visible, paginated, clickable, and safe to delete or update without hiding stale policy delivery state.

**Architecture:** Add a tenant-scoped reference read model that unions ACL and QoS references for one IP Group, joins the newest matching `policy_deliveries` row, and exposes a paginated API. The frontend loads references lazily, blocks destructive delete when references exist, and deep-links from each reference to the owning ACL/QoS rule. Agent and eBPF code are unchanged.

**Tech Stack:** Go Controller, PostgreSQL, Vue 3, Pinia/composables, Vitest, existing v2 API permission model.

---

## File Structure

- Create: `pkg/controllerstorage/ip_group_references.go`
  - Storage read model and SQL for ACL/QoS references.
- Test: `pkg/controllerstorage/ip_group_references_test.go`
  - Latest-delivery selection, pagination, and tenant isolation tests.
- Modify: `internal/api/v2/ip_groups.go`
  - Add `GET /references`, serialize reference rows, and parse pagination.
- Test: `internal/api/v2/ip_groups_test.go`
  - API permission, pagination, latest-delivery serialization, and 409 delete behavior.
- Modify: `frontend/src/composables/useIpGroupApi.ts`
  - Add `listIPGroupReferences()`.
- Modify: `frontend/src/types/policy.ts`
  - Add reference response types.
- Modify: `frontend/src/views/IPGroups.vue`
  - Add lazy references drawer and delete preflight.
- Modify: `frontend/src/views/ACLRules.vue`
  - Support `?node_id=...&rule_id=...` direct rule targeting.
- Modify: `frontend/src/views/BandwidthControl.vue`
  - Support `?node_id=...&rule_id=...` direct rule targeting.
- Test: `frontend/tests/unit/ipGroupReferences.test.ts`
  - API mapping and delete-preflight behavior.
- Test: `frontend/tests/unit/policyRuleDeepLink.test.ts`
  - ACL/QoS pages select node and highlight rule from route query.
- Modify: `docs/qos-product-decision.md`
  - Keep product contract synchronized.
- Modify: `docs/api-v2-whitepaper.md`
  - Keep API contract synchronized.

## Reference Contract

The endpoint is:

```text
GET /api/v2/tenants/{tenant_id}/ip-groups/{group_id}/references?limit=20&offset=0
```

Default `limit` is `20`, maximum `limit` is `100`, and `offset` must be `0` or greater.

Response:

```json
{
  "items": [
    {
      "domain": "acl",
      "rule_id": "54e9849d-01f4-48f1-8f05-a4c9a34d9473",
      "rule_name": "office-acl",
      "node_id": "2b3a5d52-2892-4a34-a43a-8a934e1d13d6",
      "node_name": "node-1",
      "direction": "egress",
      "enabled": true,
      "latest_delivery": {
        "status": "completed",
        "command_id": "3f671b23-d6bb-45d9-82d0-1f3f6acbd49e",
        "last_error": "",
        "created_at": "2026-06-28T10:12:00Z"
      },
      "route": {
        "name": "ACLRules",
        "path": "/policy-center/acls",
        "query": {
          "node_id": "2b3a5d52-2892-4a34-a43a-8a934e1d13d6",
          "rule_id": "54e9849d-01f4-48f1-8f05-a4c9a34d9473"
        }
      }
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0,
  "has_more": false
}
```

`latest_delivery` is selected by `created_at DESC LIMIT 1` for the exact `tenant_id + node_id + policy_domain + policy_ref` tuple. Runtime group ids must never be returned.

## Task 1: Storage Reference Query

**Files:**
- Create: `pkg/controllerstorage/ip_group_references.go`
- Test: `pkg/controllerstorage/ip_group_references_test.go`

- [ ] **Step 1: Write the failing storage test**

Create `pkg/controllerstorage/ip_group_references_test.go` with the same `sqlmock + NewStorageWithDB` style used by the existing storage tests. The test must assert that the storage layer returns latest delivery fields and pagination metadata.

```go
func TestListIPGroupReferencesUsesLatestDelivery(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	groupID := uuid.New()
	nodeID := uuid.New()
	aclID := uuid.New()
	qosID := uuid.New()
	deliveryID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(listIPGroupReferencesSQL)).
		WithArgs(tenantID, groupID, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"domain", "rule_id", "rule_name", "node_id", "node_name", "direction", "enabled", "total",
			"delivery_id", "command_id", "command_status", "last_error", "delivery_created_at",
		}).AddRow("acl", aclID, "office-acl", nodeID, "node-a", "egress", true, 2, deliveryID, "cmd-2", "completed", "", now).
			AddRow("qos", qosID, "office-qos", nodeID, "node-a", "egress", true, 2, nil, nil, nil, nil, nil))

	page, err := store.ListIPGroupReferences(ctx, tenantID, groupID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 2, page.Total)
	require.Len(t, page.Items, 2)

	aclRef := findReference(t, page.Items, "acl", aclID)
	require.NotNil(t, aclRef.LatestDelivery)
	require.Equal(t, "completed", aclRef.LatestDelivery.Status)
	require.Empty(t, aclRef.LatestDelivery.LastError)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

Also add a pagination assertion:

```go
mock.ExpectQuery(regexp.QuoteMeta(listIPGroupReferencesSQL)).
	WithArgs(tenantID, groupID, 1, 0).
	WillReturnRows(sqlmock.NewRows([]string{
		"domain", "rule_id", "rule_name", "node_id", "node_name", "direction", "enabled", "total",
		"delivery_id", "command_id", "command_status", "last_error", "delivery_created_at",
	}).AddRow("acl", aclID, "office-acl", nodeID, "node-a", "egress", true, 2, deliveryID, "cmd-2", "completed", "", now))
page, err := store.ListIPGroupReferences(ctx, tenantID, groupID, 1, 0)
require.NoError(t, err)
require.Equal(t, 2, page.Total)
require.Len(t, page.Items, 1)
require.True(t, page.HasMore)
```

- [ ] **Step 2: Run the storage test and confirm it fails**

Run:

```bash
go test ./pkg/controllerstorage -run TestListIPGroupReferences -count=1
```

Expected before implementation:

```text
FAIL
undefined: (*Storage).ListIPGroupReferences
```

- [ ] **Step 3: Add the storage types and query**

Create `pkg/controllerstorage/ip_group_references.go`:

```go
package controllerstorage

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type IPGroupReferenceDelivery struct {
	ID        uuid.UUID
	CommandID string
	Status    string
	LastError string
	CreatedAt time.Time
}

type IPGroupReferenceRecord struct {
	Domain         string
	RuleID         uuid.UUID
	RuleName       string
	NodeID         uuid.UUID
	NodeName       string
	Direction      string
	Enabled        bool
	LatestDelivery *IPGroupReferenceDelivery
}

type IPGroupReferencesPage struct {
	Items   []*IPGroupReferenceRecord
	Total   int
	Limit   int
	Offset  int
	HasMore bool
}

const listIPGroupReferencesSQL = `
WITH refs AS (
	SELECT 'acl' AS domain, ar.id AS rule_id, ar.name AS rule_name, ar.node_id, n.hostname AS node_name, ar.direction, ar.enabled
	FROM acl_rules ar
	JOIN nodes n ON n.id = ar.node_id AND n.tenant_id = ar.tenant_id
	WHERE ar.tenant_id = $1 AND (ar.src_group_id = $2 OR ar.dst_group_id = $2)
	UNION ALL
	SELECT 'qos' AS domain, qr.id AS rule_id, qr.name AS rule_name, qr.node_id, n.hostname AS node_name, qr.direction, qr.enabled
	FROM qos_rules qr
	JOIN nodes n ON n.id = qr.node_id AND n.tenant_id = qr.tenant_id
	WHERE qr.tenant_id = $1 AND qr.group_id = $2
)
SELECT refs.domain, refs.rule_id, refs.rule_name, refs.node_id, refs.node_name, refs.direction, refs.enabled,
       COUNT(*) OVER() AS total,
       pd.id AS delivery_id, pd.command_id, pd.command_status, COALESCE(pd.last_error, '') AS last_error, pd.created_at AS delivery_created_at
FROM refs
LEFT JOIN LATERAL (
	SELECT id, command_id, command_status, last_error, created_at
	FROM policy_deliveries
	WHERE tenant_id = $1
	  AND node_id = refs.node_id
	  AND policy_domain = refs.domain
	  AND policy_ref = refs.rule_id::text
	ORDER BY created_at DESC
	LIMIT 1
) pd ON true
ORDER BY refs.domain, refs.rule_name, refs.rule_id
LIMIT $3 OFFSET $4`

func (s *Storage) ListIPGroupReferences(ctx context.Context, tenantID, groupID uuid.UUID, limit, offset int) (*IPGroupReferencesPage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, listIPGroupReferencesSQL, tenantID, groupID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	page := &IPGroupReferencesPage{Limit: limit, Offset: offset}
	for rows.Next() {
		var rec IPGroupReferenceRecord
		var total int
		var deliveryID uuid.NullUUID
		var commandID, status, lastError sql.NullString
		var createdAt sql.NullTime
		if err := rows.Scan(&rec.Domain, &rec.RuleID, &rec.RuleName, &rec.NodeID, &rec.NodeName, &rec.Direction, &rec.Enabled, &total, &deliveryID, &commandID, &status, &lastError, &createdAt); err != nil {
			return nil, err
		}
		page.Total = total
		if deliveryID.Valid {
			rec.LatestDelivery = &IPGroupReferenceDelivery{
				ID:        deliveryID.UUID,
				CommandID: commandID.String,
				Status:    status.String,
				LastError: lastError.String,
				CreatedAt: createdAt.Time,
			}
		}
		page.Items = append(page.Items, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	page.HasMore = page.Offset+len(page.Items) < page.Total
	return page, nil
}
```

- [ ] **Step 4: Run the storage test and fix compile integration**

Run:

```bash
go test ./pkg/controllerstorage -run TestListIPGroupReferences -count=1
```

Expected:

```text
ok  	github.com/chenyongming211/aria-sdwan/pkg/controllerstorage
```

- [ ] **Step 5: Commit Task 1**

```bash
git add pkg/controllerstorage/ip_group_references.go pkg/controllerstorage/ip_group_references_test.go
git commit -m "feat: add ip group reference storage query"
```

## Task 2: v2 API Endpoint

**Files:**
- Modify: `internal/api/v2/ip_groups.go`
- Modify: `internal/api/v2/setup.go`
- Test: `internal/api/v2/ip_groups_test.go`

- [ ] **Step 1: Write API tests**

Add tests that call:

```text
GET /api/v2/tenants/{tenant_id}/ip-groups/{group_id}/references?limit=1&offset=0
```

Assert:

```go
require.Equal(t, http.StatusOK, rr.Code)
require.Equal(t, float64(2), body["total"])
require.Equal(t, float64(1), body["limit"])
require.Equal(t, true, body["has_more"])
items := body["items"].([]any)
first := items[0].(map[string]any)
require.Contains(t, []string{"acl", "qos"}, first["domain"])
require.NotEmpty(t, first["route"])
```

Add permission coverage:

```go
rr := performRequestWithoutPermission(router, "GET", path, tokenWithoutIPGroupsRead, nil)
require.Equal(t, http.StatusForbidden, rr.Code)
```

- [ ] **Step 2: Run API test and confirm route is missing**

Run:

```bash
go test ./internal/api/v2 -run 'TestIPGroupReferences|TestIPGroupReferencePermission' -count=1
```

Expected before implementation:

```text
FAIL
404
```

- [ ] **Step 3: Add route parsing**

In `internal/api/v2/setup.go`, route `.../ip-groups/{group_id}/references` to the IP Group handler before the generic `{group_id}` path consumes it.

The handler branch must require `ip-groups:read` and allow `GET` only. `POST`, `PUT`, and `DELETE` on `/references` return `405`.

- [ ] **Step 4: Add response serialization**

In `internal/api/v2/ip_groups.go`, add:

```go
type ipGroupReferenceResponse struct {
	Domain         string                    `json:"domain"`
	RuleID         string                    `json:"rule_id"`
	RuleName       string                    `json:"rule_name"`
	NodeID         string                    `json:"node_id"`
	NodeName       string                    `json:"node_name"`
	Direction      string                    `json:"direction"`
	Enabled        bool                      `json:"enabled"`
	LatestDelivery *policyDeliverySummary    `json:"latest_delivery,omitempty"`
	Route          ipGroupReferenceRoute      `json:"route"`
}

type ipGroupReferenceRoute struct {
	Name  string            `json:"name"`
	Path  string            `json:"path"`
	Query map[string]string `json:"query"`
}
```

Use this route mapping:

```go
func ipGroupReferenceRouteFor(domain string, nodeID, ruleID uuid.UUID) ipGroupReferenceRoute {
	if domain == "acl" {
		return ipGroupReferenceRoute{
			Name: "ACLRules",
			Path: "/policy-center/acls",
			Query: map[string]string{"node_id": nodeID.String(), "rule_id": ruleID.String()},
		}
	}
	return ipGroupReferenceRoute{
		Name: "BandwidthControl",
		Path: "/policy-center/bandwidth",
		Query: map[string]string{"node_id": nodeID.String(), "rule_id": ruleID.String()},
	}
}
```

- [ ] **Step 5: Parse pagination safely**

Use this behavior:

```go
limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
if limit > 100 {
	limit = 100
}
offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)
```

Invalid negative or non-numeric values should return `400` with `{"error":"invalid pagination"}` rather than silently changing user input.

- [ ] **Step 6: Run API tests**

```bash
go test ./internal/api/v2 -run 'TestIPGroupReferences|TestIPGroupReferencePermission' -count=1
```

Expected:

```text
ok  	github.com/chenyongming211/aria-sdwan/internal/api/v2
```

- [ ] **Step 7: Commit Task 2**

```bash
git add internal/api/v2/ip_groups.go internal/api/v2/setup.go internal/api/v2/ip_groups_test.go
git commit -m "feat: expose ip group policy references"
```

## Task 3: Frontend API Types

**Files:**
- Modify: `frontend/src/composables/useIpGroupApi.ts`
- Modify: `frontend/src/types/policy.ts`
- Test: `frontend/tests/unit/ipGroupReferences.test.ts`

- [ ] **Step 1: Add frontend types**

Add:

```ts
export interface IPGroupReferenceRoute {
  name: 'ACLRules' | 'BandwidthControl'
  path: string
  query: Record<string, string>
}

export interface IPGroupReference {
  domain: 'acl' | 'qos'
  rule_id: string
  rule_name: string
  node_id: string
  node_name: string
  direction: string
  enabled: boolean
  latest_delivery?: {
    status: string
    command_id?: string
    last_error?: string
    created_at?: string
  }
  route: IPGroupReferenceRoute
}

export interface IPGroupReferencePage {
  items: IPGroupReference[]
  total: number
  limit: number
  offset: number
  has_more: boolean
}
```

- [ ] **Step 2: Add API method**

In `useIpGroupApi.ts`:

```ts
async function listIPGroupReferences(groupId: string, params: { limit?: number; offset?: number } = {}) {
  const { data } = await api.get<IPGroupReferencePage>(`/ip-groups/${groupId}/references`, {
    params: {
      limit: params.limit ?? 20,
      offset: params.offset ?? 0,
    },
  })
  return data
}
```

Export it with the existing IP Group API methods.

- [ ] **Step 3: Test API method**

Test with mocked axios client:

```ts
it('loads ip group references with pagination', async () => {
  mockApi.get.mockResolvedValueOnce({ data: { items: [], total: 0, limit: 20, offset: 0, has_more: false } })
  const api = useIpGroupApi()
  await api.listIPGroupReferences('group-1', { limit: 20, offset: 0 })
  expect(mockApi.get).toHaveBeenCalledWith('/ip-groups/group-1/references', {
    params: { limit: 20, offset: 0 },
  })
})
```

- [ ] **Step 4: Run frontend unit test**

```bash
cd frontend
npm run test:run -- ipGroupReferences
```

Expected:

```text
Test Files  1 passed
```

- [ ] **Step 5: Commit Task 3**

```bash
git add frontend/src/composables/useIpGroupApi.ts frontend/src/types/policy.ts frontend/tests/unit/ipGroupReferences.test.ts
git commit -m "feat: add ip group references client"
```

## Task 4: IP Group References UI And Delete Preflight

**Files:**
- Modify: `frontend/src/views/IPGroups.vue`
- Test: `frontend/tests/unit/ipGroupReferences.test.ts`

- [ ] **Step 1: Add failing UI tests**

Add tests for:

```ts
it('blocks delete when references exist', async () => {
  mockIpGroupApi.listIPGroupReferences.mockResolvedValueOnce({
    items: [{ domain: 'acl', rule_id: 'rule-1', rule_name: 'office-acl', node_id: 'node-1', node_name: 'node-a', direction: 'egress', enabled: true, route: { name: 'ACLRules', path: '/policy-center/acls', query: { node_id: 'node-1', rule_id: 'rule-1' } } }],
    total: 1,
    limit: 20,
    offset: 0,
    has_more: false,
  })
  await wrapper.vm.confirmDelete(group)
  expect(mockIpGroupApi.deleteIPGroup).not.toHaveBeenCalled()
})
```

and:

```ts
it('allows delete when no references exist', async () => {
  mockIpGroupApi.listIPGroupReferences.mockResolvedValueOnce({ items: [], total: 0, limit: 20, offset: 0, has_more: false })
  await wrapper.vm.confirmDelete(group)
  expect(mockIpGroupApi.deleteIPGroup).toHaveBeenCalledWith(group.id)
})
```

- [ ] **Step 2: Add lazy references drawer**

Add state:

```ts
const referencesDrawerVisible = ref(false)
const referencesLoading = ref(false)
const references = ref<IPGroupReference[]>([])
const referencesTotal = ref(0)
const referencesOffset = ref(0)
const referencesLimit = 20
const selectedReferenceGroup = ref<IPGroup | null>(null)
```

Add loader:

```ts
async function loadReferences(group: IPGroup, offset = 0) {
  selectedReferenceGroup.value = group
  referencesLoading.value = true
  try {
    const page = await ipGroupApi.listIPGroupReferences(group.id, { limit: referencesLimit, offset })
    references.value = page.items
    referencesTotal.value = page.total
    referencesOffset.value = page.offset
  } finally {
    referencesLoading.value = false
  }
}
```

- [ ] **Step 3: Add delete preflight**

Replace direct delete with:

```ts
async function confirmDelete(group: IPGroup) {
  const page = await ipGroupApi.listIPGroupReferences(group.id, { limit: 20, offset: 0 })
  if (page.total > 0) {
    references.value = page.items
    referencesTotal.value = page.total
    selectedReferenceGroup.value = group
    referencesDrawerVisible.value = true
    ElMessage.warning(t('ipGroups.deleteBlockedByReferences', { count: page.total }))
    return
  }
  await ipGroupApi.deleteIPGroup(group.id)
  await loadGroups()
}
```

- [ ] **Step 4: Add click-through**

For each reference row:

```ts
function openReference(ref: IPGroupReference) {
  router.push({ path: ref.route.path, query: ref.route.query })
}
```

Render the rule name as a link button and show the latest delivery status and error.

- [ ] **Step 5: Run UI tests**

```bash
cd frontend
npm run test:run -- ipGroupReferences
```

Expected:

```text
Test Files  1 passed
```

- [ ] **Step 6: Commit Task 4**

```bash
git add frontend/src/views/IPGroups.vue frontend/tests/unit/ipGroupReferences.test.ts
git commit -m "feat: show ip group references before delete"
```

## Task 5: ACL And QoS Rule Deep Links

**Files:**
- Modify: `frontend/src/views/ACLRules.vue`
- Modify: `frontend/src/views/BandwidthControl.vue`
- Test: `frontend/tests/unit/policyRuleDeepLink.test.ts`

- [ ] **Step 1: Add tests for rule query handling**

Test behavior:

```ts
it('opens acl page on the route query rule id', async () => {
  mockRoute.query = { node_id: 'node-1', rule_id: 'acl-1' }
  mockAclApi.listRules.mockResolvedValueOnce([{ id: 'acl-1', name: 'office-acl' }])
  const wrapper = mount(ACLRules, mountOptions)
  await flushPromises()
  expect(wrapper.vm.selectedNodeId).toBe('node-1')
  expect(wrapper.vm.highlightedRuleId).toBe('acl-1')
})
```

Add the same shape for `BandwidthControl.vue` with `rule_id=qos-1`.

- [ ] **Step 2: Implement route query watchers**

In both views:

```ts
const highlightedRuleId = ref('')

async function applyRuleRouteTarget() {
  const nodeID = String(route.query.node_id || route.query.nodeId || '')
  const ruleID = String(route.query.rule_id || route.query.ruleId || '')
  if (nodeID && nodeID !== selectedNodeId.value) {
    selectedNodeId.value = nodeID
    await loadRules()
  }
  if (ruleID) {
    highlightedRuleId.value = ruleID
    await nextTick()
    scrollRuleIntoView(ruleID)
  }
}
```

Add:

```ts
watch(() => route.query, applyRuleRouteTarget, { immediate: true })
```

- [ ] **Step 3: Add row highlight**

Use a stable row class:

```ts
function tableRowClassName({ row }: { row: PolicyRule }) {
  return row.id === highlightedRuleId.value ? 'policy-row--highlighted' : ''
}
```

Add CSS:

```css
.policy-row--highlighted > td {
  background: rgba(47, 111, 235, 0.08) !important;
}
```

- [ ] **Step 4: Run deep-link tests**

```bash
cd frontend
npm run test:run -- policyRuleDeepLink
```

Expected:

```text
Test Files  1 passed
```

- [ ] **Step 5: Commit Task 5**

```bash
git add frontend/src/views/ACLRules.vue frontend/src/views/BandwidthControl.vue frontend/tests/unit/policyRuleDeepLink.test.ts
git commit -m "feat: deep link policy references to rules"
```

## Task 6: Integration Validation And Documentation

**Files:**
- Modify: `docs/qos-product-decision.md`
- Modify: `docs/api-v2-whitepaper.md`
- Update this plan's checkboxes during execution.

- [ ] **Step 1: Run targeted backend tests**

```bash
go test ./pkg/controllerstorage ./internal/api/v2 -run 'IPGroupReferences|IPGroupReference|IPGroup' -count=1
```

Expected:

```text
ok  	github.com/chenyongming211/aria-sdwan/pkg/controllerstorage
ok  	github.com/chenyongming211/aria-sdwan/internal/api/v2
```

- [ ] **Step 2: Run targeted frontend tests**

```bash
cd frontend
npm run test:run -- ipGroupReferences policyRuleDeepLink
```

Expected:

```text
Test Files  2 passed
```

- [ ] **Step 3: Run repository whitespace check**

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 4: Manual gray validation**

Against the deployed branch artifact:

```text
1. Create IP Group "office".
2. Create ACL rule "office-acl" referencing "office".
3. Create QoS rule "office-qos" referencing "office".
4. Open IP Group references and confirm both rules appear.
5. Trigger ACL delivery failure, then retry successfully; confirm reference shows the newest delivery.
6. Click "office-acl"; confirm ACL page selects node and highlights the rule.
7. Click "office-qos"; confirm bandwidth page selects node and highlights the rule.
8. Attempt to delete "office"; confirm UI blocks delete and DELETE still returns 409 if called directly.
9. Edit "office" members; confirm affected node delivery becomes pending then applied.
```

- [ ] **Step 5: Commit final docs if changed**

```bash
git add docs/qos-product-decision.md docs/api-v2-whitepaper.md docs/superpowers/plans/2026-06-28-ip-group-reference-closure.md
git commit -m "docs: define ip group reference closure"
```

## Self-Review

- Spec coverage:
  - Latest delivery uses newest `policy_deliveries` row by `created_at DESC`.
  - References are paginated and lazily loaded.
  - Reference rows include ACL/QoS clickable routes with `node_id` and `rule_id`.
  - Delete remains blocked when references exist.
  - Group updates continue to sync affected nodes and expose dispatch evidence.
- Placeholder scan:
  - The plan avoids placeholder tasks, vague follow-ups, and unbounded edge-case directives.
- Type consistency:
  - API names use `rule_id`, `node_id`, `latest_delivery`, `limit`, `offset`, and `has_more` consistently.
  - Frontend route queries use `node_id` and `rule_id`; camelCase aliases are accepted only for compatibility.
