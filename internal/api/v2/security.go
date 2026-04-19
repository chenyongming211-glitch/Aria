package v2

import (
	"encoding/json"
	"fmt"
	"net/http"

	"aria/internal/api/apibase"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

// handleTenantNodeSecurity 调度安全相关的请求 (ACLs, Blacklist)
func (r *Router) handleTenantNodeSecurity(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	if !r.authorizeTenant(w, req, tenantID, false) {
		return
	}

	if len(parts) < 8 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid security path", nil)
		return
	}

	switch parts[7] {
	case "acls":
		r.handleTenantNodeACLs(w, req, tenantID, node, parts)
	case "blacklist":
		r.handleTenantNodeBlacklist(w, req, tenantID, node, parts)
	default:
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown security sub-endpoint", nil)
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
	case http.MethodPut:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for update", nil)
			return
		}
		r.updateTenantNodeACL(w, req, tenantID, node.ID, ruleIDStr)
	case http.MethodDelete:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for deletion", nil)
			return
		}
		r.deleteTenantNodeACL(w, tenantID, node.ID, ruleIDStr)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) listTenantNodeACLs(w http.ResponseWriter, tenantID, nodeID uuid.UUID) {
	rules, err := r.store.ListTenantNodeACLRules(tenantID, nodeID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list ACL rules: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, rules, fmt.Sprintf("%d rules retrieved", len(rules)))
}

func (r *Router) createTenantNodeACL(w http.ResponseWriter, req *http.Request, tenantID, nodeID uuid.UUID) {
	var body struct {
		Action      string `json:"action"`
		SrcCIDR     string `json:"src_cidr"`
		DstCIDR     string `json:"dst_cidr"`
		SrcNet      string `json:"src_net"`  // 兼容前端
		DstNet      string `json:"dst_net"`  // 兼容前端
		DstPort     int    `json:"dst_port"`
		MaxPort     int    `json:"max_port"` // 兼容前端
		Protocol    int    `json:"protocol"`
		Priority    int    `json:"priority"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	// 字段合并对齐
	src := body.SrcCIDR
	if src == "" {
		src = body.SrcNet
	}
	dst := body.DstCIDR
	if dst == "" {
		dst = body.DstNet
	}
	port := body.DstPort
	if port == 0 {
		port = body.MaxPort
	}

	// 基础验证
	if body.Action != "allow" && body.Action != "deny" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Action must be 'allow' or 'deny'", nil)
		return
	}

	rule := &controllerstorage.ACLRuleRecord{
		TenantID:    tenantID,
		NodeID:      nodeID,
		Action:      body.Action,
		SrcCIDR:     src,
		DstCIDR:     dst,
		DstPort:     port,
		Protocol:    body.Protocol,
		Priority:    body.Priority,
		Enabled:     true,
		Description: body.Description,
	}

	created, err := r.store.CreateTenantNodeACLRule(rule)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to create ACL rule: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, created, "ACL rule created successfully")
}

func (r *Router) updateTenantNodeACL(w http.ResponseWriter, req *http.Request, tenantID, nodeID uuid.UUID, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	var body struct {
		Action      string `json:"action"`
		SrcCIDR     string `json:"src_cidr"`
		DstCIDR     string `json:"dst_cidr"`
		SrcNet      string `json:"src_net"`
		DstNet      string `json:"dst_net"`
		DstPort     int    `json:"dst_port"`
		MaxPort     int    `json:"max_port"`
		Protocol    int    `json:"protocol"`
		Priority    int    `json:"priority"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	src := body.SrcCIDR
	if src == "" {
		src = body.SrcNet
	}
	dst := body.DstCIDR
	if dst == "" {
		dst = body.DstNet
	}
	port := body.DstPort
	if port == 0 {
		port = body.MaxPort
	}

	if body.Action != "" && body.Action != "allow" && body.Action != "deny" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Action must be 'allow' or 'deny'", nil)
		return
	}

	rule := &controllerstorage.ACLRuleRecord{
		TenantID:    tenantID,
		NodeID:      nodeID,
		Action:      body.Action,
		SrcCIDR:     src,
		DstCIDR:     dst,
		DstPort:     port,
		Protocol:    body.Protocol,
		Priority:    body.Priority,
		Description: body.Description,
		Enabled:     true,
	}
	if body.Enabled != nil {
		rule.Enabled = *body.Enabled
	}

	updated, err := r.store.UpdateTenantNodeACLRule(tenantID, nodeID, ruleID, rule)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to update ACL rule: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, updated, "ACL rule updated successfully")
}

func (r *Router) deleteTenantNodeACL(w http.ResponseWriter, tenantID, nodeID uuid.UUID, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	if err := r.store.DeleteTenantNodeACLRuleByID(tenantID, nodeID, ruleID); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to delete ACL rule: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]string{"id": ruleIDStr, "status": "deleted"}, "ACL rule deleted successfully")
}

