package v2

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func newAuthzRequest(role string, tenantID uuid.UUID) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/nodes", nil)
	ctx := context.WithValue(req.Context(), middleware.UserRoleKey, role)
	ctx = context.WithValue(ctx, middleware.TenantIDKey, tenantID)
	return req.WithContext(ctx)
}

func TestAuthorizeTenantPermission_EnforcementModes(t *testing.T) {
	tenantID := uuid.New()
	otherTenantID := uuid.New()

	t.Run("tenant scope is always enforced", func(t *testing.T) {
		t.Setenv("RBAC_ENFORCEMENT", "off")

		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New failed: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		router := &Router{store: controllerstorage.NewStorageWithDB(db)}
		req := newAuthzRequest("admin", otherTenantID)
		rr := httptest.NewRecorder()

		ok := router.authorizeTenantPermission(rr, req, tenantID, middleware.PermNodesRead)
		if ok {
			t.Fatalf("expected tenant scope check to fail")
		}
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("off mode bypasses permission checks", func(t *testing.T) {
		t.Setenv("RBAC_ENFORCEMENT", "off")

		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New failed: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		router := &Router{store: controllerstorage.NewStorageWithDB(db)}
		req := newAuthzRequest("member", tenantID)
		rr := httptest.NewRecorder()

		ok := router.authorizeTenantPermission(rr, req, tenantID, middleware.PermUsersWrite)
		if !ok {
			t.Fatalf("expected permission check to be bypassed in off mode")
		}
	})

	t.Run("enforce mode denies missing permissions", func(t *testing.T) {
		t.Setenv("RBAC_ENFORCEMENT", "enforce")

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New failed: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = $2`)).
			WithArgs(tenantID, controllerstorage.SystemRoleOperator).
			WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{nodes:read}"))

		router := &Router{store: controllerstorage.NewStorageWithDB(db)}
		req := newAuthzRequest("member", tenantID)
		rr := httptest.NewRecorder()

		ok := router.authorizeTenantPermission(rr, req, tenantID, middleware.PermUsersWrite)
		if ok {
			t.Fatalf("expected enforce mode to deny missing permission")
		}
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("audit mode logs and allows missing permissions", func(t *testing.T) {
		t.Setenv("RBAC_ENFORCEMENT", "audit")

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New failed: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = $2`)).
			WithArgs(tenantID, "viewer").
			WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{nodes:read}"))

		router := &Router{store: controllerstorage.NewStorageWithDB(db)}
		req := newAuthzRequest("viewer", tenantID)
		rr := httptest.NewRecorder()

		ok := router.authorizeTenantPermission(rr, req, tenantID, middleware.PermTokensWrite)
		if !ok {
			t.Fatalf("expected audit mode to allow missing permission")
		}
		if got := rr.Header().Get("X-RBAC-Audit-Denied"); got != "true" {
			t.Fatalf("expected X-RBAC-Audit-Denied header to be set, got %q", got)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("audit mode denies when permission lookup fails", func(t *testing.T) {
		t.Setenv("RBAC_ENFORCEMENT", "audit")

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New failed: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = $2`)).
			WithArgs(tenantID, "viewer").
			WillReturnError(errors.New("lookup failed"))

		router := &Router{store: controllerstorage.NewStorageWithDB(db)}
		req := newAuthzRequest("viewer", tenantID)
		rr := httptest.NewRecorder()

		ok := router.authorizeTenantPermission(rr, req, tenantID, middleware.PermTokensWrite)
		if ok {
			t.Fatalf("expected audit mode to deny on permission lookup failure")
		}
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
		if got := rr.Header().Get("X-RBAC-Audit-Denied"); got != "" {
			t.Fatalf("expected no X-RBAC-Audit-Denied header on lookup failure, got %q", got)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("enforce mode allows granted permissions", func(t *testing.T) {
		t.Setenv("RBAC_ENFORCEMENT", "enforce")

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New failed: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = $2`)).
			WithArgs(tenantID, "admin").
			WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{nodes:read,users:write}"))

		router := &Router{store: controllerstorage.NewStorageWithDB(db)}
		req := newAuthzRequest("admin", tenantID)
		rr := httptest.NewRecorder()

		ok := router.authorizeTenantPermission(rr, req, tenantID, middleware.PermUsersWrite)
		if !ok {
			t.Fatalf("expected enforce mode to allow granted permission")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}
