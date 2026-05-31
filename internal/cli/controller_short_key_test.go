package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"aria/pkg/controllerstorage"
	"aria/pkg/logging"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHandleUnregisterShortPublicKeyDoesNotPanic(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger, err := logging.NewLogger(&logging.Config{LogDir: t.TempDir(), Component: "test"})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	controller := &Controller{store: controllerstorage.NewStorageWithDB(db), logger: logger}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/unregister", bytes.NewReader([]byte(`{"public_key":"abc"}`)))
	rr := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandleUnregister panicked for short public_key: %v", recovered)
		}
	}()
	controller.HandleUnregister(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without runtime token, got %d", rr.Code)
	}
}
