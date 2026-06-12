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

func validatePolicyByteField(field string, value int) error {
	if value < 0 || value > 255 {
		return fmt.Errorf("%s must be between 0 and 255", field)
	}
	return nil
}

func validatePolicyDirectionField(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "ingress", "in", "egress", "out", "both", "all":
		return nil
	default:
		return fmt.Errorf("direction must be ingress, egress, or both")
	}
}

func validateQoSModeField(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "policing":
		return nil
	case "shaping":
		return fmt.Errorf("mode shaping is not supported yet; use policing")
	default:
		return fmt.Errorf("mode must be policing")
	}
}

func validateQoSMatchFields(protocol, srcPort, dstPort int) error {
	if protocol != 0 {
		return fmt.Errorf("qos protocol matching is not supported yet")
	}
	if srcPort != 0 || dstPort != 0 {
		return fmt.Errorf("qos port matching is not supported yet")
	}
	return nil
}

func boolValueOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

var errACLRuntimeKeyConflict = errors.New("acl runtime key conflict")

type aclRuntimeKey struct {
	src       string
	dst       string
	proto     int
	direction string
}

func validateACLRuntimeKeyAvailable(existing []*controllerstorage.ACLRuleRecord, candidate *controllerstorage.ACLRuleRecord, candidateID uuid.UUID) error {
	if candidate == nil || !candidate.Enabled {
		return nil
	}

	candidateKeys := aclRuntimeKeysForRule(candidate)
	for _, existingRule := range existing {
		if existingRule == nil || !existingRule.Enabled {
			continue
		}
		if candidateID != uuid.Nil && existingRule.ID == candidateID {
			continue
		}
		existingKeys := aclRuntimeKeysForRule(existingRule)
		for _, existingKey := range existingKeys {
			for _, candidateKey := range candidateKeys {
				if existingKey == candidateKey {
					return fmt.Errorf("%w: rule %s conflicts on %s %s -> %s proto %d",
						errACLRuntimeKeyConflict,
						existingRule.ID.String(),
						candidateKey.direction,
						candidateKey.src,
						candidateKey.dst,
						candidateKey.proto,
					)
				}
			}
		}
	}
	return nil
}

func aclRuntimeKeysForRule(rule *controllerstorage.ACLRuleRecord) []aclRuntimeKey {
	directions := aclRuntimeDirections(rule.Direction)
	protocols := aclRuntimeProtocols(rule.Protocol, rule.Ports, rule.DstPort)
	keys := make([]aclRuntimeKey, 0, len(directions)*len(protocols))
	for _, direction := range directions {
		for _, proto := range protocols {
			keys = append(keys, aclRuntimeKey{
				src:       normalizeACLRuntimeCIDR(rule.SrcCIDR),
				dst:       normalizeACLRuntimeCIDR(rule.DstCIDR),
				proto:     proto,
				direction: direction,
			})
		}
	}
	return keys
}

func aclRuntimeDirections(direction string) []string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "egress", "out":
		return []string{"egress"}
	case "both", "all":
		return []string{"ingress", "egress"}
	default:
		return []string{"ingress"}
	}
}

func aclRuntimeProtocols(protocol int, ports string, dstPort int) []int {
	if protocol != 0 {
		return []int{protocol}
	}
	if strings.TrimSpace(ports) != "" || dstPort != 0 {
		return []int{6, 17}
	}
	return []int{0}
}

func normalizeACLRuntimeCIDR(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "0", "any", "0.0.0.0/0", "::/0":
		return "any"
	default:
		return normalized
	}
}

func (r *Router) ensureACLRuntimeKeyAvailable(tenantID, nodeID uuid.UUID, candidateID uuid.UUID, candidate *controllerstorage.ACLRuleRecord) error {
	if candidate == nil || !candidate.Enabled {
		return nil
	}
	existing, err := r.store.ListTenantNodeACLRules(tenantID, nodeID)
	if err != nil {
		return fmt.Errorf("load ACL rules for runtime conflict check: %w", err)
	}
	return validateACLRuntimeKeyAvailable(existing, candidate, candidateID)
}

func writePolicyValidationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
	return true
}

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
	if err := r.attachACLRuleStats(tenantID, nodeID, rules); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load ACL stats: "+err.Error(), nil)
		return
	}
	if err := r.attachACLRuleDeliveryStatus(tenantID, nodeID, rules); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load ACL delivery status: "+err.Error(), nil)
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
		Direction   string `json:"direction"`
		Ports       string `json:"ports"`
		Priority    int    `json:"priority"`
		Enabled     *bool  `json:"enabled"`
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
	if writePolicyValidationError(w, validatePolicyByteField("protocol", body.Protocol)) {
		return
	}
	if writePolicyValidationError(w, validatePolicyDirectionField(body.Direction)) {
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
		Direction:   body.Direction,
		Ports:       body.Ports,
		Priority:    body.Priority,
		Enabled:     boolValueOrDefault(body.Enabled, true),
		Description: body.Description,
	}

	if err := r.ensureACLRuntimeKeyAvailable(tenantID, node.ID, uuid.Nil, rule); err != nil {
		if errors.Is(err, errACLRuntimeKeyConflict) {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, err.Error(), nil)
		return
	}

	metadata := map[string]interface{}{
		"node_id": node.ID.String(),
	}
	r.writeTransactionalPolicyMutationSuccess(w, node, "acl", "create", "ACL rule created successfully", metadata, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		created, err := tx.CreateTenantNodeACLRule(rule)
		if err != nil {
			return nil, fmt.Errorf("failed to create ACL rule: %w", err)
		}
		metadata["rule_id"] = created.ID.String()
		metadata["description"] = created.Description
		return map[string]interface{}{
			"id":          created.ID.String(),
			"node_id":     node.ID.String(),
			"name":        created.Name,
			"action":      created.Action,
			"src_cidr":    created.SrcCIDR,
			"dst_cidr":    created.DstCIDR,
			"dst_port":    created.DstPort,
			"protocol":    created.Protocol,
			"direction":   created.Direction,
			"ports":       created.Ports,
			"priority":    created.Priority,
			"enabled":     created.Enabled,
			"description": created.Description,
			"created_at":  created.CreatedAt,
			"updated_at":  created.UpdatedAt,
		}, nil
	})
}

