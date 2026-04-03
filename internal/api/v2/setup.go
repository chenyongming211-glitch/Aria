package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"aria/internal/api/middleware"
	v1 "aria/internal/api/v1"
	"aria/internal/service"
	"aria/pkg/controllerstorage"
)

type Router struct {
	store      *controllerstorage.Storage
	authAPI    *v1.AuthAPI
	tenantAPI  *v1.TenantAPI
	tenantMgmt *v1.TenantManagementAPI
	chatAPI    *v1.ChatHandler
}

func SetupRoutes(mux *http.ServeMux, store *controllerstorage.Storage) {
	router := &Router{
		store:      store,
		authAPI:    v1.NewAuthAPI(store),
		tenantAPI:  v1.NewTenantAPI(store),
		tenantMgmt: v1.NewTenantManagementAPI(store),
		chatAPI:    v1.NewChatHandler(service.NewAIService(store)),
	}

	mux.HandleFunc("/api/v2/auth/login", router.authAPI.HandleLogin)
	mux.HandleFunc("/api/v2/auth/refresh", router.authAPI.HandleRefresh)
	mux.HandleFunc("/api/v2/auth/logout", router.authAPI.HandleLogout)
	mux.HandleFunc("/api/v2/auth/force-change-password", router.authAPI.HandleForceChangePassword)

	withJWT := middleware.JWTAuthMiddleware
	mux.HandleFunc("/api/v2/tenants", withJWT(router.HandleTenants))
	mux.HandleFunc("/api/v2/tenants/", withJWT(router.HandleTenantScoped))
}

func (r *Router) HandleTenants(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/api/v2/tenants" {
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Unknown endpoint", nil)
		return
	}

	role, exists := middleware.GetUserRole(req.Context())
	if !exists {
		v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "Access denied", nil)
		return
	}

	switch req.Method {
	case http.MethodGet:
		if role == "super_admin" {
			r.listAllTenants(w)
			return
		}
		if role == "admin" || role == "owner" {
			tenantID, ok := middleware.GetTenantID(req.Context())
			if !ok {
				v1.WriteError(w, http.StatusUnauthorized, v1.CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
				return
			}
			r.listSingleTenant(w, tenantID)
			return
		}
		v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "Access denied: admin or super_admin only", nil)
	case http.MethodPost:
		if role != "super_admin" {
			v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "Access denied: super_admin only", nil)
			return
		}
		r.tenantAPI.CreateTenant(w, req)
	default:
		v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) HandleTenantScoped(w http.ResponseWriter, req *http.Request) {
	tenantIDStr, rest, ok := splitTenantPath(req.URL.Path)
	if !ok {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid tenant path", nil)
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidTenantID, "Invalid tenant ID", nil)
		return
	}

	role, exists := middleware.GetUserRole(req.Context())
	if !exists {
		v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "Access denied", nil)
		return
	}

	switch {
	case rest == "":
		r.handleSingleTenant(w, req, tenantID, role)
	case strings.HasPrefix(rest, "policies"):
		r.handleTenantPolicies(w, req, tenantID)
	case strings.HasPrefix(rest, "users"):
		r.handleTenantUsers(w, req, tenantID, role)
	case strings.HasPrefix(rest, "tokens"):
		r.handleTenantTokens(w, req, tenantID, role)
	case strings.HasPrefix(rest, "nodes"):
		r.handleTenantNodes(w, req, tenantID, role)
	case strings.HasPrefix(rest, "agents"):
		r.handleTenantAgents(w, req, tenantID)
	case strings.HasPrefix(rest, "monitoring"):
		r.handleTenantMonitoring(w, req, tenantID)
	case strings.HasPrefix(rest, "ai"):
		r.handleTenantAI(w, req, tenantID)
	default:
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Unknown endpoint", nil)
	}
}

func (r *Router) handleTenantPolicies(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	if !r.authorizeTenant(w, req, tenantID, false) {
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.listTenantPolicies(w, req, tenantID)
	default:
		v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) handleSingleTenant(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	switch req.Method {
	case http.MethodGet:
		if !r.authorizeTenant(w, req, tenantID, false) {
			return
		}
		r.tenantAPI.GetTenant(w, withTenantContext(req, tenantID))
	case http.MethodPut:
		if !r.authorizeTenantAdmin(w, req, tenantID) {
			return
		}
		r.updateTenant(w, req, tenantID, role)
	case http.MethodDelete:
		if role != "super_admin" {
			v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "Access denied: super_admin only", nil)
			return
		}
		if _, err := r.store.DB().Exec(`UPDATE tenants SET status = 'deleted', updated_at = NOW() WHERE id = $1`, tenantID); err != nil {
			v1.WriteError(w, http.StatusInternalServerError, v1.CodeDeleteTokenFailed, "Failed to delete tenant: "+err.Error(), nil)
			return
		}
		v1.WriteSuccess(w, map[string]string{"id": tenantID.String(), "status": "deleted"}, "Tenant deleted successfully")
	default:
		v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) handleTenantUsers(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	if !r.authorizeTenantAdmin(w, req, tenantID) {
		return
	}

	parts := splitPath(req.URL.Path)
	if len(parts) == 7 && req.Method == http.MethodDelete {
		currentUserID, ok := middleware.GetUserID(req.Context())
		if ok && currentUserID == parts[6] {
			v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "You cannot delete your own account", nil)
			return
		}
	}

	req2 := withTenantContext(req, tenantID)
	r.tenantAPI.HandleTenantUsers(w, req2)
}

