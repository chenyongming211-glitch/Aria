package v2

import (
	"encoding/json"
	"fmt"
	"net/http"

	v1 "aria/internal/api/v1"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

// handleTenantNodeSecurity 调度安全相关的请求 (ACLs, Blacklist)
func (r *Router) handleTenantNodeSecurity(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	if !r.authorizeTenant(w, req, tenantID, false) {
		return
	}

	if len(parts) < 8 {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid security path", nil)
		return
	}

	switch parts[7] {
	case "acls":
		r.handleTenantNodeACLs(w, req, tenantID, node, parts)
	case "blacklist":
		v1.WriteError(w, http.StatusNotImplemented, v1.CodeNotImplemented, "Blacklist API not implemented yet", nil)
	default:
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Unknown security sub-endpoint", nil)
	}
}

// handleTenantNodeACLs 处理具体 ACL 操作
func (r *Router) handleTenantNodeACLs(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	ruleIDStr := ""
	if len(parts) > 8 {
		ruleIDStr = parts[8]
	}

	switch req.Method {
	case http.MethodGet:
		r.listTenantNodeACLs(w, tenantID, node.ID)
	case http.MethodPost:
		r.createTenantNodeACL(w, req, tenantID, node.ID)
	case http.MethodDelete:
		if ruleIDStr == "" {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Rule ID is required for deletion", nil)
			return
		}
		r.deleteTenantNodeACL(w, tenantID, node.ID, ruleIDStr)
	default:
		v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) listTenantNodeACLs(w http.ResponseWriter, tenantID, nodeID uuid.UUID) {
	rules, err := r.store.ListTenantNodeACLRules(tenantID, nodeID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to list ACL rules: "+err.Error(), nil)
		return
	}
	v1.WriteSuccess(w, rules, fmt.Sprintf("%d rules retrieved", len(rules)))
}

func (r *Router) createTenantNodeACL(w http.ResponseWriter, req *http.Request, tenantID, nodeID uuid.UUID) {
	var body struct {
		Action      string `json:"action"`
		SrcCIDR     string `json:"src_cidr"`
		DstCIDR     string `json:"dst_cidr"`
		DstPort     int    `json:"dst_port"`
		Protocol    int    `json:"protocol"`
		Priority    int    `json:"priority"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	rule := &controllerstorage.ACLRuleRecord{
		TenantID:    tenantID,
		NodeID:      nodeID,
		Action:      body.Action,
		SrcCIDR:     body.SrcCIDR,
		DstCIDR:     body.DstCIDR,
		DstPort:     body.DstPort,
		Protocol:    body.Protocol,
		Priority:    body.Priority,
		Enabled:     true,
		Description: body.Description,
	}

	created, err := r.store.CreateTenantNodeACLRule(rule)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to create ACL rule: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, created, "ACL rule created successfully")
}

func (r *Router) deleteTenantNodeACL(w http.ResponseWriter, tenantID, nodeID uuid.UUID, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	if err := r.store.DeleteTenantNodeACLRuleByID(tenantID, nodeID, ruleID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to delete ACL rule: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]string{"id": ruleIDStr, "status": "deleted"}, "ACL rule deleted successfully")
}

// QoS Handlers

func (r *Router) handleTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	if !r.authorizeTenant(w, req, tenantID, false) {
		return
	}

	if len(parts) < 8 {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "QoS category is required", nil)
		return
	}
	category := parts[7]
	if err := controllerstorage.ValidateQoSCategory(category); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	ruleIDStr := ""
	if len(parts) > 8 {
		ruleIDStr = parts[8]
	}

	switch req.Method {
	case http.MethodGet:
		r.listTenantNodeQoS(w, tenantID, node.ID, category)
	case http.MethodPost:
		r.createTenantNodeQoS(w, req, tenantID, node.ID, category)
	case http.MethodDelete:
		if ruleIDStr == "" {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Rule ID is required for deletion", nil)
			return
		}
		r.deleteTenantNodeQoS(w, tenantID, node, category, ruleIDStr)
	default:
		v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) listTenantNodeQoS(w http.ResponseWriter, tenantID, nodeID uuid.UUID, category string) {
	rules, err := r.store.ListTenantNodeQoSRules(tenantID, nodeID, category)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to list QoS rules: "+err.Error(), nil)
		return
	}
	v1.WriteSuccess(w, rules, fmt.Sprintf("%d QoS rules retrieved", len(rules)))
}

func (r *Router) createTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID, nodeID uuid.UUID, category string) {
	var body struct {
		SrcCIDR       string `json:"src_cidr"`
		DstCIDR       string `json:"dst_cidr"`
		SrcPort       int    `json:"src_port"`
		DstPort       int    `json:"dst_port"`
		Protocol      int    `json:"protocol"`
		BandwidthMbps int    `json:"bandwidth_mbps"`
		Description   string `json:"description"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	rule := &controllerstorage.QoSRuleRecord{
		TenantID:      tenantID,
		NodeID:        nodeID,
		Category:      category,
		SrcCIDR:       body.SrcCIDR,
		DstCIDR:       body.DstCIDR,
		SrcPort:       body.SrcPort,
		DstPort:       body.DstPort,
		Protocol:      body.Protocol,
		BandwidthMbps: body.BandwidthMbps,
		Enabled:       true,
		Description:   body.Description,
	}

	created, err := r.store.CreateTenantNodeQoSRule(rule)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to create QoS rule: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, created, "QoS rule created successfully")
}

func (r *Router) deleteTenantNodeQoS(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, category, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	if err := r.store.DeleteTenantNodeQoSRule(tenantID, node.ID, category, ruleID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to delete QoS rule: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]string{"id": ruleIDStr, "status": "deleted"}, "QoS rule deleted successfully")
}

