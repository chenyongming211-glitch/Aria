package v2

import (
	"encoding/json"
	"fmt"
	"net/http"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

// handleTenantNodeSecurity 调度安全相关的请求 (ACLs, Blacklist)
func (r *Router) handleTenantNodeSecurity(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
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
	requiredPermission := middleware.PermAclsRead
	if req.Method != http.MethodGet {
		requiredPermission = middleware.PermAclsWrite
	}
	if !r.authorizeTenantPermission(w, req, tenantID, requiredPermission) {
		return
	}

	ruleIDStr := ""
	if len(parts) > 8 {
		ruleIDStr = parts[8]
	}

	switch req.Method {
	case http.MethodGet:
		r.listTenantNodeACLs(w, tenantID, node.ID)
	case http.MethodPost:
		r.createTenantNodeACL(w, req, tenantID, node)
	case http.MethodPut:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for update", nil)
			return
		}
		r.updateTenantNodeACL(w, req, tenantID, node, ruleIDStr)
	case http.MethodDelete:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for deletion", nil)
			return
		}
		r.deleteTenantNodeACL(w, tenantID, node, ruleIDStr)
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

func (r *Router) createTenantNodeACL(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node) {
	var body struct {
		Name        string `json:"name"`
		Action      string `json:"action"`
		SrcCIDR     string `json:"src_cidr"`
		DstCIDR     string `json:"dst_cidr"`
		SrcNet      string `json:"src_net"` // 兼容前端
		DstNet      string `json:"dst_net"` // 兼容前端
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
		NodeID:      node.ID,
		Name:        body.Name,
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

	r.writePolicyMutationSuccess(w, node, "acl", "create", map[string]interface{}{
		"id":          created.ID.String(),
		"node_id":     node.ID.String(),
		"name":        created.Name,
		"action":      created.Action,
		"src_cidr":    created.SrcCIDR,
		"dst_cidr":    created.DstCIDR,
		"dst_port":    created.DstPort,
		"protocol":    created.Protocol,
		"priority":    created.Priority,
		"enabled":     created.Enabled,
		"description": created.Description,
		"created_at":  created.CreatedAt,
		"updated_at":  created.UpdatedAt,
	}, "ACL rule created successfully", map[string]interface{}{
		"node_id":     node.ID.String(),
		"rule_id":     created.ID.String(),
		"description": created.Description,
	})
}

func (r *Router) updateTenantNodeACL(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	var body struct {
		Name        string `json:"name"`
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
		NodeID:      node.ID,
		Name:        body.Name,
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

	updated, err := r.store.UpdateTenantNodeACLRule(tenantID, node.ID, ruleID, rule)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to update ACL rule: "+err.Error(), nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "acl", "update", map[string]interface{}{
		"id":          ruleID.String(),
		"node_id":     node.ID.String(),
		"name":        updated.Name,
		"action":      updated.Action,
		"src_cidr":    updated.SrcCIDR,
		"dst_cidr":    updated.DstCIDR,
		"dst_port":    updated.DstPort,
		"protocol":    updated.Protocol,
		"priority":    updated.Priority,
		"enabled":     updated.Enabled,
		"description": updated.Description,
		"updated_at":  updated.UpdatedAt,
	}, "ACL rule updated successfully", map[string]interface{}{
		"node_id":     node.ID.String(),
		"rule_id":     ruleID.String(),
		"description": updated.Description,
	})
}

func (r *Router) deleteTenantNodeACL(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	if err := r.store.DeleteTenantNodeACLRuleByID(tenantID, node.ID, ruleID); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to delete ACL rule: "+err.Error(), nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "acl", "delete", map[string]interface{}{
		"id":      ruleIDStr,
		"node_id": node.ID.String(),
		"status":  "deleted",
	}, "ACL rule deleted successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"rule_id": ruleIDStr,
	})
}

// Blacklist Handlers

func (r *Router) handleTenantNodeBlacklist(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	requiredPermission := middleware.PermBlacklistRead
	if req.Method != http.MethodGet {
		requiredPermission = middleware.PermBlacklistWrite
	}
	if !r.authorizeTenantPermission(w, req, tenantID, requiredPermission) {
		return
	}

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
		r.createTenantNodeBlacklistRule(w, req, tenantID, node, scope)
	case http.MethodDelete:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for deletion", nil)
			return
		}
		r.deleteTenantNodeBlacklistRule(w, tenantID, node, scope, ruleIDStr)
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

