package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"aria/internal/api/apibase"
	"aria/internal/api/handlers"
	"aria/internal/api/middleware"
	"aria/internal/service"
	"aria/internal/token"
	"aria/pkg/controllerstorage"
	"aria/pkg/victoriametrics"
)

type Router struct {
	store      *controllerstorage.Storage
	authAPI    *handlers.AuthAPI
	tenantAPI  *handlers.TenantAPI
	chatAPI    *handlers.ChatHandler
	tokenStore *token.Store
	vmClient   *victoriametrics.Client
}

func SetupRoutes(mux *http.ServeMux, store *controllerstorage.Storage, vmClient *victoriametrics.Client) {
	tokenStore := token.NewStore(store.DB())
	aiService := service.NewAIService(store)

	router := &Router{
		store:      store,
		authAPI:    handlers.NewAuthAPI(store),
		tenantAPI:  handlers.NewTenantAPI(store),
		chatAPI:    handlers.NewChatHandler(aiService),
		tokenStore: tokenStore,
		vmClient:   vmClient,
	}

	withJWT := middleware.JWTAuthMiddleware
	mux.HandleFunc("/api/v2/auth/login", router.authAPI.HandleLogin)
	mux.HandleFunc("/api/v2/auth/refresh", router.authAPI.HandleRefresh)
	mux.HandleFunc("/api/v2/auth/logout", router.authAPI.HandleLogout)
	mux.HandleFunc("/api/v2/auth/force-change-password", router.authAPI.HandleForceChangePassword)
	mux.HandleFunc("/api/v2/auth/permissions", withJWT(router.authAPI.HandlePermissions))

	mux.HandleFunc("/api/v2/tenants", withJWT(router.HandleTenants))
	mux.HandleFunc("/api/v2/tenants/", withJWT(router.HandleTenantScoped))
	mux.HandleFunc("/api/v2/settings/", withJWT(router.HandleSettings))
}

func (r *Router) HandleTenants(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/api/v2/tenants" {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown endpoint", nil)
		return
	}

	role, exists := middleware.GetUserRole(req.Context())
	if !exists {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied", nil)
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
				apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
				return
			}
			r.listSingleTenant(w, tenantID)
			return
		}
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied: admin or super_admin only", nil)
	case http.MethodPost:
		if role != "super_admin" {
			apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied: super_admin only", nil)
			return
		}
		r.tenantAPI.CreateTenant(w, req)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) HandleTenantScoped(w http.ResponseWriter, req *http.Request) {
	tenantIDStr, rest, ok := splitTenantPath(req.URL.Path)
	if !ok {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid tenant path", nil)
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidTenantID, "Invalid tenant ID", nil)
		return
	}

	role, exists := middleware.GetUserRole(req.Context())
	if !exists {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied", nil)
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
	case strings.HasPrefix(rest, "roles"):
		r.handleTenantRoles(w, req, tenantID)
	case strings.HasPrefix(rest, "nodes"):
		r.handleTenantNodes(w, req, tenantID, role)
	case strings.HasPrefix(rest, "agents"):
		r.handleTenantAgents(w, req, tenantID)
	case strings.HasPrefix(rest, "monitoring"):
		r.handleTenantMonitoring(w, req, tenantID)
	case strings.HasPrefix(rest, "ai"):
		r.handleTenantAI(w, req, tenantID)
	default:
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown endpoint", nil)
	}
}

func (r *Router) handleTenantRoles(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	requiredPermission := middleware.PermRolesRead
	if req.Method != http.MethodGet {
		requiredPermission = middleware.PermRolesWrite
	}
	if !r.authorizeTenantPermission(w, req, tenantID, requiredPermission) {
		return
	}

	parts := splitPath(req.URL.Path)
	// /api/v2/tenants/{tid}/roles
	// /api/v2/tenants/{tid}/roles/{rid}
	roleIndex := -1
	for i, part := range parts {
		if part == "roles" {
			roleIndex = i
			break
		}
	}
	if roleIndex >= 0 {
		if len(parts) > roleIndex+1 && parts[roleIndex+1] != "" {
			r.handleRoleDetail(w, req, tenantID, parts[roleIndex+1])
			return
		}
		r.handleRoles(w, req, tenantID, parts)
		return
	}
	apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown roles endpoint", nil)
}

