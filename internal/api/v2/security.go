package v2

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
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

func validateACLPriorityField(value int) error {
	if value < 1 || value > 10000 {
		return fmt.Errorf("priority must be between 1 and 10000")
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
	_, err := normalizeQoSModeField(value, "auto")
	return err
}

func normalizeQoSModeField(value, defaultMode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return defaultMode, nil
	case "auto", "policing", "shaping":
		return strings.ToLower(strings.TrimSpace(value)), nil
	default:
		return "", fmt.Errorf("mode must be auto, policing, or shaping")
	}
}

func validateQoSMatchFields(protocol, srcPort, dstPort *int) error {
	if protocol != nil {
		return fmt.Errorf("qos protocol matching is not supported yet")
	}
	if srcPort != nil || dstPort != nil {
		return fmt.Errorf("qos port matching is not supported yet")
	}
	return nil
}

func validateACLPortFields(protocol int, ports string, dstPort int) error {
	if dstPort < 0 || dstPort > 65535 {
		return fmt.Errorf("dst_port must be between 0 and 65535")
	}

	if !aclHasRuntimePortFilter(ports, dstPort) {
		return nil
	}
	switch protocol {
	case 0, 6, 17:
	default:
		return fmt.Errorf("ACL port filters require any, tcp, or udp protocol")
	}

	return validateACLPortsSyntax(ports)
}

func aclHasRuntimePortFilter(ports string, dstPort int) bool {
	normalized := strings.TrimSpace(ports)
	if normalized != "" && !strings.EqualFold(normalized, "all") {
		return true
	}
	return dstPort > 0
}

func validateACLPortsSyntax(ports string) error {
	trimmed := strings.TrimSpace(ports)
	if trimmed == "" || strings.EqualFold(trimmed, "all") {
		return nil
	}

	entries := 0
	for _, rawPart := range strings.Split(trimmed, ",") {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}
		entries++

		pieces := strings.Split(part, ":")
		if len(pieces) > 2 {
			return fmt.Errorf("invalid ACL port filter %q", part)
		}
		if len(pieces) == 2 {
			action, err := strconv.Atoi(strings.TrimSpace(pieces[1]))
			if err != nil || action < 0 || action > 1 {
				return fmt.Errorf("invalid ACL port action %q", pieces[1])
			}
		}

		rangePart := strings.TrimSpace(pieces[0])
		if rangePart == "" {
			return fmt.Errorf("invalid ACL port filter %q", part)
		}
		start, end, err := parseACLPortRange(rangePart)
		if err != nil {
			return err
		}
		if start > end {
			return fmt.Errorf("invalid ACL port range %d-%d", start, end)
		}
	}
	if entries == 0 {
		return fmt.Errorf("ACL ports must include at least one port")
	}
	return nil
}

func parseACLPortRange(value string) (int, int, error) {
	if strings.Contains(value, "-") {
		parts := strings.Split(value, "-")
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("invalid ACL port range %q", value)
		}
		start, err := parseACLPort(parts[0])
		if err != nil {
			return 0, 0, err
		}
		end, err := parseACLPort(parts[1])
		if err != nil {
			return 0, 0, err
		}
		return start, end, nil
	}
	port, err := parseACLPort(value)
	return port, port, err
}

func parseACLPort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 0 || port > 65535 {
		return 0, fmt.Errorf("ACL port must be between 0 and 65535")
	}
	return port, nil
}

func boolValueOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func intValueOrDefault(value *int, defaultValue int) int {
	if value == nil {
		return defaultValue
	}
	return *value
}

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func aclRuleResponse(rule *controllerstorage.ACLRuleRecord, nodeID uuid.UUID) map[string]interface{} {
	return map[string]interface{}{
		"id":           rule.ID.String(),
		"node_id":      nodeID.String(),
		"name":         rule.Name,
		"action":       rule.Action,
		"src_group_id": nullableUUIDString(rule.SrcGroupID),
		"dst_group_id": nullableUUIDString(rule.DstGroupID),
		"src_cidr":     rule.SrcCIDR,
		"dst_cidr":     rule.DstCIDR,
		"dst_port":     rule.DstPort,
		"protocol":     rule.Protocol,
		"direction":    rule.Direction,
		"ports":        rule.Ports,
		"priority":     rule.Priority,
		"enabled":      rule.Enabled,
		"description":  rule.Description,
		"created_at":   rule.CreatedAt,
		"updated_at":   rule.UpdatedAt,
	}
}

