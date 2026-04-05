#!/bin/bash
#
# Aria Agent 部署脚本
# 用法: sudo ./deploy-agent.sh --server <CONTROLLER_URL> --token <ENROLLMENT_TOKEN> [OPTIONS]
#
# 必选参数:
#   --server   Controller gRPC 地址 (例如 https://82.156.48.111:50051)
#   --token    注册 Token (从 Controller 创建)
#
# 可选参数:
#   --hostname    节点主机名 (默认: 自动检测)
#   --region      节点区域 (默认: default)
#   --ca-cert     CA 证书路径 (本地文件路径, 将被复制到 /etc/aria/ca.crt)
#   --tls-name    TLS Server Name (默认: 不验证)
#   --binary      aria-agent 二进制文件路径 (默认: 脚本同目录下)
#   --uninstall   卸载 agent
#

set -euo pipefail

# ======================== 颜色输出 ========================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# ======================== 默认值 ========================
AGENT_BINARY="/usr/local/bin/aria-agent"
CONFIG_DIR="/etc/aria"
STATE_DIR="/var/lib/aria"
LOG_DIR="/var/log/aria"
CONFIG_FILE="${CONFIG_DIR}/agent.yaml"
STATE_FILE="${STATE_DIR}/agent-state.yaml"
SERVICE_NAME="aria-agent"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
SOCKET_PATH="/run/aria-agent.sock"

# ======================== 解析参数 ========================
SERVER=""
TOKEN=""
HOSTNAME=""
REGION="default"
CA_CERT=""
TLS_SERVER_NAME=""
BINARY_PATH=""
ACTION="install"

while [[ $# -gt 0 ]]; do
    case $1 in
        --server)   SERVER="$2"; shift 2 ;;
        --token)    TOKEN="$2"; shift 2 ;;
        --hostname) HOSTNAME="$2"; shift 2 ;;
        --region)   REGION="$2"; shift 2 ;;
        --ca-cert)  CA_CERT="$2"; shift 2 ;;
        --tls-server-name) TLS_SERVER_NAME="$2"; shift 2 ;;
        --binary)   BINARY_PATH="$2"; shift 2 ;;
        --uninstall) ACTION="uninstall"; shift ;;
        -h|--help)
            head -n 18 "$0" | tail -n 14 | sed 's/^# //' | sed 's/^#//'
            exit 0
            ;;
        *) error "Unknown parameter: $1" ;;
    esac
done

# ======================== 卸载 ========================
uninstall() {
    info "Uninstalling Aria Agent..."

    # Stop service
    if systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; then
        systemctl stop ${SERVICE_NAME}
        info "Service stopped"
    fi
    if systemctl is-enabled --quiet ${SERVICE_NAME} 2>/dev/null; then
        systemctl disable ${SERVICE_NAME}
        info "Service disabled"
    fi

    # Remove WireGuard interface
    if ip link show aria0 &>/dev/null; then
        ip link del aria0 2>/dev/null || true
        info "WireGuard interface aria0 removed"
    fi

    # Clean files
    rm -f "${SERVICE_FILE}"
    rm -f "${AGENT_BINARY}"
    rm -f "${SOCKET_PATH}"
    rm -rf "${CONFIG_DIR}"
    rm -rf "${STATE_DIR}"
    rm -rf "${LOG_DIR}"
    systemctl daemon-reload

    info "Aria Agent uninstalled successfully"
}

if [[ "${ACTION}" == "uninstall" ]]; then
    uninstall
    exit 0
fi

# ======================== 安装前校验 ========================
[[ "${EUID}" -ne 0 ]] && error "This script must be run as root"
[[ -z "${SERVER}" ]] && error "--server is required"
[[ -z "${TOKEN}" ]] && error "--token is required"

# Determine binary path
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [[ -z "${BINARY_PATH}" ]]; then
    if [[ -f "${SCRIPT_DIR}/aria-agent" ]]; then
        BINARY_PATH="${SCRIPT_DIR}/aria-agent"
    else
        error "aria-agent binary not found. Use --binary to specify path."
    fi
fi
[[ ! -f "${BINARY_PATH}" ]] && error "Binary not found: ${BINARY_PATH}"

info "==========================================="
info "Aria Agent Deployment"
info "==========================================="
info "  Controller: ${SERVER}"
info "  Token:      ${TOKEN:0:16}..."
info "  Region:     ${REGION}"
info "  Binary:     ${BINARY_PATH}"
[[ -n "${HOSTNAME}" ]] && info "  Hostname:   ${HOSTNAME}"
[[ -n "${CA_CERT}" ]] && info "  CA Cert:    ${CA_CERT}"
[[ -n "${TLS_SERVER_NAME}" ]] && info "  TLS Name:   ${TLS_SERVER_NAME}"
info "==========================================="

# ======================== Step 1: Clean existing ========================
info "Step 1: Cleaning existing installation..."

# Stop service if running
if systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; then
    systemctl stop ${SERVICE_NAME} || true
    info "  Stopped existing service"
fi

