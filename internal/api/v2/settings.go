package v2

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"
)

const backupTimestampLayout = "2006-01-02 15:04:05"
const maxBackupUploadBytes = 10 << 20

var backupExportTables = []struct {
	Name  string
	Query string
}{
	{Name: "tenants", Query: `SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants ORDER BY created_at ASC`},
	{Name: "users", Query: `SELECT id, username, password_hash, tenant_id, role, email, COALESCE(must_change_password, false) AS must_change_password, created_at, last_login FROM users ORDER BY created_at ASC`},
	{Name: "roles", Query: `SELECT id, tenant_id, name, description, is_system, permissions, created_at, updated_at FROM roles ORDER BY created_at ASC`},
	{Name: "tokens", Query: `SELECT id, token, tag, tenant_id, max_uses, used_count, expires_at, created_at, created_by, status, last_used_at, last_used_by FROM tokens ORDER BY created_at ASC`},
	{Name: "nodes", Query: `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, runtime_mode, kernel_version, has_aesni, status, offline_since, advertised_routes, enrolled_with_token, created_at, updated_at FROM nodes ORDER BY created_at ASC`},
	{Name: "acl_rules", Query: `SELECT id, tenant_id, node_id, name, action, src_cidr, dst_cidr, dst_port, protocol, priority, enabled, description, created_at, updated_at FROM acl_rules ORDER BY created_at ASC`},
	{Name: "qos_rules", Query: `SELECT id, tenant_id, node_id, category, src_cidr, dst_cidr, src_port, dst_port, protocol, bandwidth_mbps, enabled, description, created_at, updated_at FROM qos_rules ORDER BY created_at ASC`},
	{Name: "blacklist_rules", Query: `SELECT id, tenant_id, node_id, scope, cidr, port, enabled, description, created_at, updated_at FROM blacklist_rules ORDER BY created_at ASC`},
}

type backupRecord struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Size      string `json:"size"`
	CreatedAt string `json:"created_at"`
	CreatedBy string `json:"created_by"`
}

type backupManifest struct {
	Version   string                   `json:"version"`
	CreatedAt string                   `json:"created_at"`
	CreatedBy string                   `json:"created_by"`
	Tables    map[string][]interface{} `json:"tables"`
}

type backupFile struct {
	record backupRecord
	path   string
	modAt  time.Time
}

// HandleSettings handles /api/v2/settings/* routes.
func (r *Router) HandleSettings(w http.ResponseWriter, req *http.Request) {
	role, exists := middleware.GetUserRole(req.Context())
	if !exists || role == "" {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied", nil)
		return
	}

	if role != "super_admin" && role != "admin" {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Admin access required", nil)
		return
	}

	path := strings.TrimPrefix(req.URL.Path, "/api/v2/settings")
	switch {
	case strings.HasPrefix(path, "/backups"):
		r.handleBackups(w, req, path)
	default:
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown settings endpoint", nil)
	}
}

func (r *Router) handleBackups(w http.ResponseWriter, req *http.Request, path string) {
	rest := strings.TrimPrefix(path, "/backups")

	switch {
	case rest == "" || rest == "/":
		switch req.Method {
		case http.MethodGet:
			r.listBackups(w)
		case http.MethodPost:
			r.createBackup(w, req)
		default:
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		}
	case rest == "/upload":
		if req.Method == http.MethodPost {
			r.uploadBackup(w, req)
			return
		}
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	default:
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		backupID := parts[0]

		if len(parts) == 1 {
			if req.Method == http.MethodDelete {
				r.deleteBackup(w, req, backupID)
				return
			}
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
			return
		}

		switch parts[1] {
		case "download":
			if req.Method == http.MethodGet {
				r.downloadBackup(w, backupID)
				return
			}
		case "restore":
			apibase.WriteError(w, http.StatusNotImplemented, apibase.CodeEndpointNotFound, "Backup restore is not enabled yet", nil)
			return
		}
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown backup endpoint", nil)
	}
}

func (r *Router) listBackups(w http.ResponseWriter) {
	backups, err := r.listBackupFiles()
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list backups: "+err.Error(), nil)
		return
	}

	records := make([]backupRecord, 0, len(backups))
	for _, backup := range backups {
		records = append(records, backup.record)
	}

	apibase.WriteSuccess(w, records, fmt.Sprintf("%d backups retrieved", len(records)))
}

