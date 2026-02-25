#!/bin/bash
# Aria Agent 部署脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

show_usage() {
    echo ""
    echo "Aria Agent 部署脚本"
    echo ""
    echo "用法: $0 <命令> [参数]"
    echo ""
    echo "命令:"
    echo "  install <controller-url> <token>  安装并连接到 Controller"
    echo "  connect <controller-url> <token>  仅连接（不安装服务）"
    echo "  service                           安装为 Systemd 服务"
    echo "  tune                              手动执行网络性能优化"
    echo "  status                            查看连接状态"
    echo "  stop                              断开连接"
    echo ""
    echo "示例:"
    echo "  $0 install http://10.0.0.1:8080 tk_xxxxxxxx"
    echo "  $0 tune    # 单独执行网络优化"
    echo "  $0 status"
    echo ""
    echo "网络优化 (aria up --tune 默认开启):"
    echo "  - BBR 拥塞控制：跨公网速度提升 2-10 倍"
    echo "  - RPS 软中断均衡：解决单核 CPU 100% 问题"
    echo "  - MSS Clamping：防止隧道内 PMTU 黑洞"
    echo ""
}

# 检测包管理器
detect_pkg_manager() {
    if command -v apt-get &>/dev/null; then
        PKG_MANAGER="apt"
    elif command -v yum &>/dev/null; then
        PKG_MANAGER="yum"
    elif command -v dnf &>/dev/null; then
        PKG_MANAGER="dnf"
    else
        PKG_MANAGER="unknown"
    fi
}

# 内核版本检查和 WireGuard 依赖安装
check_kernel() {
    log_info "检查内核版本..."

    KERNEL_VERSION=$(uname -r)
    KERNEL_MAJOR=$(echo "$KERNEL_VERSION" | cut -d. -f1)
    KERNEL_MINOR=$(echo "$KERNEL_VERSION" | cut -d. -f2)

    if [[ "$KERNEL_MAJOR" -lt 5 ]] || [[ "$KERNEL_MAJOR" -eq 5 && "$KERNEL_MINOR" -lt 6 ]]; then
        log_warn "内核版本 $KERNEL_VERSION < 5.6"
        log_warn "将使用 userspace 模式运行（性能约降低 30%）"
        log_warn "建议升级内核到 5.6+ 以获得最佳性能"

        # 安装 wireguard-tools (用于 wg 命令)
        log_info "安装 wireguard-tools..."
        detect_pkg_manager
        case "$PKG_MANAGER" in
            apt)
                apt-get update -qq
                apt-get install -y -qq wireguard-tools
                ;;
            yum|dnf)
                $PKG_MANAGER install -y -q wireguard-tools
                ;;
            *)
                log_warn "未知的包管理器，请手动安装 wireguard-tools"
                ;;
        esac
    else
        log_info "内核版本 $KERNEL_VERSION >= 5.6，支持 kernel 模式"

        # 确保内核模块加载
        modprobe wireguard 2>/dev/null || true

        # 如果 wireguard 模块未加载，安装 wireguard-tools
        if ! lsmod | grep -q wireguard; then
            log_info "加载 WireGuard 内核模块..."
            detect_pkg_manager
            case "$PKG_MANAGER" in
                apt)
                    apt-get update -qq
                    apt-get install -y -qq wireguard-tools
                    ;;
                yum|dnf)
                    $PKG_MANAGER install -y -q wireguard-tools
                    ;;
            esac
            modprobe wireguard 2>/dev/null || true
        fi
    fi

    # 验证 wg 命令可用
    if ! command -v wg &>/dev/null; then
        log_error "wg 命令不可用，请安装 wireguard-tools"
        exit 1
    fi

    log_info "WireGuard 工具检查通过"
}

# 安装二进制
install_binary() {
    log_info "安装 Aria 二进制..."
    cp "$SCRIPT_DIR/aria" /usr/local/bin/aria
    chmod +x /usr/local/bin/aria
    log_info "已安装到 /usr/local/bin/aria"
}

# 连接到 Controller
connect_controller() {
    local CONTROLLER_URL="$1"
    local TOKEN="$2"

    if [[ -z "$CONTROLLER_URL" ]] || [[ -z "$TOKEN" ]]; then
        log_error "缺少参数: controller-url 和 token 都是必需的"
        show_usage
        exit 1
    fi

    log_info "连接到 Controller: $CONTROLLER_URL"
    aria up --server="$CONTROLLER_URL" --token="$TOKEN"
}

# 安装为 Systemd 服务
install_service() {
    if [[ ! -f /etc/aria/agent.yaml ]]; then
        log_error "配置文件不存在，请先运行 'connect' 命令"
        exit 1
    fi

    log_info "安装 Systemd 服务..."
    aria install
}

# 完整安装流程
full_install() {
    local CONTROLLER_URL="$1"
    local TOKEN="$2"

    check_kernel
    echo ""
    install_binary
    echo ""
    connect_controller "$CONTROLLER_URL" "$TOKEN"
    echo ""
    install_service

    echo ""
    log_info "部署完成！"
    echo ""
    echo "  启动服务: systemctl start aria"
    echo "  查看状态: aria status"
    echo "  查看日志: journalctl -u aria -f"
}

# 主函数
main() {
    if [[ $# -eq 0 ]]; then
        show_usage
        exit 0
    fi

    case "$1" in
        install)
            full_install "$2" "$3"
            ;;
        connect)
            install_binary
            connect_controller "$2" "$3"
            ;;
        service)
            install_service
            ;;
        tune)
            log_info "执行网络性能优化..."
            aria tune -v
            ;;
        status)
            aria status
            echo ""
            aria tune --status
            ;;
        stop)
            aria down
            ;;
        *)
            show_usage
            exit 1
            ;;
    esac
}

main "$@"