func (r *Router) handleTenantPolicies(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermPoliciesRead) {
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.listTenantPolicies(w, req, tenantID)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) handleSingleTenant(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	switch req.Method {
	case http.MethodGet:
		if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermSettingsRead) {
			return
		}
		r.tenantAPI.GetTenant(w, withTenantContext(req, tenantID))
	case http.MethodPut:
		if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermSettingsWrite) {
			return
		}
		r.updateTenant(w, req, tenantID, role)
	case http.MethodDelete:
		if role != "super_admin" {
			apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied: super_admin only", nil)
			return
		}
		if _, err := r.store.DB().Exec(`UPDATE tenants SET status = 'deleted', updated_at = NOW() WHERE id = $1`, tenantID); err != nil {
			apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeDeleteTokenFailed, "Failed to delete tenant: "+err.Error(), nil)
			return
		}
		apibase.WriteSuccess(w, map[string]string{"id": tenantID.String(), "status": "deleted"}, "Tenant deleted successfully")
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) handleTenantUsers(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	requiredPermission := middleware.PermUsersRead
	if req.Method != http.MethodGet {
		requiredPermission = middleware.PermUsersWrite
	}
	if !r.authorizeTenantPermission(w, req, tenantID, requiredPermission) {
		return
	}

	parts := splitPath(req.URL.Path)
	if len(parts) == 6 && req.Method == http.MethodDelete {
		currentUserID, ok := middleware.GetUserID(req.Context())
		if ok && currentUserID == parts[5] {
			apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "You cannot delete your own account", nil)
			return
		}
	}

	req2 := withTenantContext(req, tenantID)
	r.tenantAPI.HandleTenantUsers(w, req2)
}

func (r *Router) handleTenantNodes(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	parts := splitPath(req.URL.Path)
	req2 := withTenantContext(req, tenantID)

	// GET /api/v2/tenants/{tid}/nodes
	if len(parts) == 5 {
		if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermNodesRead) {
			return
		}
		if req.Method == http.MethodGet {
			r.listTenantNodes(w, tenantID)
		} else {
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	// 针对单个节点的操作 /api/v2/tenants/{tid}/nodes/{nid}/...
	if len(parts) >= 6 {
		nodeID := parts[5]
		node, err := r.getTenantNodeRecord(nodeID, tenantID)
		if err != nil {
			r.writeNodeLookupError(w, err)
			return
		}

		// 检查子路径
		if len(parts) >= 7 {
			switch parts[6] {
			case "security":
				r.handleTenantNodeSecurity(w, req, tenantID, node, parts)
				return
			case "qos":
				r.handleTenantNodeQoS(w, req, tenantID, node, parts)
				return
			case "routes":
				r.handleTenantNodeRoutes(w, req, tenantID, parts)
				return
			case "agent":
				r.handleTenantNodeAgent(w, req, tenantID, parts)
				return
			}
		}

		// 如果没有子路径，则是对节点本身的 CRUD
		switch req.Method {
		case http.MethodGet:
			if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermNodesRead) {
				return
			}
			r.getTenantNodeByID(w, tenantID, nodeID)
		case http.MethodPut:
			if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermNodesWrite) {
				return
			}
			r.updateTenantNode(w, req2, tenantID, nodeID)
		case http.MethodDelete:
			if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermNodesWrite) {
				return
			}
			r.deleteTenantNode(w, tenantID, nodeID)
		default:
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid node path", nil)
}

