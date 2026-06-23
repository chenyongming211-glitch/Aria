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

func TestUpdateTenantRejectsInvalidFieldsBeforeStorage(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "blank name", body: `{"name":"   "}`},
		{name: "invalid code", body: `{"code":"Bad_Code"}`},
		{name: "unknown status", body: `{"status":"paused"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()

			router.updateTenant(rr, req, uuid.New(), "super_admin")

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected storage interaction: %v", err)
			}
		})
	}
}
