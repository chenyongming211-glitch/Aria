#!/bin/bash
# Aria Docker 镜像导入脚本
# 用途：批量导入导出的镜像文件

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGES_DIR="${SCRIPT_DIR}/images"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

if [ ! -d "${IMAGES_DIR}" ]; then
    log_error "镜像目录不存在: ${IMAGES_DIR}"
    exit 1
fi

log_info "=========================================="
log_info "导入 Aria Docker 镜像"
log_info "=========================================="

count=0
for tar_file in "${IMAGES_DIR}"/*.tar; do
    if [ -f "$tar_file" ]; then
        log_info "导入: $(basename "$tar_file")..."
        docker load -i "$tar_file"
        ((count++))
    fi
done

log_info "=========================================="
log_info "导入完成！共导入 ${count} 个镜像"
log_info "=========================================="

log_info "已导入的镜像："
docker images | grep -E "(aria-controller|victoria-metrics|grafana|vmalert|nginx)" | head -10
