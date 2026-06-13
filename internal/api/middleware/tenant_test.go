package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"aria/pkg/controllerstorage"
)

func TestTenantMiddlewareFromTokenAcceptsCaseInsensitiveBearerScheme(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	const token = "tk_test"
	tenantID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id FROM tokens WHERE token = $1 AND status = 'active'`)).
		WithArgs(token).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(tenantID))

	middleware := NewTenantMiddleware(controllerstorage.NewStorageWithDB(db))
	called := false
	handler := middleware.FromToken(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotTenantID, ok := GetTenantID(r.Context())
		if !ok {
			t.Fatalf("expected tenant ID in context")
		}
		if gotTenantID != tenantID {
			t.Fatalf("expected tenant ID %s, got %s", tenantID, gotTenantID)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	req.Header.Set("Authorization", "bearer "+token)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if !called {
		t.Fatal("expected handler to be called")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