func qosRuleResponse(rule *controllerstorage.QoSRuleRecord, nodeID uuid.UUID) map[string]interface{} {
	return map[string]interface{}{
		"id":             rule.ID.String(),
		"node_id":        nodeID.String(),
		"group_id":       nullableUUIDString(rule.GroupID),
		"src_cidr":       rule.SrcCIDR,
		"dst_cidr":       rule.DstCIDR,
		"src_port":       rule.SrcPort,
		"dst_port":       rule.DstPort,
		"protocol":       rule.Protocol,
		"bandwidth_mbps": rule.BandwidthMbps,
		"direction":      rule.Direction,
		"rate_bps":       rule.RateBps,
		"burst_bytes":    rule.BurstBytes,
		"priority":       rule.Priority,
		"mode":           qosModeOrDefault(rule.Mode),
		"enabled":        rule.Enabled,
		"description":    rule.Description,
		"created_at":     rule.CreatedAt,
		"updated_at":     rule.UpdatedAt,
	}
}

func qosModeOrDefault(mode string) string {
	normalized, err := normalizeQoSModeField(mode, "auto")
	if err != nil {
		return "auto"
	}
	return normalized
}

func nullableUUIDString(value uuid.NullUUID) interface{} {
	if value.Valid {
		return value.UUID.String()
	}
	return nil
}

func qosDirectGroupCIDR(group string, srcCIDR string, dstCIDR string, direction string) string {
	group = normalizePolicyCIDRInput(group)
	srcCIDR = normalizePolicyCIDRInput(srcCIDR)
	dstCIDR = normalizePolicyCIDRInput(dstCIDR)
	if strings.TrimSpace(group) != "" {
		return group
	}
	if strings.EqualFold(strings.TrimSpace(direction), "ingress") && strings.TrimSpace(srcCIDR) != "" {
		return srcCIDR
	}
	if strings.TrimSpace(dstCIDR) != "" {
		return dstCIDR
	}
	return srcCIDR
}

func normalizePolicyCIDRInput(value string) string {
	trimmed := strings.TrimSpace(value)
	if policyInputCIDRIsAny(trimmed) {
		return ""
	}
	return trimmed
}

func policyInputCIDRIsAny(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "any", "0", "0.0.0.0/0", "::/0":
		return true
	default:
		return false
	}
}

func hydrateACLRuleGroups(tx *controllerstorage.PolicyMutationTx, tenantID uuid.UUID, rules []*controllerstorage.ACLRuleRecord) error {
	cache := map[uuid.UUID]*controllerstorage.IPGroupRecord{}
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if rule.SrcGroupID.Valid {
			group, err := loadPolicyGroupCached(tx, tenantID, rule.SrcGroupID.UUID, cache)
			if err != nil {
				return fmt.Errorf("load ACL source IP group: %w", err)
			}
			rule.SrcGroup = group
		}
		if rule.DstGroupID.Valid {
			group, err := loadPolicyGroupCached(tx, tenantID, rule.DstGroupID.UUID, cache)
			if err != nil {
				return fmt.Errorf("load ACL destination IP group: %w", err)
			}
			rule.DstGroup = group
		}
	}
	return nil
}

func hydrateQoSRuleGroups(tx *controllerstorage.PolicyMutationTx, tenantID uuid.UUID, rules []*controllerstorage.QoSRuleRecord) error {
	cache := map[uuid.UUID]*controllerstorage.IPGroupRecord{}
	for _, rule := range rules {
		if rule == nil || !rule.GroupID.Valid {
			continue
		}
		group, err := loadPolicyGroupCached(tx, tenantID, rule.GroupID.UUID, cache)
		if err != nil {
			return fmt.Errorf("load QoS IP group: %w", err)
		}
		rule.Group = group
	}
	return nil
}

