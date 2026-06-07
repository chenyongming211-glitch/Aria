package v2

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/internal/token"

	"github.com/google/uuid"
)

func (r *Router) HandleControllerInfo(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"name":    "aria-controller",
		"version": currentControllerVersion(),
		"supported_features": []string{
			"grpc_sync",
			"runtime_token_refresh",
			"cert_renew",
			"snapshot_eose",
			"domain_version_sync",
		},
		"limits": map[string]int{
			"max_peers_per_sync": 500,
			"max_acl_rules":      1000,
		},
		"auth": map[string]interface{}{
			"enrollment":            true,
			"runtime_token_ttl_sec": 86400,
			"challenge_auth":        false,
		},
	}, "Controller capabilities retrieved")
}

func currentControllerVersion() string {
	if version := strings.TrimSpace(os.Getenv("ARIA_CONTROLLER_VERSION")); version != "" {
		return version
	}
	if version := strings.TrimSpace(os.Getenv("ARIA_VERSION")); version != "" {
		return version
	}
	return "0.2.x"
}

// handleTenantTokens 处理租户级别的注册令牌
func (r *Router) handleTenantTokens(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, role string) {
	parts := splitPath(req.URL.Path)
	// URL: /api/v2/tenants/{tid}/tokens[/{token_id}]

	if len(parts) == 5 {
		switch req.Method {
		case http.MethodGet:
			if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermTokensRead) {
				return
			}
			r.listTenantTokens(w, tenantID)
		case http.MethodPost:
			if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermTokensWrite) {
				return
			}
			r.createTenantToken(w, req, tenantID)
		default:
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	if len(parts) == 6 {
		tokenID := parts[5]
		switch req.Method {
		case http.MethodGet:
			if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermTokensRead) {
				return
			}
			r.getTenantTokenDetail(w, tenantID, tokenID)
		case http.MethodDelete:
			if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermTokensWrite) {
				return
			}
			r.deleteTenantToken(w, tenantID, tokenID)
		default:
			apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid token path", nil)
}

func (r *Router) listTenantTokens(w http.ResponseWriter, tenantID uuid.UUID) {
	rows, err := r.store.DB().Query(
		`SELECT id, token, tag, max_uses, used_count, expires_at, created_at, status 
		 FROM tokens WHERE tenant_id = $1 ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list tokens: "+err.Error(), nil)
		return
	}
	defer rows.Close()

	var tokens []map[string]interface{}
	for rows.Next() {
		var id uuid.UUID
		var tokenStr, status string
		var tag sql.NullString
		var maxUses, usedCount int
		var expiresAt, createdAt time.Time
		if err := rows.Scan(&id, &tokenStr, &tag, &maxUses, &usedCount, &expiresAt, &createdAt, &status); err != nil {
			apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to scan token: "+err.Error(), nil)
			return
		}
		tokens = append(tokens, redactedTenantTokenResponse(id.String(), tokenStr, tag.String, maxUses, usedCount, expiresAt, createdAt, status))
	}
	if err := rows.Err(); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list tokens: "+err.Error(), nil)
		return
	}
	apibase.WriteSuccess(w, tokens, fmt.Sprintf("%d tokens retrieved", len(tokens)))
}

func (r *Router) createTenantToken(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	var body struct {
		Tag     string `json:"tag"`
		MaxUses *int   `json:"max_uses"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}
	maxUses := 1
	if body.MaxUses != nil {
		if *body.MaxUses < 0 {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "max_uses cannot be negative", nil)
			return
		}
		maxUses = *body.MaxUses
	}

	// 调用 internal/token 中的 Generate 顶级函数
	t, err := token.Generate(token.GenerateOptions{
		Tag:      body.Tag,
		TenantID: tenantID.String(),
		MaxUses:  maxUses,
		TTL:      30 * 24 * time.Hour, // 默认 30 天
	})
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to generate token: "+err.Error(), nil)
		return
	}

	// 持久化
	if err := r.tokenStore.Create(t); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to save token: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, t, "Token created successfully")
}

func (r *Router) getTenantTokenDetail(w http.ResponseWriter, tenantID uuid.UUID, tokenIDStr string) {
	t, err := r.tokenStore.GetByID(tokenIDStr)
	if err != nil || t == nil {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeNodeNotFound, "Token not found", nil)
		return
	}

	if t.TenantID != tenantID.String() {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied", nil)
		return
	}

	apibase.WriteSuccess(w, tokenDetailResponse(t, false), "Token detail retrieved")
}

func (r *Router) deleteTenantToken(w http.ResponseWriter, tenantID uuid.UUID, tokenIDStr string) {
	// 先验证归属
	t, err := r.tokenStore.GetByID(tokenIDStr)
	if err != nil || t == nil {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeNodeNotFound, "Token not found", nil)
		return
	}
	if t.TenantID != tenantID.String() {
		apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Access denied", nil)
		return
	}

	// Store 的 Delete 接收的是 token 字符串
	if err := r.tokenStore.Delete(t.Token); err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to delete token", nil)
		return
	}

	apibase.WriteSuccess(w, map[string]string{"id": tokenIDStr}, "Token deleted successfully")
}

func tokenDetailResponse(t *token.Token, includeSecret bool) map[string]interface{} {
	if t == nil {
		return map[string]interface{}{}
	}

	resp := redactedTenantTokenResponse(
		t.ID.String(),
		t.Token,
		t.Tag,
		t.MaxUses,
		t.UsedCount,
		t.ExpiresAt,
		t.CreatedAt,
		string(t.Status),
	)
	resp["tenant_id"] = t.TenantID
	resp["created_by"] = t.CreatedBy
	resp["last_used_at"] = t.LastUsedAt
	resp["last_used_by"] = t.LastUsedBy
	if includeSecret {
		resp["token"] = t.Token
	}
	return resp
}

func redactedTenantTokenResponse(id, tokenStr, tag string, maxUses, usedCount int, expiresAt, createdAt time.Time, status string) map[string]interface{} {
	return map[string]interface{}{
		"id":            id,
		"token_preview": tokenSecretPreview(tokenStr),
		"tag":           tag,
		"max_uses":      maxUses,
		"used_count":    usedCount,
		"expires_at":    expiresAt,
		"created_at":    createdAt,
		"status":        status,
	}
}

func tokenSecretPreview(tokenStr string) string {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return ""
	}
	if len(tokenStr) <= 10 {
		return "redacted"
	}
	return tokenStr[:6] + "..." + tokenStr[len(tokenStr)-4:]
}

// handleTenantAI 处理租户作用域的 AI 助手请求
func (r *Router) handleTenantAI(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermAiUse) {
		return
	}

	parts := splitPath(req.URL.Path)
	// URL: /api/v2/tenants/{tid}/ai/{chat|confirm}
	if len(parts) < 6 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid AI path", nil)
		return
	}

	req2 := withTenantContext(req, tenantID)
	switch parts[5] {
	case "chat":
		r.chatAPI.HandleChat(w, req2)
	case "confirm":
		r.chatAPI.HandleConfirm(w, req2)
	default:
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown AI endpoint", nil)
	}
}
