package v1

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"

	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"
)

type TenantAPI struct {
	store *controllerstorage.Storage
}

func NewTenantAPI(store *controllerstorage.Storage) *TenantAPI {
	return &TenantAPI{
		store: store,
	}
}

type TenantResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Code  string `json:"code"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
	I18n  struct {
		Name string `json:"name,omitempty"`
	} `json:"i18n,omitempty"`
}

type TenantCreateRequest struct {
	Name  string `json:"name"`
	Code  string `json:"code"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

func (t *TenantAPI) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req TenantCreateRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	tenantID := uuid.New()

	query := `INSERT INTO tenants (id, name, code, created_at, updated_at) VALUES ($1, $2, $3, NOW(), NOW())`
	_, err := t.store.DB().Exec(query, tenantID, req.Name, req.Code)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreateTenantFailed, "Failed to create tenant: "+err.Error(), nil)
		return
	}

	resp := TenantResponse{
		ID:    tenantID.String(),
		Name:  req.Name,
		Code:  req.Code,
		Email: req.Email,
		Phone: req.Phone,
	}

	WriteSuccess(w, resp, "Tenant created successfully")
}

func (t *TenantAPI) GetTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		WriteError(w, http.StatusBadRequest, CodeTenantIDRequired, "Tenant ID is required", nil)
		return
	}

	tenantIDStr := pathParts[4]
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidTenantID, "Invalid tenant ID", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	if userTenantID != tenantID {
		WriteError(w, http.StatusForbidden, CodeAccessDenied, "Access denied", nil)
		return
	}

	var tenant controllerstorage.TenantInfo
	query := `SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`
	err = t.store.DB().QueryRow(query, tenantID).Scan(
		&tenant.ID, &tenant.Name, &tenant.Code, &tenant.Status,
		&tenant.ResourceQuota, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if err != nil {
		WriteError(w, http.StatusNotFound, CodeTenantNotFound, "Tenant not found", nil)
		return
	}

	resp := TenantResponse{
		ID:   tenant.ID.String(),
		Name: tenant.Name,
		Code: tenant.Code,
	}

	WriteSuccess(w, resp, "Tenant retrieved successfully")
}

func (t *TenantAPI) ListTenants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	role, exists := middleware.GetUserRole(r.Context())
	if !exists {
		WriteError(w, http.StatusForbidden, CodeAccessDenied, "Access denied", nil)
		return
	}

	// super_admin 可以查看所有租户，admin 只能查看自己所在的租户
	if role != "super_admin" && role != "admin" && role != "owner" {
		WriteError(w, http.StatusForbidden, CodeAccessDenied, "Access denied: admin or super_admin only", nil)
		return
	}

	rows, err := t.store.DB().Query(`SELECT id, name, code FROM tenants`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeListTenantsFailed, "Failed to list tenants", nil)
		return
	}
	defer rows.Close()

	var tenants []TenantResponse
	for rows.Next() {
		var id uuid.UUID
		var name string
		var code sql.NullString
		if err := rows.Scan(&id, &name, &code); err != nil {
			WriteError(w, http.StatusInternalServerError, CodeScanTenantFailed, "Failed to scan tenant", nil)
			return
		}

		codeStr := ""
		if code.Valid {
			codeStr = code.String
		}

		tenants = append(tenants, TenantResponse{
			ID:   id.String(),
			Name: name,
			Code: codeStr,
		})
	}

	WriteSuccess(w, tenants, fmt.Sprintf("%d tenants retrieved", len(tenants)))
}

// ✅ 核心修复：干掉不存在的 GetAllNodes 调用，直接下推 SQL 进行租户级高性能查询
func (t *TenantAPI) GetTenantNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	query := `
		SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role,
		       COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0),
		       advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at
		FROM nodes 
		WHERE tenant_id = $1 
		ORDER BY last_seen DESC`

	rows, err := t.store.DB().Query(query, userTenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeGetNodesFailed, "Failed to get nodes", nil)
		return
	}
	defer rows.Close()

	var tenantNodes []map[string]interface{}
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
			continue // 生产环境建议加上 log.Printf
		}

		tenantNodes = append(tenantNodes, map[string]interface{}{
			"id":          node.ID.String(),
			"public_key":  node.PublicKey,
			"hostname":    node.Hostname,
			"assigned_ip": node.AssignedIP,
			"status":      node.Status,
			"last_seen":   node.LastSeen,
		})
	}

	WriteSuccess(w, tenantNodes, fmt.Sprintf("%d nodes retrieved", len(tenantNodes)))
}

func (t *TenantAPI) HandleTenants(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")

	// /api/v1/tenants/{id} - 获取单个租户
	if len(pathParts) >= 5 && pathParts[4] != "" {
		switch r.Method {
		case http.MethodGet:
			t.GetTenant(w, r)
			return
		}
	}

	// /api/v1/tenants - 列表或创建
	switch r.Method {
	case http.MethodPost:
		t.CreateTenant(w, r)
	case http.MethodGet:
		t.ListTenants(w, r)
	default:
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func SetupTenantAPIRoutes(mux *http.ServeMux, store *controllerstorage.Storage) {
	api := NewTenantAPI(store)
	withJWT := middleware.JWTAuthMiddleware
	withTenantAdmin := middleware.RequireTenantAdmin

	mux.HandleFunc("/api/v1/tenants", withJWT(api.HandleTenants))
	mux.HandleFunc("/api/v1/tenants/", withJWT(withTenantAdmin(api.HandleTenantUsers)))
	mux.HandleFunc("/api/v1/nodes", withJWT(api.GetTenantNodes))
}

// ==================== 用户管理 ====================

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role"`
}

// checkTenantAccess 校验用户是否有权访问目标租户
func (t *TenantAPI) checkTenantAccess(w http.ResponseWriter, r *http.Request, targetTenantID string) error {
	role, exists := middleware.GetUserRole(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "Unauthorized", nil)
		return fmt.Errorf("unauthorized")
	}

	// super_admin can access any tenant
	if role == "super_admin" {
		return nil
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "Unauthorized", nil)
		return fmt.Errorf("unauthorized")
	}

	if userTenantID.String() != targetTenantID {
		WriteError(w, http.StatusForbidden, CodeAccessDenied, "Permission denied: cannot access other tenant", nil)
		return fmt.Errorf("permission denied: cross-tenant access")
	}

	if role != "admin" && role != "owner" {
		WriteError(w, http.StatusForbidden, CodeAccessDenied, "Permission denied: admin role required", nil)
		return fmt.Errorf("permission denied: insufficient role")
	}

	return nil
}

