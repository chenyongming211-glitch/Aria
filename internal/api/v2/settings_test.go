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

const settingsBackupRestoreFixtureManifest = `{
  "version":"v0.1.0",
  "created_at":"2026-04-22T15:00:00Z",
  "created_by":"alice",
  "tables":{
    "tenants":[{"id":"11111111-1111-1111-1111-111111111111","name":"Tenant A","code":"tenant-a","status":"active","resource_quota":{},"created_at":"2026-04-22T15:00:00Z","updated_at":"2026-04-22T15:00:00Z"}],
    "users":[{"id":"22222222-2222-2222-2222-222222222222","username":"alice","password_hash":"hash","tenant_id":"11111111-1111-1111-1111-111111111111","role":"admin","email":"alice@example.com","must_change_password":false,"created_at":"2026-04-22T15:00:00Z","last_login":"2026-04-22T15:00:00Z"}],
    "roles":[],
    "tokens":[],
    "nodes":[],
    "ip_groups":[],
    "ip_group_members":[],
    "acl_rules":[],
    "qos_rules":[],
    "blacklist_rules":[]
  }
}`

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

	createReq := withSettingsContext(httptest.NewRequest(http.MethodPost, "/api/v2/settings/backups", nil), "super_admin", "alice")
	createRR := httptest.NewRecorder()
	router.HandleSettings(createRR, createReq)
	if createRR.Code != http.StatusOK {
		t.Fatalf("expected create status %d, got %d", http.StatusOK, createRR.Code)
	}

	createdID := decodeBackupID(t, createRR.Body.Bytes())
	if !strings.HasPrefix(createdID, "aria-config-backup-") {
		t.Fatalf("unexpected backup id %q", createdID)
	}

	listReq := withSettingsContext(httptest.NewRequest(http.MethodGet, "/api/v2/settings/backups", nil), "super_admin", "")
	listRR := httptest.NewRecorder()
	router.HandleSettings(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listRR.Code)
	}
	if !strings.Contains(listRR.Body.String(), createdID) {
		t.Fatalf("expected listed backups to contain %q, got %s", createdID, listRR.Body.String())
	}

	downloadReq := withSettingsContext(httptest.NewRequest(http.MethodGet, "/api/v2/settings/backups/"+createdID+"/download", nil), "super_admin", "")
	downloadRR := httptest.NewRecorder()
	router.HandleSettings(downloadRR, downloadReq)
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("expected download status %d, got %d", http.StatusOK, downloadRR.Code)
	}
	if contentDisposition := downloadRR.Header().Get("Content-Disposition"); !strings.Contains(contentDisposition, createdID) {
		t.Fatalf("expected attachment header to contain backup id, got %q", contentDisposition)
	}

	deleteReq := withSettingsContext(httptest.NewRequest(http.MethodDelete, "/api/v2/settings/backups/"+createdID, nil), "super_admin", "alice")
	deleteRR := httptest.NewRecorder()
	router.HandleSettings(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d", http.StatusOK, deleteRR.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSettingsBackupIncludesIPGroupPolicyColumns(t *testing.T) {
	exportQueries := map[string]string{}
	for _, table := range backupExportTables {
		exportQueries[table.Name] = table.Query
	}
	for _, table := range []string{"ip_groups", "ip_group_members"} {
		if strings.TrimSpace(exportQueries[table]) == "" {
			t.Fatalf("backup export must include %s", table)
		}
	}
	if !strings.Contains(exportQueries["acl_rules"], "src_group_id") ||
		!strings.Contains(exportQueries["acl_rules"], "dst_group_id") {
		t.Fatalf("ACL backup export must include group ids: %s", exportQueries["acl_rules"])
	}
	if !strings.Contains(exportQueries["qos_rules"], "group_id") {
		t.Fatalf("QoS backup export must include group_id: %s", exportQueries["qos_rules"])
	}

	restoreColumns := map[string][]string{}
	for _, table := range backupRestoreTables {
		restoreColumns[table.Name] = table.Columns
	}
	for _, table := range []string{"ip_groups", "ip_group_members"} {
		if len(restoreColumns[table]) == 0 {
			t.Fatalf("backup restore must include %s", table)
		}
	}
	assertColumnPresent(t, restoreColumns["acl_rules"], "src_group_id")
	assertColumnPresent(t, restoreColumns["acl_rules"], "dst_group_id")
	assertColumnPresent(t, restoreColumns["qos_rules"], "group_id")
}