func (r *Router) createBackup(w http.ResponseWriter, req *http.Request) {
	dir, err := backupDir()
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to prepare backup directory: "+err.Error(), nil)
		return
	}

	username, _ := middleware.GetUsername(req.Context())
	if strings.TrimSpace(username) == "" {
		username = "system"
	}

	manifest, err := r.buildBackupManifest(username)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to build backup: "+err.Error(), nil)
		return
	}

	now := time.Now()
	id := fmt.Sprintf("aria-config-backup-%s", now.Format("20060102-150405"))
	filename := id + ".json"
	fullPath := filepath.Join(dir, filename)

	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to encode backup: "+err.Error(), nil)
		return
	}

	if err := os.WriteFile(fullPath, content, 0o600); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to write backup: "+err.Error(), nil)
		return
	}

	record := backupRecord{
		ID:        id,
		Filename:  filename,
		Size:      humanFileSize(int64(len(content))),
		CreatedAt: now.Format(backupTimestampLayout),
		CreatedBy: username,
	}

	if tenantID, ok := middleware.GetTenantID(req.Context()); ok {
		r.store.CreateAuditEvent(&controllerstorage.AuditEvent{
			TenantID:  tenantID,
			EventType: "settings_backup_created",
			Actor:     username,
			Summary:   "Configuration backup created",
			Detail: map[string]interface{}{
				"backup_id": id,
				"filename":  filename,
			},
		})
	}

	apibase.WriteSuccess(w, record, "Backup created successfully")
}

func (r *Router) uploadBackup(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseMultipartForm(maxBackupUploadBytes); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Failed to parse backup upload", nil)
		return
	}

	file, header, err := req.FormFile("file")
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Backup file is required", nil)
		return
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxBackupUploadBytes+1))
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Failed to read uploaded backup", nil)
		return
	}
	if int64(len(content)) > maxBackupUploadBytes {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Uploaded backup is too large", nil)
		return
	}

	var manifest backupManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Uploaded file is not a valid backup manifest", nil)
		return
	}
	if err := validateBackupManifest(&manifest, true); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Uploaded backup is invalid: "+err.Error(), nil)
		return
	}

	dir, err := backupDir()
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to prepare backup directory: "+err.Error(), nil)
		return
	}

	id := uploadedBackupID(header.Filename)
	filename := id + ".json"
	fullPath := filepath.Join(dir, filename)
	if _, err := os.Stat(fullPath); err == nil {
		id = fmt.Sprintf("%s-%s", id, time.Now().UTC().Format("20060102-150405"))
		filename = id + ".json"
		fullPath = filepath.Join(dir, filename)
	}

	normalized, err := json.MarshalIndent(&manifest, "", "  ")
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to encode uploaded backup", nil)
		return
	}
	if err := os.WriteFile(fullPath, normalized, 0o600); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to store uploaded backup", nil)
		return
	}

	backup, err := r.findBackupFile(id)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load uploaded backup metadata", nil)
		return
	}

	username, _ := middleware.GetUsername(req.Context())
	if strings.TrimSpace(username) == "" {
		username = "system"
	}
	if tenantID, ok := middleware.GetTenantID(req.Context()); ok {
		r.store.CreateAuditEvent(&controllerstorage.AuditEvent{
			TenantID:  tenantID,
			EventType: "settings_backup_uploaded",
			Actor:     username,
			Summary:   "Configuration backup uploaded",
			Detail: map[string]interface{}{
				"backup_id": id,
				"filename":  filename,
			},
		})
	}

	apibase.WriteSuccess(w, backup.record, "Backup uploaded successfully")
}

func (r *Router) downloadBackup(w http.ResponseWriter, backupID string) {
	backup, err := r.findBackupFile(backupID)
	if err != nil {
		if err == sql.ErrNoRows {
			apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Backup not found", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load backup: "+err.Error(), nil)
		return
	}

	content, err := os.ReadFile(backup.path)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to read backup file", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", backup.record.Filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (r *Router) deleteBackup(w http.ResponseWriter, req *http.Request, backupID string) {
	backup, err := r.findBackupFile(backupID)
	if err != nil {
		if err == sql.ErrNoRows {
			apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Backup not found", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load backup: "+err.Error(), nil)
		return
	}

	if err := os.Remove(backup.path); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to delete backup", nil)
		return
	}

	username, _ := middleware.GetUsername(req.Context())
	if strings.TrimSpace(username) == "" {
		username = "system"
	}
	if tenantID, ok := middleware.GetTenantID(req.Context()); ok {
		r.store.CreateAuditEvent(&controllerstorage.AuditEvent{
			TenantID:  tenantID,
			EventType: "settings_backup_deleted",
			Actor:     username,
			Summary:   "Configuration backup deleted",
			Detail: map[string]interface{}{
				"backup_id": backupID,
				"filename":  backup.record.Filename,
			},
		})
	}

	apibase.WriteSuccess(w, map[string]string{"id": backupID, "status": "deleted"}, "Backup deleted successfully")
}