func (r *Router) handleTenantTokens(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	if !r.authorizeTenantAdmin(w, req, tenantID) {
		return
	}

	req2 := withTenantContext(req, tenantID)
	parts := splitPath(req.URL.Path)
	if len(parts) == 5 {
		switch req.Method {
		case http.MethodGet:
			r.tenantMgmt.GetTenantTokens(w, req2)
		case http.MethodPost:
			r.tenantMgmt.CreateEnrollmentToken(w, req2)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	if len(parts) == 6 {
		tokenID := parts[5]
		switch req.Method {
		case http.MethodGet:
			r.getTenantTokenDetailByID(w, req2, tenantID, tokenID)
		case http.MethodDelete:
			rewritten := cloneRequestWithPath(req2, "/api/v1/tenant-management/tokens/"+tokenID)
			r.tenantMgmt.DeleteToken(w, rewritten)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid token path", nil)
}

func (r *Router) handleTenantNodes(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	parts := splitPath(req.URL.Path)
	req2 := withTenantContext(req, tenantID)

	if len(parts) == 5 {
		if !r.authorizeTenant(w, req, tenantID, false) {
			return
		}
		switch req.Method {
		case http.MethodGet:
			r.listTenantNodes(w, tenantID)
		default:
			v1.WriteError(w, http.StatusNotImplemented, v1.CodeNotImplemented, "Node mutation API will be implemented in the next phase", nil)
		}
		return
	}

	if len(parts) == 6 {
		nodeID := parts[5]
		switch req.Method {
		case http.MethodGet:
			if !r.authorizeTenant(w, req, tenantID, false) {
				return
			}
			r.getTenantNodeByID(w, tenantID, nodeID)
		case http.MethodPut:
			if !r.authorizeTenantAdmin(w, req, tenantID) {
				return
			}
			r.updateTenantNode(w, req2, tenantID, nodeID)
		case http.MethodDelete:
			if !r.authorizeTenantAdmin(w, req, tenantID) {
				return
			}
			r.deleteTenantNode(w, tenantID, nodeID)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	if len(parts) >= 7 && parts[6] == "routes" {
		r.handleTenantNodeRoutes(w, req2, tenantID, parts)
		return
	}

	if len(parts) >= 7 && parts[6] == "security" {
		r.handleTenantNodeSecurity(w, req2, tenantID, parts)
		return
	}

	if len(parts) >= 7 && parts[6] == "qos" {
		r.handleTenantNodeQoS(w, req2, tenantID, parts)
		return
	}

	if len(parts) >= 7 && parts[6] == "agent" {
		r.handleTenantNodeAgent(w, req2, tenantID, parts)
		return
	}

	v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid node path", nil)
}

func (r *Router) listTenantPolicies(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	nodes, err := r.store.GetNodesByTenant(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetNodesFailed, "Failed to load tenant nodes", nil)
		return
	}

	query := req.URL.Query()
	kindFilter := strings.TrimSpace(query.Get("kind"))
	nodeFilter := strings.TrimSpace(query.Get("node_id"))
	enabledFilter := strings.TrimSpace(query.Get("enabled"))

	var enabledPtr *bool
	if enabledFilter != "" {
		enabled, err := strconv.ParseBool(enabledFilter)
		if err != nil {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "enabled must be true or false", nil)
			return
		}
		enabledPtr = &enabled
	}

	policies := make([]map[string]interface{}, 0)
	for _, node := range nodes {
		if nodeFilter != "" && node.ID.String() != nodeFilter {
			continue
		}

		if kindFilter == "" || kindFilter == "acl" {
			items, err := r.buildTenantNodeACLPolicies(tenantID, node)
			if err != nil {
				v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetACLRulesFailed, "Failed to load ACL policies", nil)
				return
			}
			policies = append(policies, items...)
		}

		if kindFilter == "" || kindFilter == "qos" {
			items, err := r.buildTenantNodeQoSPolicies(tenantID, node)
			if err != nil {
				v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetLimitsFailed, "Failed to load QoS policies", nil)
				return
			}
			policies = append(policies, items...)
		}

		if kindFilter == "" || kindFilter == "route" {
			items, err := r.buildTenantNodeRoutePolicies(tenantID, node)
			if err != nil {
				v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetNodesFailed, "Failed to load route policies", nil)
				return
			}
			policies = append(policies, items...)
		}
	}

	if enabledPtr != nil {
		filtered := make([]map[string]interface{}, 0, len(policies))
		for _, policy := range policies {
			if enabled, ok := policy["enabled"].(bool); ok && enabled == *enabledPtr {
				filtered = append(filtered, policy)
			}
		}
		policies = filtered
	}

	sort.Slice(policies, func(i, j int) bool {
		leftKind := stringifyPolicyValue(policies[i]["kind"])
		rightKind := stringifyPolicyValue(policies[j]["kind"])
		if leftKind != rightKind {
			return leftKind < rightKind
		}

		leftNode := stringifyPolicyValue(policies[i]["node_name"])
		rightNode := stringifyPolicyValue(policies[j]["node_name"])
		if leftNode != rightNode {
			return leftNode < rightNode
		}

		leftName := stringifyPolicyValue(policies[i]["name"])
		rightName := stringifyPolicyValue(policies[j]["name"])
		if leftName != rightName {
			return leftName < rightName
		}

		return stringifyPolicyValue(policies[i]["policy_id"]) < stringifyPolicyValue(policies[j]["policy_id"])
	})

	v1.WriteSuccess(w, policies, fmt.Sprintf("%d unified policies retrieved", len(policies)))
}

func (r *Router) buildTenantNodeACLPolicies(tenantID uuid.UUID, node *controllerstorage.Node) ([]map[string]interface{}, error) {
	rows, err := r.store.DB().Query(
		`SELECT id, COALESCE(name, ''), COALESCE(src_node, ''), src_net, COALESCE(dst_node, ''), dst_net, protocol, min_port, max_port,
		        COALESCE(action, 'allow'), enabled, priority, COALESCE(description, ''), created_at, updated_at
		   FROM acl_rules
		  WHERE tenant_id = $1 AND (src_node = $2 OR dst_node = $2)
		  ORDER BY priority ASC, id ASC`,
		tenantID, node.PublicKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var rule controllerstorage.ACLRule
		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.SrcNode, &rule.SrcNet, &rule.DstNode, &rule.DstNet, &rule.Protocol,
			&rule.MinPort, &rule.MaxPort, &rule.Action, &rule.Enabled, &rule.Priority,
			&rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}

		policyRef := strconv.Itoa(rule.ID)
		items = append(items, map[string]interface{}{
			"policy_id":    fmt.Sprintf("acl:%s:%s", node.ID.String(), policyRef),
			"policy_ref":   policyRef,
			"tenant_id":    tenantID.String(),
			"node_id":      node.ID.String(),
			"node_name":    firstNonEmpty(node.Hostname, node.PublicKey, node.ID.String()),
			"target_nodes": []string{node.ID.String()},
			"scope":        "node",
			"kind":         "acl",
			"name":         firstNonEmpty(rule.Name, rule.Description, fmt.Sprintf("%s -> %s", rule.SrcNet, rule.DstNet)),
			"enabled":      rule.Enabled,
			"priority":     rule.Priority,
			"status":       "idle",
			"version":      "",
			"spec": map[string]interface{}{
				"src_node":    rule.SrcNode,
				"src_net":     rule.SrcNet,
				"dst_node":    rule.DstNode,
				"dst_net":     rule.DstNet,
				"protocol":    rule.Protocol,
				"min_port":    rule.MinPort,
				"max_port":    rule.MaxPort,
				"action":      rule.Action,
				"description": rule.Description,
			},
			"created_at": rule.CreatedAt,
			"updated_at": rule.UpdatedAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "acl", items, "policy_ref"); err != nil {
		return nil, err
	}

	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		attachNodeSummaryToPolicyItems(items, summary)
	}
	finalizePolicyItems(items)
	return items, nil
}

func (r *Router) buildTenantNodeQoSPolicies(tenantID uuid.UUID, node *controllerstorage.Node) ([]map[string]interface{}, error) {
	items := make([]map[string]interface{}, 0)
	for _, category := range []string{
		controllerstorage.QoSCategoryService,
		controllerstorage.QoSCategoryPeers,
		controllerstorage.QoSCategoryIP,
	} {
		rules, err := r.store.ListTenantNodeQoSRules(tenantID, node.ID, category)
		if err != nil {
			return nil, err
		}
		for _, rule := range rules {
			policyRef := rule.ID.String()
			items = append(items, map[string]interface{}{
				"policy_id":    fmt.Sprintf("qos:%s:%s", node.ID.String(), policyRef),
				"policy_ref":   policyRef,
				"tenant_id":    tenantID.String(),
				"node_id":      node.ID.String(),
				"node_name":    firstNonEmpty(node.Hostname, node.PublicKey, node.ID.String()),
				"target_nodes": []string{node.ID.String()},
				"scope":        "node",
				"kind":         "qos",
				"name":         firstNonEmpty(rule.Description, fmt.Sprintf("%s %s -> %s", category, firstNonEmpty(rule.SrcCIDR, "*"), firstNonEmpty(rule.DstCIDR, "*"))),
				"enabled":      rule.Enabled,
				"priority":     100,
				"status":       "idle",
				"version":      "",
				"spec": map[string]interface{}{
					"category":       category,
					"src_ip":         rule.SrcCIDR,
					"dst_ip":         rule.DstCIDR,
					"src_port":       rule.SrcPort,
					"dst_port":       rule.DstPort,
					"protocol":       rule.Protocol,
					"bandwidth_mbps": rule.BandwidthMbps,
					"description":    rule.Description,
				},
				"created_at": rule.CreatedAt,
				"updated_at": rule.UpdatedAt,
			})
		}
	}

	if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "qos", items, "policy_ref"); err != nil {
		return nil, err
	}

	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		attachNodeSummaryToPolicyItems(items, summary)
	}
	finalizePolicyItems(items)
	return items, nil
}

func (r *Router) buildTenantNodeRoutePolicies(tenantID uuid.UUID, node *controllerstorage.Node) ([]map[string]interface{}, error) {
	items := make([]map[string]interface{}, 0, len(node.AdvertisedRoutes))
	for _, route := range node.AdvertisedRoutes {
		route = strings.TrimSpace(route)
		if route == "" {
			continue
		}

		items = append(items, map[string]interface{}{
			"policy_id":    fmt.Sprintf("route:%s:%s", node.ID.String(), route),
			"policy_ref":   route,
			"tenant_id":    tenantID.String(),
			"node_id":      node.ID.String(),
			"node_name":    firstNonEmpty(node.Hostname, node.PublicKey, node.ID.String()),
			"target_nodes": []string{node.ID.String()},
			"scope":        "node",
			"kind":         "route",
			"name":         route,
			"enabled":      true,
			"priority":     100,
			"status":       "idle",
			"version":      "",
			"spec": map[string]interface{}{
				"cidr": route,
			},
			"created_at": node.UpdatedAt,
			"updated_at": node.UpdatedAt,
		})
	}

	if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "route", items, "policy_ref"); err != nil {
		return nil, err
	}

	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		attachNodeSummaryToPolicyItems(items, summary)
	}
	finalizePolicyItems(items)
	return items, nil
}

func finalizePolicyItems(items []map[string]interface{}) {
	for _, item := range items {
		if status := stringifyPolicyValue(item["policy_status"]); status != "" {
			item["status"] = status
		}

		if lastDelivery, ok := item["last_delivery"].(map[string]interface{}); ok {
			if version := stringifyPolicyValue(lastDelivery["id"]); version != "" {
				item["version"] = version
			}
		}
	}
}

