package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestSettingsBackupUploadStoresValidatedFile(t *testing.T) {
	t.Setenv("ARIA_BACKUP_DIR", t.TempDir())

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", "uploaded-backup.json")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	if _, err := fileWriter.Write([]byte(`{
	  "version":"v0.1.0",
	  "created_at":"2026-04-22T15:00:00Z",
	  "created_by":"alice",
	  "tables":{
	    "tenants":[{"id":"11111111-1111-1111-1111-111111111111","name":"Tenant A","code":"tenant-a","status":"active","resource_quota":{},"created_at":"2026-04-22T15:00:00Z","updated_at":"2026-04-22T15:00:00Z"}],
	    "users":[{"id":"22222222-2222-2222-2222-222222222222","username":"alice","password_hash":"hash","tenant_id":"11111111-1111-1111-1111-111111111111","role":"admin","email":"alice@example.com","must_change_password":false,"created_at":"2026-04-22T15:00:00Z","last_login":"2026-04-22T15:00:00Z"}],
	    "roles":[],
	    "tokens":[],
	    "nodes":[],
	    "acl_rules":[],
	    "qos_rules":[],
	    "blacklist_rules":[]
	  }
	}`)); err != nil {
		t.Fatalf("failed to write upload payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close failed: %v", err)
	}

	req := withSettingsContext(httptest.NewRequest(http.MethodPost, "/api/v2/settings/backups/upload", body), "admin", "bob")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	router.HandleSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected upload status %d, got %d, body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	uploadedID := decodeBackupID(t, rr.Body.Bytes())
	if uploadedID != "uploaded-backup" {
		t.Fatalf("expected uploaded backup id %q, got %q", "uploaded-backup", uploadedID)
	}

	listReq := withSettingsContext(httptest.NewRequest(http.MethodGet, "/api/v2/settings/backups", nil), "admin", "")
	listRR := httptest.NewRecorder()
	router.HandleSettings(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listRR.Code)
	}
	if !strings.Contains(listRR.Body.String(), uploadedID) {
		t.Fatalf("expected listed backups to contain %q, got %s", uploadedID, listRR.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSettingsBackupRestoreAppliesManifest(t *testing.T) {
	t.Setenv("ARIA_BACKUP_DIR", t.TempDir())

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	backupDir, err := backupDir()
	if err != nil {
		t.Fatalf("backupDir failed: %v", err)
	}
	backupID := "restore-fixture"
	backupPath := backupDir + "/" + backupID + ".json"
	manifest := `{
	  "version":"v0.1.0",
	  "created_at":"2026-04-22T15:00:00Z",
	  "created_by":"alice",
	  "tables":{
	    "tenants":[{"id":"11111111-1111-1111-1111-111111111111","name":"Tenant A","code":"tenant-a","status":"active","resource_quota":{},"created_at":"2026-04-22T15:00:00Z","updated_at":"2026-04-22T15:00:00Z"}],
	    "users":[{"id":"22222222-2222-2222-2222-222222222222","username":"alice","password_hash":"hash","tenant_id":"11111111-1111-1111-1111-111111111111","role":"admin","email":"alice@example.com","must_change_password":false,"created_at":"2026-04-22T15:00:00Z","last_login":"2026-04-22T15:00:00Z"}],
	    "roles":[],
	    "tokens":[],
	    "nodes":[],
	    "acl_rules":[],
	    "qos_rules":[],
	    "blacklist_rules":[]
	  }
	}`
	if err := os.WriteFile(backupPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	mock.ExpectBegin()
	for _, table := range backupRestoreCleanupTables {
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM " + table)).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tenants (id, name, code, status, resource_quota, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)")).
		WithArgs(
			"11111111-1111-1111-1111-111111111111",
			"Tenant A",
			"tenant-a",
			"active",
			"{}",
			"2026-04-22T15:00:00Z",
			"2026-04-22T15:00:00Z",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users (id, username, password_hash, tenant_id, role, email, must_change_password, created_at, last_login) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)")).
		WithArgs(
			"22222222-2222-2222-2222-222222222222",
			"alice",
			"hash",
			"11111111-1111-1111-1111-111111111111",
			"admin",
			"alice@example.com",
			false,
			"2026-04-22T15:00:00Z",
			"2026-04-22T15:00:00Z",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSettingsContext(httptest.NewRequest(http.MethodPost, "/api/v2/settings/backups/"+backupID+"/restore", nil), "admin", "bob")
	rr := httptest.NewRecorder()

	router.HandleSettings(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected restore status %d, got %d, body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if data["backup_id"] != backupID {
		t.Fatalf("expected backup_id=%q, got %#v", backupID, data["backup_id"])
	}
	restoredTables, ok := data["restored_tables"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected restored_tables field, got %#v", data["restored_tables"])
	}
	if restoredTables["tenants"] != float64(1) {
		t.Fatalf("expected tenants restore count 1, got %#v", restoredTables["tenants"])
	}
	if restoredTables["users"] != float64(1) {
		t.Fatalf("expected users restore count 1, got %#v", restoredTables["users"])
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
