package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"aria/internal/api/apibase"
	"aria/internal/auth"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestHandleRefreshReloadsUserRoleAndTenantFromDatabase(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	oldTenantID := uuid.New()
	newTenantID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "alice", "admin", oldTenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE) FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"username", "role", "tenant_id", "must_change_password"}).
			AddRow("alice", "viewer", newTenantID.String(), false))
	expectAuthTenantStatus(mock, newTenantID, "active")

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	api.HandleRefresh(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	data := resp.Data.(map[string]interface{})
	refreshedToken, ok := data["token"].(string)
	if !ok || refreshedToken == "" {
		t.Fatalf("expected refreshed token, got %#v", data["token"])
	}
	claims, err := auth.ValidateToken(refreshedToken)
	if err != nil {
		t.Fatalf("refreshed token invalid: %v", err)
	}
	if claims.Role != "viewer" || claims.TenantID != newTenantID.String() {
		t.Fatalf("expected refreshed claims from DB, got role=%s tenant=%s", claims.Role, claims.TenantID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRefreshRejectsDeletedUser(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "alice", "admin", uuid.New().String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE) FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnError(sql.ErrNoRows)

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	api.HandleRefresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRefreshRejectsInactiveTenantUser(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	tenantID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "alice", "admin", tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE) FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"username", "role", "tenant_id", "must_change_password"}).
			AddRow("alice", "viewer", tenantID.String(), false))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM tenants WHERE id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("suspended"))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	api.HandleRefresh(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for inactive tenant refresh, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRefreshRejectsTenantUserWithoutTenantID(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "alice", "admin", uuid.New().String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE) FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"username", "role", "tenant_id", "must_change_password"}).
			AddRow("alice", "viewer", nil, false))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	api.HandleRefresh(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for tenant user without tenant_id, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
