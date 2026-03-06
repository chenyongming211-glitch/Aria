#!/bin/bash
# Aria Controller 部署脚本
# PostgreSQL/Redis 使用系统包安装，Controller 使用 Docker

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
IMAGES_DIR="$SCRIPT_DIR/images"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# 检查 Docker
check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装，请先安装 Docker"
        exit 1
    fi
    if ! docker info &> /dev/null; then
        log_error "Docker 未运行，请启动 Docker 服务"
        exit 1
    fi
    log_info "Docker 检查通过"
}

# 安装 PostgreSQL 和 Redis（系统包）
install_database() {
    log_info "安装 PostgreSQL 和 Redis..."

    # 检测系统类型
    if [ -f /etc/debian_version ]; then
        PKG_MANAGER="apt"
        log_info "检测到 Debian/Ubuntu 系统"
    elif [ -f /etc/redhat-release ]; then
        PKG_MANAGER="yum"
        log_info "检测到 RHEL/CentOS 系统"
    else
        log_error "不支持的系统类型"
        exit 1
    fi

    # 安装 PostgreSQL
    log_info "安装 PostgreSQL..."
    if [ "$PKG_MANAGER" = "apt" ]; then
        apt-get update -qq
        apt-get install -y -qq postgresql postgresql-contrib
    else
        yum install -y -q postgresql-server postgresql-contrib
        postgresql-setup --initdb 2>/dev/null || true
    fi

    systemctl enable postgresql
    systemctl start postgresql

    # 配置 PostgreSQL
    log_info "配置 PostgreSQL 数据库..."
    sudo -u postgres psql -c "DROP USER IF EXISTS aria;" 2>/dev/null || true
    sudo -u postgres psql <<EOF
CREATE USER aria WITH PASSWORD 'aria-local-password';
CREATE DATABASE aria OWNER aria;
GRANT ALL PRIVILEGES ON DATABASE aria TO aria;
EOF

    # 配置 pg_hba.conf 允许密码认证
    PG_HBA=$(sudo -u postgres psql -t -c "SHOW hba_file;" | xargs)
    if ! grep -q "^host.*aria.*aria.*127.0.0.1" "$PG_HBA" 2>/dev/null; then
        echo "host    aria    aria    127.0.0.1/32    md5" >> "$PG_HBA"
        echo "host    aria    aria    ::1/128         md5" >> "$PG_HBA"
    fi
    systemctl restart postgresql

    # 安装 Redis
    log_info "安装 Redis..."
    if [ "$PKG_MANAGER" = "apt" ]; then
        apt-get install -y -qq redis-server
        systemctl enable redis-server
        systemctl start redis-server
    else
        yum install -y -q redis
        systemctl enable redis
        systemctl start redis
    fi

    log_info "数据库安装完成"
    echo ""
    echo "  PostgreSQL: 127.0.0.1:5432 (用户: aria, 数据库: aria)"
    echo "  Redis:      127.0.0.1:6379"
}

# 加载 Controller 镜像
load_controller_image() {
    log_info "加载 Controller Docker 镜像..."

    if [[ -f "$IMAGES_DIR/aria-controller.tar" ]]; then
        docker load -i "$IMAGES_DIR/aria-controller.tar"
        log_info "镜像加载完成"
    else
        log_error "找不到镜像文件: $IMAGES_DIR/aria-controller.tar"
        exit 1
    fi
}

# 部署 Controller
deploy_controller() {
    log_info "部署 Controller..."

    # 创建目录
    mkdir -p /var/log/aria /etc/aria

    # 复制配置文件
    cp "$SCRIPT_DIR/controller.yaml" /etc/aria/
    cp "$SCRIPT_DIR/docker-compose.yml" /etc/aria/

    # 启动 Controller
    cd /etc/aria
    docker compose up -d

    log_info "等待服务启动..."
    sleep 5

    # 检查服务状态
    if curl -s http://localhost:8080/nodes > /dev/null 2>&1; then
        log_info "Controller 部署成功"
        echo ""
        echo "  API 地址: http://$(hostname -I | awk '{print $1}'):8080"
        echo ""
        echo "  创建 Token: docker exec aria_controller aria token create --tag=test"
    else
        log_warn "Controller 启动中，请稍后检查..."
        docker logs aria_controller --tail 20
    fi
}

# 完整部署（数据库 + Controller）
deploy_full() {
    install_database
    echo ""
    load_controller_image
    echo ""
    deploy_controller
}

# 停止服务
stop_services() {
    log_info "停止 Controller..."
    cd /etc/aria 2>/dev/null && docker compose down 2>/dev/null || docker stop aria_controller 2>/dev/null || true
    log_info "服务已停止"
}

# 查看状态
show_status() {
    echo ""
    echo "=== 系统服务状态 ==="
    systemctl is-active postgresql 2>/dev/null && echo "PostgreSQL: 运行中" || echo "PostgreSQL: 未运行"
    systemctl is-active redis-server 2>/dev/null && echo "Redis: 运行中" || systemctl is-active redis 2>/dev/null && echo "Redis: 运行中" || echo "Redis: 未运行"

    echo ""
    echo "=== Docker 容器状态 ==="
    docker ps -a --filter name=aria_controller --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || echo "无 Controller 容器"

    echo ""
    echo "=== API 健康检查 ==="
    if curl -s http://localhost:8080/nodes > /dev/null 2>&1; then
        echo "Controller API: 正常"
        echo "已注册节点: $(curl -s http://localhost:8080/nodes | grep -o '"hostname"' | wc -l)"
    else
        echo "Controller API: 无响应"
    fi
}

# 使用说明
show_usage() {
    echo ""
    echo "Aria Controller 部署脚本"
    echo ""
    echo "用法: $0 <命令>"
    echo ""
    echo "命令:"
    echo "  install     完整安装（PostgreSQL + Redis + Controller）"
    echo "  db          仅安装数据库（PostgreSQL + Redis）"
    echo "  controller  仅部署 Controller（需先安装数据库）"
    echo "  stop        停止 Controller"
    echo "  status      查看服务状态"
    echo ""
    echo "示例:"
    echo "  $0 install     # 首次部署，安装全部组件"
    echo "  $0 status      # 检查服务状态"
    echo ""
}

# 主函数
main() {
    if [[ $# -eq 0 ]]; then
        show_usage
        exit 0
    fi

    case "$1" in
        install)
            check_docker
            deploy_full
            ;;
        db)
            install_database
            ;;
        controller)
            check_docker
            load_controller_image
            deploy_controller
            ;;
        stop)
            stop_services
            ;;
        status)
            show_status
            ;;
        *)
            show_usage
            exit 1
            ;;
    esac
}

main "$@"