func attachNodeSummaryToPolicyItems(items []map[string]interface{}, summary map[string]interface{}) {
	if len(items) == 0 || summary == nil {
		return
	}

	for _, item := range items {
		item["desired_state_version"] = summary["desired_state_version"]
		item["desired_state_updated_at"] = summary["desired_state_updated_at"]
		item["applied_state_version"] = summary["applied_state_version"]
		item["applied_state_updated_at"] = summary["applied_state_updated_at"]
		item["observed_state"] = summary["observed_state"]
		item["observed_message"] = summary["observed_message"]
		item["observed_at"] = summary["observed_at"]
		item["last_sync_error"] = summary["last_sync_error"]
		item["state_convergence"] = summary["state_convergence"]
		if version := stringifyPolicyValue(summary["desired_state_version"]); version != "" {
			item["version"] = version
		}
	}
}

func (r *Router) handleTenantAI(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	if !r.authorizeTenant(w, req, tenantID, false) {
		return
	}

	parts := splitPath(req.URL.Path)
	if len(parts) != 6 || parts[4] != "ai" {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid AI path", nil)
		return
	}

	req2 := withTenantContext(req, tenantID)
	switch parts[5] {
	case "chat":
		r.chatAPI.HandleChat(w, req2)
	case "confirm":
		r.chatAPI.HandleConfirm(w, req2)
	default:
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Unknown endpoint", nil)
	}
}

func (r *Router) listTenantNodes(w http.ResponseWriter, tenantID uuid.UUID) {
	nodes, err := r.store.GetNodesByTenant(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetNodesFailed, "Failed to get nodes", nil)
		return
	}

	items := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, r.buildTenantNodeResponse(node))
	}

	v1.WriteSuccess(w, items, fmt.Sprintf("%d nodes retrieved", len(items)))
}

func (r *Router) handleTenantNodeSecurity(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, parts []string) {
	if len(parts) < 8 {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid security path", nil)
		return
	}

	node, err := r.getTenantNodeRecord(parts[5], tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	switch parts[7] {
	case "acls":
		r.handleTenantNodeACLs(w, req, tenantID, node, parts)
	case "blacklist":
		r.handleTenantNodeBlacklist(w, req, tenantID, node, parts)
	default:
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Unknown security endpoint", nil)
	}
}

func (r *Router) handleTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, parts []string) {
	if len(parts) < 8 {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid QoS path", nil)
		return
	}

	node, err := r.getTenantNodeRecord(parts[5], tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	switch parts[7] {
	case "service", "peers", "ip":
		r.handleTenantNodeQoSCategory(w, req, tenantID, node, parts)
	default:
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Unknown QoS endpoint", nil)
	}
}

func (r *Router) listAllTenants(w http.ResponseWriter) {
	rows, err := r.store.DB().Query(`SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeListTenantsFailed, "Failed to list tenants", nil)
		return
	}
	defer rows.Close()

	var tenants []map[string]interface{}
	for rows.Next() {
		var id uuid.UUID
		var name string
		var code sql.NullString
		var status string
		var quota string
		var createdAt, updatedAt interface{}
		if err := rows.Scan(&id, &name, &code, &status, &quota, &createdAt, &updatedAt); err != nil {
			v1.WriteError(w, http.StatusInternalServerError, v1.CodeScanTenantFailed, "Failed to scan tenant", nil)
			return
		}
		tenants = append(tenants, map[string]interface{}{
			"id":             id.String(),
			"name":           name,
			"code":           code.String,
			"status":         status,
			"resource_quota": quota,
			"created_at":     createdAt,
			"updated_at":     updatedAt,
		})
	}

	v1.WriteSuccess(w, tenants, fmt.Sprintf("%d tenants retrieved", len(tenants)))
}

func (r *Router) listSingleTenant(w http.ResponseWriter, tenantID uuid.UUID) {
	var info controllerstorage.TenantInfo
	err := r.store.DB().QueryRow(`SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`, tenantID).
		Scan(&info.ID, &info.Name, &info.Code, &info.Status, &info.ResourceQuota, &info.CreatedAt, &info.UpdatedAt)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, v1.CodeTenantNotFound, "Tenant not found", nil)
		return
	}

	v1.WriteSuccess(w, []map[string]interface{}{{
		"id":             info.ID.String(),
		"name":           info.Name,
		"code":           info.Code,
		"status":         info.Status,
		"resource_quota": info.ResourceQuota,
		"created_at":     info.CreatedAt,
		"updated_at":     info.UpdatedAt,
	}}, "1 tenants retrieved")
}

func (r *Router) updateTenant(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	var body struct {
		Name          string                 `json:"name"`
		Code          string                 `json:"code"`
		Status        string                 `json:"status"`
		ResourceQuota map[string]interface{} `json:"resource_quota"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	resourceQuota := ""
	if body.ResourceQuota != nil {
		if role != "super_admin" {
			v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "Only super_admin can update resource quota", nil)
			return
		}
		raw, err := json.Marshal(body.ResourceQuota)
		if err != nil {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidResourceQuota, "Invalid resource quota format", nil)
			return
		}
		resourceQuota = string(raw)
	}

	query := `UPDATE tenants
		SET name = COALESCE(NULLIF($1, ''), name),
		    code = COALESCE(NULLIF($2, ''), code),
		    status = COALESCE(NULLIF($3, ''), status),
		    resource_quota = CASE WHEN $4 = '' THEN resource_quota ELSE $4 END,
		    updated_at = NOW()
		WHERE id = $5`
	if _, err := r.store.DB().Exec(query, body.Name, body.Code, body.Status, resourceQuota, tenantID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeCreateTenantFailed, "Failed to update tenant: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]string{"id": tenantID.String()}, "Tenant updated successfully")
}

func (r *Router) getTenantTokenDetailByID(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, tokenID string) {
	tokenUUID, err := uuid.Parse(tokenID)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidTokenID, "Invalid token ID", nil)
		return
	}

	var id uuid.UUID
	var tokenValue, tag, createdBy, status string
	var maxUses, usedCount int
	var expiresAt, createdAt, lastUsedAt, lastUsedBy interface{}
	err = r.store.DB().QueryRow(
		`SELECT id, token, tag, COALESCE(created_by, ''), status, max_uses, used_count, expires_at, created_at, last_used_at, COALESCE(last_used_by, '')
		 FROM tokens WHERE id = $1 AND tenant_id = $2`,
		tokenUUID, tenantID,
	).Scan(&id, &tokenValue, &tag, &createdBy, &status, &maxUses, &usedCount, &expiresAt, &createdAt, &lastUsedAt, &lastUsedBy)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, v1.CodeTokenNotFound, "Token not found", nil)
		return
	}

	v1.WriteSuccess(w, map[string]interface{}{
		"id":           id.String(),
		"token":        tokenValue,
		"tenant_id":    tenantID.String(),
		"tag":          tag,
		"created_by":   createdBy,
		"status":       status,
		"max_uses":     maxUses,
		"used_count":   usedCount,
		"expires_at":   expiresAt,
		"created_at":   createdAt,
		"last_used_at": lastUsedAt,
		"last_used_by": lastUsedBy,
	}, "Token retrieved successfully")
}

func (r *Router) getTenantNodeByID(w http.ResponseWriter, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	data := r.buildTenantNodeResponse(node)

	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		data["operations"] = summary
	}
	if commands, err := r.store.ListRecentAgentCommands(node.PublicKey, 10); err == nil {
		items := make([]map[string]interface{}, 0, len(commands))
		for _, cmd := range commands {
			items = append(items, agentCommandToMap(cmd))
		}
		data["recent_commands"] = items
	}

	v1.WriteSuccess(w, data, "Node retrieved successfully")
}

func (r *Router) buildTenantNodeResponse(node *controllerstorage.Node) map[string]interface{} {
	response := map[string]interface{}{
		"id":                  node.ID.String(),
		"public_key":          node.PublicKey,
		"machine_id":          node.MachineID,
		"tenant_id":           node.TenantID.String(),
		"endpoint":            node.Endpoint,
		"private_ip":          node.PrivateIP,
		"public_ip":           node.PublicIP,
		"region":              node.Region,
		"vpc_id":              node.VPCID,
		"hostname":            node.Hostname,
		"assigned_ip":         node.AssignedIP,
		"ip_offset":           node.IPOffset,
		"last_seen":           node.LastSeen,
		"registered_at":       node.RegisteredAt,
		"role":                node.Role,
		"runtime_mode":        node.RuntimeMode,
		"kernel_version":      node.KernelVersion,
		"has_aesni":           node.HasAESNI,
		"status":              node.Status,
		"availability_status": nodeAvailabilityStatus(node),
		"advertised_routes":   node.AdvertisedRoutes,
		"enrolled_with_token": node.EnrolledWithToken,
		"created_at":          node.CreatedAt,
		"updated_at":          node.UpdatedAt,
	}

	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		response["operations"] = summary
		response["pending_cmds"] = summary["pending_cmds"]
		response["last_command"] = summary["last_command"]
		response["last_command_status"] = summary["last_command_status"]
		response["last_command_error"] = summary["last_command_error"]
		response["configuration_status"] = summary["configuration_status"]
		response["last_sync_at"] = summary["last_sync_at"]
	}

	return response
}