# Kill any stray processes
pkill -f aria-agent 2>/dev/null || true
sleep 1

# Remove old WireGuard interface
if ip link show aria0 &>/dev/null; then
    ip link del aria0 2>/dev/null || true
    info "  Removed old WireGuard interface"
fi

# Clean old state
rm -f "${SOCKET_PATH}"
rm -f "${STATE_FILE}"
rm -f "${CONFIG_FILE}"
info "  Cleaned old state"

# ======================== Step 2: Install dependencies ========================
info "Step 2: Installing dependencies..."

if ! command -v wg &>/dev/null; then
    apt-get update -qq
    apt-get install -y -qq wireguard-tools iproute2 2>&1 | tail -1
    info "  Installed wireguard-tools"
else
    info "  wireguard-tools already installed"
fi

# Load WireGuard kernel module
modprobe wireguard 2>/dev/null || true
if lsmod | grep -q wireguard; then
    info "  WireGuard kernel module loaded"
else
    warn "  WireGuard kernel module not loaded - may need manual load"
fi

# ======================== Step 3: Create directories ========================
info "Step 3: Creating directories..."
mkdir -p "${CONFIG_DIR}"
mkdir -p "${STATE_DIR}"
mkdir -p "${LOG_DIR}"

# ======================== Step 4: Install binary ========================
info "Step 4: Installing agent binary..."
cp "${BINARY_PATH}" "${AGENT_BINARY}"
chmod +x "${AGENT_BINARY}"
info "  Installed to ${AGENT_BINARY}"

# ======================== Step 5: Install CA cert ========================
if [[ -n "${CA_CERT}" ]]; then
    info "Step 5: Installing CA certificate..."
    cp "${CA_CERT}" "${CONFIG_DIR}/ca.crt"
    chmod 644 "${CONFIG_DIR}/ca.crt"
    info "  CA cert installed to ${CONFIG_DIR}/ca.crt"
else
    info "Step 5: Skipping CA cert (not provided)"
fi

# ======================== Step 6: Initialize agent ========================
info "Step 6: Initializing agent..."

INIT_ARGS=(
    init
    --server "${SERVER}"
    --token "${TOKEN}"
    --region "${REGION}"
    --config "${CONFIG_FILE}"
)

[[ -n "${HOSTNAME}" ]] && INIT_ARGS+=(--hostname "${HOSTNAME}")
[[ -f "${CONFIG_DIR}/ca.crt" ]] && INIT_ARGS+=(--ca-cert "${CONFIG_DIR}/ca.crt")
[[ -n "${TLS_SERVER_NAME}" ]] && INIT_ARGS+=(--tls-server-name "${TLS_SERVER_NAME}")

if ! "${AGENT_BINARY}" "${INIT_ARGS[@]}"; then
    error "Agent initialization failed! Check token and controller connectivity."
fi
info "  Agent initialized successfully"

# ======================== Step 7: Install systemd service ========================
info "Step 7: Installing systemd service..."

cat > "${SERVICE_FILE}" << 'EOF'
[Unit]
Description=Aria SD-WAN Agent
Documentation=https://github.com/aria-sdwan
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/aria-agent up --interface aria0 --config /etc/aria/agent.yaml
ExecStopPost=/bin/bash -c 'ip link del aria0 2>/dev/null || true'
Restart=always
RestartSec=5
LimitNOFILE=65536

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=aria-agent

# Environment
Environment=RUST_LOG=info

[Install]
WantedBy=multi-user.target
EOF

chmod 644 "${SERVICE_FILE}"
systemctl daemon-reload
info "  Service file installed"

# ======================== Step 8: Start service ========================
info "Step 8: Starting agent service..."
systemctl enable ${SERVICE_NAME}
systemctl start ${SERVICE_NAME}
sleep 3

# ======================== Step 9: Verify ========================
info "Step 9: Verifying deployment..."

if systemctl is-active --quiet ${SERVICE_NAME}; then
    info "  Service: ${GREEN}running${NC}"
else
    error "  Service: ${RED}failed${NC} — check: journalctl -u ${SERVICE_NAME} -n 50"
fi

# Check assigned IP
sleep 2
if [[ -f "${STATE_FILE}" ]]; then
    ASSIGNED_IP=$(grep 'assigned_ip' "${STATE_FILE}" | awk '{print $2}' | tr -d '"' || echo "")
    if [[ -n "${ASSIGNED_IP}" ]]; then
        info "  Assigned IP: ${ASSIGNED_IP}"
    fi
fi

# Check WireGuard interface
if ip link show aria0 &>/dev/null; then
    info "  WireGuard:  ${GREEN}aria0 active${NC}"
else
    warn "  WireGuard:  interface aria0 not found yet (may take a few seconds)"
fi

echo ""
info "==========================================="
info "Aria Agent deployed successfully!"
info "==========================================="
info "  Service:  systemctl status ${SERVICE_NAME}"
info "  Logs:     journalctl -u ${SERVICE_NAME} -f"
info "  Config:   ${CONFIG_FILE}"
info "  State:    ${STATE_FILE}"
info "==========================================="
