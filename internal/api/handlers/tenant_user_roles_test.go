package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"aria/internal/api/apibase"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

const tenantRoleExistsQuery = `SELECT EXISTS (SELECT 1 FROM roles WHERE tenant_id = $1 AND name = $2)`

func expectTenantRoleExists(mock sqlmock.Sqlmock, tenantID uuid.UUID, role string, exists bool) {
	mock.ExpectQuery(regexp.QuoteMeta(tenantRoleExistsQuery)).
		WithArgs(tenantID, role).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func TestCreateUserRejectsSuperAdminRole(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v2/tenants/"+tenantID.String()+"/users",
		bytes.NewReader([]byte(`{"username":"evil","password":"Secret123!","role":"super_admin"}`)),
	)
	rr := httptest.NewRecorder()

	api.CreateUser(rr, req, tenantID)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.Code != apibase.CodeAccessDenied {
		t.Fatalf("expected code %q, got %q", apibase.CodeAccessDenied, resp.Code)
	}
}

func TestCreateUserRejectsUnknownRole(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantRoleExists(mock, tenantID, "root", false)

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v2/tenants/"+tenantID.String()+"/users",
		bytes.NewReader([]byte(`{"username":"evil","password":"Secret123!","role":"root"}`)),
	)
	rr := httptest.NewRecorder()

	api.CreateUser(rr, req, tenantID)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateUserAllowsOperatorRole(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantRoleExists(mock, tenantID, "operator", true)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users (id, username, password_hash, tenant_id, role, email, created_at)`)).
		WithArgs(sqlmock.AnyArg(), "operator-user", sqlmock.AnyArg(), tenantID, "operator", "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v2/tenants/"+tenantID.String()+"/users",
		bytes.NewReader([]byte(`{"username":"operator-user","password":"Secret123!","role":"operator"}`)),
	)
	rr := httptest.NewRecorder()

	api.CreateUser(rr, req, tenantID)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateUserAllowsCustomTenantRole(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantRoleExists(mock, tenantID, "support", true)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users (id, username, password_hash, tenant_id, role, email, created_at)`)).
		WithArgs(sqlmock.AnyArg(), "support-user", sqlmock.AnyArg(), tenantID, "support", "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v2/tenants/"+tenantID.String()+"/users",
		bytes.NewReader([]byte(`{"username":"support-user","password":"Secret123!","role":"support"}`)),
	)
	rr := httptest.NewRecorder()

	api.CreateUser(rr, req, tenantID)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateUserRejectsSuperAdminRole(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/users/"+userID.String(),
		bytes.NewReader([]byte(`{"role":"super_admin"}`)),
	)
	rr := httptest.NewRecorder()

	api.UpdateUser(rr, req, tenantID, userID.String())

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestUpdateUserAllowsAdminRole(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantRoleExists(mock, tenantID, "admin", true)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET role = COALESCE(NULLIF($1, ''), role), email = COALESCE(NULLIF($2, ''), email), updated_at = NOW() WHERE id = $3 AND tenant_id = $4`)).
		WithArgs("admin", "", userID, tenantID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/users/"+userID.String(),
		bytes.NewReader([]byte(`{"role":"admin"}`)),
	)
	rr := httptest.NewRecorder()

	api.UpdateUser(rr, req, tenantID, userID.String())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateUserReturnsNotFoundWhenNoRowsUpdated(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantRoleExists(mock, tenantID, "admin", true)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET role = COALESCE(NULLIF($1, ''), role), email = COALESCE(NULLIF($2, ''), email), updated_at = NOW() WHERE id = $3 AND tenant_id = $4`)).
		WithArgs("admin", "", userID, tenantID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/users/"+userID.String(),
		bytes.NewReader([]byte(`{"role":"admin"}`)),
	)
	rr := httptest.NewRecorder()

	api.UpdateUser(rr, req, tenantID, userID.String())

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateUserAllowsCustomTenantRole(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantRoleExists(mock, tenantID, "support", true)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET role = COALESCE(NULLIF($1, ''), role), email = COALESCE(NULLIF($2, ''), email), updated_at = NOW() WHERE id = $3 AND tenant_id = $4`)).
		WithArgs("support", "", userID, tenantID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/users/"+userID.String(),
		bytes.NewReader([]byte(`{"role":"support"}`)),
	)
	rr := httptest.NewRecorder()

	api.UpdateUser(rr, req, tenantID, userID.String())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateUserRejectsMissingCustomRole(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantRoleExists(mock, tenantID, "support", false)

	api := NewTenantAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/users/"+userID.String(),
		bytes.NewReader([]byte(`{"role":"support"}`)),
	)
	rr := httptest.NewRecorder()

	api.UpdateUser(rr, req, tenantID, userID.String())

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
