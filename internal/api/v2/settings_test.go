package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSettingsBackupsLifecycle(t *testing.T) {
	t.Setenv("ARIA_BACKUP_DIR", t.TempDir())

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, table := range backupExportTables {
		mock.ExpectQuery(regexp.QuoteMeta(table.Query)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
	}

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}

	createReq := withSettingsContext(httptest.NewRequest(http.MethodPost, "/api/v2/settings/backups", nil), "admin", "alice")
	createRR := httptest.NewRecorder()
	router.HandleSettings(createRR, createReq)
	if createRR.Code != http.StatusOK {
		t.Fatalf("expected create status %d, got %d", http.StatusOK, createRR.Code)
	}

	createdID := decodeBackupID(t, createRR.Body.Bytes())
	if !strings.HasPrefix(createdID, "aria-config-backup-") {
		t.Fatalf("unexpected backup id %q", createdID)
	}

	listReq := withSettingsContext(httptest.NewRequest(http.MethodGet, "/api/v2/settings/backups", nil), "admin", "")
	listRR := httptest.NewRecorder()
	router.HandleSettings(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listRR.Code)
	}
	if !strings.Contains(listRR.Body.String(), createdID) {
		t.Fatalf("expected listed backups to contain %q, got %s", createdID, listRR.Body.String())
	}

	downloadReq := withSettingsContext(httptest.NewRequest(http.MethodGet, "/api/v2/settings/backups/"+createdID+"/download", nil), "admin", "")
	downloadRR := httptest.NewRecorder()
	router.HandleSettings(downloadRR, downloadReq)
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("expected download status %d, got %d", http.StatusOK, downloadRR.Code)
	}
	if contentDisposition := downloadRR.Header().Get("Content-Disposition"); !strings.Contains(contentDisposition, createdID) {
		t.Fatalf("expected attachment header to contain backup id, got %q", contentDisposition)
	}

	deleteReq := withSettingsContext(httptest.NewRequest(http.MethodDelete, "/api/v2/settings/backups/"+createdID, nil), "admin", "alice")
	deleteRR := httptest.NewRecorder()
	router.HandleSettings(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d", http.StatusOK, deleteRR.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func withSettingsContext(req *http.Request, role, username string) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.UserRoleKey, role)
	if username != "" {
		ctx = context.WithValue(ctx, middleware.UsernameKey, username)
	}
	return req.WithContext(ctx)
}

func decodeBackupID(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to decode backup response: %v", err)
	}
	return payload.Data.ID
}