func loadPolicyGroupCached(tx *controllerstorage.PolicyMutationTx, tenantID uuid.UUID, groupID uuid.UUID, cache map[uuid.UUID]*controllerstorage.IPGroupRecord) (*controllerstorage.IPGroupRecord, error) {
	if group, ok := cache[groupID]; ok {
		return group, nil
	}
	group, err := tx.GetIPGroup(tenantID, groupID)
	if err != nil {
		return nil, err
	}
	cache[groupID] = group
	return group, nil
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
	if aclHasRuntimePortFilter(ports, dstPort) {
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
		if err := r.failTimedOutNodeCommands(node); err != nil {
			apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to refresh timed out commands: "+err.Error(), nil)
			return
		}
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
		Name        string     `json:"name"`
		Action      string     `json:"action"`
		SrcGroupID  *uuid.UUID `json:"src_group_id"`
		DstGroupID  *uuid.UUID `json:"dst_group_id"`
		SrcCIDR     string     `json:"src_cidr"`
		DstCIDR     string     `json:"dst_cidr"`
		SrcNet      string     `json:"src_net"` // 兼容前端
		DstNet      string     `json:"dst_net"` // 兼容前端
		DstPort     int        `json:"dst_port"`
		MaxPort     int        `json:"max_port"` // 兼容前端
		Protocol    int        `json:"protocol"`
		Direction   string     `json:"direction"`
		Ports       string     `json:"ports"`
		Priority    *int       `json:"priority"`
		Enabled     *bool      `json:"enabled"`
		Description string     `json:"description"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}
	if writeInactivePolicyMutationError(w, node) {
		return
	}

	// 字段合并对齐
	src := body.SrcCIDR
	if src == "" {
		src = body.SrcNet
	}
	src = normalizePolicyCIDRInput(src)
	dst := body.DstCIDR
	if dst == "" {
		dst = body.DstNet
	}
	dst = normalizePolicyCIDRInput(dst)
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
	if writePolicyValidationError(w, validateACLPortFields(body.Protocol, body.Ports, port)) {
		return
	}
	if writePolicyValidationError(w, validateACLPriorityField(intValueOrDefault(body.Priority, 100))) {
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
		Priority:    intValueOrDefault(body.Priority, 100),
		Enabled:     boolValueOrDefault(body.Enabled, true),
		Description: body.Description,
	}
	explicitSrcGroup := nullUUIDFromPtr(body.SrcGroupID)
	explicitDstGroup := nullUUIDFromPtr(body.DstGroupID)

	metadata := map[string]interface{}{
		"node_id": node.ID.String(),
	}
	r.writeTransactionalPolicyMutationSuccess(w, node, "acl", "create", "ACL rule created successfully", metadata, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		ruleForWrite := *rule
		ruleForValidation := *rule
		var err error
		ruleForWrite.SrcGroupID, err = tx.ResolvePolicyGroupRef(tenantID, explicitSrcGroup, rule.SrcCIDR)
		if err != nil {
			return nil, fmt.Errorf("resolve source IP group: %w", err)
		}
		if ruleForWrite.SrcGroupID.Valid {
			ruleForWrite.SrcCIDR = ""
			ruleForValidation.SrcGroupID = ruleForWrite.SrcGroupID
			if explicitSrcGroup.Valid {
				if ruleForValidation.SrcGroup, err = tx.GetIPGroup(tenantID, ruleForWrite.SrcGroupID.UUID); err != nil {
					return nil, fmt.Errorf("load source IP group: %w", err)
				}
			}
		}
		ruleForWrite.DstGroupID, err = tx.ResolvePolicyGroupRef(tenantID, explicitDstGroup, rule.DstCIDR)
		if err != nil {
			return nil, fmt.Errorf("resolve destination IP group: %w", err)
		}
		if ruleForWrite.DstGroupID.Valid {
			ruleForWrite.DstCIDR = ""
			ruleForValidation.DstGroupID = ruleForWrite.DstGroupID
			if explicitDstGroup.Valid {
				if ruleForValidation.DstGroup, err = tx.GetIPGroup(tenantID, ruleForWrite.DstGroupID.UUID); err != nil {
					return nil, fmt.Errorf("load destination IP group: %w", err)
				}
			}
		}
		existing, err := tx.ListTenantNodeACLRules(tenantID, node.ID)
		if err != nil {
			return nil, fmt.Errorf("load ACL rules for conflict check: %w", err)
		}
		if err := hydrateACLRuleGroups(tx, tenantID, existing); err != nil {
			return nil, err
		}
		if err := controllerstorage.DetectACLPolicyConflict(existing, &ruleForValidation, uuid.Nil); err != nil {
			return nil, err
		}

		created, err := tx.CreateTenantNodeACLRule(&ruleForWrite)
		if err != nil {
			return nil, fmt.Errorf("failed to create ACL rule: %w", err)
		}
		metadata["rule_id"] = created.ID.String()
		metadata["description"] = created.Description
		return aclRuleResponse(created, node.ID), nil
	})
}

func (r *Router) updateTenantNodeACL(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	var body struct {
		Name        *string    `json:"name"`
		Action      *string    `json:"action"`
		SrcGroupID  *uuid.UUID `json:"src_group_id"`
		DstGroupID  *uuid.UUID `json:"dst_group_id"`
		SrcCIDR     *string    `json:"src_cidr"`
		DstCIDR     *string    `json:"dst_cidr"`
		SrcNet      *string    `json:"src_net"`
		DstNet      *string    `json:"dst_net"`
		DstPort     *int       `json:"dst_port"`
		MaxPort     *int       `json:"max_port"`
		Protocol    *int       `json:"protocol"`
		Direction   *string    `json:"direction"`
		Ports       *string    `json:"ports"`
		Priority    *int       `json:"priority"`
		Description *string    `json:"description"`
		Enabled     *bool      `json:"enabled"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}
	if writeInactivePolicyMutationError(w, node) {
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
		rule.SrcCIDR = normalizePolicyCIDRInput(*body.SrcCIDR)
		rule.SrcGroupID = uuid.NullUUID{}
	} else if body.SrcNet != nil {
		rule.SrcCIDR = normalizePolicyCIDRInput(*body.SrcNet)
		rule.SrcGroupID = uuid.NullUUID{}
	} else if body.SrcGroupID != nil {
		rule.SrcCIDR = ""
		rule.SrcGroupID = nullUUIDFromPtr(body.SrcGroupID)
	}
	if body.DstCIDR != nil {
		rule.DstCIDR = normalizePolicyCIDRInput(*body.DstCIDR)
		rule.DstGroupID = uuid.NullUUID{}
	} else if body.DstNet != nil {
		rule.DstCIDR = normalizePolicyCIDRInput(*body.DstNet)
		rule.DstGroupID = uuid.NullUUID{}
	} else if body.DstGroupID != nil {
		rule.DstCIDR = ""
		rule.DstGroupID = nullUUIDFromPtr(body.DstGroupID)
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
	if writePolicyValidationError(w, validateACLPortFields(rule.Protocol, rule.Ports, rule.DstPort)) {
		return
	}
	if writePolicyValidationError(w, validateACLPriorityField(rule.Priority)) {
		return
	}

	metadata := map[string]interface{}{
		"node_id":     node.ID.String(),
		"rule_id":     ruleID.String(),
		"description": rule.Description,
	}
	r.writeTransactionalPolicyMutationSuccess(w, node, "acl", "update", "ACL rule updated successfully", metadata, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		ruleForWrite := rule
		ruleForValidation := rule
		var err error
		ruleForWrite.SrcGroupID, err = tx.ResolvePolicyGroupRef(tenantID, rule.SrcGroupID, rule.SrcCIDR)
		if err != nil {
			return nil, fmt.Errorf("resolve source IP group: %w", err)
		}
		if ruleForWrite.SrcGroupID.Valid {
			ruleForWrite.SrcCIDR = ""
			ruleForValidation.SrcGroupID = ruleForWrite.SrcGroupID
			if rule.SrcGroupID.Valid && strings.TrimSpace(rule.SrcCIDR) == "" {
				if ruleForValidation.SrcGroup, err = tx.GetIPGroup(tenantID, ruleForWrite.SrcGroupID.UUID); err != nil {
					return nil, fmt.Errorf("load source IP group: %w", err)
				}
			}
		}
		ruleForWrite.DstGroupID, err = tx.ResolvePolicyGroupRef(tenantID, rule.DstGroupID, rule.DstCIDR)
		if err != nil {
			return nil, fmt.Errorf("resolve destination IP group: %w", err)
		}
		if ruleForWrite.DstGroupID.Valid {
			ruleForWrite.DstCIDR = ""
			ruleForValidation.DstGroupID = ruleForWrite.DstGroupID
			if rule.DstGroupID.Valid && strings.TrimSpace(rule.DstCIDR) == "" {
				if ruleForValidation.DstGroup, err = tx.GetIPGroup(tenantID, ruleForWrite.DstGroupID.UUID); err != nil {
					return nil, fmt.Errorf("load destination IP group: %w", err)
				}
			}
		}
		existingRules, err := tx.ListTenantNodeACLRules(tenantID, node.ID)
		if err != nil {
			return nil, fmt.Errorf("load ACL rules for conflict check: %w", err)
		}
		if err := hydrateACLRuleGroups(tx, tenantID, existingRules); err != nil {
			return nil, err
		}
		if err := controllerstorage.DetectACLPolicyConflict(existingRules, &ruleForValidation, ruleID); err != nil {
			return nil, err
		}
		updated, err := tx.UpdateTenantNodeACLRule(tenantID, node.ID, ruleID, &ruleForWrite)
		if err != nil {
			return nil, fmt.Errorf("failed to update ACL rule: %w", err)
		}
		metadata["description"] = updated.Description
		return aclRuleResponse(updated, node.ID), nil
	})
}

