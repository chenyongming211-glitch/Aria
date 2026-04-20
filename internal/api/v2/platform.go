package v2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/internal/token"

	"github.com/google/uuid"
)

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
		var tokenStr, tag, status string
		var maxUses, usedCount int
		var expiresAt, createdAt time.Time
		if err := rows.Scan(&id, &tokenStr, &tag, &maxUses, &usedCount, &expiresAt, &createdAt, &status); err != nil {
			continue
		}
		tokens = append(tokens, map[string]interface{}{
			"id":         id.String(),
			"token":      tokenStr,
			"tag":        tag,
			"max_uses":   maxUses,
			"used_count": usedCount,
			"expires_at": expiresAt,
			"created_at": createdAt,
			"status":     status,
		})
	}
	apibase.WriteSuccess(w, tokens, fmt.Sprintf("%d tokens retrieved", len(tokens)))
}

func (r *Router) createTenantToken(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	var body struct {
		Tag     string `json:"tag"`
		MaxUses int    `json:"max_uses"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	// 调用 internal/token 中的 Generate 顶级函数
	t, err := token.Generate(token.GenerateOptions{
		Tag:      body.Tag,
		TenantID: tenantID.String(),
		MaxUses:  body.MaxUses,
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

	apibase.WriteSuccess(w, t, "Token detail retrieved")
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