// Blacklist Handlers

func (r *Router) handleTenantNodeBlacklist(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	if len(parts) < 9 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Blacklist scope is required (src, dst, ports)", nil)
		return
	}
	scope := parts[8]
	if err := controllerstorage.ValidateBlacklistScope(scope); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}

	ruleIDStr := ""
	if len(parts) > 9 {
		ruleIDStr = parts[9]
	}

	switch req.Method {
	case http.MethodGet:
		r.listTenantNodeBlacklistRules(w, tenantID, node.ID, scope)
	case http.MethodPost:
		r.createTenantNodeBlacklistRule(w, req, tenantID, node.ID, scope)
	case http.MethodDelete:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for deletion", nil)
			return
		}
		r.deleteTenantNodeBlacklistRule(w, tenantID, node.ID, scope, ruleIDStr)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) listTenantNodeBlacklistRules(w http.ResponseWriter, tenantID, nodeID uuid.UUID, scope string) {
	rules, err := r.store.ListTenantNodeBlacklistRules(tenantID, nodeID, scope)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list blacklist rules: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, rules, fmt.Sprintf("%d blacklist rules retrieved", len(rules)))
}

func (r *Router) createTenantNodeBlacklistRule(w http.ResponseWriter, req *http.Request, tenantID, nodeID uuid.UUID, scope string) {
	var body struct {
		CIDR        string `json:"cidr"`
		Port        int    `json:"port"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if scope != "ports" && body.CIDR == "" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "CIDR is required for src/dst scope", nil)
		return
	}
	if scope == "ports" && body.Port == 0 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Port is required for ports scope", nil)
		return
	}

	rule := &controllerstorage.BlacklistRuleRecord{
		TenantID:    tenantID,
		NodeID:      nodeID,
		Scope:       scope,
		CIDR:        body.CIDR,
		Port:        body.Port,
		Enabled:     true,
		Description: body.Description,
	}

	created, err := r.store.CreateTenantNodeBlacklistRule(rule)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to create blacklist rule: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, created, "Blacklist rule created successfully")
}

func (r *Router) deleteTenantNodeBlacklistRule(w http.ResponseWriter, tenantID, nodeID uuid.UUID, scope string, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	if err := r.store.DeleteTenantNodeBlacklistRuleByID(tenantID, nodeID, scope, ruleID); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to delete blacklist rule: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]string{"id": ruleIDStr, "status": "deleted"}, "Blacklist rule deleted successfully")
}

// QoS Handlers

func (r *Router) handleTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	if !r.authorizeTenant(w, req, tenantID, false) {
		return
	}

	if len(parts) < 8 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "QoS category is required", nil)
		return
	}
	category := parts[7]
	if err := controllerstorage.ValidateQoSCategory(category); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
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
	case http.MethodPut:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for update", nil)
			return
		}
		r.updateTenantNodeQoS(w, req, tenantID, node.ID, category, ruleIDStr)
	case http.MethodDelete:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for deletion", nil)
			return
		}
		r.deleteTenantNodeQoS(w, tenantID, node, category, ruleIDStr)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) listTenantNodeQoS(w http.ResponseWriter, tenantID, nodeID uuid.UUID, category string) {
	rules, err := r.store.ListTenantNodeQoSRules(tenantID, nodeID, category)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list QoS rules: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, rules, fmt.Sprintf("%d QoS rules retrieved", len(rules)))
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
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
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
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to create QoS rule: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, created, "QoS rule created successfully")
}

func (r *Router) updateTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID, nodeID uuid.UUID, category, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	var body struct {
		SrcCIDR       string `json:"src_cidr"`
		DstCIDR       string `json:"dst_cidr"`
		SrcPort       int    `json:"src_port"`
		DstPort       int    `json:"dst_port"`
		Protocol      int    `json:"protocol"`
		BandwidthMbps int    `json:"bandwidth_mbps"`
		Description   string `json:"description"`
		Enabled       *bool  `json:"enabled"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
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
		Description:   body.Description,
		Enabled:       true,
	}
	if body.Enabled != nil {
		rule.Enabled = *body.Enabled
	}

	updated, err := r.store.UpdateTenantNodeQoSRule(tenantID, nodeID, ruleID, category, rule)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to update QoS rule: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, updated, "QoS rule updated successfully")
}

func (r *Router) deleteTenantNodeQoS(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, category, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	if err := r.store.DeleteTenantNodeQoSRule(tenantID, node.ID, category, ruleID); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to delete QoS rule: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]string{"id": ruleIDStr, "status": "deleted"}, "QoS rule deleted successfully")
}
