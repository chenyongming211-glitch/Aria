package v1

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"aria/internal/api/middleware"
	"aria/internal/token"
	"aria/pkg/controllerstorage"
)

// TenantManagementAPI handles tenant management operations
type TenantManagementAPI struct {
	store *controllerstorage.Storage
}

// NewTenantManagementAPI creates a new tenant management API instance
func NewTenantManagementAPI(store *controllerstorage.Storage) *TenantManagementAPI {
	return &TenantManagementAPI{
		store: store,
	}
}

// User represents a user in the system
type User struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	Password string    `json:"password,omitempty"`
	TenantID uuid.UUID `json:"tenant_id"`
	Role     string    `json:"role"` // owner, admin, viewer
	Email    string    `json:"email,omitempty"`
}

// EnrollmentToken represents an enrollment token for agent registration
type EnrollmentToken struct {
	ID          uuid.UUID `json:"id"`
	Token       string    `json:"token"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Tag         string    `json:"tag"`
	MaxUses     int       `json:"max_uses"`
	UsedCount   int       `json:"used_count"`
	ExpiresAt   string    `json:"expires_at"`
	CreatedAt   string    `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
	Status      string    `json:"status"`
	LastUsedAt  string    `json:"last_used_at,omitempty"`
	LastUsedBy  string    `json:"last_used_by,omitempty"`
	I18n struct {
		Status string `json:"status,omitempty"`
		Tag    string `json:"tag,omitempty"`
	} `json:"i18n,omitempty"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Email    string `json:"email"`
}

type CreateTokenRequest struct {
	Tag      string `json:"tag"`
	MaxUses  int    `json:"max_uses"`
	TTL      string `json:"ttl"`
}

type CreateTenantRequest struct {
	Name          string                 `json:"name"`
	Code          string                 `json:"code"`
	Status        string                 `json:"status"`
	ResourceQuota map[string]interface{} `json:"resource_quota"`
}

// SuperAdminOnly 基于 JWT Role 的真实超管防线
func (t *TenantManagementAPI) SuperAdminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, exists := middleware.GetUserRole(r.Context())
		if !exists || role != "admin" {
			log.Printf("⚠️ WARNING: Unauthorized super admin access attempt from IP: %s", r.RemoteAddr)
			WriteError(w, http.StatusForbidden, CodeAccessDenied, "Access denied: Super admin only", nil)
			return
		}
		next(w, r)
	}
}

func (t *TenantManagementAPI) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req CreateTenantRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	tenantID := uuid.New()
	var resourceQuota string
	if req.ResourceQuota != nil {
		quotaBytes, err := json.Marshal(req.ResourceQuota)
		if err != nil {
			WriteError(w, http.StatusBadRequest, CodeInvalidResourceQuota, "Invalid resource quota format", nil)
			return
		}
		resourceQuota = string(quotaBytes)
	} else {
		resourceQuota = "{}"
	}

	status := req.Status
	if status == "" {
		status = "active"
	}

	query := `INSERT INTO tenants (id, name, code, status, resource_quota, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`
	_, err := t.store.DB().Exec(query, tenantID, req.Name, req.Code, status, resourceQuota)
	if err != nil {
		log.Printf("⚠️ ERROR: Failed to create tenant (Name: %s): %v", req.Name, err)
		WriteError(w, http.StatusInternalServerError, CodeCreateTenantFailed, "Failed to create tenant: "+err.Error(), nil)
		return
	}

	resp := map[string]interface{}{
		"id":             tenantID.String(),
		"name":           req.Name,
		"code":           req.Code,
		"status":         req.Status,
		"resource_quota": req.ResourceQuota,
		"i18n": map[string]interface{}{
			"name":   fmt.Sprintf("tenant.%s", req.Name),
			"status": fmt.Sprintf("status.%s", req.Status),
		},
	}

	WriteSuccess(w, resp, "Tenant created successfully")
}

func (t *TenantManagementAPI) GetUserTenants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	var tenant controllerstorage.TenantInfo
	query := `SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`
	err := t.store.DB().QueryRow(query, userTenantID).Scan(
		&tenant.ID, &tenant.Name, &tenant.Code, &tenant.Status,
		&tenant.ResourceQuota, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if err != nil {
		WriteError(w, http.StatusNotFound, CodeTenantNotFound, "Tenant not found", nil)
		return
	}

	resp := map[string]interface{}{
		"id":             tenant.ID.String(),
		"name":           tenant.Name,
		"code":           tenant.Code,
		"status":         tenant.Status,
		"resource_quota": tenant.ResourceQuota,
		"i18n": map[string]interface{}{
			"name":   fmt.Sprintf("tenant.%s", tenant.Name),
			"status": fmt.Sprintf("status.%s", tenant.Status),
		},
	}

	WriteSuccess(w, []interface{}{resp}, "User tenant retrieved successfully")
}

func (t *TenantManagementAPI) CreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	var req CreateTokenRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	maxUses := req.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}

	opts := token.GenerateOptions{
		Tag:      req.Tag,
		TenantID: userTenantID.String(),
		MaxUses:  maxUses,
		Creator:  "system_admin",
	}

	if req.TTL != "" {
		if parsedTTL, err := time.ParseDuration(req.TTL); err == nil {
			opts.TTL = parsedTTL
		} else {
			opts.TTL = 24 * time.Hour
		}
	} else {
		opts.TTL = 24 * time.Hour
	}

	tk, err := token.Generate(opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreateTokenFailed, "Failed to generate token", nil)
		return
	}

	tokenStore := token.NewStore(t.store.DB())
	if err := tokenStore.Create(tk); err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreateTokenFailed, "Failed to save token: "+err.Error(), nil)
		return
	}

	resp := map[string]interface{}{
		"id":         tk.ID.String(),
		"token":      tk.Token,
		"tenant_id":  tk.TenantID,
		"tag":        tk.Tag,
		"max_uses":   tk.MaxUses,
		"used_count": tk.UsedCount,
		"expires_at": tk.ExpiresAt.Format(time.RFC3339),
		"created_at": tk.CreatedAt.Format(time.RFC3339),
		"status":     string(tk.Status),
		"i18n": map[string]interface{}{
			"status": fmt.Sprintf("token.status.%s", tk.Status),
			"tag":    fmt.Sprintf("token.tag.%s", tk.Tag),
		},
	}

	WriteSuccess(w, resp, "Token created successfully")
}

func (t *TenantManagementAPI) GetTenantTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	rows, err := t.store.DB().Query(
		`SELECT id, token, tenant_id, tag, max_uses, used_count, expires_at, created_at, created_by, status, last_used_at, last_used_by
		  FROM tokens WHERE tenant_id = $1 ORDER BY created_at DESC`, userTenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeListTokensFailed, "Failed to list tokens", nil)
		return
	}
	defer rows.Close()

	var tokens []map[string]interface{}
	for rows.Next() {
		var id, tenantID uuid.UUID
		var tokenStr, tag, createdBy, status string
		var maxUses, usedCount int
		var expiresAt, createdAt, lastUsedAt, lastUsedBy interface{}

		if err := rows.Scan(&id, &tokenStr, &tenantID, &tag, &maxUses, &usedCount, &expiresAt, &createdAt, &createdBy, &status, &lastUsedAt, &lastUsedBy); err != nil {
			WriteError(w, http.StatusInternalServerError, CodeScanTokenFailed, "Failed to scan token", nil)
			return
		}

		tokenMap := map[string]interface{}{
			"id":          id.String(),
			"token":       tokenStr,
			"tenant_id":   tenantID.String(),
			"tag":         tag,
			"max_uses":    maxUses,
			"used_count":  usedCount,
			"expires_at":  expiresAt,
			"created_at":  createdAt,
			"created_by":  createdBy,
			"status":      status,
			"last_used_at": lastUsedAt,
			"last_used_by": lastUsedBy,
			"i18n": map[string]interface{}{
				"status": fmt.Sprintf("token.status.%s", status),
				"tag":    fmt.Sprintf("token.tag.%s", tag),
			},
		}

		tokens = append(tokens, tokenMap)
	}

	WriteSuccess(w, tokens, fmt.Sprintf("%d tokens retrieved", len(tokens)))
}

func (t *TenantManagementAPI) DeleteToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		WriteError(w, http.StatusBadRequest, CodeTokenIDRequired, "Token ID is required", nil)
		return
	}

	tokenIDStr := pathParts[4]
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidTokenID, "Invalid token ID", nil)
		return
	}

	query := `DELETE FROM tokens WHERE id = $1 AND tenant_id = $2`
	result, err := t.store.DB().Exec(query, tokenID, userTenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeDeleteTokenFailed, "Failed to delete token", nil)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		WriteError(w, http.StatusNotFound, CodeTokenNotFound, "Token not found or access denied", nil)
		return
	}

	WriteSuccess(w, map[string]string{"status": "success"}, "Token deleted successfully")
}

func (t *TenantManagementAPI) GetTenantNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	rows, err := t.store.DB().Query(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role,
		         COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0),
		         advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at
		  FROM nodes WHERE tenant_id = $1 ORDER BY last_seen DESC`, userTenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeGetNodesFailed, "Failed to get nodes", nil)
		return
	}
	defer rows.Close()

	var nodes []map[string]interface{}
	for rows.Next() {
		var node controllerstorage.Node
		var advertisedRoutes interface{}
		var enrolledWithToken interface{}

		err := rows.Scan(
			&node.ID, &node.PublicKey, &node.MachineID, &node.TenantID,
			&node.Endpoint, &node.PrivateIP, &node.PublicIP, &node.Region,
			&node.VPCID, &node.Hostname, &node.AssignedIP, &node.IPOffset,
			&node.LastSeen, &node.RegisteredAt, &node.Role,
			&node.RuntimeMode, &node.KernelVersion, &node.HasAESNI,
			&node.Status, &node.OfflineSince,
			&advertisedRoutes, &enrolledWithToken,
			&node.CreatedAt, &node.UpdatedAt,
		)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, CodeScanNodeFailed, "Failed to scan node", nil)
			return
		}

		nodeMap := map[string]interface{}{
			"id":                node.ID.String(),
			"public_key":        node.PublicKey,
			"machine_id":        node.MachineID,
			"tenant_id":         node.TenantID.String(),
			"endpoint":          node.Endpoint,
			"private_ip":        node.PrivateIP,
			"public_ip":         node.PublicIP,
			"region":            node.Region,
			"vpc_id":            node.VPCID,
			"hostname":          node.Hostname,
			"assigned_ip":       node.AssignedIP,
			"ip_offset":         node.IPOffset,
			"last_seen":         node.LastSeen,
			"registered_at":     node.RegisteredAt,
			"role":              node.Role,
			"runtime_mode":      node.RuntimeMode,
			"kernel_version":    node.KernelVersion,
			"has_aesni":         node.HasAESNI,
			"status":            node.Status,
			"offline_since":     node.OfflineSince,
			"advertised_routes": advertisedRoutes,
			"enrolled_with_token": enrolledWithToken,
			"created_at":        node.CreatedAt,
			"updated_at":        node.UpdatedAt,
			"i18n": map[string]interface{}{
				"status": fmt.Sprintf("node.status.%s", node.Status),
				"role":   fmt.Sprintf("node.role.%s", node.Role),
			},
		}

		nodes = append(nodes, nodeMap)
	}

	WriteSuccess(w, nodes, fmt.Sprintf("%d nodes retrieved", len(nodes)))
}

