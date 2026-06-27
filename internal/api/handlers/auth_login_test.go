package handlers

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"aria/internal/auth"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestHandleLoginRejectsInactiveTenantUser(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	tenantID := uuid.New()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, role, tenant_id, password_hash, COALESCE(must_change_password, FALSE) FROM users WHERE username = $1`)).
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "tenant_id", "password_hash", "must_change_password"}).
			AddRow(userID.String(), controllerstorage.SystemRoleAdmin, tenantID.String(), string(passwordHash), false))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM tenants WHERE id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("suspended"))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", strings.NewReader(`{"username":"alice","password":"secret123"}`))
	rr := httptest.NewRecorder()
	api.HandleLogin(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for inactive tenant login, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleLoginRejectsTenantUserWithoutTenantID(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, role, tenant_id, password_hash, COALESCE(must_change_password, FALSE) FROM users WHERE username = $1`)).
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "tenant_id", "password_hash", "must_change_password"}).
			AddRow(userID.String(), controllerstorage.SystemRoleAdmin, nil, string(passwordHash), false))

	api := NewAuthAPI(controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", strings.NewReader(`{"username":"alice","password":"secret123"}`))
	rr := httptest.NewRecorder()
	api.HandleLogin(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for tenant user without tenant_id, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
