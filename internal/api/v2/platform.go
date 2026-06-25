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

func (r *Router) HandleAgentInstallerScript(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(agentInstallerScript()))
}

func (r *Router) HandleAgentBinaryDownload(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}
	path := currentAgentArtifactPath()
	if path == "" {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Agent artifact is not configured", nil)
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Agent artifact not found", nil)
		return
	}
	checksum := sha256Hex(content)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="aria-agent-linux-amd64"`)
	w.Header().Set("X-Aria-Artifact-SHA256", checksum)
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
	path := currentAgentArtifactPath()
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return sha256Hex(content)
}

func currentAgentArtifactPath() string {
	if path := strings.TrimSpace(os.Getenv("ARIA_AGENT_ARTIFACT_PATH")); path != "" {
		return path
	}
	return "/root/aria-controller/artifacts/aria-agent-linux-amd64"
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

func agentInstallerScript() string {
	return `#!/usr/bin/env bash
set -euo pipefail

CONTROLLER_API_URL=""
SERVER=""
TOKEN=""
CA_URL=""
CA_SHA256=""
TLS_SERVER_NAME=""
REGION="default"
INTERFACE="aria0"
HOSTNAME_VALUE="$(hostname)"
PUBLIC_IP=""
PUBLIC_ENDPOINT=""
AGENT_URL=""
AGENT_SHA256=""
CONFIG_PATH="/etc/aria/agent.yaml"
CA_PATH="/etc/aria/certs/ca.crt"

usage() {
  cat <<'USAGE'
Usage: install-agent.sh --controller-api-url URL --server URL --token TOKEN [options]

Options:
  --ca-url URL
  --ca-sha256 SHA256
  --tls-server-name NAME
  --region REGION
  --interface IFACE
  --hostname HOSTNAME
  --public-ip IP|auto
  --public-endpoint HOSTPORT|auto
  --agent-url URL
  --agent-sha256 SHA256
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --controller-api-url) CONTROLLER_API_URL="${2:-}"; shift 2 ;;
    --server) SERVER="${2:-}"; shift 2 ;;
    --token) TOKEN="${2:-}"; shift 2 ;;
    --ca-url) CA_URL="${2:-}"; shift 2 ;;
    --ca-sha256) CA_SHA256="${2:-}"; shift 2 ;;
    --tls-server-name) TLS_SERVER_NAME="${2:-}"; shift 2 ;;
    --region) REGION="${2:-}"; shift 2 ;;
    --interface) INTERFACE="${2:-}"; shift 2 ;;
    --hostname) HOSTNAME_VALUE="${2:-}"; shift 2 ;;
    --public-ip) PUBLIC_IP="${2:-}"; shift 2 ;;
    --public-endpoint) PUBLIC_ENDPOINT="${2:-}"; shift 2 ;;
    --agent-url) AGENT_URL="${2:-}"; shift 2 ;;
    --agent-sha256) AGENT_SHA256="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

fail() {
  echo "aria-agent installer failed: $*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

install_runtime_packages() {
  echo "Installing Aria Agent runtime dependencies"
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update || fail "apt-get update failed while installing runtime dependencies"
    apt-get install -y iproute2 wireguard-tools || fail "apt-get install iproute2 wireguard-tools failed"
    return
  fi
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y iproute wireguard-tools || fail "dnf install iproute wireguard-tools failed"
    return
  fi
  if command -v yum >/dev/null 2>&1; then
    yum install -y iproute wireguard-tools || fail "yum install iproute wireguard-tools failed"
    return
  fi
  if command -v apk >/dev/null 2>&1; then
    apk add --no-cache iproute2 wireguard-tools || fail "apk add iproute2 wireguard-tools failed"
    return
  fi
  fail "missing ip or wg; install iproute2 and wireguard-tools before running this installer"
}

ensure_runtime_commands() {
  if command -v ip >/dev/null 2>&1 && command -v wg >/dev/null 2>&1; then
    return
  fi
  install_runtime_packages
  need ip
  need wg
}

[ "$(id -u)" = "0" ] || fail "run as root, for example: curl ... | sudo bash -s -- ..."
[ -n "$CONTROLLER_API_URL" ] || fail "--controller-api-url is required"
[ -n "$SERVER" ] || fail "--server is required"
[ -n "$TOKEN" ] || fail "--token is required"
[ -n "$INTERFACE" ] || fail "--interface is required"

need curl
need install
need systemctl
need sha256sum

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64|Linux-amd64) ;;
  *) fail "only linux/amd64 is supported by this installer" ;;