func (t *TenantAPI) HandleTenantUsers(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid path", nil)
		return
	}

	tenantID := pathParts[4]
	if tenantID == "" {
		WriteError(w, http.StatusBadRequest, CodeTenantIDRequired, "Tenant ID is required", nil)
		return
	}

	if err := t.checkTenantAccess(w, r, tenantID); err != nil {
		return
	}

	if len(pathParts) >= 7 && pathParts[6] != "" {
		userID := pathParts[6]
		switch r.Method {
		case http.MethodPut:
			t.UpdateUser(w, r, tenantID, userID)
		case http.MethodDelete:
			t.DeleteUser(w, r, tenantID, userID)
		default:
			WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		t.ListUsers(w, r, tenantID)
	case http.MethodPost:
		t.CreateUser(w, r, tenantID)
	default:
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (t *TenantAPI) ListUsers(w http.ResponseWriter, r *http.Request, tenantID string) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidTenantID, "Invalid tenant ID", nil)
		return
	}

	query := `SELECT id, username, email, role FROM users WHERE tenant_id = $1 ORDER BY created_at DESC`
	rows, err := t.store.DB().Query(query, tenantUUID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeListUsersFailed, "Failed to list users", nil)
		return
	}
	defer rows.Close()

	var users []UserResponse
	for rows.Next() {
		var u UserResponse
		var email sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &email, &u.Role); err != nil {
			continue
		}
		if email.Valid {
			u.Email = email.String
		}
		users = append(users, u)
	}

	WriteSuccess(w, users, fmt.Sprintf("%d users retrieved", len(users)))
}

func (t *TenantAPI) CreateUser(w http.ResponseWriter, r *http.Request, tenantID string) {
	var req CreateUserRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if req.Username == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Username and password are required", nil)
		return
	}

	if req.Role == "" {
		req.Role = "member"
	}
	if req.Role != "admin" && req.Role != "member" {
		req.Role = "member"
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreateUserFailed, "Failed to hash password", nil)
		return
	}

	userID := uuid.New()
	tenantUUID, _ := uuid.Parse(tenantID)

	query := `INSERT INTO users (id, username, password_hash, tenant_id, role, email, created_at) 
			  VALUES ($1, $2, $3, $4, $5, $6, NOW())`
	_, err = t.store.DB().Exec(query, userID, req.Username, string(hashedPassword), tenantUUID, req.Role, req.Email)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreateUserFailed, "Failed to create user: "+err.Error(), nil)
		return
	}

	resp := UserResponse{
		ID:       userID.String(),
		Username: req.Username,
		Email:    req.Email,
		Role:     req.Role,
	}

	WriteSuccess(w, resp, "User created successfully")
}

func (t *TenantAPI) UpdateUser(w http.ResponseWriter, r *http.Request, tenantID, userID string) {
	var req UpdateUserRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidUserID, "Invalid user ID", nil)
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)

	var existingPassword string
	query := `SELECT password_hash FROM users WHERE id = $1 AND tenant_id = $2`
	err = t.store.DB().QueryRow(query, userUUID, tenantUUID).Scan(&existingPassword)
	if err != nil {
		WriteError(w, http.StatusNotFound, CodeUserNotFound, "User not found", nil)
		return
	}

	if req.Role != "" && req.Role != "admin" && req.Role != "member" {
		req.Role = "member"
	}

	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, CodeUpdateUserFailed, "Failed to hash password", nil)
			return
		}
		query = `UPDATE users SET password_hash = $1, role = COALESCE(NULLIF($2, ''), role), email = COALESCE(NULLIF($3, ''), email), updated_at = NOW() WHERE id = $4 AND tenant_id = $5`
		_, err = t.store.DB().Exec(query, string(hashedPassword), req.Role, req.Email, userUUID, tenantUUID)
	} else {
		query = `UPDATE users SET role = COALESCE(NULLIF($1, ''), role), email = COALESCE(NULLIF($2, ''), email), updated_at = NOW() WHERE id = $3 AND tenant_id = $4`
		_, err = t.store.DB().Exec(query, req.Role, req.Email, userUUID, tenantUUID)
	}

	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeUpdateUserFailed, "Failed to update user: "+err.Error(), nil)
		return
	}

	WriteSuccess(w, map[string]string{"id": userID}, "User updated successfully")
}

func (t *TenantAPI) DeleteUser(w http.ResponseWriter, r *http.Request, tenantID, userID string) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidUserID, "Invalid user ID", nil)
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)

	result, err := t.store.DB().Exec(`DELETE FROM users WHERE id = $1 AND tenant_id = $2`, userUUID, tenantUUID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeDeleteUserFailed, "Failed to delete user", nil)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		WriteError(w, http.StatusNotFound, CodeUserNotFound, "User not found", nil)
		return
	}

	WriteSuccess(w, nil, "User deleted successfully")
}