func (r *Router) deleteTenantNode(w http.ResponseWriter, tenantID uuid.UUID, nodeID string) {
	nodeUUID, err := uuid.Parse(nodeID)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid node ID", nil)
		return
	}

	var publicKey string
	err = r.store.DB().QueryRow(`SELECT public_key FROM nodes WHERE id = $1 AND tenant_id = $2`, nodeUUID, tenantID).Scan(&publicKey)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, v1.CodeNodeNotFound, "Node not found", nil)
		return
	}

	if err := r.store.MarkNodeDeleted(publicKey); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeUpdateNodeFailed, "Failed to delete node: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]string{"id": nodeID, "status": "deleted"}, "Node deleted successfully")
}

func (r *Router) updateTenantNode(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	var body struct {
		Hostname         string   `json:"hostname"`
		Endpoint         string   `json:"endpoint"`
		PrivateIP        string   `json:"private_ip"`
		PublicIP         string   `json:"public_ip"`
		Region           string   `json:"region"`
		VPCID            string   `json:"vpc_id"`
		Role             string   `json:"role"`
		AdvertisedRoutes []string `json:"advertised_routes"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	routes := node.AdvertisedRoutes
	if body.AdvertisedRoutes != nil {
		normalized, err := normalizeRoutes(body.AdvertisedRoutes)
		if err != nil {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
			return
		}
		routes = normalized
	}

	role := node.Role
	if body.Role != "" {
		if body.Role != "hub" && body.Role != "spoke" {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Role must be either hub or spoke", nil)
			return
		}
		role = body.Role
	}

	query := `UPDATE nodes
		SET hostname = COALESCE(NULLIF($1, ''), hostname),
		    endpoint = COALESCE(NULLIF($2, ''), endpoint),
		    private_ip = COALESCE(NULLIF($3, ''), private_ip),
		    public_ip = COALESCE(NULLIF($4, ''), public_ip),
		    region = COALESCE(NULLIF($5, ''), region),
		    vpc_id = COALESCE(NULLIF($6, ''), vpc_id),
		    role = $7,
		    advertised_routes = $8,
		    updated_at = NOW()
		WHERE id = $9 AND tenant_id = $10`
	if _, err := r.store.DB().Exec(query,
		body.Hostname,
		body.Endpoint,
		body.PrivateIP,
		body.PublicIP,
		body.Region,
		body.VPCID,
		role,
		pq.Array(routes),
		node.ID,
		tenantID,
	); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeUpdateNodeFailed, "Failed to update node", nil)
		return
	}

	v1.WriteSuccess(w, map[string]interface{}{
		"id":                node.ID.String(),
		"tenant_id":         tenantID.String(),
		"role":              role,
		"advertised_routes": routes,
	}, "Node updated successfully")
}

func (r *Router) handleTenantNodeACLs(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	nodeRef := node.PublicKey
	if len(parts) == 8 {
		switch req.Method {
		case http.MethodGet:
			if !r.authorizeTenant(w, req, tenantID, false) {
				return
			}
			rows, err := r.store.DB().Query(
				`SELECT id, COALESCE(name, ''), COALESCE(src_node, ''), src_net, COALESCE(dst_node, ''), dst_net, protocol, min_port, max_port,
				        COALESCE(action, 'allow'), enabled, priority, COALESCE(description, ''), created_at, updated_at
				   FROM acl_rules
				  WHERE tenant_id = $1 AND (src_node = $2 OR dst_node = $2)
				  ORDER BY priority ASC, id ASC`,
				tenantID, nodeRef,
			)
			if err != nil {
				v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetACLRulesFailed, "Failed to list ACL rules", nil)
				return
			}
			defer rows.Close()

			var rules []map[string]interface{}
			for rows.Next() {
				var rule controllerstorage.ACLRule
				if err := rows.Scan(
					&rule.ID, &rule.Name, &rule.SrcNode, &rule.SrcNet, &rule.DstNode, &rule.DstNet, &rule.Protocol,
					&rule.MinPort, &rule.MaxPort, &rule.Action, &rule.Enabled, &rule.Priority,
					&rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
				); err != nil {
					v1.WriteError(w, http.StatusInternalServerError, v1.CodeScanACLRuleFailed, "Failed to scan ACL rule", nil)
					return
				}
				rules = append(rules, map[string]interface{}{
					"id":          rule.ID,
					"name":        rule.Name,
					"node_id":     node.ID.String(),
					"src_node":    rule.SrcNode,
					"src_net":     rule.SrcNet,
					"dst_node":    rule.DstNode,
					"dst_net":     rule.DstNet,
					"protocol":    rule.Protocol,
					"min_port":    rule.MinPort,
					"max_port":    rule.MaxPort,
					"action":      rule.Action,
					"enabled":     rule.Enabled,
					"priority":    rule.Priority,
					"description": rule.Description,
					"created_at":  rule.CreatedAt,
					"updated_at":  rule.UpdatedAt,
				})
			}
			if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "acl", rules, "id"); err != nil {
				v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetACLRulesFailed, "Failed to load ACL delivery history", nil)
				return
			}
			if summary, err := r.buildNodeOperationsSummary(node); err == nil {
				attachNodeSummaryToPolicyItems(rules, summary)
			}
			v1.WriteSuccess(w, rules, fmt.Sprintf("%d ACL rules retrieved", len(rules)))
		case http.MethodPost:
			if !r.authorizeTenantAdmin(w, req, tenantID) {
				return
			}
			var body controllerstorage.ACLRule
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid request body", nil)
				return
			}
			if body.SrcNet == "" || body.DstNet == "" {
				v1.WriteError(w, http.StatusBadRequest, v1.CodeValidationFailed, "src_net and dst_net are required", map[string]string{
					"src_net": "required",
					"dst_net": "required",
				})
				return
			}
			if body.Action == "" {
				body.Action = "allow"
			}
			if body.Priority == 0 {
				body.Priority = 100
			}
			if !body.Enabled {
				body.Enabled = true
			}
			if body.SrcNode == "" && body.DstNode == "" {
				body.SrcNode = nodeRef
			}
			body.SrcNode = normalizeNodeRef(body.SrcNode, node)
			body.DstNode = normalizeNodeRef(body.DstNode, node)

			rewritten := cloneRequestWithPath(req, "/api/v1/tenant-management/acl-rules")
			reqBody, _ := json.Marshal(body)
			rewritten.Body = newReadCloser(reqBody)
			r.proxyLegacyPolicyMutation(
				w,
				rewritten,
				node,
				"acl",
				"create",
				map[string]interface{}{
					"node_id":     node.ID.String(),
					"rule_name":   body.Name,
					"src_net":     body.SrcNet,
					"dst_net":     body.DstNet,
					"rule_action": body.Action,
				},
				r.tenantMgmt.CreateTenantACLRule,
			)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	if len(parts) == 9 {
		ruleID := parts[8]
		switch req.Method {
		case http.MethodDelete:
			if !r.authorizeTenantAdmin(w, req, tenantID) {
				return
			}
			var count int
			err := r.store.DB().QueryRow(
				`SELECT COUNT(*) FROM acl_rules WHERE id = $1 AND tenant_id = $2 AND (src_node = $3 OR dst_node = $3)`,
				ruleID, tenantID, nodeRef,
			).Scan(&count)
			if err != nil || count == 0 {
				v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "ACL rule not found", nil)
				return
			}
			rewritten := cloneRequestWithPath(req, "/api/v1/tenant-management/acl-rules/"+ruleID)
			r.proxyLegacyPolicyMutation(
				w,
				rewritten,
				node,
				"acl",
				"delete",
				map[string]interface{}{
					"node_id": node.ID.String(),
					"rule_id": ruleID,
				},
				r.tenantMgmt.DeleteTenantACLRule,
			)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid ACL path", nil)
}

func (r *Router) handleTenantNodeBlacklist(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	if len(parts) < 9 {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid blacklist path", nil)
		return
	}

	scope := parts[8]
	if err := controllerstorage.ValidateBlacklistScope(scope); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	if len(parts) == 9 {
		switch req.Method {
		case http.MethodGet:
			if !r.authorizeTenant(w, req, tenantID, false) {
				return
			}
			r.listTenantNodeBlacklistRules(w, tenantID, node, scope)
		case http.MethodPost:
			if !r.authorizeTenantAdmin(w, req, tenantID) {
				return
			}
			r.createTenantNodeBlacklistRule(w, req, tenantID, node, scope)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	if len(parts) == 10 {
		if !r.authorizeTenantAdmin(w, req, tenantID) {
			return
		}
		switch req.Method {
		case http.MethodDelete:
			r.deleteTenantNodeBlacklistRule(w, tenantID, node, scope, parts[9])
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid blacklist path", nil)
}

func (r *Router) handleTenantNodeQoSCategory(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, parts []string) {
	category := parts[7]
	if len(parts) == 8 {
		switch req.Method {
		case http.MethodGet:
			if !r.authorizeTenant(w, req, tenantID, false) {
				return
			}
			r.listTenantNodeQoS(w, tenantID, node, category)
		case http.MethodPost:
			if !r.authorizeTenantAdmin(w, req, tenantID) {
				return
			}
			r.createTenantNodeQoS(w, req, tenantID, node, category)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	if len(parts) == 9 {
		if !r.authorizeTenantAdmin(w, req, tenantID) {
			return
		}
		switch req.Method {
		case http.MethodDelete:
			r.deleteTenantNodeQoS(w, tenantID, node, category, parts[8])
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid QoS path", nil)
}

func (r *Router) listTenantNodeQoS(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, category string) {
	if err := controllerstorage.ValidateQoSCategory(category); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	rules, err := r.store.ListTenantNodeQoSRules(tenantID, node.ID, category)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetLimitsFailed, "Failed to list QoS rules", nil)
		return
	}
	var items []map[string]interface{}
	for _, rule := range rules {
		items = append(items, map[string]interface{}{
			"id":             rule.ID.String(),
			"node_id":        node.ID.String(),
			"tenant_id":      tenantID.String(),
			"src_ip":         rule.SrcCIDR,
			"dst_ip":         rule.DstCIDR,
			"src_port":       rule.SrcPort,
			"dst_port":       rule.DstPort,
			"protocol":       rule.Protocol,
			"bandwidth_mbps": rule.BandwidthMbps,
			"enabled":        rule.Enabled,
			"description":    rule.Description,
			"created_at":     rule.CreatedAt,
			"updated_at":     rule.UpdatedAt,
			"category":       category,
		})
	}
	if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "qos", items, "id"); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetLimitsFailed, "Failed to load QoS delivery history", nil)
		return
	}
	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		attachNodeSummaryToPolicyItems(items, summary)
	}

	v1.WriteSuccess(w, items, fmt.Sprintf("%d QoS rules retrieved", len(items)))
}

func (r *Router) createTenantNodeQoS(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, category string) {
	if err := controllerstorage.ValidateQoSCategory(category); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	var body struct {
		SrcIP         string `json:"src_ip,omitempty"`
		DstIP         string `json:"dst_ip,omitempty"`
		SrcPort       int    `json:"src_port,omitempty"`
		DstPort       int    `json:"dst_port,omitempty"`
		Protocol      int    `json:"protocol,omitempty"`
		Bandwidth     int    `json:"bandwidth,omitempty"`
		BandwidthMbps int    `json:"bandwidth_mbps,omitempty"`
		Enabled       *bool  `json:"enabled,omitempty"`
		Description   string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	bandwidth := body.Bandwidth
	if bandwidth <= 0 {
		bandwidth = body.BandwidthMbps
	}
	if bandwidth <= 0 {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidBandwidth, "bandwidth_mbps must be greater than 0", nil)
		return
	}

	srcIP := strings.TrimSpace(body.SrcIP)
	dstIP := strings.TrimSpace(body.DstIP)
	defaultIP := primaryNodeIP(node)
	if srcIP == "" {
		srcIP = defaultIP
	}
	if srcIP == "" {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Node has no usable IP for QoS scoping", nil)
		return
	}

	switch category {
	case "service":
		if dstIP == "" {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "dst_ip is required for service QoS", nil)
			return
		}
		if body.DstPort <= 0 {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "dst_port is required for service QoS", nil)
			return
		}
		if body.Protocol == 0 {
			body.Protocol = 6
		}
	case "peers":
		if dstIP == "" {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "dst_ip is required for peer QoS", nil)
			return
		}
		body.SrcPort = 0
		body.DstPort = 0
		body.Protocol = 0
	case "ip":
		dstIP = ""
		body.SrcPort = 0
		body.DstPort = 0
		body.Protocol = 0
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	created, err := r.store.CreateTenantNodeQoSRule(&controllerstorage.QoSRuleRecord{
		TenantID:      tenantID,
		NodeID:        node.ID,
		Category:      category,
		SrcCIDR:       srcIP,
		DstCIDR:       dstIP,
		SrcPort:       body.SrcPort,
		DstPort:       body.DstPort,
		Protocol:      body.Protocol,
		BandwidthMbps: bandwidth,
		Enabled:       enabled,
		Description:   body.Description,
	})
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeLimitApplyFailed, "Failed to create QoS rule: "+err.Error(), nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "qos", "create", map[string]interface{}{
		"id":             created.ID.String(),
		"node_id":        node.ID.String(),
		"tenant_id":      tenantID.String(),
		"category":       category,
		"src_ip":         created.SrcCIDR,
		"dst_ip":         created.DstCIDR,
		"src_port":       created.SrcPort,
		"dst_port":       created.DstPort,
		"protocol":       created.Protocol,
		"bandwidth_mbps": created.BandwidthMbps,
		"enabled":        created.Enabled,
		"description":    created.Description,
	}, "QoS rule created successfully", map[string]interface{}{
		"node_id":  node.ID.String(),
		"rule_id":  created.ID.String(),
		"category": category,
	})
}

func (r *Router) deleteTenantNodeQoS(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, category, id string) {
	if err := controllerstorage.ValidateQoSCategory(category); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	ruleID, err := uuid.Parse(id)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid QoS rule ID", nil)
		return
	}

	if err := r.store.DeleteTenantNodeQoSRule(tenantID, node.ID, category, ruleID); err != nil {
		if err == sql.ErrNoRows {
			v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "QoS rule not found", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeLimitApplyFailed, "Failed to delete QoS rule", nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "qos", "delete", map[string]interface{}{
		"id":       ruleID.String(),
		"status":   "deleted",
		"node_id":  node.ID.String(),
		"category": category,
	}, "QoS rule deleted successfully", map[string]interface{}{
		"node_id":  node.ID.String(),
		"rule_id":  ruleID.String(),
		"category": category,
	})
}

func (r *Router) listTenantNodeBlacklistRules(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, scope string) {
	rules, err := r.store.ListTenantNodeBlacklistRules(tenantID, node.ID, scope)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetACLRulesFailed, "Failed to list blacklist rules", nil)
		return
	}

	items := make([]map[string]interface{}, 0, len(rules))
	for _, rule := range rules {
		items = append(items, map[string]interface{}{
			"id":          rule.ID.String(),
			"tenant_id":   tenantID.String(),
			"node_id":     node.ID.String(),
			"scope":       rule.Scope,
			"cidr":        rule.CIDR,
			"port":        rule.Port,
			"enabled":     rule.Enabled,
			"description": rule.Description,
			"created_at":  rule.CreatedAt,
			"updated_at":  rule.UpdatedAt,
		})
	}

	v1.WriteSuccess(w, items, fmt.Sprintf("%d blacklist rules retrieved", len(items)))
}

func (r *Router) createTenantNodeBlacklistRule(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node, scope string) {
	var body struct {
		CIDR        string `json:"cidr,omitempty"`
		SrcIP       string `json:"src_ip,omitempty"`
		DstIP       string `json:"dst_ip,omitempty"`
		Port        int    `json:"port,omitempty"`
		Enabled     *bool  `json:"enabled,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	record := &controllerstorage.BlacklistRuleRecord{
		TenantID:    tenantID,
		NodeID:      node.ID,
		Scope:       scope,
		Description: body.Description,
		Enabled:     true,
	}
	if body.Enabled != nil {
		record.Enabled = *body.Enabled
	}

	switch scope {
	case controllerstorage.BlacklistScopeSrc:
		record.CIDR = strings.TrimSpace(firstNonEmpty(body.CIDR, body.SrcIP))
		if record.CIDR == "" {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeValidationFailed, "cidr is required for source blacklist", nil)
			return
		}
	case controllerstorage.BlacklistScopeDst:
		record.CIDR = strings.TrimSpace(firstNonEmpty(body.CIDR, body.DstIP))
		if record.CIDR == "" {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeValidationFailed, "cidr is required for destination blacklist", nil)
			return
		}
	case controllerstorage.BlacklistScopePorts:
		if body.Port <= 0 || body.Port > 65535 {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeValidationFailed, "port must be between 1 and 65535", nil)
			return
		}
		record.Port = body.Port
	}

	created, err := r.store.CreateTenantNodeBlacklistRule(record)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeUpdateNodeFailed, "Failed to create blacklist rule: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]interface{}{
		"id":          created.ID.String(),
		"tenant_id":   tenantID.String(),
		"node_id":     node.ID.String(),
		"scope":       created.Scope,
		"cidr":        created.CIDR,
		"port":        created.Port,
		"enabled":     created.Enabled,
		"description": created.Description,
	}, "Blacklist rule created successfully")
}