func (r *Router) deleteTenantNodeACL(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}
	if writeInactivePolicyMutationError(w, node) {
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
	if writeInactivePolicyMutationError(w, node) {
		return
	}

	if scope != "ports" && body.CIDR == "" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "CIDR is required for src/dst scope", nil)
		return
	}
	if scope == "ports" && (body.Port < 1 || body.Port > 65535) {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Port must be in range 1..65535 for ports scope", nil)
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
	if writeInactivePolicyMutationError(w, node) {
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
		if err := r.failTimedOutNodeCommands(node); err != nil {
			apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to refresh timed out commands: "+err.Error(), nil)
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
	statusByRef, err := r.loadPolicyDeliveryStatusByRef(tenantID, nodeID, "acl", r.aclPolicyDeliveryErrorResolver(tenantID, nodeID, rules))
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
	statusByRef, err := r.loadPolicyDeliveryStatusByRef(tenantID, nodeID, "qos", r.qosPolicyDeliveryErrorResolver(tenantID, nodeID, rules))
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

var runtimeGroupCIDRConflictRes = []*regexp.Regexp{
	regexp.MustCompile(`CIDR\s+([^\s]+)\s+already belongs to runtime group\s+\d+`),
	regexp.MustCompile(`duplicate CIDR\s+([^\s]+)\s+in runtime groups\s+\S+\s+and\s+\S+`),
}

type policyCIDRUsage struct {
	policyRef  string
	policyName string
	domain     string
	cidr       string
}

func (r *Router) loadPolicyDeliveryStatusByRef(tenantID, nodeID uuid.UUID, domain string, resolver policyDeliveryErrorResolver) (map[string]policyDeliveryFields, error) {
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
			serializedHistory = append(serializedHistory, policyDeliveryToMapWithErrorResolver(delivery, resolver))
			pendingCount += pendingCountForCommandStatus(delivery.CommandStatus)
		}
		result[ref] = policyDeliveryFields{
			status:    mapCommandStatusToPolicyStatus(history[0].CommandStatus),
			pending:   pendingCount,
			last:      serializedHistory[0],
			history:   serializedHistory,
			lastError: stringifyPolicyValue(serializedHistory[0]["last_error"]),
		}
	}
	return result, nil
}

func (r *Router) aclPolicyDeliveryErrorResolver(tenantID, nodeID uuid.UUID, rules []*controllerstorage.ACLRuleRecord) policyDeliveryErrorResolver {
	return r.policyDeliveryErrorResolverForNodeRules(tenantID, nodeID, rules, nil)
}

func (r *Router) qosPolicyDeliveryErrorResolver(tenantID, nodeID uuid.UUID, rules []*controllerstorage.QoSRuleRecord) policyDeliveryErrorResolver {
	return r.policyDeliveryErrorResolverForNodeRules(tenantID, nodeID, nil, rules)
}

func (r *Router) policyDeliveryErrorResolverForNode(tenantID, nodeID uuid.UUID) policyDeliveryErrorResolver {
	return r.policyDeliveryErrorResolverForNodeRules(tenantID, nodeID, nil, nil)
}

func (r *Router) policyDeliveryErrorResolverForNodeRules(
	tenantID, nodeID uuid.UUID,
	knownACLRules []*controllerstorage.ACLRuleRecord,
	knownQoSRules []*controllerstorage.QoSRuleRecord,
) policyDeliveryErrorResolver {
	var usages []policyCIDRUsage
	loaded := false

	return func(delivery *controllerstorage.PolicyDelivery, raw string) string {
		if runtimeGroupConflictCIDR(raw) == "" {
			return raw
		}
		if !loaded {
			usages = r.policyCIDRUsagesForNode(tenantID, nodeID, knownACLRules, knownQoSRules)
			loaded = true
		}
		return policyDeliveryCIDRErrorResolver(usages)(delivery, raw)
	}
}

func (r *Router) policyCIDRUsagesForNode(
	tenantID, nodeID uuid.UUID,
	knownACLRules []*controllerstorage.ACLRuleRecord,
	knownQoSRules []*controllerstorage.QoSRuleRecord,
) []policyCIDRUsage {
	aclRules := knownACLRules
	if aclRules == nil {
		if loaded, err := r.store.ListTenantNodeACLRules(tenantID, nodeID); err == nil {
			aclRules = loaded
		}
	}
	qosRules := knownQoSRules
	if qosRules == nil {
		if loaded, err := r.store.ListTenantNodeQoSRules(tenantID, nodeID); err == nil {
			qosRules = loaded
		}
	}

	usages := make([]policyCIDRUsage, 0, len(aclRules)*2+len(qosRules)*3)
	usages = append(usages, r.aclPolicyCIDRUsages(tenantID, aclRules)...)
	usages = append(usages, r.qosPolicyCIDRUsages(tenantID, qosRules)...)
	return usages
}

func (r *Router) aclPolicyCIDRUsages(tenantID uuid.UUID, rules []*controllerstorage.ACLRuleRecord) []policyCIDRUsage {
	usages := make([]policyCIDRUsage, 0, len(rules)*2)
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		ref := rule.ID.String()
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			name = strings.TrimSpace(rule.Description)
		}
		if name == "" {
			name = ref
		}

		usages = appendPolicyCIDRUsage(usages, "acl", ref, name, rule.SrcCIDR)
		usages = appendPolicyCIDRUsage(usages, "acl", ref, name, rule.DstCIDR)
		usages = appendPolicyGroupCIDRUsages(usages, "acl", ref, name, r.policyIPGroupCIDRs(tenantID, rule.SrcGroupID, rule.SrcGroup)...)
		usages = appendPolicyGroupCIDRUsages(usages, "acl", ref, name, r.policyIPGroupCIDRs(tenantID, rule.DstGroupID, rule.DstGroup)...)
	}
	return usages
}