func TestSettingsBackupsRequireSuperAdmin(t *testing.T) {
	cases := []struct {
		name string
		role string
	}{
		{name: "tenant admin denied", role: "admin"},
		{name: "viewer denied", role: "viewer"},
		{name: "missing role denied", role: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := &Router{}
			req := httptest.NewRequest(http.MethodGet, "/api/v2/settings/backups", nil)
			if tc.role != "" {
				req = withSettingsContext(req, tc.role, "alice")
			}
			rr := httptest.NewRecorder()

			router.HandleSettings(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
			}
		})
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
	    "ip_groups":[],
	    "ip_group_members":[],
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

	req := withSettingsContext(httptest.NewRequest(http.MethodPost, "/api/v2/settings/backups/upload", body), "super_admin", "bob")
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

	listReq := withSettingsContext(httptest.NewRequest(http.MethodGet, "/api/v2/settings/backups", nil), "super_admin", "")
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
	    "ip_groups":[],
	    "ip_group_members":[],
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
	mock.ExpectExec(regexp.QuoteMeta(`
			INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
			VALUES ($1, NULL, $2, $3, $4, $5)
		`)).
		WithArgs(
			"11111111-1111-1111-1111-111111111111",
			"settings_backup_restored",
			"bob",
			"Configuration backup restored",
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSettingsContext(httptest.NewRequest(http.MethodPost, "/api/v2/settings/backups/"+backupID+"/restore", nil), "super_admin", "bob")
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

func TestSettingsBackupRestoreDryRunDoesNotModifyDatabase(t *testing.T) {
	t.Setenv("ARIA_BACKUP_DIR", t.TempDir())

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	backupID := "dry-run-fixture"
	writeSettingsBackupFixture(t, backupID, settingsBackupRestoreFixtureManifest)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSettingsContext(
		httptest.NewRequest(
			http.MethodPost,
			"/api/v2/settings/backups/"+backupID+"/restore",
			strings.NewReader(`{"dry_run":true,"tables":["tenants","users"]}`),
		),
		"super_admin",
		"bob",
	)
	rr := httptest.NewRecorder()

	router.HandleSettings(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected dry-run status %d, got %d, body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if data["backup_id"] != backupID {
		t.Fatalf("expected backup_id=%q, got %#v", backupID, data["backup_id"])
	}
	if data["dry_run"] != true {
		t.Fatalf("expected dry_run=true, got %#v", data["dry_run"])
	}
	tableCounts, ok := data["table_counts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected table_counts field, got %#v", data["table_counts"])
	}
	if tableCounts["tenants"] != float64(1) || tableCounts["users"] != float64(1) {
		t.Fatalf("expected tenants/users counts of 1, got %#v", tableCounts)
	}
	if _, exists := tableCounts["roles"]; exists {
		t.Fatalf("expected dry-run table_counts to omit unselected table roles, got %#v", tableCounts)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("dry-run should not execute SQL, got unmet expectations: %v", err)
	}
}

func TestSettingsBackupRestoreDryRunRejectsUnknownTable(t *testing.T) {
	t.Setenv("ARIA_BACKUP_DIR", t.TempDir())

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	backupID := "dry-run-unknown-table"
	writeSettingsBackupFixture(t, backupID, settingsBackupRestoreFixtureManifest)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSettingsContext(
		httptest.NewRequest(
			http.MethodPost,
			"/api/v2/settings/backups/"+backupID+"/restore",
			strings.NewReader(`{"dry_run":true,"tables":["tenants","unknown_table"]}`),
		),
		"super_admin",
		"bob",
	)
	rr := httptest.NewRecorder()

	router.HandleSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusBadRequest, rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "unknown restore table: unknown_table") {
		t.Fatalf("expected unknown table error, got %s", rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("dry-run validation should not execute SQL, got unmet expectations: %v", err)
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

func writeSettingsBackupFixture(t *testing.T, backupID, manifest string) {
	t.Helper()
	backupDir, err := backupDir()
	if err != nil {
		t.Fatalf("backupDir failed: %v", err)
	}
	backupPath := backupDir + "/" + backupID + ".json"
	if err := os.WriteFile(backupPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func assertColumnPresent(t *testing.T, columns []string, expected string) {
	t.Helper()
	for _, column := range columns {
		if column == expected {
			return
		}
	}
	t.Fatalf("expected column %q in %#v", expected, columns)
}
