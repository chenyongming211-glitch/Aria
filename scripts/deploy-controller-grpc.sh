#!/bin/bash
# 部署 Controller gRPC 服务到服务器
# 使用方法：./scripts/deploy-controller-grpc.sh

set -e

VERSION="0.2.26-grpc-test"
SERVER="112.124.8.241"
IMAGE_FILE="bin/images/aria-controller-${VERSION}.tar"

echo "=================================="
echo "Controller gRPC 部署脚本"
echo "=================================="
echo "版本: $VERSION"
echo "服务器: $SERVER"
echo ""

# 步骤 1: 构建镜像
echo "📦 步骤 1/5: 构建 Docker 镜像..."
read -p "是否已启动 Docker? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ 请先启动 Docker Desktop"
    exit 1
fi

docker buildx build --platform linux/amd64 --no-cache \
    -t aria-controller:${VERSION} \
    -t aria-controller:latest \
    --build-arg VERSION=${VERSION} \
    -f Dockerfile.controller . --load

if [ $? -ne 0 ]; then
    echo "❌ 镜像构建失败"
    exit 1
fi

echo "✅ 镜像构建成功"

# 步骤 2: 保存镜像
echo ""
echo "💾 步骤 2/5: 保存镜像到文件..."
mkdir -p bin/images
docker save aria-controller:${VERSION} -o ${IMAGE_FILE}

if [ $? -ne 0 ]; then
    echo "❌ 镜像保存失败"
    exit 1
fi

echo "✅ 镜像已保存到 ${IMAGE_FILE}"
echo "   大小: $(du -h ${IMAGE_FILE} | cut -f1)"

# 步骤 3: 上传镜像
echo ""
echo "📤 步骤 3/5: 上传镜像到服务器..."
rsync -avz --progress ${IMAGE_FILE} root@${SERVER}:/root/aria-controller/bin/images/

if [ $? -ne 0 ]; then
    echo "❌ 镜像上传失败"
    exit 1
fi

echo "✅ 镜像已上传"

# 步骤 4: 在服务器上加载镜像
echo ""
echo "🔄 步骤 4/5: 在服务器上加载镜像..."
ssh root@${SERVER} << 'EOF'
cd /root/aria-controller
docker load -i bin/images/aria-controller-0.2.26-grpc-test.tar
docker tag aria-controller:0.2.26-grpc-test aria-controller:latest
echo "✅ 镜像已加载"
EOF

if [ $? -ne 0 ]; then
    echo "❌ 镜像加载失败"
    exit 1
fi

# 步骤 5: 重启 Controller
echo ""
echo "🚀 步骤 5/5: 重启 Controller..."
ssh root@${SERVER} << 'EOF'
cd /root/aria-controller
./deploy-controller.sh deploy
echo ""
echo "等待服务启动..."
sleep 5
docker logs aria_controller --tail 20
EOF

if [ $? -ne 0 ]; then
    echo "❌ Controller 启动失败"
    exit 1
fi

echo ""
echo "✅ 部署完成！"
echo ""
echo "=================================="
echo "下一步："
echo "=================================="
echo "1. 检查 gRPC 端口："
echo "   ssh root@${SERVER} 'netstat -tlnp | grep 50051'"
echo ""
echo "2. 运行测试："
echo "   ./scripts/test-grpc.sh ${SERVER}:50051"
echo ""
echo "3. 查看日志："
echo "   ssh root@${SERVER} 'docker logs aria_controller -f'"
echo ""