func (r *Router) listTenantPolicies(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	nodes, err := r.store.GetNodesByTenant(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeGetNodesFailed, "Failed to load tenant nodes", nil)
		return
	}

	query := req.URL.Query()
	kindFilter := strings.TrimSpace(query.Get("kind"))
	nodeFilter := strings.TrimSpace(query.Get("node_id"))
	enabledFilter := strings.TrimSpace(query.Get("enabled"))

	var enabledPtr *bool
	if enabledFilter != "" {
		val := enabledFilter == "true"
		enabledPtr = &val
	}

	policies := make([]map[string]interface{}, 0)
	for _, node := range nodes {
		if nodeFilter != "" && node.ID.String() != nodeFilter {
			continue
		}

		if kindFilter == "" || kindFilter == "acl" {
			items, err := r.buildTenantNodeACLPolicies(tenantID, node)
			if err != nil {
				apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeGetACLRulesFailed, "Failed to load ACL policies", nil)
				return
			}
			policies = append(policies, items...)
		}

		if kindFilter == "" || kindFilter == "qos" {
			items, err := r.buildTenantNodeQoSPolicies(tenantID, node)
			if err != nil {
				apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeGetLimitsFailed, "Failed to load QoS policies", nil)
				return
			}
			policies = append(policies, items...)
		}

		if kindFilter == "" || kindFilter == "route" {
			items, err := r.buildTenantNodeRoutePolicies(tenantID, node)
			if err != nil {
				apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeGetNodesFailed, "Failed to load route policies", nil)
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

	apibase.WriteSuccess(w, policies, fmt.Sprintf("%d policies retrieved", len(policies)))
}

func (r *Router) listTenantNodes(w http.ResponseWriter, tenantID uuid.UUID) {
	nodes, err := r.store.GetNodesByTenant(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeGetNodesFailed, "Failed to get nodes", nil)
		return
	}

	items := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, r.buildTenantNodeResponse(node))
	}

	apibase.WriteSuccess(w, items, fmt.Sprintf("%d nodes retrieved", len(items)))
}

func (r *Router) listAllTenants(w http.ResponseWriter) {
	rows, err := r.store.DB().Query(`SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeListTenantsFailed, "Failed to list tenants", nil)
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
			apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeScanTenantFailed, "Failed to scan tenant", nil)
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
	apibase.WriteSuccess(w, tenants, "All tenants retrieved")
}

func (r *Router) listSingleTenant(w http.ResponseWriter, tenantID uuid.UUID) {
	var id uuid.UUID
	var name string
	var code sql.NullString
	var status string
	var quota string
	var createdAt, updatedAt interface{}

	err := r.store.DB().QueryRow(
		`SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`,
		tenantID,
	).Scan(&id, &name, &code, &status, &quota, &createdAt, &updatedAt)

	if err != nil {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeTenantNotFound, "Tenant not found", nil)
		return
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"id":             id.String(),
		"name":           name,
		"code":           code.String,
		"status":         status,
		"resource_quota": quota,
		"created_at":     createdAt,
		"updated_at":     updatedAt,
	}, "Tenant retrieved")
}

func (r *Router) updateTenant(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	var body struct {
		Name          string                 `json:"name"`
		Code          string                 `json:"code"`
		Status        string                 `json:"status"`
		ResourceQuota map[string]interface{} `json:"resource_quota"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}
	if strings.EqualFold(strings.TrimSpace(body.Status), "deleted") {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Tenant deletion must use DELETE /api/v2/tenants/{id}", nil)
		return
	}

	var resourceQuota string
	if body.ResourceQuota != nil {
		qb, _ := json.Marshal(body.ResourceQuota)
		resourceQuota = string(qb)
	}

	query := `UPDATE tenants 
		SET name = COALESCE(NULLIF($1, ''), name),
		    code = COALESCE(NULLIF($2, ''), code),
		    status = COALESCE(NULLIF($3, ''), status),
		    resource_quota = CASE WHEN $4 = '' THEN resource_quota ELSE $4 END,
		    updated_at = NOW()
		WHERE id = $5`
	if _, err := r.store.DB().Exec(query, body.Name, body.Code, body.Status, resourceQuota, tenantID); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeCreateTenantFailed, "Failed to update tenant: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]string{"id": tenantID.String()}, "Tenant updated successfully")
}

func (r *Router) getTenantNodeByID(w http.ResponseWriter, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	apibase.WriteSuccess(w, r.buildTenantNodeResponse(node), "Node detail retrieved")
}

func (r *Router) buildTenantNodeResponse(node *controllerstorage.Node) map[string]interface{} {
	response := map[string]interface{}{
		"id":                  node.ID.String(),
		"tenant_id":           node.TenantID.String(),
		"public_key":          node.PublicKey,
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
		response["convergence_status"] = summary["state_convergence"]
		response["last_sync_error"] = summary["last_sync_error"]
	} else {
		response["convergence_status"] = string(controllerstorage.StatusOffline)
		if nodeAvailabilityStatus(node) == "online" {
			response["convergence_status"] = string(controllerstorage.StatusConverged)
		}
	}

	return response
}