func (r *Router) createTenantNodeBlacklistRule(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, scope string) {
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
		NodeID:      node.ID,
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

	r.writePolicyMutationSuccess(w, node, "blacklist", "create", map[string]interface{}{
		"id":          created.ID.String(),
		"node_id":     node.ID.String(),
		"scope":       created.Scope,
		"cidr":        created.CIDR,
		"port":        created.Port,
		"enabled":     created.Enabled,
		"description": created.Description,
		"created_at":  created.CreatedAt,
		"updated_at":  created.UpdatedAt,
	}, "Blacklist rule created successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"rule_id": created.ID.String(),
		"scope":   created.Scope,
	})
}

func (r *Router) deleteTenantNodeBlacklistRule(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, scope string, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	if err := r.store.DeleteTenantNodeBlacklistRuleByID(tenantID, node.ID, scope, ruleID); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to delete blacklist rule: "+err.Error(), nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "blacklist", "delete", map[string]interface{}{
		"id":      ruleIDStr,
		"node_id": node.ID.String(),
		"scope":   scope,
		"status":  "deleted",
	}, "Blacklist rule deleted successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"rule_id": ruleIDStr,
		"scope":   scope,
	})
}

// QoS Handlers

func (r *Router) handleTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	requiredPermission := middleware.PermQosRead
	if req.Method != http.MethodGet {
		requiredPermission = middleware.PermQosWrite
	}
	if !r.authorizeTenantPermission(w, req, tenantID, requiredPermission) {
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
		r.createTenantNodeQoS(w, req, tenantID, node, category)
	case http.MethodPut:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for update", nil)
			return
		}
		r.updateTenantNodeQoS(w, req, tenantID, node, category, ruleIDStr)
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

func (r *Router) createTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, category string) {
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
	if body.BandwidthMbps <= 0 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "bandwidth_mbps must be greater than 0", nil)
		return
	}

	rule := &controllerstorage.QoSRuleRecord{
		TenantID:      tenantID,
		NodeID:        node.ID,
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

	r.writePolicyMutationSuccess(w, node, "qos", "create", map[string]interface{}{
		"id":             created.ID.String(),
		"node_id":        node.ID.String(),
		"category":       created.Category,
		"src_cidr":       created.SrcCIDR,
		"dst_cidr":       created.DstCIDR,
		"src_port":       created.SrcPort,
		"dst_port":       created.DstPort,
		"protocol":       created.Protocol,
		"bandwidth_mbps": created.BandwidthMbps,
		"enabled":        created.Enabled,
		"description":    created.Description,
		"created_at":     created.CreatedAt,
		"updated_at":     created.UpdatedAt,
	}, "QoS rule created successfully", map[string]interface{}{
		"node_id":  node.ID.String(),
		"rule_id":  created.ID.String(),
		"category": created.Category,
	})
}

func (r *Router) updateTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, category, ruleIDStr string) {
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
	if body.BandwidthMbps <= 0 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "bandwidth_mbps must be greater than 0", nil)
		return
	}

	rule := &controllerstorage.QoSRuleRecord{
		TenantID:      tenantID,
		NodeID:        node.ID,
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

	updated, err := r.store.UpdateTenantNodeQoSRule(tenantID, node.ID, ruleID, category, rule)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to update QoS rule: "+err.Error(), nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "qos", "update", map[string]interface{}{
		"id":             ruleID.String(),
		"node_id":        node.ID.String(),
		"category":       updated.Category,
		"src_cidr":       updated.SrcCIDR,
		"dst_cidr":       updated.DstCIDR,
		"src_port":       updated.SrcPort,
		"dst_port":       updated.DstPort,
		"protocol":       updated.Protocol,
		"bandwidth_mbps": updated.BandwidthMbps,
		"enabled":        updated.Enabled,
		"description":    updated.Description,
		"updated_at":     updated.UpdatedAt,
	}, "QoS rule updated successfully", map[string]interface{}{
		"node_id":  node.ID.String(),
		"rule_id":  ruleID.String(),
		"category": updated.Category,
	})
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

	r.writePolicyMutationSuccess(w, node, "qos", "delete", map[string]interface{}{
		"id":       ruleIDStr,
		"node_id":  node.ID.String(),
		"category": category,
		"status":   "deleted",
	}, "QoS rule deleted successfully", map[string]interface{}{
		"node_id":  node.ID.String(),
		"rule_id":  ruleIDStr,
		"category": category,
	})
}
