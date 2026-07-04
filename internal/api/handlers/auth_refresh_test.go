package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"aria/internal/api/apibase"
	"aria/internal/auth"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE), COALESCE(token_version, 0) FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"username", "role", "tenant_id", "must_change_password", "token_version"}).
			AddRow("alice", "viewer", newTenantID.String(), false, 0))
	expectAuthTenantStatus(mock, newTenantID, "active")
	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE users SET token_version = COALESCE(token_version, 0) + 1, updated_at = NOW() WHERE id = $1 AND COALESCE(token_version, 0) = $2 RETURNING COALESCE(token_version, 0)`)).
		WithArgs(userID.String(), 0).
		WillReturnRows(sqlmock.NewRows([]string{"token_version"}).AddRow(1))

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

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE), COALESCE(token_version, 0) FROM users WHERE id = $1`)).
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

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE), COALESCE(token_version, 0) FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"username", "role", "tenant_id", "must_change_password", "token_version"}).
			AddRow("alice", "viewer", tenantID.String(), false, 0))
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

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE), COALESCE(token_version, 0) FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"username", "role", "tenant_id", "must_change_password", "token_version"}).
			AddRow("alice", "viewer", nil, false, 0))

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

func TestHandleRefreshRejectsStaleTokenVersion(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	tenantID := uuid.New()
	token, err := auth.GenerateTokenWithVersion(userID.String(), "alice", "admin", tenantID.String(), 1)
	if err != nil {
		t.Fatalf("GenerateTokenWithVersion failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE), COALESCE(token_version, 0) FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"username", "role", "tenant_id", "must_change_password", "token_version"}).
			AddRow("alice", "admin", tenantID.String(), false, 2))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	api.HandleRefresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for stale token version, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleLogoutRevokesCurrentTokenVersion(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	token, err := auth.GenerateTokenWithVersion(userID.String(), "alice", "admin", uuid.New().String(), 3)
	if err != nil {
		t.Fatalf("GenerateTokenWithVersion failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(token_version, 0) FROM users WHERE id = $1`)).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"token_version"}).AddRow(3))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET token_version = COALESCE(token_version, 0) + 1, updated_at = NOW() WHERE id = $1 AND COALESCE(token_version, 0) = $2`)).
		WithArgs(userID.String(), 3).
		WillReturnResult(sqlmock.NewResult(0, 1))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	api.HandleLogout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleForceChangePasswordAcceptsCaseInsensitiveBearerScheme(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	token, err := auth.GenerateToken(userID.String(), "alice", "admin", uuid.New().String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	oldHash, err := bcrypt.GenerateFromPassword([]byte("old-password"), 4)
	if err != nil {
		t.Fatalf("GenerateFromPassword failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT password_hash FROM users WHERE id = $1`)).
		WithArgs(userID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(string(oldHash)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET password_hash = $1, must_change_password = FALSE, token_version = COALESCE(token_version, 0) + 1 WHERE id = $2`)).
		WithArgs(sqlmock.AnyArg(), userID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/force-change-password", strings.NewReader(`{"old_password":"old-password","new_password":"new-password"}`))
	req.Header.Set("Authorization", "bearer "+token)
	rr := httptest.NewRecorder()
	api.HandleForceChangePassword(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