func (r *Router) updateTenantNodeACL(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	var body struct {
		Name        *string `json:"name"`
		Action      *string `json:"action"`
		SrcCIDR     *string `json:"src_cidr"`
		DstCIDR     *string `json:"dst_cidr"`
		SrcNet      *string `json:"src_net"`
		DstNet      *string `json:"dst_net"`
		DstPort     *int    `json:"dst_port"`
		MaxPort     *int    `json:"max_port"`
		Protocol    *int    `json:"protocol"`
		Direction   *string `json:"direction"`
		Ports       *string `json:"ports"`
		Priority    *int    `json:"priority"`
		Description *string `json:"description"`
		Enabled     *bool   `json:"enabled"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	existing, err := r.store.GetTenantNodeACLRule(tenantID, node.ID, ruleID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apibase.WriteError(w, http.StatusNotFound, apibase.CodeNotFound, "ACL rule not found", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load ACL rule: "+err.Error(), nil)
		return
	}

	rule := *existing
	if body.Name != nil {
		rule.Name = *body.Name
	}
	if body.Action != nil {
		if *body.Action != "allow" && *body.Action != "deny" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Action must be 'allow' or 'deny'", nil)
			return
		}
		rule.Action = *body.Action
	}
	if body.SrcCIDR != nil {
		rule.SrcCIDR = *body.SrcCIDR
	} else if body.SrcNet != nil {
		rule.SrcCIDR = *body.SrcNet
	}
	if body.DstCIDR != nil {
		rule.DstCIDR = *body.DstCIDR
	} else if body.DstNet != nil {
		rule.DstCIDR = *body.DstNet
	}
	if body.DstPort != nil {
		rule.DstPort = *body.DstPort
	} else if body.MaxPort != nil {
		rule.DstPort = *body.MaxPort
	}
	if body.Protocol != nil {
		rule.Protocol = *body.Protocol
	}
	if body.Direction != nil {
		rule.Direction = *body.Direction
	}
	if body.Ports != nil {
		rule.Ports = *body.Ports
	}
	if body.Priority != nil {
		rule.Priority = *body.Priority
	}
	if body.Description != nil {
		rule.Description = *body.Description
	}
	if body.Enabled != nil {
		rule.Enabled = *body.Enabled
	}
	if writePolicyValidationError(w, validatePolicyByteField("protocol", rule.Protocol)) {
		return
	}
	if writePolicyValidationError(w, validatePolicyDirectionField(rule.Direction)) {
		return
	}

	if err := r.ensureACLRuntimeKeyAvailable(tenantID, node.ID, ruleID, &rule); err != nil {
		if errors.Is(err, errACLRuntimeKeyConflict) {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, err.Error(), nil)
		return
	}

	metadata := map[string]interface{}{
		"node_id":     node.ID.String(),
		"rule_id":     ruleID.String(),
		"description": rule.Description,
	}
	r.writeTransactionalPolicyMutationSuccess(w, node, "acl", "update", "ACL rule updated successfully", metadata, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		updated, err := tx.UpdateTenantNodeACLRule(tenantID, node.ID, ruleID, &rule)
		if err != nil {
			return nil, fmt.Errorf("failed to update ACL rule: %w", err)
		}
		metadata["description"] = updated.Description
		return map[string]interface{}{
			"id":          ruleID.String(),
			"node_id":     node.ID.String(),
			"name":        updated.Name,
			"action":      updated.Action,
			"src_cidr":    updated.SrcCIDR,
			"dst_cidr":    updated.DstCIDR,
			"dst_port":    updated.DstPort,
			"protocol":    updated.Protocol,
			"direction":   updated.Direction,
			"ports":       updated.Ports,
			"priority":    updated.Priority,
			"enabled":     updated.Enabled,
			"description": updated.Description,
			"updated_at":  updated.UpdatedAt,
		}, nil
	})
}

func (r *Router) deleteTenantNodeACL(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	r.writeTransactionalPolicyMutationSuccess(w, node, "acl", "delete", "ACL rule deleted successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"rule_id": ruleIDStr,
	}, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		if err := tx.DeleteTenantNodeACLRuleByID(tenantID, node.ID, ruleID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("ACL rule not found: %w", err)
			}
			return nil, fmt.Errorf("failed to delete ACL rule: %w", err)
		}
		return map[string]interface{}{
			"id":      ruleIDStr,
			"node_id": node.ID.String(),
			"status":  "deleted",
		}, nil
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

	metadata := map[string]interface{}{
		"node_id": node.ID.String(),
		"scope":   scope,
	}
	r.writeTransactionalPolicyMutationSuccess(w, node, "blacklist", "create", "Blacklist rule created successfully", metadata, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		created, err := tx.CreateTenantNodeBlacklistRule(rule)
		if err != nil {
			return nil, fmt.Errorf("failed to create blacklist rule: %w", err)
		}
		metadata["rule_id"] = created.ID.String()
		metadata["scope"] = created.Scope
		return map[string]interface{}{
			"id":          created.ID.String(),
			"node_id":     node.ID.String(),
			"scope":       created.Scope,
			"cidr":        created.CIDR,
			"port":        created.Port,
			"enabled":     created.Enabled,
			"description": created.Description,
			"created_at":  created.CreatedAt,
			"updated_at":  created.UpdatedAt,
		}, nil
	})
}

func (r *Router) deleteTenantNodeBlacklistRule(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, scope string, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	r.writeTransactionalPolicyMutationSuccess(w, node, "blacklist", "delete", "Blacklist rule deleted successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"rule_id": ruleIDStr,
		"scope":   scope,
	}, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		if err := tx.DeleteTenantNodeBlacklistRuleByID(tenantID, node.ID, scope, ruleID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("blacklist rule not found: %w", err)
			}
			return nil, fmt.Errorf("failed to delete blacklist rule: %w", err)
		}
		return map[string]interface{}{
			"id":      ruleIDStr,
			"node_id": node.ID.String(),
			"scope":   scope,
			"status":  "deleted",
		}, nil
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

	if len(parts) > 8 {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown QoS endpoint", nil)
		return
	}

	ruleIDStr := ""
	if len(parts) == 8 {
		ruleIDStr = parts[7]
		if _, err := uuid.Parse(ruleIDStr); err != nil {
			apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown QoS endpoint", nil)
			return
		}
	}

	switch req.Method {
	case http.MethodGet:
		if ruleIDStr != "" {
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
			return
		}
		r.listTenantNodeQoS(w, tenantID, node.ID)
	case http.MethodPost:
		if ruleIDStr != "" {
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
			return
		}
		r.createTenantNodeQoS(w, req, tenantID, node)
	case http.MethodPut:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for update", nil)
			return
		}
		r.updateTenantNodeQoS(w, req, tenantID, node, ruleIDStr)
	case http.MethodDelete:
		if ruleIDStr == "" {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Rule ID is required for deletion", nil)
			return
		}
		r.deleteTenantNodeQoS(w, tenantID, node, ruleIDStr)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) listTenantNodeQoS(w http.ResponseWriter, tenantID, nodeID uuid.UUID) {
	rules, err := r.store.ListTenantNodeQoSRules(tenantID, nodeID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list QoS rules: "+err.Error(), nil)
		return
	}
	if err := r.attachQoSRuleStats(tenantID, nodeID, rules); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load QoS stats: "+err.Error(), nil)
		return
	}
	if err := r.attachQoSRuleDeliveryStatus(tenantID, nodeID, rules); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load QoS delivery status: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, rules, fmt.Sprintf("%d QoS rules retrieved", len(rules)))
}

func (r *Router) attachACLRuleStats(tenantID, nodeID uuid.UUID, rules []*controllerstorage.ACLRuleRecord) error {
	policyStats, err := r.store.GetNodePolicyStats(tenantID, nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if policyStats == nil {
		return nil
	}
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if stats := policyStats.ACLRuleStats(rule.ID); stats != nil {
			rule.Stats = stats
		}
	}
	return nil
}

func (r *Router) attachQoSRuleStats(tenantID, nodeID uuid.UUID, rules []*controllerstorage.QoSRuleRecord) error {
	policyStats, err := r.store.GetNodePolicyStats(tenantID, nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if policyStats == nil {
		return nil
	}
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if stats := policyStats.QoSRuleStats(rule.ID); stats != nil {
			rule.Stats = stats
		}
	}
	return nil
}

func (r *Router) attachACLRuleDeliveryStatus(tenantID, nodeID uuid.UUID, rules []*controllerstorage.ACLRuleRecord) error {
	statusByRef, err := r.loadPolicyDeliveryStatusByRef(tenantID, nodeID, "acl")
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		fields := statusByRef[rule.ID.String()]
		applyPolicyDeliveryFields(&rule.PolicyStatus, &rule.PendingCmds, &rule.LastDelivery, &rule.DeliveryHistory, &rule.LastDeliveryError, fields)
	}
	return nil
}

func (r *Router) attachQoSRuleDeliveryStatus(tenantID, nodeID uuid.UUID, rules []*controllerstorage.QoSRuleRecord) error {
	statusByRef, err := r.loadPolicyDeliveryStatusByRef(tenantID, nodeID, "qos")
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		fields := statusByRef[rule.ID.String()]
		applyPolicyDeliveryFields(&rule.PolicyStatus, &rule.PendingCmds, &rule.LastDelivery, &rule.DeliveryHistory, &rule.LastDeliveryError, fields)
	}
	return nil
}

type policyDeliveryFields struct {
	status    string
	pending   int
	last      map[string]interface{}
	history   []map[string]interface{}
	lastError string
}

func (r *Router) loadPolicyDeliveryStatusByRef(tenantID, nodeID uuid.UUID, domain string) (map[string]policyDeliveryFields, error) {
	const limit = 100
	deliveries, err := r.store.ListPolicyDeliveriesByNodeAndDomain(tenantID, nodeID, domain, limit)
	if err != nil {
		return nil, err
	}

	historyByRef := make(map[string][]*controllerstorage.PolicyDelivery, len(deliveries))
	for _, delivery := range deliveries {
		if delivery == nil {
			continue
		}
		ref := strings.TrimSpace(delivery.PolicyRef)
		if ref == "" {
			continue
		}
		historyByRef[ref] = append(historyByRef[ref], delivery)
	}

	result := make(map[string]policyDeliveryFields, len(historyByRef))
	for ref, history := range historyByRef {
		if len(history) == 0 {
			continue
		}
		serializedHistory := make([]map[string]interface{}, 0, len(history))
		pendingCount := 0
		for _, delivery := range history {
			serializedHistory = append(serializedHistory, policyDeliveryToMap(delivery))
			pendingCount += pendingCountForCommandStatus(delivery.CommandStatus)
		}
		result[ref] = policyDeliveryFields{
			status:    mapCommandStatusToPolicyStatus(history[0].CommandStatus),
			pending:   pendingCount,
			last:      serializedHistory[0],
			history:   serializedHistory,
			lastError: history[0].LastError,
		}
	}
	return result, nil
}

func applyPolicyDeliveryFields(
	status *string,
	pending *int,
	last *map[string]interface{},
	history *[]map[string]interface{},
	lastError *string,
	fields policyDeliveryFields,
) {
	if fields.status == "" {
		*status = "idle"
		*pending = 0
		*history = []map[string]interface{}{}
		return
	}
	*status = fields.status
	*pending = fields.pending
	*last = fields.last
	*history = fields.history
	*lastError = fields.lastError
}

func (r *Router) createTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node) {
	var body struct {
		SrcCIDR       string `json:"src_cidr"`
		DstCIDR       string `json:"dst_cidr"`
		SrcPort       int    `json:"src_port"`
		DstPort       int    `json:"dst_port"`
		Protocol      int    `json:"protocol"`
		BandwidthMbps int    `json:"bandwidth_mbps"`
		Direction     string `json:"direction"`
		RateBps       uint64 `json:"rate_bps"`
		BurstBytes    uint64 `json:"burst_bytes"`
		Priority      int    `json:"priority"`
		Mode          string `json:"mode"`
		Enabled       *bool  `json:"enabled"`
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
	if writePolicyValidationError(w, validatePolicyByteField("protocol", body.Protocol)) {
		return
	}
	if writePolicyValidationError(w, validateQoSMatchFields(body.Protocol, body.SrcPort, body.DstPort)) {
		return
	}
	if writePolicyValidationError(w, validatePolicyByteField("priority", body.Priority)) {
		return
	}
	if writePolicyValidationError(w, validatePolicyDirectionField(body.Direction)) {
		return
	}
	if writePolicyValidationError(w, validateQoSModeField(body.Mode)) {
		return
	}

	rule := &controllerstorage.QoSRuleRecord{
		TenantID:      tenantID,
		NodeID:        node.ID,
		SrcCIDR:       body.SrcCIDR,
		DstCIDR:       body.DstCIDR,
		SrcPort:       body.SrcPort,
		DstPort:       body.DstPort,
		Protocol:      body.Protocol,
		BandwidthMbps: body.BandwidthMbps,
		Direction:     body.Direction,
		RateBps:       body.RateBps,
		BurstBytes:    body.BurstBytes,
		Priority:      body.Priority,
		Mode:          body.Mode,
		Enabled:       boolValueOrDefault(body.Enabled, true),
		Description:   body.Description,
	}

	metadata := map[string]interface{}{
		"node_id": node.ID.String(),
	}
	r.writeTransactionalPolicyMutationSuccess(w, node, "qos", "create", "QoS rule created successfully", metadata, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		created, err := tx.CreateTenantNodeQoSRule(rule)
		if err != nil {
			return nil, fmt.Errorf("failed to create QoS rule: %w", err)
		}
		metadata["rule_id"] = created.ID.String()
		return map[string]interface{}{
			"id":             created.ID.String(),
			"node_id":        node.ID.String(),
			"src_cidr":       created.SrcCIDR,
			"dst_cidr":       created.DstCIDR,
			"src_port":       created.SrcPort,
			"dst_port":       created.DstPort,
			"protocol":       created.Protocol,
			"bandwidth_mbps": created.BandwidthMbps,
			"direction":      created.Direction,
			"rate_bps":       created.RateBps,
			"burst_bytes":    created.BurstBytes,
			"priority":       created.Priority,
			"mode":           created.Mode,
			"enabled":        created.Enabled,
			"description":    created.Description,
			"created_at":     created.CreatedAt,
			"updated_at":     created.UpdatedAt,
		}, nil
	})
}

func (r *Router) updateTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	var body struct {
		SrcCIDR       *string `json:"src_cidr"`
		DstCIDR       *string `json:"dst_cidr"`
		SrcPort       *int    `json:"src_port"`
		DstPort       *int    `json:"dst_port"`
		Protocol      *int    `json:"protocol"`
		BandwidthMbps *int    `json:"bandwidth_mbps"`
		Direction     *string `json:"direction"`
		RateBps       *uint64 `json:"rate_bps"`
		BurstBytes    *uint64 `json:"burst_bytes"`
		Priority      *int    `json:"priority"`
		Mode          *string `json:"mode"`
		Description   *string `json:"description"`
		Enabled       *bool   `json:"enabled"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	existing, err := r.store.GetTenantNodeQoSRule(tenantID, node.ID, ruleID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apibase.WriteError(w, http.StatusNotFound, apibase.CodeNotFound, "QoS rule not found", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to load QoS rule: "+err.Error(), nil)
		return
	}

	rule := *existing
	if body.SrcCIDR != nil {
		rule.SrcCIDR = *body.SrcCIDR
	}
	if body.DstCIDR != nil {
		rule.DstCIDR = *body.DstCIDR
	}
	if body.SrcPort != nil {
		rule.SrcPort = *body.SrcPort
	}
	if body.DstPort != nil {
		rule.DstPort = *body.DstPort
	}
	if body.Protocol != nil {
		rule.Protocol = *body.Protocol
	}
	if body.BandwidthMbps != nil {
		if *body.BandwidthMbps <= 0 {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "bandwidth_mbps must be greater than 0", nil)
			return
		}
		rule.BandwidthMbps = *body.BandwidthMbps
	}
	if body.Direction != nil {
		rule.Direction = *body.Direction
	}
	if body.RateBps != nil {
		rule.RateBps = *body.RateBps
	}
	if body.BurstBytes != nil {
		rule.BurstBytes = *body.BurstBytes
	}
	if body.Priority != nil {
		rule.Priority = *body.Priority
	}
	if body.Mode != nil {
		rule.Mode = *body.Mode
	}
	if body.Description != nil {
		rule.Description = *body.Description
	}
	if body.Enabled != nil {
		rule.Enabled = *body.Enabled
	}
	if writePolicyValidationError(w, validatePolicyByteField("protocol", rule.Protocol)) {
		return
	}
	if writePolicyValidationError(w, validateQoSMatchFields(rule.Protocol, rule.SrcPort, rule.DstPort)) {
		return
	}
	if writePolicyValidationError(w, validatePolicyByteField("priority", rule.Priority)) {
		return
	}
	if writePolicyValidationError(w, validatePolicyDirectionField(rule.Direction)) {
		return
	}
	if writePolicyValidationError(w, validateQoSModeField(rule.Mode)) {
		return
	}

	r.writeTransactionalPolicyMutationSuccess(w, node, "qos", "update", "QoS rule updated successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"rule_id": ruleID.String(),
	}, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		updated, err := tx.UpdateTenantNodeQoSRule(tenantID, node.ID, ruleID, &rule)
		if err != nil {
			return nil, fmt.Errorf("failed to update QoS rule: %w", err)
		}
		return map[string]interface{}{
			"id":             ruleID.String(),
			"node_id":        node.ID.String(),
			"src_cidr":       updated.SrcCIDR,
			"dst_cidr":       updated.DstCIDR,
			"src_port":       updated.SrcPort,
			"dst_port":       updated.DstPort,
			"protocol":       updated.Protocol,
			"bandwidth_mbps": updated.BandwidthMbps,
			"direction":      updated.Direction,
			"rate_bps":       updated.RateBps,
			"burst_bytes":    updated.BurstBytes,
			"priority":       updated.Priority,
			"mode":           updated.Mode,
			"enabled":        updated.Enabled,
			"description":    updated.Description,
			"updated_at":     updated.UpdatedAt,
		}, nil
	})
}

func (r *Router) deleteTenantNodeQoS(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	r.writeTransactionalPolicyMutationSuccess(w, node, "qos", "delete", "QoS rule deleted successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"rule_id": ruleIDStr,
	}, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		if err := tx.DeleteTenantNodeQoSRule(tenantID, node.ID, ruleID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("QoS rule not found: %w", err)
			}
			return nil, fmt.Errorf("failed to delete QoS rule: %w", err)
		}
		return map[string]interface{}{
			"id":      ruleIDStr,
			"node_id": node.ID.String(),
			"status":  "deleted",
		}, nil
	})
}
