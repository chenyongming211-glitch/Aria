package v2

import (
	"encoding/json"
	"fmt"
	"net/http"

	"aria/internal/api/apibase"
	controllerstorage "aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

func (r *Router) handleRoles(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, parts []string) {
	switch req.Method {
	case http.MethodGet:
		r.listRoles(w, tenantID)
	case http.MethodPost:
		r.createRole(w, req, tenantID)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) handleRoleDetail(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, roleIDStr string) {
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid role ID", nil)
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.getRole(w, tenantID, roleID)
	case http.MethodPut:
		r.updateRole(w, req, tenantID, roleID)
	case http.MethodDelete:
		r.deleteRole(w, tenantID, roleID)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) listRoles(w http.ResponseWriter, tenantID uuid.UUID) {
	roles, err := r.store.ListRoles(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list roles: "+err.Error(), nil)
		return
	}
	if roles == nil {
		roles = []*controllerstorage.Role{}
	}
	apibase.WriteSuccess(w, roles, fmt.Sprintf("%d roles retrieved", len(roles)))
}

func (r *Router) getRole(w http.ResponseWriter, tenantID, roleID uuid.UUID) {
	role, err := r.store.GetRoleByID(tenantID, roleID)
	if err != nil {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeNotFound, "Role not found", nil)
		return
	}
	apibase.WriteSuccess(w, role, "Role retrieved")
}

func (r *Router) createRole(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}
	if body.Name == "" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Role name is required", nil)
		return
	}
	if len(body.Permissions) == 0 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "At least one permission is required", nil)
		return
	}

	role := &controllerstorage.Role{
		TenantID:    tenantID,
		Name:        body.Name,
		Description: body.Description,
		Permissions: body.Permissions,
	}

	created, err := r.store.CreateRole(role)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to create role: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, created, "Role created successfully")
}

func (r *Router) updateRole(w http.ResponseWriter, req *http.Request, tenantID, roleID uuid.UUID) {
	var body struct {
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	updated, err := r.store.UpdateRole(tenantID, roleID, body.Description, body.Permissions)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Failed to update role: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, updated, "Role updated successfully")
}

func (r *Router) deleteRole(w http.ResponseWriter, tenantID, roleID uuid.UUID) {
	if err := r.store.DeleteRole(tenantID, roleID); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, map[string]string{"id": roleID.String(), "status": "deleted"}, "Role deleted successfully")
}
