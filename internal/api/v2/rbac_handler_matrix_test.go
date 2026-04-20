package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/api/handlers"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

type handlerMatrixCase struct {
	name         string
	mode         string
	role         string
	permissions  []string
	expectStatus int
	expectAudit  bool
}

func roleLookupName(role string) string {
	switch role {
	case "member", "owner":
		return controllerstorage.SystemRoleOperator
	default:
		return role
	}
}

func withAuthContext(req *http.Request, role string, tenantID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.UserRoleKey, role)
	ctx = context.WithValue(ctx, middleware.TenantIDKey, tenantID)
	return req.WithContext(ctx)
}

func expectPermissionLookup(mock sqlmock.Sqlmock, tenantID uuid.UUID, role string, permissions []string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = $2`)).
		WithArgs(tenantID, roleLookupName(role)).
		WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{" + strings.Join(permissions, ",") + "}"))
}

func expectTokenListSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, token, tag, max_uses, used_count, expires_at, created_at, status 
		 FROM tokens WHERE tenant_id = $1 ORDER BY created_at DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "token", "tag", "max_uses", "used_count", "expires_at", "created_at", "status",
		}).AddRow(uuid.New(), "tk_demo", "default", 10, 1, now.Add(24*time.Hour), now, "active"))
}

func expectTenantUpdateSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE tenants 
		SET name = COALESCE(NULLIF($1, ''), name),
		    code = COALESCE(NULLIF($2, ''), code),
		    status = COALESCE(NULLIF($3, ''), status),
		    resource_quota = CASE WHEN $4 = '' THEN resource_quota ELSE $4 END,
		    updated_at = NOW()
		WHERE id = $5`)).
		WithArgs("", "", "", "", tenantID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectResolveAlertSuccess(mock sqlmock.Sqlmock, tenantID, alertID uuid.UUID) {
	now := time.Now()
	title := "CPU high usage"
	alertColumns := []string{
		"id", "tenant_id", "node_id", "alert_type", "severity", "title", "message",
		"context", "status", "created_at", "resolved_at",
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE id = $1`)).
		WithArgs(alertID).
		WillReturnRows(sqlmock.NewRows(alertColumns).AddRow(
			alertID,
			tenantID,
			nil,
			"cpu_high",
			"warning",
			title,
			"high cpu",
			[]byte(`{}`),
			"active",
			now.Add(-10*time.Minute),
			nil,
		))

	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE alerts
		SET status = 'resolved', resolved_at = NOW()
		WHERE id = $1 AND status = 'active'
		RETURNING id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		          context, status, created_at, resolved_at`)).
		WithArgs(alertID).
		WillReturnRows(sqlmock.NewRows(alertColumns).AddRow(
			alertID,
			tenantID,
			nil,
			"cpu_high",
			"warning",
			title,
			"high cpu",
			[]byte(`{}`),
			"resolved",
			now.Add(-10*time.Minute),
			now,
		))

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at`)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nil, "alert_resolved", "user", "Alert resolved: "+title, []byte(`{}`), now))
}

func expectRolesListSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, name, description, is_system, permissions, created_at, updated_at
		FROM roles WHERE tenant_id = $1 ORDER BY is_system DESC, name ASC
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "description", "is_system", "permissions", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, "viewer", "read only", true, "{nodes:read,roles:read}", now, now))
}

func expectUsersListSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, email, role FROM users WHERE tenant_id = $1 ORDER BY created_at DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "role"}).AddRow(uuid.New().String(), "alice", "alice@example.com", "viewer"))
}

func expectNodeLookup(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, routes string) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID,
			"pub-key-1",
			"machine-1",
			tenantID,
			"1.1.1.1:51820",
			"10.0.0.1",
			"1.1.1.1",
			"sh",
			"vpc-1",
			"node-1",
			"10.0.0.10",
			10,
			time.Now().Unix(),
			time.Now().Add(-time.Hour).Unix(),
			"member",
			"kernel",
			"6.0",
			true,
			"online",
			int64(0),
			routes,
			"",
			now,
			now,
		))
}

func expectACLListSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, action, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(dst_port, 0), COALESCE(protocol, 0), priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority DESC, created_at DESC`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, 100, true, "allow web", now, now))
}

func expectQoSListSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND category = $3
		  ORDER BY created_at DESC`)).
		WithArgs(tenantID, nodeID, "service").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "category", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "service", "", "", 0, 443, 6, 200, true, "https limit", now, now))
}

func expectRolesCreateSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO roles (tenant_id, name, description, is_system, permissions)
		VALUES ($1, $2, $3, false, $4)
		RETURNING id, tenant_id, name, description, is_system, permissions, created_at, updated_at`)).
		WithArgs(tenantID, "custom-operator", "custom", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "description", "is_system", "permissions", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, "custom-operator", "custom", false, "{roles:read,roles:write}", now, now))
}

func expectUsersCreateSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users (id, username, password_hash, tenant_id, role, email, created_at) 
			  VALUES ($1, $2, $3, $4, $5, $6, NOW())`)).
		WithArgs(sqlmock.AnyArg(), "bob", sqlmock.AnyArg(), tenantID, "member", "bob@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectACLCreateSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, action, src_cidr, dst_cidr, dst_port, protocol, priority, enabled, description)
		 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, NULLIF($5, '')::cidr, $6, $7, $8, $9, $10)
		 RETURNING id, tenant_id, node_id, action, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		           COALESCE(dst_port, 0), COALESCE(protocol, 0), priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, 100, true, "allow web").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, 100, true, "allow web", now, now))
}

func expectQoSCreateSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO qos_rules (tenant_id, node_id, category, src_cidr, dst_cidr, src_port, dst_port, protocol, bandwidth_mbps, enabled, description)
		 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, NULLIF($5, '')::cidr, $6, $7, $8, $9, $10, $11)
		 RETURNING id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		           COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		           bandwidth_mbps, enabled, COALESCE(description, ''), created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "service", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 200, true, "qos web").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "category", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "service", "", "", 0, 443, 6, 200, true, "qos web", now, now))
}

func expectBumpDesiredState(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO node_control_states (tenant_id, node_id, desired_state_version, desired_state_updated_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (node_id) DO UPDATE SET
		    desired_state_version = EXCLUDED.desired_state_version,
		    desired_state_updated_at = NOW(),
		    updated_at = NOW()`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestRBACHandlerMatrix_TokensRead(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses permission checks", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows granted read permission", mode: "enforce", role: "viewer", permissions: []string{"tokens:read"}, expectStatus: http.StatusOK},
		{name: "super admin bypasses role permissions", mode: "enforce", role: "super_admin", expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}

			if tc.expectStatus == http.StatusOK {
				expectTokenListSuccess(mock, tenantID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/tokens", nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if !tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "" {
				t.Fatalf("unexpected audit denied header")
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_SettingsWrite(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied write with marker", mode: "audit", role: "viewer", permissions: []string{"settings:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing write permission", mode: "enforce", role: "viewer", permissions: []string{"settings:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows granted write permission", mode: "enforce", role: "admin", permissions: []string{"settings:write"}, expectStatus: http.StatusOK},
		{name: "super admin bypasses role permissions", mode: "enforce", role: "super_admin", expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}

			if tc.expectStatus == http.StatusOK {
				expectTenantUpdateSuccess(mock, tenantID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(
				httptest.NewRequest(http.MethodPut, "/api/v2/tenants/"+tenantID.String(), strings.NewReader(`{}`)),
				tc.role,
				tenantID,
			)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if !tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "" {
				t.Fatalf("unexpected audit denied header")
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_CommandsWrite(t *testing.T) {
	tenantID := uuid.New()
	alertID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/monitoring/alerts/" + alertID.String() + "/resolve"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses commands permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied commands with marker", mode: "audit", role: "viewer", permissions: []string{"monitoring:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing commands permission", mode: "enforce", role: "viewer", permissions: []string{"monitoring:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows granted commands permission", mode: "enforce", role: "admin", permissions: []string{"commands:write"}, expectStatus: http.StatusOK},
		{name: "super admin bypasses role permissions", mode: "enforce", role: "super_admin", expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}

			if tc.expectStatus == http.StatusOK {
				expectResolveAlertSuccess(mock, tenantID, alertID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodPost, path, nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if !tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "" {
				t.Fatalf("unexpected audit denied header")
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_RolesRead(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses roles read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied roles read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing roles read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows roles read permission", mode: "enforce", role: "viewer", permissions: []string{"roles:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectRolesListSuccess(mock, tenantID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			// Call handler directly with a path shape that reaches role listing branch.
			req := withAuthContext(httptest.NewRequest(http.MethodGet, "/x/x/x/x/x/x/roles", nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.handleTenantRoles(rr, req, tenantID)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_UsersRead(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses users read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied users read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing users read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows users read permission", mode: "enforce", role: "admin", permissions: []string{"users:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectUsersListSuccess(mock, tenantID)
			}

			store := controllerstorage.NewStorageWithDB(db)
			router := &Router{
				store:     store,
				tenantAPI: handlers.NewTenantAPI(store),
			}
			// Call handler directly with short path so TenantAPI treats GET as list-users branch.
			req := withAuthContext(httptest.NewRequest(http.MethodGet, "/x/x/x/x", nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.handleTenantUsers(rr, req, tenantID, tc.role)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_RoutesRead(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/routes/route-a"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses routes read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied routes read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing routes read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows routes read permission", mode: "enforce", role: "viewer", permissions: []string{"routes:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			// First lookup happens in handleTenantNodes before route permission check.
			expectNodeLookup(mock, tenantID, nodeID, "{route-a}")
			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				// Additional lookups happen in handleTenantNodeRoutes and getTenantNodeRoute.
				expectNodeLookup(mock, tenantID, nodeID, "{route-a}")
				expectNodeLookup(mock, tenantID, nodeID, "{route-a}")
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, path, nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_ACLsRead(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/security/acls"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses ACL read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied ACL read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing ACL read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows ACL read permission", mode: "enforce", role: "viewer", permissions: []string{"acls:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectACLListSuccess(mock, tenantID, nodeID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, path, nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_QoSRead(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/qos/service"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses QoS read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied QoS read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing QoS read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows QoS read permission", mode: "enforce", role: "viewer", permissions: []string{"qos:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectQoSListSuccess(mock, tenantID, nodeID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, path, nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_RolesWrite(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses roles write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied roles write with marker", mode: "audit", role: "viewer", permissions: []string{"roles:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing roles write permission", mode: "enforce", role: "viewer", permissions: []string{"roles:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows roles write permission", mode: "enforce", role: "admin", permissions: []string{"roles:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectRolesCreateSuccess(mock, tenantID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				"/x/x/x/x/x/x/roles",
				strings.NewReader(`{"name":"custom-operator","description":"custom","permissions":["roles:read","roles:write"]}`),
			), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.handleTenantRoles(rr, req, tenantID)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_UsersWrite(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses users write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied users write with marker", mode: "audit", role: "viewer", permissions: []string{"users:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing users write permission", mode: "enforce", role: "viewer", permissions: []string{"users:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows users write permission", mode: "enforce", role: "admin", permissions: []string{"users:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectUsersCreateSuccess(mock, tenantID)
			}

			store := controllerstorage.NewStorageWithDB(db)
			router := &Router{
				store:     store,
				tenantAPI: handlers.NewTenantAPI(store),
			}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				"/x/x/x/x",
				strings.NewReader(`{"username":"bob","password":"P@ssw0rd","email":"bob@example.com","role":"member"}`),
			), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.handleTenantUsers(rr, req, tenantID, tc.role)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_RoutesWrite(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/routes"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses routes write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied routes write with marker", mode: "audit", role: "viewer", permissions: []string{"routes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing routes write permission", mode: "enforce", role: "viewer", permissions: []string{"routes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows routes write permission", mode: "enforce", role: "admin", permissions: []string{"routes:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{10.0.0.0/24}")
			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				// Route POST hits addTenantNodeRoute with secondary lookup.
				expectNodeLookup(mock, tenantID, nodeID, "{10.0.0.0/24}")
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"cidr":"10.0.0.0/24"}`)), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_ACLsWrite(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/security/acls"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses ACL write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied ACL write with marker", mode: "audit", role: "viewer", permissions: []string{"acls:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing ACL write permission", mode: "enforce", role: "viewer", permissions: []string{"acls:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows ACL write permission", mode: "enforce", role: "admin", permissions: []string{"acls:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectACLCreateSuccess(mock, tenantID, nodeID)
				expectBumpDesiredState(mock, tenantID, nodeID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				path,
				strings.NewReader(`{"action":"allow","src_cidr":"10.0.0.0/24","dst_cidr":"0.0.0.0/0","dst_port":443,"protocol":6,"priority":100,"description":"allow web"}`),
			), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_QoSWrite(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/qos/service"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses QoS write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied QoS write with marker", mode: "audit", role: "viewer", permissions: []string{"qos:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing QoS write permission", mode: "enforce", role: "viewer", permissions: []string{"qos:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows QoS write permission", mode: "enforce", role: "admin", permissions: []string{"qos:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectQoSCreateSuccess(mock, tenantID, nodeID)
				expectBumpDesiredState(mock, tenantID, nodeID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				path,
				strings.NewReader(`{"dst_port":443,"protocol":6,"bandwidth_mbps":200,"description":"qos web"}`),
			), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

