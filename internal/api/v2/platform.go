package v2

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
		"controller_api_url": publicControllerAPIURL(req),
		"grpc": map[string]interface{}{
			"server": publicGRPCServerURL(req),
		},
		"grpc_tls": map[string]interface{}{
			"mode":            currentGRPCTLSMode(),
			"ca_required":     currentGRPCTLSMode() != "disabled",
			"ca_cert_path":    currentGRPCCACertPath(),
			"ca_cert_url":     absoluteURL(req, "/api/v2/controller-info/grpc-ca.crt"),
			"ca_cert_sha256":  currentGRPCCACertSHA256(),
			"server_name":     currentGRPCTLSServerName(req),
			"download_method": "https",
		},
		"agent": map[string]interface{}{
			"supported_os":      []string{"linux"},
			"supported_arch":    []string{"amd64"},
			"default_interface": "aria0",
			"default_region":    "default",
			"install_dir":       "/usr/local/bin",
			"config_path":       "/etc/aria/agent.yaml",
			"state_path":        "/var/lib/aria/agent-state.yaml",
			"systemd_unit":      "aria-agent.service",
			"download_url":      absoluteURL(req, "/api/v2/downloads/aria-agent/linux/amd64"),
			"sha256":            currentAgentArtifactSHA256(),
		},
	}, "Controller capabilities retrieved")
}

func (r *Router) HandleControllerGRPCCA(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}
	if currentGRPCTLSMode() == "disabled" {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "gRPC TLS is disabled", nil)
		return
	}
	content, err := os.ReadFile(currentGRPCCACertPath())
	if err != nil {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "gRPC CA certificate not found", nil)
		return
	}
	if containsPrivateKeyMaterial(content) {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Configured gRPC CA file contains private key material", nil)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("X-Aria-CA-SHA256", sha256Hex(content))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
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

func currentGRPCTLSMode() string {
	mode := strings.TrimSpace(os.Getenv("ARIA_GRPC_TLS_MODE"))
	if mode == "" {
		return "server"
	}
	return mode
}

func currentGRPCCACertPath() string {
	path := strings.TrimSpace(os.Getenv("ARIA_GRPC_CA_CERT"))
	if path == "" {
		return "/etc/aria/certs/ca.crt"
	}
	return path
}

func currentGRPCTLSServerName(req ...*http.Request) string {
	if name := strings.TrimSpace(os.Getenv("ARIA_GRPC_TLS_SERVER_NAME")); name != "" {
		return name
	}
	if name := strings.TrimSpace(os.Getenv("ARIA_GRPC_SERVER_NAME")); name != "" {
		return name
	}
	if name := strings.TrimSpace(os.Getenv("ARIA_PUBLIC_HOST")); name != "" {
		return name
	}
	if len(req) > 0 && req[0] != nil {
		if host := requestHostWithoutPort(req[0]); host != "" {
			return host
		}
	}
	return "aria.yun"
}

func currentGRPCCACertSHA256() string {
	content, err := os.ReadFile(currentGRPCCACertPath())
	if err != nil || containsPrivateKeyMaterial(content) {
		return ""
	}
	return sha256Hex(content)
}

func currentAgentArtifactSHA256() string {
	if checksum := strings.TrimSpace(os.Getenv("ARIA_AGENT_ARTIFACT_SHA256")); checksum != "" {
		return checksum
	}
	path := strings.TrimSpace(os.Getenv("ARIA_AGENT_ARTIFACT_PATH"))
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return sha256Hex(content)
}

func publicControllerAPIURL(req *http.Request) string {
	if value := strings.TrimSpace(os.Getenv("ARIA_CONTROLLER_PUBLIC_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	if value := strings.TrimSpace(os.Getenv("ARIA_PUBLIC_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return fmt.Sprintf("%s://%s", requestScheme(req), req.Host)
}

func publicGRPCServerURL(req *http.Request) string {
	if value := strings.TrimSpace(os.Getenv("ARIA_GRPC_PUBLIC_SERVER")); value != "" {
		return value
	}
	host := currentGRPCTLSServerName(req)
	if host == "" {
		host = requestHostWithoutPort(req)
	}
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	return fmt.Sprintf("https://%s:50051", host)
}

func absoluteURL(req *http.Request, path string) string {
	return publicControllerAPIURL(req) + path
}

func requestScheme(req *http.Request) string {
	if req == nil {
		return "https"
	}
	if proto := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")); proto != "" {
		return strings.Split(proto, ",")[0]
	}
	if req.TLS != nil {
		return "https"
	}
	if req.URL != nil && req.URL.Scheme != "" {
		return req.URL.Scheme
	}
	return "https"
}

func requestHostWithoutPort(req *http.Request) string {
	if req == nil {
		return ""
	}
	host := strings.TrimSpace(req.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = req.Host
	}
	if host == "" && req.URL != nil {
		host = req.URL.Host
	}
	host = strings.Split(host, ",")[0]
	if idx := strings.LastIndex(host, ":"); idx > -1 && !strings.Contains(host[idx+1:], "]") {
		return strings.Trim(host[:idx], "[]")
	}
	return strings.Trim(host, "[]")
}

func containsPrivateKeyMaterial(content []byte) bool {
	text := strings.ToUpper(string(content))
	return strings.Contains(text, "PRIVATE KEY")
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
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
		TTL     string `json:"ttl"`
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
	ttl, err := parseTenantTokenTTL(body.TTL)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, err.Error(), nil)
		return
	}

	// 调用 internal/token 中的 Generate 顶级函数
	t, err := token.Generate(token.GenerateOptions{
		Tag:      body.Tag,
		TenantID: tenantID.String(),
		MaxUses:  maxUses,
		TTL:      ttl,
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

const (
	defaultTenantTokenTTL = 30 * 24 * time.Hour
	maxTenantTokenTTL     = 365 * 24 * time.Hour
)

func parseTenantTokenTTL(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultTenantTokenTTL, nil
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid ttl format; use values like 1h, 24h, or 168h")
	}
	if ttl <= 0 {
		return 0, fmt.Errorf("ttl must be greater than 0")
	}
	if ttl > maxTenantTokenTTL {
		return 0, fmt.Errorf("ttl cannot exceed 8760h")
	}
	return ttl, nil
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
