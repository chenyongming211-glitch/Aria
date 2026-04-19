package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"

	"aria/internal/api/apibase"
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
}

type TenantCreateRequest struct {
	Name  string `json:"name"`
	Code  string `json:"code"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

func (t *TenantAPI) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req TenantCreateRequest
	if err := apibase.ParseRequestJSON(r, &req); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	tenantID := uuid.New()

	query := `INSERT INTO tenants (id, name, code, email, phone, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`
	_, err := t.store.DB().Exec(query, tenantID, req.Name, req.Code, req.Email, req.Phone)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeCreateTenantFailed, "Failed to create tenant: "+err.Error(), nil)
		return
	}

	resp := TenantResponse{
		ID:    tenantID.String(),
		Name:  req.Name,
		Code:  req.Code,
		Email: req.Email,
		Phone: req.Phone,
	}

	apibase.WriteSuccess(w, resp, "Tenant created successfully")
}

func (t *TenantAPI) GetTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	tenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeTenantContextNotFound, "Tenant context required", nil)
		return
	}

	var tenant controllerstorage.TenantInfo
	query := `SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`
	err := t.store.DB().QueryRow(query, tenantID).Scan(
		&tenant.ID, &tenant.Name, &tenant.Code, &tenant.Status,
		&tenant.ResourceQuota, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if err != nil {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeTenantNotFound, "Tenant not found", nil)
		return
	}

	resp := TenantResponse{
		ID:   tenant.ID.String(),
		Name: tenant.Name,
		Code: tenant.Code,
	}

	apibase.WriteSuccess(w, resp, "Tenant retrieved successfully")
}

func (t *TenantAPI) ListTenants(w http.ResponseWriter, r *http.Request) {
	role, exists := middleware.GetUserRole(r.Context())
	if !exists || role != "super_admin" {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied: super_admin only", nil)
		return
	}

	rows, err := t.store.DB().Query(`SELECT id, name, code FROM tenants`)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeListTenantsFailed, "Failed to list tenants", nil)
		return
	}
	defer rows.Close()

	var tenants []TenantResponse
	for rows.Next() {
		var id uuid.UUID
		var name string
		var code sql.NullString
		if err := rows.Scan(&id, &name, &code); err != nil {
			continue
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

	apibase.WriteSuccess(w, tenants, fmt.Sprintf("%d tenants retrieved", len(tenants)))
}

// ==================== 用户管理 ====================

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role"`
}

func (t *TenantAPI) HandleTenantUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeTenantContextNotFound, "Tenant context required", nil)
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// URL: api/v2/tenants/{tid}/users[/{uid}]
	
	if len(pathParts) >= 5 && pathParts[4] != "" {
		userID := pathParts[4]
		switch r.Method {
		case http.MethodPut:
			t.UpdateUser(w, r, tenantID, userID)
		case http.MethodDelete:
			t.DeleteUser(w, r, tenantID, userID)
		default:
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		t.ListUsers(w, r, tenantID)
	case http.MethodPost:
		t.CreateUser(w, r, tenantID)
	default:
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (t *TenantAPI) ListUsers(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	query := `SELECT id, username, email, role FROM users WHERE tenant_id = $1 ORDER BY created_at DESC`
	rows, err := t.store.DB().Query(query, tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeListUsersFailed, "Failed to list users", nil)
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

	apibase.WriteSuccess(w, users, fmt.Sprintf("%d users retrieved", len(users)))
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func (t *TenantAPI) CreateUser(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req CreateUserRequest
	if err := apibase.ParseRequestJSON(r, &req); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if req.Username == "" || req.Password == "" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Username and password are required", nil)
		return
	}

	if req.Role == "" {
		req.Role = "member"
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeCreateUserFailed, "Failed to hash password", nil)
		return
	}

	userID := uuid.New()
	query := `INSERT INTO users (id, username, password_hash, tenant_id, role, email, created_at) 
			  VALUES ($1, $2, $3, $4, $5, $6, NOW())`
	_, err = t.store.DB().Exec(query, userID, req.Username, string(hashedPassword), tenantID, req.Role, req.Email)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeCreateUserFailed, "Failed to create user: "+err.Error(), nil)
		return
	}

	resp := UserResponse{
		ID:       userID.String(),
		Username: req.Username,
		Email:    req.Email,
		Role:     req.Role,
	}

	apibase.WriteSuccess(w, resp, "User created successfully")
}

type UpdateUserRequest struct {
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func (t *TenantAPI) UpdateUser(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, userID string) {
	var req UpdateUserRequest
	if err := apibase.ParseRequestJSON(r, &req); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidUserID, "Invalid user ID", nil)
		return
	}

	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeUpdateUserFailed, "Failed to hash password", nil)
			return
		}
		query := `UPDATE users SET password_hash = $1, role = COALESCE(NULLIF($2, ''), role), email = COALESCE(NULLIF($3, ''), email), updated_at = NOW() WHERE id = $4 AND tenant_id = $5`
		_, err = t.store.DB().Exec(query, string(hashedPassword), req.Role, req.Email, userUUID, tenantID)
	} else {
		query := `UPDATE users SET role = COALESCE(NULLIF($1, ''), role), email = COALESCE(NULLIF($2, ''), email), updated_at = NOW() WHERE id = $3 AND tenant_id = $4`
		_, err = t.store.DB().Exec(query, req.Role, req.Email, userUUID, tenantID)
	}

	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeUpdateUserFailed, "Failed to update user: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]string{"id": userID}, "User updated successfully")
}

func (t *TenantAPI) DeleteUser(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, userID string) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidUserID, "Invalid user ID", nil)
		return
	}

	result, err := t.store.DB().Exec(`DELETE FROM users WHERE id = $1 AND tenant_id = $2`, userUUID, tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeDeleteUserFailed, "Failed to delete user", nil)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeUserNotFound, "User not found", nil)
		return
	}

	apibase.WriteSuccess(w, nil, "User deleted successfully")
}