func (r *Router) deleteTenantNodeBlacklistRule(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node, scope, identifier string) {
	if scope == controllerstorage.BlacklistScopePorts {
		if port, err := parsePortIdentifier(identifier); err == nil {
			if err := r.store.DeleteTenantNodePortBlacklistRule(tenantID, node.ID, port); err != nil {
				if err == sql.ErrNoRows {
					v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Blacklist rule not found", nil)
					return
				}
				v1.WriteError(w, http.StatusInternalServerError, v1.CodeUpdateNodeFailed, "Failed to delete blacklist rule", nil)
				return
			}
			v1.WriteSuccess(w, map[string]string{"port": identifier, "status": "deleted"}, "Blacklist rule deleted successfully")
			return
		}
	}

	ruleID, err := uuid.Parse(identifier)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid blacklist rule identifier", nil)
		return
	}

	if err := r.store.DeleteTenantNodeBlacklistRuleByID(tenantID, node.ID, scope, ruleID); err != nil {
		if err == sql.ErrNoRows {
			v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Blacklist rule not found", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeUpdateNodeFailed, "Failed to delete blacklist rule", nil)
		return
	}

	v1.WriteSuccess(w, map[string]string{"id": ruleID.String(), "status": "deleted"}, "Blacklist rule deleted successfully")
}

