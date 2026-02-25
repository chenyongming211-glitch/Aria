#!/bin/bash
# Aria 统一部署脚本
# 用途：一键部署 Controller + 监控服务
# 使用：./deploy-all.sh [start|stop|restart|status]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTROLLER_DIR="${SCRIPT_DIR}/controller-web"
MONITORING_DIR="${CONTROLLER_DIR}/monitoring"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查 Docker 和 Docker Compose
check_requirements() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装，请先安装 Docker"
        exit 1
    fi

    if ! docker compose version &> /dev/null; then
        log_error "Docker Compose 未安装或版本过低，请升级到 v2+"
        exit 1
    fi
}

# 创建共享网络
create_network() {
    if ! docker network inspect aria_shared_net &> /dev/null; then
        log_info "创建共享网络 aria_shared_net..."
        docker network create aria_shared_net --subnet 172.30.0.0/16
    else
        log_info "共享网络 aria_shared_net 已存在"
    fi
}

# 启动服务
start_services() {
    log_info "=========================================="
    log_info "开始部署 Aria 服务"
    log_info "=========================================="

    check_requirements
    create_network

    # 1. 启动监控服务
    log_info "步骤 1/2: 启动监控服务 (VictoriaMetrics, Grafana, vmalert)..."
    cd "${MONITORING_DIR}"
    docker compose up -d

    log_info "等待监控服务健康检查..."
    sleep 10

    # 2. 启动 Controller 服务
    log_info "步骤 2/2: 启动 Controller 服务 (Nginx + API)..."
    cd "${CONTROLLER_DIR}"
    docker compose up -d

    log_info "等待 Controller 服务启动..."
    sleep 5

    log_info "=========================================="
    log_info "部署完成！"
    log_info "=========================================="
    show_status
}

# 停止服务
stop_services() {
    log_info "停止 Aria 服务..."

    cd "${CONTROLLER_DIR}"
    docker compose down

    cd "${MONITORING_DIR}"
    docker compose down

    log_info "所有服务已停止"
}

# 重启服务
restart_services() {
    log_info "重启 Aria 服务..."
    stop_services
    sleep 2
    start_services
}

# 查看状态
show_status() {
    log_info "=========================================="
    log_info "服务状态"
    log_info "=========================================="

    docker ps --filter "name=aria" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

    echo ""
    log_info "访问地址："
    echo "  - Controller Web UI: https://localhost (或配置的域名)"
    echo "  - Grafana 监控面板: http://localhost:3000 (admin/admin)"
    echo "  - VictoriaMetrics: http://localhost:8428"
    echo "  - vmalert: http://localhost:8880"
}

# 查看日志
show_logs() {
    local service=$1
    if [ -z "$service" ]; then
        log_info "显示所有服务日志..."
        docker logs aria_controller --tail 50 -f &
        docker logs aria_web --tail 50 -f &
        docker logs aria-victoria-metrics --tail 50 -f &
        wait
    else
        docker logs "$service" --tail 100 -f
    fi
}

# 健康检查
health_check() {
    log_info "执行健康检查..."

    local all_healthy=true

    # 检查 Controller
    if curl -sf http://localhost:8080/nodes > /dev/null 2>&1; then
        log_info "✓ Controller API 健康"
    else
        log_error "✗ Controller API 不健康"
        all_healthy=false
    fi

    # 检查 VictoriaMetrics
    if curl -sf http://localhost:8428/health > /dev/null 2>&1; then
        log_info "✓ VictoriaMetrics 健康"
    else
        log_error "✗ VictoriaMetrics 不健康"
        all_healthy=false
    fi

    # 检查 Grafana
    if curl -sf http://localhost:3000/api/health > /dev/null 2>&1; then
        log_info "✓ Grafana 健康"
    else
        log_error "✗ Grafana 不健康"
        all_healthy=false
    fi

    if [ "$all_healthy" = true ]; then
        log_info "所有服务健康 ✓"
        return 0
    else
        log_error "部分服务不健康，请检查日志"
        return 1
    fi
}

# 主函数
main() {
    case "${1:-start}" in
        start)
            start_services
            ;;
        stop)
            stop_services
            ;;
        restart)
            restart_services
            ;;
        status)
            show_status
            ;;
        logs)
            show_logs "$2"
            ;;
        health)
            health_check
            ;;
        *)
            echo "用法: $0 {start|stop|restart|status|logs [service]|health}"
            echo ""
            echo "命令说明："
            echo "  start   - 启动所有服务"
            echo "  stop    - 停止所有服务"
            echo "  restart - 重启所有服务"
            echo "  status  - 查看服务状态"
            echo "  logs    - 查看日志 (可选指定服务名)"
            echo "  health  - 健康检查"
            exit 1
            ;;
    esac
}

main "$@"
