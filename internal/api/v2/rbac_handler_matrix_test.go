package v2

import (
	"context"
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