func (r *Router) handleTenantNodeRoutes(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, parts []string) {
	if len(parts) < 7 || parts[6] != "routes" {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid route path", nil)
		return
	}

	nodeID := parts[5]
	switch {
	case len(parts) == 7:
		if req.Method == http.MethodGet {
			if !r.authorizeTenant(w, req, tenantID, false) {
				return
			}
			r.listTenantNodeRoutes(w, tenantID, nodeID)
			return
		}
		if !r.authorizeTenantAdmin(w, req, tenantID) {
			return
		}
		switch req.Method {
		case http.MethodPost:
			r.addTenantNodeRoute(w, req, tenantID, nodeID)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
	case len(parts) == 8:
		routeID, err := url.PathUnescape(parts[7])
		if err != nil {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid route identifier", nil)
			return
		}
		if req.Method == http.MethodGet {
			if !r.authorizeTenant(w, req, tenantID, false) {
				return
			}
			r.getTenantNodeRoute(w, tenantID, nodeID, routeID)
			return
		}
		if !r.authorizeTenantAdmin(w, req, tenantID) {
			return
		}
		switch req.Method {
		case http.MethodPut:
			r.replaceTenantNodeRoute(w, req, tenantID, nodeID, routeID)
		case http.MethodDelete:
			r.deleteTenantNodeRoute(w, tenantID, nodeID, routeID)
		default:
			v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		}
	default:
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid route path", nil)
	}
}

func (r *Router) listTenantNodeRoutes(w http.ResponseWriter, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	routes := make([]map[string]interface{}, 0, len(node.AdvertisedRoutes))
	for _, route := range node.AdvertisedRoutes {
		routes = append(routes, map[string]interface{}{
			"id":        route,
			"cidr":      route,
			"tenant_id": tenantID.String(),
			"node_id":   node.ID.String(),
			"node_name": firstNonEmpty(node.Hostname, node.PublicKey, node.ID.String()),
		})
	}
	if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "route", routes, "id"); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetNodesFailed, "Failed to load route delivery history", nil)
		return
	}
	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		attachNodeSummaryToPolicyItems(routes, summary)
	}

	v1.WriteSuccess(w, routes, fmt.Sprintf("%d routes retrieved", len(routes)))
}

func (r *Router) getTenantNodeRoute(w http.ResponseWriter, tenantID uuid.UUID, nodeID, routeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	for _, route := range node.AdvertisedRoutes {
		if route == routeID {
			v1.WriteSuccess(w, map[string]string{
				"id":   route,
				"cidr": route,
			}, "Route retrieved successfully")
			return
		}
	}

	v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Route not found", nil)
}

func (r *Router) addTenantNodeRoute(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	route, err := decodeRouteBody(req)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	routes := append(append([]string{}, node.AdvertisedRoutes...), route)
	normalized, err := normalizeRoutes(routes)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	if containsString(node.AdvertisedRoutes, route) {
		v1.WriteSuccess(w, map[string]string{"id": route, "cidr": route}, "Route already exists")
		return
	}

	if err := r.updateTenantNodeRoutes(node.ID, tenantID, normalized); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeUpdateNodeFailed, "Failed to add route", nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "route", "create", map[string]interface{}{
		"id":      route,
		"cidr":    route,
		"node_id": node.ID.String(),
	}, "Route created successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"route":   route,
	})
}

func (r *Router) replaceTenantNodeRoute(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, nodeID, routeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	if !containsString(node.AdvertisedRoutes, routeID) {
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Route not found", nil)
		return
	}

	newRoute, err := decodeRouteBody(req)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	updated := make([]string, 0, len(node.AdvertisedRoutes))
	for _, route := range node.AdvertisedRoutes {
		if route == routeID {
			updated = append(updated, newRoute)
			continue
		}
		updated = append(updated, route)
	}

	normalized, err := normalizeRoutes(updated)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, err.Error(), nil)
		return
	}

	if err := r.updateTenantNodeRoutes(node.ID, tenantID, normalized); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeUpdateNodeFailed, "Failed to update route", nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "route", "update", map[string]interface{}{
		"id":       newRoute,
		"cidr":     newRoute,
		"node_id":  node.ID.String(),
		"previous": routeID,
	}, "Route updated successfully", map[string]interface{}{
		"node_id":  node.ID.String(),
		"route":    newRoute,
		"previous": routeID,
	})
}

func (r *Router) deleteTenantNodeRoute(w http.ResponseWriter, tenantID uuid.UUID, nodeID, routeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	if !containsString(node.AdvertisedRoutes, routeID) {
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Route not found", nil)
		return
	}

	updated := make([]string, 0, len(node.AdvertisedRoutes))
	for _, route := range node.AdvertisedRoutes {
		if route != routeID {
			updated = append(updated, route)
		}
	}

	if err := r.updateTenantNodeRoutes(node.ID, tenantID, updated); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeUpdateNodeFailed, "Failed to delete route", nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "route", "delete", map[string]interface{}{
		"id":      routeID,
		"status":  "deleted",
		"node_id": node.ID.String(),
	}, "Route deleted successfully", map[string]interface{}{
		"node_id": node.ID.String(),
		"route":   routeID,
	})
}

func (r *Router) getTenantNodeRecord(nodeID string, tenantID uuid.UUID) (*controllerstorage.Node, error) {
	nodeUUID, err := uuid.Parse(nodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node id: %w", err)
	}

	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role,
		        COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0),
		        advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at
		   FROM nodes WHERE id = $1 AND tenant_id = $2`
	row := r.store.DB().QueryRow(query, nodeUUID, tenantID)

	var node controllerstorage.Node
	var advertisedRoutes pq.StringArray
	if err := row.Scan(
		&node.ID, &node.PublicKey, &node.MachineID, &node.TenantID, &node.Endpoint, &node.PrivateIP, &node.PublicIP,
		&node.Region, &node.VPCID, &node.Hostname, &node.AssignedIP, &node.IPOffset, &node.LastSeen, &node.RegisteredAt,
		&node.Role, &node.RuntimeMode, &node.KernelVersion, &node.HasAESNI, &node.Status, &node.OfflineSince,
		&advertisedRoutes, &node.EnrolledWithToken, &node.CreatedAt, &node.UpdatedAt,
	); err != nil {
		return nil, err
	}

	node.AdvertisedRoutes = []string(advertisedRoutes)
	return &node, nil
}

func (r *Router) updateTenantNodeRoutes(nodeID, tenantID uuid.UUID, routes []string) error {
	_, err := r.store.DB().Exec(
		`UPDATE nodes SET advertised_routes = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
		pq.Array(routes),
		nodeID,
		tenantID,
	)
	return err
}

