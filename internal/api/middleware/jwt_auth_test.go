package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"aria/internal/auth"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestJWTContextGettersReturnFalseForWrongTypes(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserIDKey, 123)
	ctx = context.WithValue(ctx, UsernameKey, 456)
	ctx = context.WithValue(ctx, UserRoleKey, 789)

	if _, ok := GetUserID(ctx); ok {
		t.Fatalf("expected GetUserID to reject wrong type")
	}
	if _, ok := GetUsername(ctx); ok {
		t.Fatalf("expected GetUsername to reject wrong type")
	}
	if _, ok := GetUserRole(ctx); ok {
		t.Fatalf("expected GetUserRole to reject wrong type")
	}
}

func TestJWTAuthMiddlewareAllowsPermissionsDuringForcedPasswordChange(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	token, err := auth.GenerateToken(uuid.New().String(), "alice", "admin", uuid.New().String(), true)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	called := false
	handler := JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v2/auth/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if !called {
		t.Fatalf("expected handler to be called")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestJWTAuthMiddlewareAcceptsCaseInsensitiveBearerScheme(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	token, err := auth.GenerateToken(uuid.New().String(), "alice", "admin", uuid.New().String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	called := false
	handler := JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v2/tenants", nil)
	req.Header.Set("Authorization", "bearer "+token)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if !called {
		t.Fatalf("expected handler to be called")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestJWTAuthMiddlewareBlocksOtherPathsDuringForcedPasswordChange(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	token, err := auth.GenerateToken(uuid.New().String(), "alice", "admin", uuid.New().String(), true)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	handler := JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("handler should not be called")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v2/tenants", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestJWTAuthMiddlewareWithStoreRejectsStaleTokenVersion(t *testing.T) {
	auth.SetSecret("test-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	userID := uuid.New()
	token, err := auth.GenerateTokenWithVersion(userID.String(), "alice", "admin", uuid.New().String(), 1)
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
		WillReturnRows(sqlmock.NewRows([]string{"token_version"}).AddRow(2))

	handler := JWTAuthMiddlewareWithStore(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("handler should not be called for stale token")
	}, controllerstorage.NewStorageWithDB(db))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/tenants", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
