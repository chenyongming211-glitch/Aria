package v2

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

type ipGroupMemberPayload struct {
	CIDR string `json:"cidr"`
	Note string `json:"note"`
}

type ipGroupPayload struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Kind        string                 `json:"kind"`
	Members     []ipGroupMemberPayload `json:"members"`
}

func (r *Router) handleTenantIPGroups(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	requiredPermission := middleware.PermIPGroupsRead
	if req.Method != http.MethodGet {
		requiredPermission = middleware.PermIPGroupsWrite
	}
	if !r.authorizeTenantPermission(w, req, tenantID, requiredPermission) {
		return
	}

	groupID, hasGroupID, ok := ipGroupIDFromPath(req.URL.Path)
	if !ok {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown IP group endpoint", nil)
		return
	}

	switch req.Method {
	case http.MethodGet:
		if hasGroupID {
			r.getTenantIPGroup(w, tenantID, groupID)
			return
		}
		r.listTenantIPGroups(w, tenantID)
	case http.MethodPost:
		if hasGroupID {
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
			return
		}
		r.createTenantIPGroup(w, req, tenantID)
	case http.MethodPut:
		if !hasGroupID {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "IP group ID is required for update", nil)
			return
		}
		r.updateTenantIPGroup(w, req, tenantID, groupID)
	case http.MethodDelete:
		if !hasGroupID {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "IP group ID is required for deletion", nil)
			return
		}
		r.deleteTenantIPGroup(w, tenantID, groupID)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func ipGroupIDFromPath(path string) (uuid.UUID, bool, bool) {
	parts := splitPath(path)
	for i, part := range parts {
		if part != "ip-groups" {
			continue
		}
		if len(parts) == i+1 {
			return uuid.Nil, false, true
		}
		if len(parts) == i+2 {
			groupID, err := uuid.Parse(parts[i+1])
			if err != nil {
				return uuid.Nil, true, false
			}
			return groupID, true, true
		}
		return uuid.Nil, false, false
	}
	return uuid.Nil, false, false
}

func (r *Router) listTenantIPGroups(w http.ResponseWriter, tenantID uuid.UUID) {
	groups, err := r.store.ListIPGroups(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list IP groups: "+err.Error(), nil)
		return
	}
	items := make([]map[string]interface{}, 0, len(groups))
	for _, group := range groups {
		items = append(items, ipGroupResponse(group))
	}
	apibase.WriteSuccess(w, items, fmt.Sprintf("%d IP groups retrieved", len(items)))
}

func (r *Router) getTenantIPGroup(w http.ResponseWriter, tenantID, groupID uuid.UUID) {
	group, err := r.store.GetIPGroup(tenantID, groupID)
	if err != nil {
		writeIPGroupError(w, "get", err)
		return
	}
	if err := r.attachIPGroupWarnings(tenantID, group); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load IP group warnings: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, ipGroupResponse(group), "IP group retrieved")
}

func (r *Router) createTenantIPGroup(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	record, err := decodeIPGroupPayload(req, tenantID, uuid.Nil)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}
	created, err := r.store.CreateIPGroup(record)
	if err != nil {
		writeIPGroupError(w, "create", err)
		return
	}
	if err := r.attachIPGroupWarnings(tenantID, created); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load IP group warnings: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, ipGroupResponse(created), "IP group created successfully")
}

func (r *Router) updateTenantIPGroup(w http.ResponseWriter, req *http.Request, tenantID, groupID uuid.UUID) {
	record, err := decodeIPGroupPayload(req, tenantID, groupID)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}
	updated, err := r.store.UpdateIPGroup(record)
	if err != nil {
		writeIPGroupError(w, "update", err)
		return
	}
	if err := r.attachIPGroupWarnings(tenantID, updated); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load IP group warnings: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, ipGroupResponse(updated), "IP group updated successfully")
}

func (r *Router) deleteTenantIPGroup(w http.ResponseWriter, tenantID, groupID uuid.UUID) {
	if err := r.store.DeleteIPGroup(tenantID, groupID); err != nil {
		writeIPGroupError(w, "delete", err)
		return
	}
	apibase.WriteSuccess(w, map[string]string{"id": groupID.String(), "status": "deleted"}, "IP group deleted successfully")
}

func decodeIPGroupPayload(req *http.Request, tenantID, groupID uuid.UUID) (*controllerstorage.IPGroupRecord, error) {
	var body ipGroupPayload
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid request body")
	}
	kind := strings.TrimSpace(body.Kind)
	if kind != "" && !strings.EqualFold(kind, controllerstorage.IPGroupKindCustom) {
		return nil, fmt.Errorf("ip group kind %q is reserved", kind)
	}
	members := make([]controllerstorage.IPGroupMemberRecord, 0, len(body.Members))
	for _, member := range body.Members {
		members = append(members, controllerstorage.IPGroupMemberRecord{
			CIDR: member.CIDR,
			Note: member.Note,
		})
	}
	return &controllerstorage.IPGroupRecord{
		ID:          groupID,
		TenantID:    tenantID,
		Name:        body.Name,
		Description: body.Description,
		Kind:        controllerstorage.IPGroupKindCustom,
		Members:     members,
	}, nil
}

func (r *Router) attachIPGroupWarnings(tenantID uuid.UUID, group *controllerstorage.IPGroupRecord) error {
	if group == nil {
		return nil
	}
	warnings, err := r.store.FindIPGroupOverlapWarnings(tenantID, group.ID, group.Members)
	if err != nil {
		return err
	}
	group.Warnings = warnings
	return nil
}

func ipGroupResponse(group *controllerstorage.IPGroupRecord) map[string]interface{} {
	if group == nil {
		return map[string]interface{}{}
	}
	members := make([]map[string]interface{}, 0, len(group.Members))
	for _, member := range group.Members {
		item := map[string]interface{}{
			"cidr": member.CIDR,
			"note": member.Note,
		}
		if member.ID != uuid.Nil {
			item["id"] = member.ID.String()
		}
		if member.GroupID != uuid.Nil {
			item["group_id"] = member.GroupID.String()
		}
		members = append(members, item)
	}

	response := map[string]interface{}{
		"id":          group.ID.String(),
		"tenant_id":   group.TenantID.String(),
		"name":        group.Name,
		"description": group.Description,
		"kind":        group.Kind,
		"members":     members,
		"warnings":    group.Warnings,
		"created_at":  group.CreatedAt,
		"updated_at":  group.UpdatedAt,
	}
	if group.CreatedBy.Valid {
		response["created_by"] = group.CreatedBy.String
	}
	return response
}

func writeIPGroupError(w http.ResponseWriter, action string, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeNotFound, "IP group not found", nil)
	case strings.Contains(err.Error(), "duplicate CIDR"):
		apibase.WriteError(w, http.StatusConflict, apibase.CodeConflict, err.Error(), nil)
	case strings.Contains(err.Error(), "ip group is referenced by policy rules"):
		apibase.WriteError(w, http.StatusConflict, apibase.CodeConflict, err.Error(), nil)
	case strings.Contains(err.Error(), "invalid CIDR"),
		strings.Contains(err.Error(), "ip group name is required"),
		strings.Contains(err.Error(), "requires at least one CIDR"),
		strings.Contains(err.Error(), "unsupported ip group kind"):
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
	default:
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to "+action+" IP group: "+err.Error(), nil)
	}
}