func (r *Router) qosPolicyCIDRUsages(tenantID uuid.UUID, rules []*controllerstorage.QoSRuleRecord) []policyCIDRUsage {
	usages := make([]policyCIDRUsage, 0, len(rules)*3)
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		ref := rule.ID.String()
		name := strings.TrimSpace(rule.Description)
		if name == "" {
			name = ref
		}

		usages = appendPolicyCIDRUsage(usages, "qos", ref, name, rule.SrcCIDR)
		usages = appendPolicyCIDRUsage(usages, "qos", ref, name, rule.DstCIDR)
		usages = appendPolicyGroupCIDRUsages(usages, "qos", ref, name, r.policyIPGroupCIDRs(tenantID, rule.GroupID, rule.Group)...)
	}
	return usages
}

func (r *Router) policyIPGroupCIDRs(tenantID uuid.UUID, groupID uuid.NullUUID, group *controllerstorage.IPGroupRecord) []string {
	if group == nil && groupID.Valid {
		loaded, err := r.store.GetIPGroup(tenantID, groupID.UUID)
		if err == nil {
			group = loaded
		}
	}
	if group == nil {
		return nil
	}

	cidrs := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		cidrs = append(cidrs, member.CIDR)
	}
	return cidrs
}

func appendPolicyGroupCIDRUsages(usages []policyCIDRUsage, domain, ref, name string, cidrs ...string) []policyCIDRUsage {
	for _, cidr := range cidrs {
		usages = appendPolicyCIDRUsage(usages, domain, ref, name, cidr)
	}
	return usages
}

