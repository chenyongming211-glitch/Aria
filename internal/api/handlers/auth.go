package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/internal/auth"
	"aria/pkg/controllerstorage"
)

type AuthAPI struct {
	store *controllerstorage.Storage
}

func NewAuthAPI(store *controllerstorage.Storage) *AuthAPI {
	return &AuthAPI{
		store: store,
	}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type PermissionsResponse struct {
	Role        string   `json:"role"`
	TenantID    string   `json:"tenant_id"`
	Permissions []string `json:"permissions"`
}

func normalizeAuthRole(role string) string {
	return controllerstorage.NormalizeRoleName(role)
}

func (a *AuthAPI) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req LoginRequest
	if err := apibase.ParseRequestJSON(r, &req); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if req.Username == "" || req.Password == "" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeValidationFailed, "Username and password are required", nil)
		return
	}

	var userID, role, dbPasswordHash string
	var tenantID sql.NullString
	var mustChangePassword bool

	query := `SELECT id, role, tenant_id, password_hash, COALESCE(must_change_password, FALSE) FROM users WHERE username = $1`
	err := a.store.DB().QueryRow(query, req.Username).Scan(&userID, &role, &tenantID, &dbPasswordHash, &mustChangePassword)
	if err != nil {
		if err == sql.ErrNoRows {
			apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidCredentials, "Invalid username or password", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, "DB_ERROR", "Database query failed", nil)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(dbPasswordHash), []byte(req.Password)); err != nil {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidCredentials, "Invalid username or password", nil)
		return
	}

	tID := ""
	if tenantID.Valid {
		tID = tenantID.String
	}

	token, err := auth.GenerateToken(userID, req.Username, role, tID, mustChangePassword)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeCreateTokenFailed, "Failed to generate authentication token", nil)
		return
	}

	resp := map[string]interface{}{
		"token":                   token,
		"expires_in":              7200,
		"require_password_change": mustChangePassword,
		"user": map[string]interface{}{
			"id":        userID,
			"username":  req.Username,
			"role":      role,
			"tenant_id": tID,
		},
	}

	apibase.WriteSuccess(w, resp, "Login successful")
}

func (a *AuthAPI) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeAuthHeaderRequired, "Authorization header required", nil)
		return
	}

	var tokenString string
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		tokenString = authHeader[7:]
	} else {
		tokenString = authHeader
	}

	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidToken, "Invalid token", nil)
		return
	}

	var username, role string
	var tenantID sql.NullString
	var mustChangePassword bool
	err = a.store.DB().QueryRow(
		`SELECT username, role, tenant_id, COALESCE(must_change_password, FALSE) FROM users WHERE id = $1`,
		claims.UserID,
	).Scan(&username, &role, &tenantID, &mustChangePassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidToken, "User no longer exists", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, "DB_ERROR", "Database query failed", nil)
		return
	}

	tID := ""
	if tenantID.Valid {
		tID = tenantID.String
	}

	newToken, err := auth.GenerateToken(claims.UserID, username, role, tID, mustChangePassword)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeCreateTokenFailed, "Failed to refresh token", nil)
		return
	}

	resp := map[string]interface{}{
		"token":      newToken,
		"expires_in": 7200,
		"user": map[string]interface{}{
			"id":        claims.UserID,
			"username":  username,
			"role":      role,
			"tenant_id": tID,
		},
		"require_password_change": mustChangePassword,
	}

	apibase.WriteSuccess(w, resp, "Token refreshed successfully")
}

func (a *AuthAPI) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	resp := map[string]interface{}{
		"status": "success",
	}

	apibase.WriteSuccess(w, resp, "Logout successful")
}

func (a *AuthAPI) HandlePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	role, exists := middleware.GetUserRole(r.Context())
	if !exists {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeUnauthorized, "Unauthorized", nil)
		return
	}

	roleName := normalizeAuthRole(role)
	if roleName == "super_admin" {
		apibase.WriteSuccess(w, PermissionsResponse{
			Role:        roleName,
			TenantID:    "",
			Permissions: []string{"*"},
		}, "Permissions loaded successfully")
		return
	}

	tenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeTenantContextNotFound, "Tenant context missing", nil)
		return
	}

	permissions, err := a.store.GetRolePermissions(tenantID, roleName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Role not found", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Role permission lookup failed", nil)
		return
	}
	if permissions == nil {
		permissions = []string{}
	}

	apibase.WriteSuccess(w, PermissionsResponse{
		Role:        roleName,
		TenantID:    tenantID.String(),
		Permissions: permissions,
	}, "Permissions loaded successfully")
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (a *AuthAPI) HandleForceChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req ChangePasswordRequest
	if err := apibase.ParseRequestJSON(r, &req); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if req.NewPassword == "" || len(req.NewPassword) < 6 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeValidationFailed, "Password must be at least 6 characters", nil)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidToken, "Missing or malformed Authorization header", nil)
		return
	}
	claims, err := auth.ValidateToken(authHeader[7:])
	if err != nil {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidToken, "Invalid token", nil)
		return
	}

	var dbPasswordHash string
	err = a.store.DB().QueryRow(`SELECT password_hash FROM users WHERE id = $1`, claims.UserID).Scan(&dbPasswordHash)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, "DB_ERROR", "Database query failed", nil)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(dbPasswordHash), []byte(req.OldPassword)); err != nil {
		apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidCredentials, "Incorrect old password", nil)
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, "HASH_ERROR", "Failed to hash password", nil)
		return
	}

	_, err = a.store.DB().Exec(`UPDATE users SET password_hash = $1, must_change_password = FALSE WHERE id = $2`,
		string(newHash), claims.UserID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to update password", nil)
		return
	}

	apibase.WriteSuccess(w, map[string]string{"message": "Password changed successfully, please login again"}, "Password changed successfully")
}