// ========================
// ✅ 阶段1：新增的 ACL 生命周期 CRUD 接口
// ========================

// GetTenantACLRules 获取当前租户的规则列表
func (t *TenantManagementAPI) GetTenantACLRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	// Handle query parameters for pagination and filtering
	queryParams := r.URL.Query()
	pageStr := queryParams.Get("page")
	pageSizeStr := queryParams.Get("page_size")

	var page, pageSize int
	var err error

	// Set default values
	if pageStr == "" {
		page = 1
	} else {
		page, err = strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			page = 1
		}
	}

	if pageSizeStr == "" {
		pageSize = 50 // default page size
	} else {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil || pageSize < 1 || pageSize > 100 { // max 100 items per page
			pageSize = 50
		}
	}

	// Build dynamic query with filtering options
	var whereClauses []string
	var args []interface{}
	args = append(args, userTenantID) // First argument is always tenant_id

	// Add filters based on query parameters
	if nameFilter := queryParams.Get("name"); nameFilter != "" {
		args = append(args, nameFilter)
		whereClauses = append(whereClauses, fmt.Sprintf("name ILIKE $%d", len(args)))
	}

	if actionFilter := queryParams.Get("action"); actionFilter != "" {
		args = append(args, actionFilter)
		whereClauses = append(whereClauses, fmt.Sprintf("action = $%d", len(args)))
	}

	if enabledFilter := queryParams.Get("enabled"); enabledFilter != "" {
		enabled, parseErr := strconv.ParseBool(enabledFilter)
		if parseErr == nil {
			args = append(args, enabled)
			whereClauses = append(whereClauses, fmt.Sprintf("enabled = $%d", len(args)))
		}
	}

	if priorityFilter := queryParams.Get("priority"); priorityFilter != "" {
		priority, parseErr := strconv.Atoi(priorityFilter)
		if parseErr == nil {
			args = append(args, priority)
			whereClauses = append(whereClauses, fmt.Sprintf("priority = $%d", len(args)))
		}
	}

	// Base query
	baseQuery := `SELECT id, COALESCE(name, ''), COALESCE(src_node, ''), src_net, COALESCE(dst_node, ''), dst_net, protocol, min_port, max_port,
		          COALESCE(action, 'allow'), enabled, priority, COALESCE(description, ''), created_at, updated_at
		          FROM acl_rules
		          WHERE tenant_id = $1`

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = " AND " + strings.Join(whereClauses, " AND ")
	}

	// Get total count for pagination metadata
	countQuery := `SELECT COUNT(*) FROM acl_rules WHERE tenant_id = $1` + whereClause
	if len(whereClauses) > 0 {
		countQuery += " AND " + strings.Join(whereClauses, " AND ")
	}

	var totalCount int
	err = t.store.DB().QueryRow(countQuery, args...).Scan(&totalCount)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeGetACLRulesFailed, "Failed to count ACL rules", nil)
		return
	}

	// Apply pagination to base query
	query := baseQuery + whereClause +
	         fmt.Sprintf(" ORDER BY priority ASC, id ASC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)

	// Add pagination parameters to args
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := t.store.DB().Query(query, args...)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeGetACLRulesFailed, "Failed to get ACL rules", nil)
		return
	}
	defer rows.Close()

	var rules []map[string]interface{}
	for rows.Next() {
		var rule controllerstorage.ACLRule

		err := rows.Scan(
			&rule.ID, &rule.Name, &rule.SrcNode, &rule.SrcNet, &rule.DstNode, &rule.DstNet, &rule.Protocol,
			&rule.MinPort, &rule.MaxPort, &rule.Action, &rule.Enabled, &rule.Priority,
			&rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
		)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, CodeScanACLRuleFailed, "Failed to scan ACL rule", nil)
			return
		}

		ruleMap := map[string]interface{}{
			"id":          rule.ID,
			"name":        rule.Name,
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
			"i18n": map[string]interface{}{
				"action": fmt.Sprintf("acl.action.%s", rule.Action),
				"name":   fmt.Sprintf("acl.rule.%s", rule.Name),
			},
		}

		rules = append(rules, ruleMap)
	}

	// Calculate pagination metadata
	totalPages := (totalCount + pageSize - 1) / pageSize
	hasNext := page < totalPages
	hasPrev := page > 1

	meta := &APIMeta{
		Total:    totalCount,
		Page:     page,
		PageSize: pageSize,
	}

	if hasNext {
		nextPage := page + 1
		meta.Next = fmt.Sprintf("/api/v1/tenant-management/acl-rules?page=%d&page_size=%d", nextPage, pageSize)
	}

	if hasPrev {
		prevPage := page - 1
		meta.Prev = fmt.Sprintf("/api/v1/tenant-management/acl-rules?page=%d&page_size=%d", prevPage, pageSize)
	}

	// Prepare response
	response := APIResponse{
		Success: true,
		Data:    rules,
		Message: fmt.Sprintf("%d ACL rules retrieved", len(rules)),
		Meta:    meta,
		Code:    CodeOK,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// CreateTenantACLRule 创建一条 ACL 策略
func (t *TenantManagementAPI) CreateTenantACLRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	var req controllerstorage.ACLRule
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	// Validate required fields
	if req.SrcNet == "" || req.DstNet == "" {
		errorDetails := map[string]string{
			"src_net": "Source network is required",
			"dst_net": "Destination network is required",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	// Set default values if not provided
	if req.Action == "" {
		req.Action = "allow"
	}
	if req.Priority == 0 {
		req.Priority = 100 // Default priority
	}
	if !req.Enabled {
		req.Enabled = true // Default to enabled if not specified
	}

	// Validate action values
	if req.Action != "allow" && req.Action != "deny" && req.Action != "pass" && req.Action != "drop" {
		errorDetails := map[string]string{
			"action": "Action must be one of: allow, deny, pass, drop",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	// Validate network format
	if _, _, err := net.ParseCIDR(req.SrcNet); err != nil && net.ParseIP(req.SrcNet) == nil {
		errorDetails := map[string]string{
			"src_net": "Source network must be a valid IP or CIDR",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	if _, _, err := net.ParseCIDR(req.DstNet); err != nil && net.ParseIP(req.DstNet) == nil {
		errorDetails := map[string]string{
			"dst_net": "Destination network must be a valid IP or CIDR",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	// Validate port ranges
	if req.MinPort > req.MaxPort {
		errorDetails := map[string]string{
			"min_port": "Min port cannot be greater than max port",
			"max_port": "Max port cannot be less than min port",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	if req.MinPort > 65535 || req.MaxPort > 65535 {
		errorDetails := map[string]string{
			"ports": "Port values must be between 0 and 65535",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	// Validate protocol
	if req.Protocol > 255 {
		errorDetails := map[string]string{
			"protocol": "Protocol value must be between 0 and 255",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	query := `
		INSERT INTO acl_rules (name, tenant_id, src_node, src_net, dst_node, dst_net, protocol, min_port, max_port, action, enabled, priority, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`
	err := t.store.DB().QueryRow(query,
		req.Name, userTenantID, req.SrcNode, req.SrcNet, req.DstNode, req.DstNet,
		req.Protocol, req.MinPort, req.MaxPort, req.Action, req.Enabled, req.Priority, req.Description,
	).Scan(&req.ID, &req.CreatedAt, &req.UpdatedAt)

	if err != nil {
		log.Printf("⚠️ ERROR: Failed to create ACL rule: %v", err)
		WriteError(w, http.StatusInternalServerError, "CREATE_ACL_FAILED", "Failed to create ACL rule", nil)
		return
	}

	// Return the created rule with success response
	responseData := map[string]interface{}{
		"rule": req,
	}

	WriteSuccess(w, responseData, "ACL rule created successfully")
}

// UpdateTenantACLRule 更新一条 ACL 策略
func (t *TenantManagementAPI) UpdateTenantACLRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	// 安全解析 URL 末尾的 ID
	pathParts := strings.Split(strings.TrimRight(r.URL.Path, "/"), "/")
	ruleIDStr := pathParts[len(pathParts)-1]
	ruleID, err := strconv.Atoi(ruleIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidPath, "Invalid rule ID", nil)
		return
	}

	var req controllerstorage.ACLRule
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	// Validate action values
	if req.Action != "" && req.Action != "allow" && req.Action != "deny" && req.Action != "pass" && req.Action != "drop" {
		errorDetails := map[string]string{
			"action": "Action must be one of: allow, deny, pass, drop",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	// Validate network format if provided
	if req.SrcNet != "" {
		if _, _, err := net.ParseCIDR(req.SrcNet); err != nil && net.ParseIP(req.SrcNet) == nil {
			errorDetails := map[string]string{
				"src_net": "Source network must be a valid IP or CIDR",
			}
			WriteValidationError(w, errorDetails)
			return
		}
	}

	if req.DstNet != "" {
		if _, _, err := net.ParseCIDR(req.DstNet); err != nil && net.ParseIP(req.DstNet) == nil {
			errorDetails := map[string]string{
				"dst_net": "Destination network must be a valid IP or CIDR",
			}
			WriteValidationError(w, errorDetails)
			return
		}
	}

	// Validate port ranges
	if req.MinPort > req.MaxPort {
		errorDetails := map[string]string{
			"min_port": "Min port cannot be greater than max port",
			"max_port": "Max port cannot be less than min port",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	if req.MinPort > 65535 || req.MaxPort > 65535 {
		errorDetails := map[string]string{
			"ports": "Port values must be between 0 and 65535",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	// Validate protocol
	if req.Protocol > 255 {
		errorDetails := map[string]string{
			"protocol": "Protocol value must be between 0 and 255",
		}
		WriteValidationError(w, errorDetails)
		return
	}

	query := `
		UPDATE acl_rules SET
			name = $1, src_node = $2, src_net = $3, dst_node = $4, dst_net = $5, protocol = $6,
			min_port = $7, max_port = $8, action = $9, enabled = $10, priority = $11, description = $12, updated_at = NOW()
		WHERE id = $13 AND tenant_id = $14
		RETURNING updated_at
	`
	err = t.store.DB().QueryRow(query,
		req.Name, req.SrcNode, req.SrcNet, req.DstNode, req.DstNet, req.Protocol,
		req.MinPort, req.MaxPort, req.Action, req.Enabled, req.Priority, req.Description, ruleID, userTenantID,
	).Scan(&req.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			WriteError(w, http.StatusNotFound, "RULE_NOT_FOUND", "ACL rule not found or access denied", nil)
			return
		}
		log.Printf("⚠️ ERROR: Failed to update ACL rule %d: %v", ruleID, err)
		WriteError(w, http.StatusInternalServerError, "UPDATE_ACL_FAILED", "Failed to update ACL rule", nil)
		return
	}

	req.ID = ruleID
	responseData := map[string]interface{}{
		"rule": req,
	}
	WriteSuccess(w, responseData, "ACL rule updated successfully")
}

// DeleteTenantACLRule 删除一条 ACL 策略
func (t *TenantManagementAPI) DeleteTenantACLRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	pathParts := strings.Split(strings.TrimRight(r.URL.Path, "/"), "/")
	ruleIDStr := pathParts[len(pathParts)-1]
	ruleID, err := strconv.Atoi(ruleIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidPath, "Invalid rule ID", nil)
		return
	}

	query := `DELETE FROM acl_rules WHERE id = $1 AND tenant_id = $2`
	result, err := t.store.DB().Exec(query, ruleID, userTenantID)
	if err != nil {
		log.Printf("⚠️ ERROR: Failed to delete ACL rule %d: %v", ruleID, err)
		WriteError(w, http.StatusInternalServerError, "DELETE_ACL_FAILED", "Failed to delete ACL rule", nil)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		WriteError(w, http.StatusNotFound, "RULE_NOT_FOUND", "ACL rule not found or access denied", nil)
		return
	}

	WriteSuccess(w, map[string]int{"id": ruleID}, "ACL rule deleted successfully")
}

// ========================
// ✅ 核心路由分发修复
// ========================

func (t *TenantManagementAPI) HandleTenantManagement(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		WriteError(w, http.StatusBadRequest, CodeInvalidPath, "Invalid path", nil)
		return
	}

	if len(pathParts) >= 5 {
		subResource := pathParts[4] 
		switch subResource {
		case "tokens":
			if len(pathParts) >= 6 {
				tokenID := pathParts[5]
				if r.Method == http.MethodDelete {
					originalURL := r.URL.Path
					r.URL.Path = fmt.Sprintf("/api/v1/tenant-management/tokens/%s", tokenID)
					t.DeleteToken(w, r)
					r.URL.Path = originalURL
					return
				}
			}

			switch r.Method {
			case http.MethodGet:
				t.GetTenantTokens(w, r)
			case http.MethodPost:
				t.CreateEnrollmentToken(w, r)
			default:
				WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
			}
			return

		case "nodes":
			switch r.Method {
			case http.MethodGet:
				t.GetTenantNodes(w, r)
			default:
				WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
			}
			return

		case "acl-rules", "acl", "policies":
			// 检测是否请求特定资源（例如 /api/v1/tenant-management/acl-rules/12）
			if len(pathParts) >= 6 && pathParts[5] != "" {
				switch r.Method {
				case http.MethodPut:
					t.UpdateTenantACLRule(w, r)
				case http.MethodDelete:
					t.DeleteTenantACLRule(w, r)
				default:
					WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
				}
				return
			}

			// 否则处理集合请求
			switch r.Method {
			case http.MethodGet:
				t.GetTenantACLRules(w, r)
			case http.MethodPost:
				t.CreateTenantACLRule(w, r)
			default:
				WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
			}
			return
		}
	}

	WriteError(w, http.StatusNotFound, CodeEndpointNotFound, "Unknown endpoint", nil)
}

func SetupTenantManagementRoutes(mux *http.ServeMux, store *controllerstorage.Storage) {
	api := NewTenantManagementAPI(store)
	withJWT := middleware.JWTAuthMiddleware

	mux.HandleFunc("/api/v1/tenant-management/", withJWT(api.HandleTenantManagement))
	
	mux.HandleFunc("/api/v1/system/tenants", withJWT(api.SuperAdminOnly(api.CreateTenant)))

	mux.HandleFunc("/api/v1/tokens", withJWT(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			api.GetTenantTokens(w, r)
		case http.MethodPost:
			api.CreateEnrollmentToken(w, r)
		default:
			WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		}
	}))

	mux.HandleFunc("/api/v1/tokens/", withJWT(func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) >= 5 {
			action := pathParts[4]

			if action == "detail" {
				var err error 
				tokenStr := r.URL.Query().Get("token")
				if tokenStr == "" {
					WriteError(w, http.StatusBadRequest, CodeTokenParamRequired, "token parameter required", nil)
					return
				}

				var id uuid.UUID
				var tokenValue string
				var tenantID uuid.UUID
				var tag string
				var maxUses int
				var usedCount int
				var expiresAt string
				var createdAt string
				var createdBy string
				var status string
				var lastUsedAt interface{}
				var lastUsedBy interface{}

				err = store.DB().QueryRow(
					`SELECT id, token, tenant_id, tag, max_uses, used_count, expires_at, created_at, created_by, status, last_used_at, last_used_by
					  FROM tokens WHERE token = $1`, tokenStr).Scan(
					&id, &tokenValue, &tenantID, &tag, &maxUses, &usedCount, &expiresAt, &createdAt, &createdBy, &status, &lastUsedAt, &lastUsedBy)

				if err != nil {
					WriteError(w, http.StatusNotFound, CodeTokenNotFound, "Token not found", nil)
					return
				}

				userTenantID, exists := middleware.GetTenantID(r.Context())
				if !exists {
					WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
					return
				}

				rows, err := store.DB().Query(
					`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role,
						 COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0),
						 advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at
					 FROM nodes
					 WHERE enrolled_with_token = $1 AND tenant_id = $2`, tokenStr, userTenantID)
				if err != nil {
					WriteError(w, http.StatusInternalServerError, CodeGetTokenNodesFailed, "Failed to get nodes", nil)
					return
				}
				defer rows.Close()

				var usedByNodes []map[string]interface{}
				for rows.Next() {
					var node controllerstorage.Node
					var advertisedRoutes interface{}
					var enrolledWithToken interface{}

					err := rows.Scan(
						&node.ID, &node.PublicKey, &node.MachineID, &node.TenantID,
						&node.Endpoint, &node.PrivateIP, &node.PublicIP, &node.Region,
						&node.VPCID, &node.Hostname, &node.AssignedIP, &node.IPOffset,
						&node.LastSeen, &node.RegisteredAt, &node.Role,
						&node.RuntimeMode, &node.KernelVersion, &node.HasAESNI,
						&node.Status, &node.OfflineSince,
						&advertisedRoutes, &enrolledWithToken,
						&node.CreatedAt, &node.UpdatedAt,
					)
					if err != nil {
						continue
					}

					nodeMap := map[string]interface{}{
						"public_key":        node.PublicKey,
						"hostname":          node.Hostname,
						"assigned_ip":       node.AssignedIP,
						"region":            node.Region,
						"public_ip":         node.PublicIP,
						"last_seen":         node.LastSeen,
						"status":            node.Status,
						"runtime_mode":      node.RuntimeMode,
						"role":              node.Role,
					}
					usedByNodes = append(usedByNodes, nodeMap)
				}

				response := map[string]interface{}{
					"token": tokenStr,
					"nodes": usedByNodes,
				}
				WriteSuccess(w, response, "Token nodes retrieved successfully")
			} else if action == "revoke" || action == "delete" {
				api.DeleteToken(w, r)
			} else {
				tokenID := action
				originalPath := r.URL.Path
				r.URL.Path = fmt.Sprintf("/api/v1/tenant-management/tokens/%s", tokenID)
				api.DeleteToken(w, r)
				r.URL.Path = originalPath
			}
		}
	}))
}