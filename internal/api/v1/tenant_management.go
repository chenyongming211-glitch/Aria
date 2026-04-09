package v1

import (
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
	ID         uuid.UUID `json:"id"`
	Token      string    `json:"token"`
	TenantID   uuid.UUID `json:"tenant_id"`
	Tag        string    `json:"tag"`
	MaxUses    int       `json:"max_uses"`
	UsedCount  int       `json:"used_count"`
	ExpiresAt  string    `json:"expires_at"`
	CreatedAt  string    `json:"created_at"`
	CreatedBy  string    `json:"created_by"`
	Status     string    `json:"status"`
	LastUsedAt string    `json:"last_used_at,omitempty"`
	LastUsedBy string    `json:"last_used_by,omitempty"`
	I18n       struct {
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

type UpdateUserRequest struct {
	Password string `json:"password,omitempty"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role,omitempty"`
}

type CreateTokenRequest struct {
	Tag     string `json:"tag"`
	MaxUses int    `json:"max_uses"`
	TTL     string `json:"ttl"`
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
			"id":           id.String(),
			"token":        tokenStr,
			"tenant_id":    tenantID.String(),
			"tag":          tag,
			"max_uses":     maxUses,
			"used_count":   usedCount,
			"expires_at":   expiresAt,
			"created_at":   createdAt,
			"created_by":   createdBy,
			"status":       status,
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

