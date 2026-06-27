package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/internal/auth"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestHandlePermissionsReturnsTenantRolePermissions(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "operator", "operator", tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectAuthTenantStatus(mock, tenantID, "active")
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`)).
		WithArgs(tenantID, controllerstorage.SystemRoleOperator).
		WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{nodes:read,custom:use}"))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/auth/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	middleware.JWTAuthMiddleware(api.HandlePermissions)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected response data object, got %T", resp.Data)
	}
	if got := data["role"]; got != controllerstorage.SystemRoleOperator {
		t.Fatalf("expected role %q, got %#v", controllerstorage.SystemRoleOperator, got)
	}
	if got := data["tenant_id"]; got != tenantID.String() {
		t.Fatalf("expected tenant_id %q, got %#v", tenantID.String(), got)
	}
	rawPermissions, ok := data["permissions"].([]interface{})
	if !ok {
		t.Fatalf("expected permissions array, got %T", data["permissions"])
	}
	gotPermissions := make([]string, 0, len(rawPermissions))
	for _, permission := range rawPermissions {
		gotPermissions = append(gotPermissions, permission.(string))
	}
	wantPermissions := []string{"nodes:read", "custom:use"}
	if len(gotPermissions) != len(wantPermissions) {
		t.Fatalf("expected permissions %v, got %v", wantPermissions, gotPermissions)
	}
	for i := range wantPermissions {
		if gotPermissions[i] != wantPermissions[i] {
			t.Fatalf("expected permissions %v, got %v", wantPermissions, gotPermissions)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func expectAuthTenantStatus(mock sqlmock.Sqlmock, tenantID uuid.UUID, status string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM tenants WHERE id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(status))
}

func TestHandlePermissionsRejectsInactiveTenant(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "operator", "operator", tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectAuthTenantStatus(mock, tenantID, "suspended")

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/auth/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	middleware.JWTAuthMiddleware(api.HandlePermissions)(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandlePermissionsStripsWildcardForTenantRole(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "operator", "operator", tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectAuthTenantStatus(mock, tenantID, "active")
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`)).
		WithArgs(tenantID, controllerstorage.SystemRoleOperator).
		WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{nodes:read,*,custom:use}"))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/auth/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	middleware.JWTAuthMiddleware(api.HandlePermissions)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	data := resp.Data.(map[string]interface{})
	rawPermissions, ok := data["permissions"].([]interface{})
	if !ok {
		t.Fatalf("expected permissions array, got %T", data["permissions"])
	}
	gotPermissions := make([]string, 0, len(rawPermissions))
	for _, permission := range rawPermissions {
		gotPermissions = append(gotPermissions, permission.(string))
	}
	wantPermissions := []string{"nodes:read", "custom:use"}
	if len(gotPermissions) != len(wantPermissions) {
		t.Fatalf("expected permissions %v, got %v", wantPermissions, gotPermissions)
	}
	for i := range wantPermissions {
		if gotPermissions[i] != wantPermissions[i] {
			t.Fatalf("expected permissions %v, got %v", wantPermissions, gotPermissions)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandlePermissionsMatchesCustomRoleCaseInsensitively(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "operator", "OpsRole", tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectAuthTenantStatus(mock, tenantID, "active")
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`)).
		WithArgs(tenantID, "OpsRole").
		WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{nodes:read,custom:use}"))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/auth/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	middleware.JWTAuthMiddleware(api.HandlePermissions)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	data := resp.Data.(map[string]interface{})
	if got := data["role"]; got != "OpsRole" {
		t.Fatalf("expected role %q, got %#v", "OpsRole", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandlePermissionsMapsOwnerToAdmin(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "owner", "owner", tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectAuthTenantStatus(mock, tenantID, "active")
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`)).
		WithArgs(tenantID, controllerstorage.SystemRoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{settings:read,users:read}"))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/auth/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	middleware.JWTAuthMiddleware(api.HandlePermissions)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	data := resp.Data.(map[string]interface{})
	if got := data["role"]; got != controllerstorage.SystemRoleAdmin {
		t.Fatalf("expected role %q, got %#v", controllerstorage.SystemRoleAdmin, got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandlePermissionsReturnsInternalServerErrorOnPermissionLookupFailure(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "operator", "operator", tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectAuthTenantStatus(mock, tenantID, "active")
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`)).
		WithArgs(tenantID, controllerstorage.SystemRoleOperator).
		WillReturnError(errors.New("database unavailable"))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/auth/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	middleware.JWTAuthMiddleware(api.HandlePermissions)(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusInternalServerError, rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandlePermissionsReturnsSuperAdminWildcard(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "sysadmin", "super_admin", "")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/auth/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	middleware.JWTAuthMiddleware(api.HandlePermissions)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected response data object, got %T", resp.Data)
	}
	rawPermissions, ok := data["permissions"].([]interface{})
	if !ok || len(rawPermissions) != 1 || rawPermissions[0] != "*" {
		t.Fatalf("expected wildcard permissions, got %#v", data["permissions"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