func (r *Router) buildBackupManifest(createdBy string) (*backupManifest, error) {
	tables := make(map[string][]interface{}, len(backupExportTables))
	for _, table := range backupExportTables {
		rows, err := queryRowsAsObjects(r.store.DB(), table.Query)
		if err != nil {
			return nil, fmt.Errorf("%s export failed: %w", table.Name, err)
		}
		tables[table.Name] = rows
	}

	return &backupManifest{
		Version:   "v0.1.0",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedBy: createdBy,
		Tables:    tables,
	}, nil
}

func (r *Router) listBackupFiles() ([]backupFile, error) {
	dir, err := backupDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	backups := make([]backupFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		createdAt := info.ModTime().Format(backupTimestampLayout)
		createdBy := "system"
		if meta, err := readBackupManifestHeader(filepath.Join(dir, entry.Name())); err == nil {
			if meta.CreatedBy != "" {
				createdBy = meta.CreatedBy
			}
			if ts, err := time.Parse(time.RFC3339, meta.CreatedAt); err == nil {
				createdAt = ts.Local().Format(backupTimestampLayout)
			}
		}
		backups = append(backups, backupFile{
			record: backupRecord{
				ID:        id,
				Filename:  entry.Name(),
				Size:      humanFileSize(info.Size()),
				CreatedAt: createdAt,
				CreatedBy: createdBy,
			},
			path:  filepath.Join(dir, entry.Name()),
			modAt: info.ModTime(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modAt.After(backups[j].modAt)
	})

	return backups, nil
}

func (r *Router) findBackupFile(backupID string) (*backupFile, error) {
	backups, err := r.listBackupFiles()
	if err != nil {
		return nil, err
	}
	for _, backup := range backups {
		if backup.record.ID == backupID {
			copy := backup
			return &copy, nil
		}
	}
	return nil, sql.ErrNoRows
}

func queryRowsAsObjects(db *sql.DB, query string) ([]interface{}, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	items := make([]interface{}, 0)
	for rows.Next() {
		rawValues := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range rawValues {
			scanArgs[i] = &rawValues[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		item := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			item[col] = normalizeSQLValue(rawValues[i])
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func normalizeSQLValue(value interface{}) interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		var decoded interface{}
		if len(v) > 0 && json.Unmarshal(v, &decoded) == nil {
			return decoded
		}
		return string(v)
	default:
		return v
	}
}

func backupDir() (string, error) {
	dir := strings.TrimSpace(os.Getenv("ARIA_BACKUP_DIR"))
	if dir == "" {
		dir = filepath.Join(".", "data", "backups")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func humanFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func readBackupManifestHeader(path string) (*backupManifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest backupManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func validateBackupManifest(manifest *backupManifest, requireRestoreFields bool) error {
	if manifest == nil {
		return fmt.Errorf("manifest is required")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if len(manifest.Tables) == 0 {
		return fmt.Errorf("tables are required")
	}

	requiredTables := []string{"tenants", "users", "roles", "tokens", "nodes", "acl_rules", "qos_rules", "blacklist_rules"}
	for _, table := range requiredTables {
		if _, ok := manifest.Tables[table]; !ok {
			return fmt.Errorf("missing table %q", table)
		}
	}

	if requireRestoreFields {
		users, ok := manifest.Tables["users"]
		if !ok {
			return fmt.Errorf("missing table %q", "users")
		}
		for _, row := range users {
			item, ok := row.(map[string]interface{})
			if !ok {
				return fmt.Errorf("users rows must be objects")
			}
			if strings.TrimSpace(eventDetailString(item, "password_hash")) == "" {
				return fmt.Errorf("users backup rows must include password_hash")
			}
		}
	}

	return nil
}

func uploadedBackupID(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		base = "uploaded-backup"
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range base {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteRune('-')
			lastDash = true
		}
	}
	id := strings.Trim(builder.String(), "-")
	if id == "" {
		id = "uploaded-backup"
	}
	return id
}
