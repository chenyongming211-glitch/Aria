package v2

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"aria/internal/api/handlers"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestTenantRolesRealPathListsRoles(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectRolesListSuccess(mock, tenantID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/roles", nil), "super_admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestTenantUsersRealCollectionPathListsUsers(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, email, role FROM users WHERE tenant_id = $1 ORDER BY created_at DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "role"}).
			AddRow(uuid.New().String(), "alice", "alice@example.com", "member"))

	store := controllerstorage.NewStorageWithDB(db)
	router := &Router{store: store, tenantAPI: handlers.NewTenantAPI(store)}
	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/users", nil), "super_admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestTenantUserRealDetailPathUpdatesUser(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM roles WHERE tenant_id = $1 AND name = $2)`)).
		WithArgs(tenantID, "admin").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET role = COALESCE(NULLIF($1, ''), role), email = COALESCE(NULLIF($2, ''), email), updated_at = NOW() WHERE id = $3 AND tenant_id = $4`)).
		WithArgs("admin", "", userID, tenantID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := controllerstorage.NewStorageWithDB(db)
	router := &Router{store: store, tenantAPI: handlers.NewTenantAPI(store)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/users/"+userID.String(),
		bytes.NewReader([]byte(`{"role":"admin"}`)),
	), "super_admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestTenantUpdateRejectsDeletedStatusViaPut(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectPermissionLookup(mock, tenantID, "admin", []string{"settings:write"})

	store := controllerstorage.NewStorageWithDB(db)
	router := &Router{store: store, tenantAPI: handlers.NewTenantAPI(store)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String(),
		bytes.NewReader([]byte(`{"status":"deleted"}`)),
	), "admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func expectSingleTenantSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "status", "resource_quota", "created_at", "updated_at"}).
			AddRow(tenantID, "Default", "default", "active", "{}", now, now))
}

func TestTenantListForAdminRequiresSettingsRead(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectPermissionLookup(mock, tenantID, "admin", []string{"nodes:read"})

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v2/tenants", nil), "admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenants(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestTenantListForAdminReturnsOnlyCurrentTenant(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectPermissionLookup(mock, tenantID, "admin", []string{"settings:read"})
	expectSingleTenantSuccess(mock, tenantID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v2/tenants", nil), "admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestTenantListMapsOwnerToAdminRole(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectPermissionLookup(mock, tenantID, "owner", []string{"settings:read"})
	expectSingleTenantSuccess(mock, tenantID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v2/tenants", nil), "owner", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestTenantUsersRejectsExtraPathAfterUserID(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := controllerstorage.NewStorageWithDB(db)
	router := &Router{store: store, tenantAPI: handlers.NewTenantAPI(store)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodDelete,
		"/api/v2/tenants/"+tenantID.String()+"/users/"+userID.String()+"/extra",
		nil,
	), "super_admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTenantCreatesSystemRoles(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO tenants (id, name, code, email, phone, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`)).
		WithArgs(sqlmock.AnyArg(), "Acme", "acme", "", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	for _, role := range []string{"admin", "operator", "viewer"} {
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO roles (tenant_id, name, description, is_system, permissions)
				VALUES ($1, $2, $3, true, $4)
				ON CONFLICT (tenant_id, name) DO NOTHING`)).
			WithArgs(sqlmock.AnyArg(), role, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	api := handlers.NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants", bytes.NewReader([]byte(`{"name":"Acme","code":"acme"}`)))
	rr := httptest.NewRecorder()
	api.CreateTenant(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTenantRollsBackWhenSystemRoleCreationFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO tenants (id, name, code, email, phone, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`)).
		WithArgs(sqlmock.AnyArg(), "Acme", "acme", "", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO roles (tenant_id, name, description, is_system, permissions)
			VALUES ($1, $2, $3, true, $4)
			ON CONFLICT (tenant_id, name) DO NOTHING`)).
		WithArgs(sqlmock.AnyArg(), "admin", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("role insert failed"))
	mock.ExpectRollback()

	api := handlers.NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants", bytes.NewReader([]byte(`{"name":"Acme","code":"acme"}`)))
	rr := httptest.NewRecorder()
	api.CreateTenant(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