func (r *Router) deleteTenantNode(w http.ResponseWriter, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	if _, err := r.store.ApplyNodeLifecycleTransition(node.PublicKey, controllerstorage.NodeLifecycleTransition{
		TargetStatus:   "deleted",
		RevokeReason:   "node deleted via API",
		AuditEventType: "node_deleted",
		AuditActor:     "user",
		AuditSummary:   "Node deleted via API",
		AuditDetail: map[string]interface{}{
			"node_id": node.ID.String(),
		},
	}); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeUpdateNodeFailed, "Failed to delete node: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]string{"id": nodeID, "status": "deleted"}, "Node deleted successfully")
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
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	// 合并并去重路由
	routes := body.AdvertisedRoutes
	if routes == nil {
		routes = node.AdvertisedRoutes
	}

	role := body.Role
	if role == "" {
		role = node.Role
	}

	query := `UPDATE nodes SET 
		hostname = COALESCE(NULLIF($1, ''), hostname),
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
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeUpdateNodeFailed, "Failed to update node", nil)
		return
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"id":                node.ID.String(),
		"tenant_id":         tenantID.String(),
		"role":              role,
		"advertised_routes": routes,
	}, "Node updated successfully")
}

func (r *Router) handleTenantNodeRoutes(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, parts []string) {
	if len(parts) < 7 || parts[6] != "routes" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid route path", nil)
		return
	}

	requiredPermission := middleware.PermRoutesRead
	if req.Method != http.MethodGet {
		requiredPermission = middleware.PermRoutesWrite
	}
	if !r.authorizeTenantPermission(w, req, tenantID, requiredPermission) {
		return
	}

	nodeID := parts[5]
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	// 修复 BUG-15: 处理 CIDR 路由 ID，可能包含 '/'
	// parts[7:] 包含路由标识符
	routeID := ""
	if len(parts) >= 8 {
		routeID = strings.Join(parts[7:], "/")
	}

	if routeID == "" {
		switch req.Method {
		case http.MethodGet:
			r.listTenantNodeRoutes(w, tenantID, node)
		case http.MethodPost:
			r.addTenantNodeRoute(w, req, tenantID, node)
		default:
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	// 单条路由操作
	switch req.Method {
	case http.MethodGet:
		r.getTenantNodeRoute(w, tenantID, nodeID, routeID)
	case http.MethodPut:
		r.replaceTenantNodeRoute(w, req, tenantID, nodeID, routeID)
	case http.MethodDelete:
		r.deleteTenantNodeRoute(w, tenantID, nodeID, routeID)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (r *Router) listTenantNodeRoutes(w http.ResponseWriter, tenantID uuid.UUID, node *controllerstorage.Node) {
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
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeGetNodesFailed, "Failed to load route delivery history", nil)
		return
	}
	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		attachNodeSummaryToPolicyItems(routes, summary)
	}

	apibase.WriteSuccess(w, routes, fmt.Sprintf("%d routes retrieved", len(routes)))
}

func (r *Router) getTenantNodeRoute(w http.ResponseWriter, tenantID uuid.UUID, nodeID, routeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	if !containsString(node.AdvertisedRoutes, routeID) {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Route not found", nil)
		return
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"id":      routeID,
		"cidr":    routeID,
		"node_id": node.ID.String(),
	}, "Route retrieved")
}

func (r *Router) addTenantNodeRoute(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, node *controllerstorage.Node) {
	route, err := decodeRouteBody(req)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}

	routes := append(append([]string{}, node.AdvertisedRoutes...), route)
	normalized, err := normalizeRoutes(routes)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}

	if containsString(node.AdvertisedRoutes, route) {
		apibase.WriteSuccess(w, map[string]string{"id": route, "cidr": route}, "Route already exists")
		return
	}

	if err := r.updateTenantNodeRoutes(node.ID, tenantID, normalized); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeUpdateNodeFailed, "Failed to add route", nil)
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
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Route not found", nil)
		return
	}

	newRoute, err := decodeRouteBody(req)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
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
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}

	if err := r.updateTenantNodeRoutes(node.ID, tenantID, normalized); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeUpdateNodeFailed, "Failed to update route", nil)
		return
	}

	r.writePolicyMutationSuccess(w, node, "route", "update", map[string]interface{}{
		"id":       newRoute,
		"cidr":     newRoute,
		"node_id":  node.ID.String(),
		"previous": routeID,
	}, "Route updated successfully", map[string]interface{}{
		"node_id":   node.ID.String(),
		"new_route": newRoute,
		"old_route": routeID,
	})
}

func (r *Router) deleteTenantNodeRoute(w http.ResponseWriter, tenantID uuid.UUID, nodeID, routeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	if !containsString(node.AdvertisedRoutes, routeID) {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Route not found", nil)
		return
	}

	updated := make([]string, 0, len(node.AdvertisedRoutes))
	for _, route := range node.AdvertisedRoutes {
		if route != routeID {
			updated = append(updated, route)
		}
	}

	if err := r.updateTenantNodeRoutes(node.ID, tenantID, updated); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeUpdateNodeFailed, "Failed to delete route", nil)
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

	node, err := r.store.GetNodeByID(nodeUUID)
	if err != nil {
		return nil, err
	}
	// Storage returns (nil, nil) for missing rows; normalize to sql.ErrNoRows.
	if node == nil {
		return nil, sql.ErrNoRows
	}
	// Enforce tenant isolation at lookup stage.
	if node.TenantID != tenantID {
		return nil, sql.ErrNoRows
	}
	return node, nil
}

func (r *Router) writeNodeLookupError(w http.ResponseWriter, err error) {
	if err == sql.ErrNoRows {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeNodeNotFound, "Node not found", nil)
		return
	}
	if strings.Contains(err.Error(), "invalid node id") {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid node ID", nil)
		return
	}
	apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeGetNodesFailed, "Failed to load node", nil)
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

	policyRef = strings.TrimSpace(policyRef)

	deliveryMetadata := clonePolicyMetadata(metadata)
	deliveryMetadata["domain"] = domain
	deliveryMetadata["action"] = action
	deliveryMetadata["command"] = "sync"
	deliveryMetadata["desired_state_version"] = desiredVersion

	result, err := r.store.QueuePolicySync(controllerstorage.PolicySyncRequest{
		TenantID:            node.TenantID,
		NodeID:              node.ID,
		NodePublicKey:       node.PublicKey,
		Domain:              domain,
		Action:              action,
		PolicyRef:           policyRef,
		PolicyName:          strings.TrimSpace(policyName),
		DesiredStateVersion: desiredVersion,
		DesiredMetadata:     desiredMetadata,
		CommandParams:       params,
		DeliveryMetadata:    deliveryMetadata,
		Priority:            1,
		TimeoutSeconds:      60,
	})
	if err != nil {
		return nil, nil, err
	}

	dispatch := map[string]interface{}{
		"command_id":            result.Command.ID,
		"status":                result.Command.Status,
		"desired_state_version": result.DesiredStateVersion,
	}
	if result.ControlState != nil {
		dispatch["desired_state_updated_at"] = result.ControlState.DesiredStateUpdatedAt
	}
	if result.Delivery != nil {
		if result.Delivery.PolicyName != "" {
			dispatch["policy_name"] = result.Delivery.PolicyName
		}
		dispatch["last_delivery"] = policyDeliveryToMap(result.Delivery)
	}

	return dispatch, result.Delivery, nil
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

	if data == nil {
		data = map[string]interface{}{}
	}
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, message+": policy dispatch failed", map[string]string{
			"dispatch_error": err.Error(),
		})
		return
	}

	data["dispatch"] = dispatch
	if delivery != nil {
		data["last_delivery"] = policyDeliveryToMap(delivery)
		data["delivery_history"] = []map[string]interface{}{policyDeliveryToMap(delivery)}
	}

	if summary, err := r.buildNodeOperationsSummary(node); err == nil {
		data["last_command"] = summary["last_command"]
		data["last_command_status"] = summary["last_command_status"]
		data["last_command_error"] = summary["last_command_error"]
	}

	apibase.WriteSuccess(w, data, message)
}

func (r *Router) authorizeTenant(w http.ResponseWriter, req *http.Request, targetTenantID uuid.UUID, requireAdmin bool) bool {
	role, exists := middleware.GetUserRole(req.Context())
	if !exists {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeUnauthorized, "Unauthorized", nil)
		return false
	}

	if role == "super_admin" {
		return true
	}

	userTenantID, exists := middleware.GetTenantID(req.Context())
	if !exists {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeTenantContextNotFound, "Tenant context missing", nil)
		return false
	}

	if userTenantID != targetTenantID {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied to this tenant", nil)
		return false
	}

	if requireAdmin && role != "admin" && role != "owner" {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Admin privileges required", nil)
		return false
	}

	return true
}

func (r *Router) authorizeTenantAdmin(w http.ResponseWriter, req *http.Request, targetTenantID uuid.UUID) bool {
	return r.authorizeTenant(w, req, targetTenantID, true)
}

func (r *Router) authorizeTenantPermission(w http.ResponseWriter, req *http.Request, targetTenantID uuid.UUID, permission string) bool {
	if !r.authorizeTenant(w, req, targetTenantID, false) {
		return false
	}

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("RBAC_ENFORCEMENT")))
	if mode == "" {
		mode = "enforce"
	}
	if mode == "off" || permission == "" {
		return true
	}

	role, _ := middleware.GetUserRole(req.Context())
	if role == "super_admin" {
		return true
	}

	roleName := role
	if roleName == "member" || roleName == "owner" {
		roleName = controllerstorage.SystemRoleOperator
	}

	permissions, err := r.store.GetRolePermissions(targetTenantID, roleName)
	hasPermission := err == nil && containsString(permissions, permission)
	if hasPermission {
		return true
	}
	if err != nil {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Role permission lookup failed", nil)
		return false
	}

	if mode == "audit" {
		w.Header().Set("X-RBAC-Audit-Denied", "true")
		log.Printf(
			"[RBAC][audit] denied-but-allowed role=%s tenant=%s method=%s path=%s required_permission=%s err=%v",
			roleName,
			targetTenantID.String(),
			req.Method,
			req.URL.Path,
			permission,
			err,
		)
		return true
	}

	apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Insufficient permissions", nil)
	return false
}

// 辅助工具函数

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func splitTenantPath(path string) (string, string, bool) {
	parts := splitPath(path)
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v2" || parts[2] != "tenants" {
		return "", "", false
	}
	tenantID := parts[3]
	rest := strings.Join(parts[4:], "/")
	return tenantID, rest, true
}

func withTenantContext(req *http.Request, tenantID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID.String())
	return req.WithContext(ctx)
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

	limit := 100
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

		serializedHistory := make([]map[string]interface{}, 0, len(history))
		for _, h := range history {
			serializedHistory = append(serializedHistory, policyDeliveryToMap(h))
		}
		item["delivery_history"] = serializedHistory
		item["last_delivery"] = serializedHistory[0]
		item["policy_status"] = mapCommandStatusToPolicyStatus(history[0].CommandStatus)

		pendingCount := 0
		for _, h := range history {
			pendingCount += pendingCountForCommandStatus(h.CommandStatus)
		}
		item["pending_cmds"] = pendingCount
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
	if delivery.CompletedAt != nil {
		payload["completed_at"] = delivery.CompletedAt
	}

	return payload
}

func inferPolicyDeliveryIdentity(domain string, data map[string]interface{}, metadata map[string]interface{}) (string, string) {
	candidates := []map[string]interface{}{data, metadata}
	refKeys := []string{"id", "rule_id", "policy_ref", "cidr", "route"}
	nameKeys := []string{"name", "rule_name", "policy_name", "description"}

	var policyRef, policyName string
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		for _, key := range refKeys {
			if val, ok := candidate[key].(string); ok && val != "" {
				policyRef = val
				break
			}
		}
		if policyRef != "" {
			break
		}
	}

	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		for _, key := range nameKeys {
			if val, ok := candidate[key].(string); ok && val != "" {
				policyName = val
				break
			}
		}
		if policyName != "" {
			break
		}
	}

	return policyRef, policyName
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

func stringifyPolicyValue(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func clonePolicyMetadata(m map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{})
	for k, v := range m {
		cloned[k] = v
	}
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func normalizeRoutes(routes []string) ([]string, error) {
	unique := make(map[string]bool)
	var result []string
	for _, r := range routes {
		trimmed := strings.TrimSpace(r)
		if trimmed == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(trimmed); err != nil {
			if net.ParseIP(trimmed) == nil {
				return nil, fmt.Errorf("invalid CIDR or IP: %s", trimmed)
			}
			if !strings.Contains(trimmed, "/") {
				trimmed = trimmed + "/32"
			}
		}
		if !unique[trimmed] {
			unique[trimmed] = true
			result = append(result, trimmed)
		}
	}
	sort.Strings(result)
	return result, nil
}

func (r *Router) buildTenantNodeACLPolicies(tenantID uuid.UUID, node *controllerstorage.Node) ([]map[string]interface{}, error) {
	rules, err := r.store.ListTenantNodeACLRules(tenantID, node.ID)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]interface{}, 0, len(rules))
	for _, rule := range rules {
		items = append(items, map[string]interface{}{
			"id":         rule.ID.String(),
			"node_id":    node.ID.String(),
			"node_name":  firstNonEmpty(node.Hostname, node.PublicKey),
			"kind":       "acl",
			"name":       rule.Description,
			"src_net":    rule.SrcCIDR,
			"dst_net":    rule.DstCIDR,
			"action":     rule.Action,
			"enabled":    rule.Enabled,
			"priority":   rule.Priority,
			"created_at": rule.CreatedAt,
			"updated_at": rule.UpdatedAt,
		})
	}

	if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "acl", items, "id"); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *Router) buildTenantNodeQoSPolicies(tenantID uuid.UUID, node *controllerstorage.Node) ([]map[string]interface{}, error) {
	// 汇总所有分类的 QoS
	categories := []string{"service", "peers", "ip"}
	allPolicies := make([]map[string]interface{}, 0)

	for _, cat := range categories {
		rules, err := r.store.ListTenantNodeQoSRules(tenantID, node.ID, cat)
		if err != nil {
			continue
		}
		for _, rule := range rules {
			allPolicies = append(allPolicies, map[string]interface{}{
				"id":             rule.ID.String(),
				"node_id":        node.ID.String(),
				"node_name":      firstNonEmpty(node.Hostname, node.PublicKey),
				"kind":           "qos",
				"category":       cat,
				"name":           rule.Description,
				"bandwidth_mbps": rule.BandwidthMbps,
				"enabled":        rule.Enabled,
				"created_at":     rule.CreatedAt,
				"updated_at":     rule.UpdatedAt,
			})
		}
	}

	if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "qos", allPolicies, "id"); err != nil {
		return nil, err
	}

	return allPolicies, nil
}

func (r *Router) buildTenantNodeRoutePolicies(tenantID uuid.UUID, node *controllerstorage.Node) ([]map[string]interface{}, error) {
	items := make([]map[string]interface{}, 0, len(node.AdvertisedRoutes))
	for _, route := range node.AdvertisedRoutes {
		items = append(items, map[string]interface{}{
			"id":        route,
			"node_id":   node.ID.String(),
			"node_name": firstNonEmpty(node.Hostname, node.PublicKey),
			"kind":      "route",
			"name":      route,
			"enabled":   true,
		})
	}

	if err := attachPolicyDeliveriesToItems(r.store, tenantID, node.ID, "route", items, "id"); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *Router) updateTenantNodeRoutes(nodeID uuid.UUID, tenantID uuid.UUID, routes []string) error {
	_, err := r.store.DB().Exec(
		`UPDATE nodes SET advertised_routes = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
		pq.Array(routes), nodeID, tenantID,
	)
	return err
}

func attachNodeSummaryToPolicyItems(items []map[string]interface{}, summary map[string]interface{}) {
	for _, item := range items {
		item["node_status"] = summary["status"]
		item["node_sync_status"] = summary["sync_status"]
		item["node_availability"] = summary["availability_status"]
		item["convergence_status"] = summary["state_convergence"]
	}
}

func decodeRouteBody(req *http.Request) (string, error) {
	var body struct {
		CIDR  string `json:"cidr"`
		Route string `json:"route"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("invalid request body")
	}
	return firstNonEmpty(body.CIDR, body.Route), nil
}
