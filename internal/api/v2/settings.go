package v2

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"

	"github.com/google/uuid"
)

// backupRecord represents a system backup metadata entry
type backupRecord struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Size      string `json:"size"`
	CreatedAt string `json:"created_at"`
	CreatedBy string `json:"created_by"`
}

// HandleSettings handles /api/v2/settings/* routes
func (r *Router) HandleSettings(w http.ResponseWriter, req *http.Request) {
	role, exists := middleware.GetUserRole(req.Context())
	if !exists || role == "" {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied", nil)
		return
	}

	// Only admin and super_admin can manage settings
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
	// /backups — list or create
	// /backups/upload — upload restore
	// /backups/{id}/download — download
	// /backups/{id}/restore — restore
	// /backups/{id} — delete (DELETE)

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
		if req.Method != http.MethodPost {
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
			return
		}
		r.uploadBackup(w, req)
	default:
		// Parse /{id}, /{id}/download, /{id}/restore
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		backupID := parts[0]

		if len(parts) == 1 {
			if req.Method == http.MethodDelete {
				r.deleteBackup(w, backupID)
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
			if req.Method == http.MethodPost {
				r.restoreBackup(w, backupID)
				return
			}
		}
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown backup endpoint", nil)
	}
}

func (r *Router) listBackups(w http.ResponseWriter) {
	apibase.WriteSuccess(w, []backupRecord{}, "No backups available")
}

func (r *Router) createBackup(w http.ResponseWriter, req *http.Request) {
	now := time.Now()
	backup := backupRecord{
		ID:        uuid.New().String(),
		Filename:  fmt.Sprintf("aria-config-backup-%s.json", now.Format("20060102-1504")),
		Size:      "0 KB",
		CreatedAt: now.Format("2006-01-02 15:04:05"),
		CreatedBy: "system",
	}
	apibase.WriteSuccess(w, backup, "Backup created successfully (placeholder)")
}

func (r *Router) uploadBackup(w http.ResponseWriter, req *http.Request) {
	apibase.WriteSuccess(w, nil, "Backup uploaded successfully (placeholder)")
}

func (r *Router) downloadBackup(w http.ResponseWriter, backupID string) {
	apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Backup not found", nil)
}

func (r *Router) restoreBackup(w http.ResponseWriter, backupID string) {
	apibase.WriteSuccess(w, map[string]string{"id": backupID, "status": "restored"}, "Backup restored successfully (placeholder)")
}

func (r *Router) deleteBackup(w http.ResponseWriter, backupID string) {
	apibase.WriteSuccess(w, map[string]string{"id": backupID, "status": "deleted"}, "Backup deleted successfully (placeholder)")
}
