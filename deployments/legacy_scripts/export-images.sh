#!/bin/bash
# Aria Docker 镜像导出脚本
# 用途：将线上测试通过的镜像导出为 tar 文件，方便离线部署

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGES_DIR="${SCRIPT_DIR}/images"
VERSION=$(cat "${SCRIPT_DIR}/../../VERSION" 2>/dev/null || echo "latest")

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# 创建镜像目录
mkdir -p "${IMAGES_DIR}"

log_info "=========================================="
log_info "导出 Aria Docker 镜像 (版本: ${VERSION})"
log_info "=========================================="

# 导出 Controller 镜像
log_info "1/5 导出 aria-controller:${VERSION}..."
if docker image inspect aria-controller:${VERSION} &> /dev/null; then
    docker save aria-controller:${VERSION} -o "${IMAGES_DIR}/aria-controller-${VERSION}.tar"
    log_info "✓ 已保存到: ${IMAGES_DIR}/aria-controller-${VERSION}.tar"
else
    log_warn "镜像 aria-controller:${VERSION} 不存在，跳过"
fi

# 导出 VictoriaMetrics 镜像
log_info "2/5 导出 victoriametrics/victoria-metrics:latest..."
docker pull victoriametrics/victoria-metrics:latest
docker save victoriametrics/victoria-metrics:latest -o "${IMAGES_DIR}/victoria-metrics.tar"
log_info "✓ 已保存到: ${IMAGES_DIR}/victoria-metrics.tar"

# 导出 Grafana 镜像
log_info "3/5 导出 grafana/grafana:latest..."
docker pull grafana/grafana:latest
docker save grafana/grafana:latest -o "${IMAGES_DIR}/grafana.tar"
log_info "✓ 已保存到: ${IMAGES_DIR}/grafana.tar"

# 导出 vmalert 镜像
log_info "4/5 导出 victoriametrics/vmalert:latest..."
docker pull victoriametrics/vmalert:latest
docker save victoriametrics/vmalert:latest -o "${IMAGES_DIR}/vmalert.tar"
log_info "✓ 已保存到: ${IMAGES_DIR}/vmalert.tar"

# 导出 Nginx 镜像
log_info "5/5 导出 nginx:alpine..."
docker pull nginx:alpine
docker save nginx:alpine -o "${IMAGES_DIR}/nginx-alpine.tar"
log_info "✓ 已保存到: ${IMAGES_DIR}/nginx-alpine.tar"

# 生成镜像清单
log_info "生成镜像清单..."
cat > "${IMAGES_DIR}/manifest.txt" <<EOF
Aria Docker 镜像清单
版本: ${VERSION}
导出时间: $(date '+%Y-%m-%d %H:%M:%S')

镜像列表:
1. aria-controller-${VERSION}.tar
   - 镜像: aria-controller:${VERSION}
   - 用途: Aria Controller API 服务

2. victoria-metrics.tar
   - 镜像: victoriametrics/victoria-metrics:latest
   - 用途: 时序数据库

3. grafana.tar
   - 镜像: grafana/grafana:latest
   - 用途: 监控可视化面板

4. vmalert.tar
   - 镜像: victoriametrics/vmalert:latest
   - 用途: 告警规则引擎

5. nginx-alpine.tar
   - 镜像: nginx:alpine
   - 用途: Web 服务器和反向代理

导入命令:
  docker load -i aria-controller-${VERSION}.tar
  docker load -i victoria-metrics.tar
  docker load -i grafana.tar
  docker load -i vmalert.tar
  docker load -i nginx-alpine.tar

或使用批量导入脚本:
  ./import-images.sh
EOF

log_info "✓ 清单已保存到: ${IMAGES_DIR}/manifest.txt"

# 计算文件大小
log_info "=========================================="
log_info "导出完成！"
log_info "=========================================="
du -sh "${IMAGES_DIR}"/*.tar | awk '{print "  " $2 ": " $1}'

log_info ""
log_info "镜像已保存到: ${IMAGES_DIR}/"
log_info "查看清单: cat ${IMAGES_DIR}/manifest.txt"