func (r *Router) writeNodeLookupError(w http.ResponseWriter, err error) {
	if err == sql.ErrNoRows {
		v1.WriteError(w, http.StatusNotFound, v1.CodeNodeNotFound, "Node not found", nil)
		return
	}
	if strings.Contains(err.Error(), "invalid node id") {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid node ID", nil)
		return
	}
	v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetNodesFailed, "Failed to load node", nil)
}

func decodeRouteBody(req *http.Request) (string, error) {
	var body struct {
		CIDR  string `json:"cidr"`
		Route string `json:"route"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("invalid request body")
	}

	route := strings.TrimSpace(body.CIDR)
	if route == "" {
		route = strings.TrimSpace(body.Route)
	}
	if route == "" {
		return "", fmt.Errorf("cidr is required")
	}
	if _, _, err := net.ParseCIDR(route); err != nil {
		return "", fmt.Errorf("invalid cidr")
	}
	return route, nil
}

func normalizeRoutes(routes []string) ([]string, error) {
	normalized := make([]string, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		route = strings.TrimSpace(route)
		if route == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(route); err != nil {
			return nil, fmt.Errorf("invalid cidr: %s", route)
		}
		if _, exists := seen[route]; exists {
			continue
		}
		seen[route] = struct{}{}
		normalized = append(normalized, route)
	}
	return normalized, nil
}

func normalizeNodeRef(value string, node *controllerstorage.Node) string {
	value = strings.TrimSpace(value)
	switch value {
	case "", "self", node.ID.String(), node.Hostname:
		return node.PublicKey
	default:
		return value
	}
}

func nodeIPCandidates(node *controllerstorage.Node) []string {
	seen := map[string]struct{}{}
	var values []string
	for _, candidate := range []string{node.AssignedIP, node.PrivateIP, node.PublicIP} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		values = append(values, candidate)
	}
	return values
}

func primaryNodeIP(node *controllerstorage.Node) string {
	candidates := nodeIPCandidates(node)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func nullableInt(value int) interface{} {
	if value == 0 {
		return nil
	}
	return value
}

type byteReadCloser struct {
	*strings.Reader
}

func (b *byteReadCloser) Close() error {
	return nil
}

func newReadCloser(body []byte) *byteReadCloser {
	return &byteReadCloser{Reader: strings.NewReader(string(body))}
}

func (r *Router) queueNodePolicySync(
	node *controllerstorage.Node,
	domain string,
	action string,
	policyRef string,
	policyName string,
	metadata map[string]interface{},
) (map[string]interface{}, *controllerstorage.PolicyDelivery, error) {
	desiredVersion := controllerstorage.NewDesiredStateVersion()
	desiredMetadata := clonePolicyMetadata(metadata)
	desiredMetadata["domain"] = domain
	desiredMetadata["action"] = action
	desiredMetadata["policy_ref"] = strings.TrimSpace(policyRef)
	if name := strings.TrimSpace(policyName); name != "" {
		desiredMetadata["policy_name"] = name
	}
	controlState, err := r.store.UpsertNodeDesiredState(node.TenantID, node.ID, desiredVersion, desiredMetadata)
	if err != nil {
		return nil, nil, err
	}

	params := map[string]interface{}{
		"domain":                domain,
		"action":                action,
		"node_id":               node.ID.String(),
		"hostname":              node.Hostname,
		"desired_state_version": desiredVersion,
	}
	for key, value := range metadata {
		params[key] = value
	}

	cmd, err := r.store.QueueAgentCommand(node.PublicKey, "sync", params, 1, 60)
	if err != nil {
		return nil, nil, err
	}

	dispatch := map[string]interface{}{
		"command_id":            cmd.ID,
		"command":               cmd.Command,
		"status":                cmd.Status,
		"message":               "Policy sync queued",
		"created_at":            cmd.CreatedAt,
		"timeout_seconds":       cmd.TimeoutSeconds,
		"desired_state_version": desiredVersion,
	}
	if controlState != nil {
		dispatch["desired_state_updated_at"] = controlState.DesiredStateUpdatedAt
	}

	policyRef = strings.TrimSpace(policyRef)
	if policyRef == "" {
		return dispatch, nil, nil
	}

	deliveryMetadata := clonePolicyMetadata(metadata)
	deliveryMetadata["domain"] = domain
	deliveryMetadata["action"] = action
	deliveryMetadata["command"] = cmd.Command
	deliveryMetadata["desired_state_version"] = desiredVersion

	delivery, err := r.store.CreatePolicyDelivery(&controllerstorage.PolicyDelivery{
		TenantID:      node.TenantID,
		NodeID:        node.ID,
		PolicyDomain:  domain,
		PolicyRef:     policyRef,
		PolicyName:    strings.TrimSpace(policyName),
		Action:        action,
		CommandID:     cmd.ID,
		CommandStatus: cmd.Status,
		Metadata:      deliveryMetadata,
	})
	if err != nil {
		return nil, nil, err
	}

	dispatch["delivery_id"] = delivery.ID.String()
	dispatch["policy_ref"] = delivery.PolicyRef
	if delivery.PolicyName != "" {
		dispatch["policy_name"] = delivery.PolicyName
	}
	dispatch["last_delivery"] = policyDeliveryToMap(delivery)

	return dispatch, delivery, nil
}

func (r *Router) writePolicyMutationSuccess(
	w http.ResponseWriter,
	node *controllerstorage.Node,
	domain string,
	action string,
	data map[string]interface{},
	message string,
	metadata map[string]interface{},
) {
	policyRef, policyName := inferPolicyDeliveryIdentity(domain, data, metadata)
	dispatch, delivery, err := r.queueNodePolicySync(node, domain, action, policyRef, policyName, metadata)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, codeCommandDispatchFailed, "Policy updated but sync dispatch failed: "+err.Error(), nil)
		return
	}

	if data == nil {
		data = map[string]interface{}{}
	}
	data["dispatch"] = dispatch
	if delivery != nil {
		data["last_delivery"] = policyDeliveryToMap(delivery)
		data["delivery_history"] = []map[string]interface{}{policyDeliveryToMap(delivery)}
		data["policy_status"] = mapCommandStatusToPolicyStatus(delivery.CommandStatus)
		data["pending_cmds"] = pendingCountForCommandStatus(delivery.CommandStatus)
		data["last_delivery_error"] = delivery.LastError
		data["last_delivery_command_id"] = delivery.CommandID
		data["last_delivery_action"] = delivery.Action
		data["last_delivery_at"] = delivery.UpdatedAt
	}
	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		if _, exists := data["policy_status"]; !exists {
			data["policy_status"] = summary["configuration_status"]
		}
		if _, exists := data["pending_cmds"]; !exists {
			data["pending_cmds"] = summary["pending_cmds"]
		}
		data["desired_state_version"] = summary["desired_state_version"]
		data["desired_state_updated_at"] = summary["desired_state_updated_at"]
		data["applied_state_version"] = summary["applied_state_version"]
		data["applied_state_updated_at"] = summary["applied_state_updated_at"]
		data["observed_state"] = summary["observed_state"]
		data["observed_message"] = summary["observed_message"]
		data["observed_at"] = summary["observed_at"]
		data["state_convergence"] = summary["state_convergence"]
		data["last_sync_error"] = summary["last_sync_error"]
		data["last_command_error"] = summary["last_command_error"]
	}

	v1.WriteSuccess(w, data, message)
}

func (r *Router) proxyLegacyPolicyMutation(
	w http.ResponseWriter,
	req *http.Request,
	node *controllerstorage.Node,
	domain string,
	action string,
	metadata map[string]interface{},
	handler func(http.ResponseWriter, *http.Request),
) {
	recorder := httptest.NewRecorder()
	handler(recorder, req)

	if recorder.Code >= http.StatusBadRequest {
		copyCapturedResponse(w, recorder)
		return
	}

	var payload v1.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || !payload.Success {
		copyCapturedResponse(w, recorder)
		return
	}

	dispatchData := map[string]interface{}{}
	if existing, ok := payload.Data.(map[string]interface{}); ok {
		for key, value := range existing {
			dispatchData[key] = value
		}
	} else if payload.Data != nil {
		dispatchData["resource"] = payload.Data
	}

	policyRef, policyName := inferPolicyDeliveryIdentity(domain, dispatchData, metadata)
	dispatch, delivery, err := r.queueNodePolicySync(node, domain, action, policyRef, policyName, metadata)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, codeCommandDispatchFailed, "Policy updated but sync dispatch failed: "+err.Error(), nil)
		return
	}

	data := map[string]interface{}{}
	for key, value := range dispatchData {
		data[key] = value
	}
	data["dispatch"] = dispatch
	if delivery != nil {
		data["last_delivery"] = policyDeliveryToMap(delivery)
		data["delivery_history"] = []map[string]interface{}{policyDeliveryToMap(delivery)}
		data["policy_status"] = mapCommandStatusToPolicyStatus(delivery.CommandStatus)
		data["pending_cmds"] = pendingCountForCommandStatus(delivery.CommandStatus)
		data["last_delivery_error"] = delivery.LastError
		data["last_delivery_command_id"] = delivery.CommandID
		data["last_delivery_action"] = delivery.Action
		data["last_delivery_at"] = delivery.UpdatedAt
	}
	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		if _, exists := data["policy_status"]; !exists {
			data["policy_status"] = summary["configuration_status"]
		}
		if _, exists := data["pending_cmds"]; !exists {
			data["pending_cmds"] = summary["pending_cmds"]
		}
		data["desired_state_version"] = summary["desired_state_version"]
		data["desired_state_updated_at"] = summary["desired_state_updated_at"]
		data["applied_state_version"] = summary["applied_state_version"]
		data["applied_state_updated_at"] = summary["applied_state_updated_at"]
		data["observed_state"] = summary["observed_state"]
		data["observed_message"] = summary["observed_message"]
		data["observed_at"] = summary["observed_at"]
		data["state_convergence"] = summary["state_convergence"]
		data["last_sync_error"] = summary["last_sync_error"]
		data["last_command_error"] = summary["last_command_error"]
	}

	payload.Data = data
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(recorder.Code)
	_ = json.NewEncoder(w).Encode(payload)
}

func copyCapturedResponse(w http.ResponseWriter, recorder *httptest.ResponseRecorder) {
	for key, values := range recorder.Header() {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(recorder.Code)
	_, _ = w.Write(recorder.Body.Bytes())
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (r *Router) authorizeTenant(w http.ResponseWriter, req *http.Request, targetTenantID uuid.UUID, requireAdmin bool) bool {
	role, exists := middleware.GetUserRole(req.Context())
	if !exists {
		v1.WriteError(w, http.StatusUnauthorized, v1.CodeUnauthorized, "Unauthorized", nil)
		return false
	}

	if role == "super_admin" {
		return true
	}

	userTenantID, ok := middleware.GetTenantID(req.Context())
	if !ok {
		v1.WriteError(w, http.StatusUnauthorized, v1.CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return false
	}

	if userTenantID != targetTenantID {
		v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "Permission denied: cannot access other tenant", nil)
		return false
	}

	if requireAdmin && role != "admin" && role != "owner" {
		v1.WriteError(w, http.StatusForbidden, v1.CodeAccessDenied, "Permission denied: admin role required", nil)
		return false
	}

	return true
}

func (r *Router) authorizeTenantAdmin(w http.ResponseWriter, req *http.Request, targetTenantID uuid.UUID) bool {
	return r.authorizeTenant(w, req, targetTenantID, true)
}

func withTenantContext(req *http.Request, tenantID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID.String())
	return req.WithContext(ctx)
}

func splitTenantPath(path string) (string, string, bool) {
	trimmed := strings.TrimPrefix(path, "/api/v2/tenants/")
	if trimmed == path || trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 1 {
		return parts[0], "", true
	}
	return parts[0], parts[1], true
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func cloneRequestWithPath(req *http.Request, path string) *http.Request {
	cloned := req.Clone(req.Context())
	cloned.URL.Path = path
	return cloned
}

func attachPolicyDeliveriesToItems(
	store *controllerstorage.Storage,
	tenantID uuid.UUID,
	nodeID uuid.UUID,
	domain string,
	items []map[string]interface{},
	refField string,
) error {
	if len(items) == 0 {
		return nil
	}

	limit := len(items) * 10
	if limit < 100 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	deliveries, err := store.ListPolicyDeliveriesByNodeAndDomain(tenantID, nodeID, domain, limit)
	if err != nil {
		return err
	}

	byRef := make(map[string][]*controllerstorage.PolicyDelivery, len(deliveries))
	for _, delivery := range deliveries {
		ref := strings.TrimSpace(delivery.PolicyRef)
		if ref == "" {
			continue
		}
		byRef[ref] = append(byRef[ref], delivery)
	}

	for _, item := range items {
		ref := stringifyPolicyValue(item[refField])
		if ref == "" {
			item["delivery_history"] = []map[string]interface{}{}
			continue
		}

		history := byRef[ref]
		if len(history) == 0 {
			item["delivery_history"] = []map[string]interface{}{}
			if _, exists := item["policy_status"]; !exists {
				item["policy_status"] = "idle"
			}
			if _, exists := item["pending_cmds"]; !exists {
				item["pending_cmds"] = 0
			}
			continue
		}

		historyItems := make([]map[string]interface{}, 0, minInt(len(history), 5))
		for idx, delivery := range history {
			if idx >= 5 {
				break
			}
			historyItems = append(historyItems, policyDeliveryToMap(delivery))
		}

		lastDelivery := history[0]
		item["last_delivery"] = policyDeliveryToMap(lastDelivery)
		item["delivery_history"] = historyItems
		item["policy_status"] = mapCommandStatusToPolicyStatus(lastDelivery.CommandStatus)
		item["pending_cmds"] = pendingCountForCommandStatus(lastDelivery.CommandStatus)
		item["last_delivery_error"] = lastDelivery.LastError
		item["last_delivery_command_id"] = lastDelivery.CommandID
		item["last_delivery_action"] = lastDelivery.Action
		item["last_delivery_at"] = lastDelivery.UpdatedAt
	}

	return nil
}

func policyDeliveryToMap(delivery *controllerstorage.PolicyDelivery) map[string]interface{} {
	if delivery == nil {
		return nil
	}

	payload := map[string]interface{}{
		"id":             delivery.ID.String(),
		"tenant_id":      delivery.TenantID.String(),
		"node_id":        delivery.NodeID.String(),
		"policy_domain":  delivery.PolicyDomain,
		"policy_ref":     delivery.PolicyRef,
		"policy_name":    delivery.PolicyName,
		"action":         delivery.Action,
		"command_id":     delivery.CommandID,
		"command_status": delivery.CommandStatus,
		"last_error":     delivery.LastError,
		"metadata":       delivery.Metadata,
		"created_at":     delivery.CreatedAt,
		"updated_at":     delivery.UpdatedAt,
	}
	if desiredStateVersion := stringifyPolicyValue(delivery.Metadata["desired_state_version"]); desiredStateVersion != "" {
		payload["desired_state_version"] = desiredStateVersion
	}
	if delivery.CompletedAt != nil {
		payload["completed_at"] = delivery.CompletedAt
	}

	return payload
}

func inferPolicyDeliveryIdentity(domain string, data map[string]interface{}, metadata map[string]interface{}) (string, string) {
	candidates := []map[string]interface{}{data}
	for _, nestedKey := range []string{"rule", "resource"} {
		if nested, ok := data[nestedKey].(map[string]interface{}); ok && nested != nil {
			candidates = append([]map[string]interface{}{nested}, candidates...)
		}
	}
	if metadata != nil {
		candidates = append(candidates, metadata)
	}

	refKeys := []string{"id", "rule_id", "policy_ref"}
	nameKeys := []string{"name", "rule_name", "policy_name", "description"}
	switch domain {
	case "route":
		refKeys = append([]string{"cidr", "route"}, refKeys...)
		nameKeys = append([]string{"cidr", "route"}, nameKeys...)
	case "qos":
		nameKeys = append([]string{"category"}, nameKeys...)
	}

	var policyRef string
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if value := firstMapString(candidate, refKeys...); value != "" {
			policyRef = value
			break
		}
	}

	var policyName string
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if value := firstMapString(candidate, nameKeys...); value != "" {
			policyName = value
			break
		}
	}

	return policyRef, policyName
}

func clonePolicyMetadata(metadata map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return map[string]interface{}{}
	}

	cloned := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func firstMapString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, exists := values[key]; exists {
			if result := stringifyPolicyValue(value); result != "" {
				return result
			}
		}
	}
	return ""
}

func stringifyPolicyValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case int:
		return strconv.Itoa(typed)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float32:
		if typed == float32(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func mapCommandStatusToPolicyStatus(status string) string {
	switch status {
	case controllerstorage.AgentCommandStatusPending:
		return "pending"
	case controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged:
		return "in_progress"
	case controllerstorage.AgentCommandStatusCompleted:
		return "applied"
	case controllerstorage.AgentCommandStatusFailed:
		return "error"
	default:
		return "idle"
	}
}

func pendingCountForCommandStatus(status string) int {
	switch status {
	case controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged:
		return 1
	default:
		return 0
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parsePortIdentifier(raw string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return port, nil
}
