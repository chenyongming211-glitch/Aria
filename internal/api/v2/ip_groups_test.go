package v2

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

var ipGroupReferenceAPIColumns = []string{
	"domain",
	"rule_id",
	"rule_name",
	"node_id",
	"node_name",
	"direction",
	"enabled",
	"total",
	"delivery_id",
	"command_id",
	"command_status",
	"last_error",
	"delivery_created_at",
}

func TestIPGroupCreateRejectsInvalidCIDR(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	req := withAuthContext(
		httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/ip-groups", strings.NewReader(`{"name":"bad","members":[{"cidr":"not-a-cidr"}]}`)),
		"admin",
		tenantID,
	)
	rec := httptest.NewRecorder()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	router.createTenantIPGroup(rec, req, tenantID)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIPGroupCreateRejectsReservedKind(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	req := withAuthContext(
		httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/ip-groups", strings.NewReader(`{"name":"reserved","kind":"inline","members":[{"cidr":"10.10.0.0/16"}]}`)),
		"admin",
		tenantID,
	)
	rec := httptest.NewRecorder()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	router.createTenantIPGroup(rec, req, tenantID)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleTenantIPGroupsListsWithReadPermission(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	groupID := uuid.New()
	memberID := uuid.New()
	now := time.Now()

	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "viewer", []string{middleware.PermIPGroupsRead})
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, name, COALESCE(description, ''), kind, created_by, created_at, updated_at
		   FROM ip_groups
		  WHERE tenant_id = $1
		  ORDER BY kind ASC, name ASC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "kind", "created_by", "created_at", "updated_at"}).
			AddRow(groupID, tenantID, "office", "office networks", controllerstorage.IPGroupKindCustom, sql.NullString{}, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, group_id, cidr::text, COALESCE(note, '')
		   FROM ip_group_members
		  WHERE tenant_id = $1 AND group_id = $2
		  ORDER BY cidr::text ASC`)).
		WithArgs(tenantID, groupID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "cidr", "note"}).
			AddRow(memberID, groupID, "10.10.0.0/16", "office"))

	req := withAuthContext(
		httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/ip-groups", nil),
		"viewer",
		tenantID,
	)
	rec := httptest.NewRecorder()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	router.handleTenantIPGroups(rec, req, tenantID)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "office") || !strings.Contains(rec.Body.String(), "10.10.0.0/16") {
		t.Fatalf("expected listed group response, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleTenantIPGroupsRequiresWritePermissionForCreate(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "viewer", []string{middleware.PermIPGroupsRead})

	req := withAuthContext(
		httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/ip-groups", strings.NewReader(`{"name":"office","members":[{"cidr":"10.10.0.0/16"}]}`)),
		"viewer",
		tenantID,
	)
	rec := httptest.NewRecorder()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	router.handleTenantIPGroups(rec, req, tenantID)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleTenantIPGroupReferencesListsWithReadPermission(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	groupID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	qosRuleID := uuid.New()
	deliveryID := uuid.New()
	now := time.Now()

	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "viewer", []string{middleware.PermIPGroupsRead})
	mock.ExpectQuery(regexp.QuoteMeta("WITH refs AS")).
		WithArgs(tenantID, groupID, 2, 0).
		WillReturnRows(sqlmock.NewRows(ipGroupReferenceAPIColumns).
			AddRow("acl", ruleID, "office-acl", nodeID, "node-a", "egress", true, 2, deliveryID, "cmd-1", controllerstorage.AgentCommandStatusCompleted, "", now).
			AddRow("qos", qosRuleID, "office-qos", nodeID, "node-a", "egress", true, 2, deliveryID, "cmd-2", controllerstorage.AgentCommandStatusCompleted, "", now))

	req := withAuthContext(
		httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/ip-groups/"+groupID.String()+"/references?limit=2&offset=0", nil),
		"viewer",
		tenantID,
	)
	rec := httptest.NewRecorder()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	router.handleTenantIPGroups(rec, req, tenantID)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{"office-acl", "office-qos", "latest_delivery", "completed", "/policy-center/acl-rules", "/policy-center/bandwidth-control", "rule_id"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected response to contain %q, got %s", expected, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleTenantIPGroupReferencesRejectsInvalidPagination(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	groupID := uuid.New()
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "viewer", []string{middleware.PermIPGroupsRead})

	req := withAuthContext(
		httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/ip-groups/"+groupID.String()+"/references?limit=0", nil),
		"viewer",
		tenantID,
	)
	rec := httptest.NewRecorder()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	router.handleTenantIPGroups(rec, req, tenantID)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid pagination") {
		t.Fatalf("expected invalid pagination error, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleTenantIPGroupDeleteRejectsReferencedGroup(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	groupID := uuid.New()
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "admin", []string{middleware.PermIPGroupsWrite})
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (
		SELECT 1 FROM acl_rules
		 WHERE tenant_id = $1 AND (src_group_id = $2 OR dst_group_id = $2)
		UNION ALL
		SELECT 1 FROM qos_rules
		 WHERE tenant_id = $1 AND group_id = $2
	) AS referenced`)).
		WithArgs(tenantID, groupID).
		WillReturnRows(sqlmock.NewRows([]string{"referenced"}).AddRow(true))

	req := withAuthContext(
		httptest.NewRequest(http.MethodDelete, "/api/v2/tenants/"+tenantID.String()+"/ip-groups/"+groupID.String(), nil),
		"admin",
		tenantID,
	)
	rec := httptest.NewRecorder()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	router.handleTenantIPGroups(rec, req, tenantID)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