esac

if [ -z "$AGENT_URL" ]; then
  AGENT_URL="${CONTROLLER_API_URL%/}/api/v2/downloads/aria-agent/linux/amd64"
fi
if [ -z "$CA_URL" ]; then
  CA_URL="${CONTROLLER_API_URL%/}/api/v2/controller-info/grpc-ca.crt"
fi

if [ -e "$CONFIG_PATH" ] || [ -e "/var/lib/aria/agent-state.yaml" ]; then
  fail "existing Aria config or state found; inspect /etc/aria and /var/lib/aria before reinstalling"
fi

ensure_runtime_commands

install -d -m 0755 /etc/aria /etc/aria/certs /var/lib/aria /var/log/aria /usr/local/bin

tmp_agent="$(mktemp)"
tmp_ca="$(mktemp)"
trap 'rm -f "$tmp_agent" "$tmp_ca"' EXIT

echo "Downloading aria-agent from $AGENT_URL"
curl -fsSL "$AGENT_URL" -o "$tmp_agent" || fail "failed to download aria-agent from $AGENT_URL"
if [ -n "$AGENT_SHA256" ]; then
  echo "$AGENT_SHA256  $tmp_agent" | sha256sum -c - || fail "aria-agent checksum mismatch"
fi
install -m 0755 "$tmp_agent" /usr/local/bin/aria-agent

echo "Downloading Controller CA from $CA_URL"
curl -fsSL "$CA_URL" -o "$tmp_ca" || fail "failed to download Controller CA from $CA_URL"
if [ -n "$CA_SHA256" ]; then
  echo "$CA_SHA256  $tmp_ca" | sha256sum -c - || fail "Controller CA checksum mismatch"
fi
install -m 0600 "$tmp_ca" "$CA_PATH"

init_args=(
  init
  --server "$SERVER"
  --controller-api-url "$CONTROLLER_API_URL"
  --token "$TOKEN"
  --ca-cert "$CA_PATH"
  --region "$REGION"
  --interface "$INTERFACE"
)

[ -n "$TLS_SERVER_NAME" ] && init_args+=(--tls-server-name "$TLS_SERVER_NAME")
[ -n "$HOSTNAME_VALUE" ] && init_args+=(--hostname "$HOSTNAME_VALUE")
[ -n "$PUBLIC_IP" ] && [ "$PUBLIC_IP" != "auto" ] && init_args+=(--public-ip "$PUBLIC_IP")
[ -n "$PUBLIC_ENDPOINT" ] && [ "$PUBLIC_ENDPOINT" != "auto" ] && init_args+=(--public-endpoint "$PUBLIC_ENDPOINT")

echo "Initializing aria-agent"
/usr/local/bin/aria-agent "${init_args[@]}" || fail "aria-agent init failed"

cat >/etc/systemd/system/aria-agent.service <<UNIT
[Unit]
Description=Aria SD-WAN Agent
Documentation=https://github.com/chenyongming211-glitch/Aria
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/aria-agent up --interface ${INTERFACE} --config ${CONFIG_PATH}
Restart=always
RestartSec=5
LimitMEMLOCK=infinity
AmbientCapabilities=CAP_NET_ADMIN CAP_BPF CAP_SYS_RESOURCE
CapabilityBoundingSet=CAP_NET_ADMIN CAP_BPF CAP_SYS_RESOURCE
NoNewPrivileges=false
ExecStopPost=/bin/bash -c 'for i in {0..3}; do ip link del aria\$i 2>/dev/null || true; done'

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now aria-agent || fail "failed to start aria-agent.service"

echo "aria-agent installed and started"
systemctl status aria-agent --no-pager || true
journalctl -u aria-agent -n 80 --no-pager || true
`
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
