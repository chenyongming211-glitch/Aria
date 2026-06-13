package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCreateRoleRejectsWildcardPermission(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ops","permissions":["*"]}`))
	rr := httptest.NewRecorder()

	router.createRole(rr, req, uuid.New())

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid permission") {
		t.Fatalf("expected invalid permission error, body=%s", rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateRoleRejectsUnknownPermission(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"description":"ops","permissions":["nodes:read","tenants:delete"]}`))
	rr := httptest.NewRecorder()

	router.updateRole(rr, req, uuid.New(), uuid.New())

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "tenants:delete") {
		t.Fatalf("expected unknown permission in error, body=%s", rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
