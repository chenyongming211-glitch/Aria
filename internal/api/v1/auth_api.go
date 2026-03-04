package v1

import (
	"database/sql"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"aria/internal/auth"
	"aria/pkg/controllerstorage"
)

type AuthAPI struct {
	store *controllerstorage.Storage // ✅ 注入存储引擎
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

func (a *AuthAPI) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req LoginRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if req.Username == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, CodeValidationFailed, "Username and password are required", nil)
		return
	}

	// ✅ 核心修复：彻底根除硬编码后门，改为查库验证
	var userID, role, dbPasswordHash string
	var tenantID sql.NullString

	query := `SELECT id, role, tenant_id, password_hash FROM users WHERE username = $1`
	err := a.store.DB().QueryRow(query, req.Username).Scan(&userID, &role, &tenantID, &dbPasswordHash)
	if err != nil {
		if err == sql.ErrNoRows {
			WriteError(w, http.StatusUnauthorized, CodeInvalidCredentials, "Invalid username or password", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "DB_ERROR", "Database query failed", nil)
		return
	}

	// 使用 bcrypt 比对哈希密码 (创建用户时需使用 bcrypt.GenerateFromPassword)
	if err := bcrypt.CompareHashAndPassword([]byte(dbPasswordHash), []byte(req.Password)); err != nil {
		WriteError(w, http.StatusUnauthorized, CodeInvalidCredentials, "Invalid username or password", nil)
		return
	}

	tID := ""
	if tenantID.Valid {
		tID = tenantID.String
	}

	token, err := auth.GenerateToken(userID, req.Username, role, tID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreateTokenFailed, "Failed to generate authentication token", nil)
		return
	}

	resp := map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":        userID,
			"username":  req.Username,
			"role":      role,
			"tenant_id": tID,
		},
	}

	WriteSuccess(w, resp, "Login successful")
}

func (a *AuthAPI) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		WriteError(w, http.StatusUnauthorized, CodeAuthHeaderRequired, "Authorization header required", nil)
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
		WriteError(w, http.StatusUnauthorized, CodeInvalidToken, "Invalid token", nil)
		return
	}

	newToken, err := auth.GenerateToken(claims.UserID, claims.Username, claims.Role, claims.TenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreateTokenFailed, "Failed to refresh token", nil)
		return
	}

	resp := map[string]interface{}{
		"token": newToken,
	}

	WriteSuccess(w, resp, "Token refreshed successfully")
}

func (a *AuthAPI) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	resp := map[string]interface{}{
		"status": "success",
	}

	WriteSuccess(w, resp, "Logout successful")
}

// ✅ 注意：在路由注册处需要传入 store
func SetupAuthRoutes(mux *http.ServeMux, store *controllerstorage.Storage) {
	api := NewAuthAPI(store)
	mux.HandleFunc("/api/v1/auth/login", api.HandleLogin)
	mux.HandleFunc("/api/v1/auth/refresh", api.HandleRefresh)
	mux.HandleFunc("/api/v1/auth/logout", api.HandleLogout)
}