func appendPolicyCIDRUsage(usages []policyCIDRUsage, domain, ref, name, cidr string) []policyCIDRUsage {
	cidr = strings.TrimSpace(cidr)
	if policyErrorCIDRIsAny(cidr) {
		return usages
	}
	return append(usages, policyCIDRUsage{
		policyRef:  strings.TrimSpace(ref),
		policyName: strings.TrimSpace(name),
		domain:     strings.TrimSpace(strings.ToLower(domain)),
		cidr:       cidr,
	})
}

func policyDeliveryCIDRErrorResolver(usages []policyCIDRUsage) policyDeliveryErrorResolver {
	return func(delivery *controllerstorage.PolicyDelivery, raw string) string {
		cidr := runtimeGroupConflictCIDR(raw)
		if cidr == "" {
			return raw
		}

		matches := policyCIDRUsageMatches(usages, cidr, delivery, true)
		if len(matches) == 0 {
			matches = policyCIDRUsageMatches(usages, cidr, delivery, false)
		}
		if len(matches) == 0 {
			return raw
		}
		return formatPolicyCIDRConflictError(cidr, matches)
	}
}

func runtimeGroupConflictCIDR(raw string) string {
	for _, re := range runtimeGroupCIDRConflictRes {
		match := re.FindStringSubmatch(raw)
		if len(match) == 2 {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

func policyCIDRUsageMatches(usages []policyCIDRUsage, cidr string, delivery *controllerstorage.PolicyDelivery, excludeCurrent bool) []policyCIDRUsage {
	currentRef := ""
	if delivery != nil {
		currentRef = strings.TrimSpace(delivery.PolicyRef)
	}

	matches := make([]policyCIDRUsage, 0, 2)
	seen := make(map[string]struct{})
	for _, usage := range usages {
		if !strings.EqualFold(strings.TrimSpace(usage.cidr), cidr) {
			continue
		}
		if excludeCurrent && currentRef != "" && strings.EqualFold(strings.TrimSpace(usage.policyRef), currentRef) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(usage.domain)) + "\x00" + strings.TrimSpace(usage.policyRef) + "\x00" + strings.TrimSpace(usage.policyName)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		matches = append(matches, usage)
	}
	return matches
}

func formatPolicyCIDRConflictError(cidr string, matches []policyCIDRUsage) string {
	labels := make([]string, 0, len(matches))
	for _, match := range matches {
		name := strings.TrimSpace(match.policyName)
		if name == "" {
			name = strings.TrimSpace(match.policyRef)
		}
		if name == "" {
			continue
		}
		labels = append(labels, fmt.Sprintf("%s 策略 %q", policyDomainLabel(match.domain), name))
	}
	if len(labels) == 0 {
		return ""
	}
	if len(labels) == 1 {
		return fmt.Sprintf("CIDR %s 已被 %s 使用，请修改或删除该策略后再同步。", cidr, labels[0])
	}
	return fmt.Sprintf("CIDR %s 已被多个策略使用：%s，请修改或删除冲突策略后再同步。", cidr, strings.Join(labels, "、"))
}

func policyDomainLabel(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case "acl":
		return "ACL"
	case "qos":
		return "QoS"
	default:
		if strings.TrimSpace(domain) == "" {
			return "Policy"
		}
		return strings.ToUpper(strings.TrimSpace(domain))
	}
}

func policyErrorCIDRIsAny(cidr string) bool {
	switch strings.ToLower(strings.TrimSpace(cidr)) {
	case "", "any", "0.0.0.0/0", "::/0":
		return true
	default:
		return false
	}
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
		GroupID       *uuid.UUID `json:"group_id"`
		Group         string     `json:"group"`
		SrcCIDR       string     `json:"src_cidr"`
		DstCIDR       string     `json:"dst_cidr"`
		SrcPort       *int       `json:"src_port"`
		DstPort       *int       `json:"dst_port"`
		Protocol      *int       `json:"protocol"`
		BandwidthMbps int        `json:"bandwidth_mbps"`
		Direction     string     `json:"direction"`
		RateBps       uint64     `json:"rate_bps"`
		BurstBytes    uint64     `json:"burst_bytes"`
		Priority      *int       `json:"priority"`
		Mode          string     `json:"mode"`
		Enabled       *bool      `json:"enabled"`
		Description   string     `json:"description"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}
	if writeInactivePolicyMutationError(w, node) {
		return
	}
	if body.BandwidthMbps <= 0 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "bandwidth_mbps must be greater than 0", nil)
		return
	}
	if writePolicyValidationError(w, validateQoSMatchFields(body.Protocol, body.SrcPort, body.DstPort)) {
		return
	}
	priority := intValueOrDefault(body.Priority, 100)
	if writePolicyValidationError(w, validatePolicyByteField("priority", priority)) {
		return
	}
	if writePolicyValidationError(w, validatePolicyDirectionField(body.Direction)) {
		return
	}
	mode, modeErr := normalizeQoSModeField(body.Mode, "auto")
	if writePolicyValidationError(w, modeErr) {
		return
	}

	rule := &controllerstorage.QoSRuleRecord{
		TenantID:      tenantID,
		NodeID:        node.ID,
		SrcCIDR:       normalizePolicyCIDRInput(body.SrcCIDR),
		DstCIDR:       normalizePolicyCIDRInput(body.DstCIDR),
		BandwidthMbps: body.BandwidthMbps,
		Direction:     body.Direction,
		RateBps:       body.RateBps,
		BurstBytes:    body.BurstBytes,
		Priority:      priority,
		Mode:          mode,
		Enabled:       boolValueOrDefault(body.Enabled, true),
		Description:   body.Description,
	}
	explicitGroup := nullUUIDFromPtr(body.GroupID)
	directGroup := qosDirectGroupCIDR(body.Group, body.SrcCIDR, body.DstCIDR, body.Direction)

	metadata := map[string]interface{}{
		"node_id": node.ID.String(),
	}
	r.writeTransactionalPolicyMutationSuccess(w, node, "qos", "create", "QoS rule created successfully", metadata, func(tx *controllerstorage.PolicyMutationTx) (map[string]interface{}, error) {
		ruleForWrite := *rule
		ruleForValidation := *rule
		var err error
		ruleForWrite.GroupID, err = tx.ResolvePolicyGroupRef(tenantID, explicitGroup, directGroup)
		if err != nil {
			return nil, fmt.Errorf("resolve QoS IP group: %w", err)
		}
		if ruleForWrite.GroupID.Valid {
			ruleForWrite.SrcCIDR = ""
			ruleForWrite.DstCIDR = ""
			ruleForValidation.GroupID = ruleForWrite.GroupID
			if explicitGroup.Valid {
				if ruleForValidation.Group, err = tx.GetIPGroup(tenantID, ruleForWrite.GroupID.UUID); err != nil {
					return nil, fmt.Errorf("load QoS IP group: %w", err)
				}
			}
			if strings.TrimSpace(directGroup) != "" {
				if strings.EqualFold(strings.TrimSpace(ruleForValidation.Direction), "ingress") {
					ruleForValidation.SrcCIDR = directGroup
					ruleForValidation.DstCIDR = ""
				} else {
					ruleForValidation.DstCIDR = directGroup
					ruleForValidation.SrcCIDR = ""
				}
			}
		}
		existing, err := tx.ListTenantNodeQoSRules(tenantID, node.ID)
		if err != nil {
			return nil, fmt.Errorf("load QoS rules for conflict check: %w", err)
		}
		if err := hydrateQoSRuleGroups(tx, tenantID, existing); err != nil {
			return nil, err
		}
		if err := controllerstorage.DetectQoSPolicyConflict(existing, &ruleForValidation, uuid.Nil); err != nil {
			return nil, err
		}

		created, err := tx.CreateTenantNodeQoSRule(&ruleForWrite)
		if err != nil {
			return nil, fmt.Errorf("failed to create QoS rule: %w", err)
		}
		metadata["rule_id"] = created.ID.String()
		return qosRuleResponse(created, node.ID), nil
	})
}

func (r *Router) updateTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}

	var body struct {
		GroupID       *uuid.UUID `json:"group_id"`
		Group         *string    `json:"group"`
		SrcCIDR       *string    `json:"src_cidr"`
		DstCIDR       *string    `json:"dst_cidr"`
		SrcPort       *int       `json:"src_port"`
		DstPort       *int       `json:"dst_port"`
		Protocol      *int       `json:"protocol"`
		BandwidthMbps *int       `json:"bandwidth_mbps"`
		Direction     *string    `json:"direction"`
		RateBps       *uint64    `json:"rate_bps"`
		BurstBytes    *uint64    `json:"burst_bytes"`
		Priority      *int       `json:"priority"`
		Mode          *string    `json:"mode"`
		Description   *string    `json:"description"`
		Enabled       *bool      `json:"enabled"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}
	if writeInactivePolicyMutationError(w, node) {
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
	directGroup := ""
	explicitGroup := rule.GroupID
	groupChanged := false
	if body.SrcCIDR != nil {
		rule.SrcCIDR = normalizePolicyCIDRInput(*body.SrcCIDR)
		rule.GroupID = uuid.NullUUID{}
		explicitGroup = uuid.NullUUID{}
		directGroup = rule.SrcCIDR
		groupChanged = true
	}
	if body.DstCIDR != nil {
		rule.DstCIDR = normalizePolicyCIDRInput(*body.DstCIDR)
		rule.GroupID = uuid.NullUUID{}
		explicitGroup = uuid.NullUUID{}
		directGroup = rule.DstCIDR
		groupChanged = true
	}
	if body.Group != nil {
		directGroup = normalizePolicyCIDRInput(*body.Group)
		rule.SrcCIDR = ""
		rule.DstCIDR = ""
		rule.GroupID = uuid.NullUUID{}
		explicitGroup = uuid.NullUUID{}
		groupChanged = true
	}
	if body.GroupID != nil {
		explicitGroup = nullUUIDFromPtr(body.GroupID)
		rule.GroupID = explicitGroup
		rule.SrcCIDR = ""
		rule.DstCIDR = ""
		groupChanged = true
	}
	if writePolicyValidationError(w, validateQoSMatchFields(body.Protocol, body.SrcPort, body.DstPort)) {
		return
	}
	rule.SrcPort = 0
	rule.DstPort = 0
	rule.Protocol = 0
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
		mode, err := normalizeQoSModeField(*body.Mode, "auto")
		if writePolicyValidationError(w, err) {
			return
		}
		rule.Mode = mode
	}
	if body.Description != nil {
		rule.Description = *body.Description
	}
	if body.Enabled != nil {
		rule.Enabled = *body.Enabled
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
		ruleForWrite := rule
		ruleForValidation := rule
		var err error
		if groupChanged {
			ruleForWrite.GroupID, err = tx.ResolvePolicyGroupRef(tenantID, explicitGroup, directGroup)
		} else {
			ruleForWrite.GroupID, err = tx.ResolvePolicyGroupRef(tenantID, rule.GroupID, qosDirectGroupCIDR("", rule.SrcCIDR, rule.DstCIDR, rule.Direction))
		}
		if err != nil {
			return nil, fmt.Errorf("resolve QoS IP group: %w", err)
		}
		if ruleForWrite.GroupID.Valid {
			ruleForWrite.SrcCIDR = ""
			ruleForWrite.DstCIDR = ""
			ruleForValidation.GroupID = ruleForWrite.GroupID
			if body.GroupID != nil || (!groupChanged && rule.GroupID.Valid && strings.TrimSpace(rule.SrcCIDR) == "" && strings.TrimSpace(rule.DstCIDR) == "") {
				if ruleForValidation.Group, err = tx.GetIPGroup(tenantID, ruleForWrite.GroupID.UUID); err != nil {
					return nil, fmt.Errorf("load QoS IP group: %w", err)
				}
			}
			if strings.TrimSpace(directGroup) != "" {
				if strings.EqualFold(strings.TrimSpace(ruleForValidation.Direction), "ingress") {
					ruleForValidation.SrcCIDR = directGroup
					ruleForValidation.DstCIDR = ""
				} else {
					ruleForValidation.DstCIDR = directGroup
					ruleForValidation.SrcCIDR = ""
				}
			}
		}
		existingRules, err := tx.ListTenantNodeQoSRules(tenantID, node.ID)
		if err != nil {
			return nil, fmt.Errorf("load QoS rules for conflict check: %w", err)
		}
		if err := hydrateQoSRuleGroups(tx, tenantID, existingRules); err != nil {
			return nil, err
		}
		if err := controllerstorage.DetectQoSPolicyConflict(existingRules, &ruleForValidation, ruleID); err != nil {
			return nil, err
		}
		updated, err := tx.UpdateTenantNodeQoSRule(tenantID, node.ID, ruleID, &ruleForWrite)
		if err != nil {
			return nil, fmt.Errorf("failed to update QoS rule: %w", err)
		}
		return qosRuleResponse(updated, node.ID), nil
	})
}

func (r *Router) deleteTenantNodeQoS(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, ruleIDStr string) {
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid rule ID format", nil)
		return
	}
	if writeInactivePolicyMutationError(w, node) {
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
