#!/bin/bash
# gRPC Controller 测试脚本

set -e

CONTROLLER_HOST="${1:-localhost:50051}"
PUBLIC_KEY="test-$(date +%s)"
TOKEN="${2:-}"

echo "=================================="
echo "gRPC Controller 测试"
echo "=================================="
echo "目标: $CONTROLLER_HOST"
echo "测试公钥: $PUBLIC_KEY"
echo ""

# 检查 grpcurl 是否安装
if ! command -v grpcurl &> /dev/null; then
    echo "❌ grpcurl 未安装"
    echo "安装方法："
    echo "  macOS: brew install grpcurl"
    echo "  Linux: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

echo "1️⃣  列出所有服务..."
echo "---"
grpcurl -plaintext $CONTROLLER_HOST list
echo ""

echo "2️⃣  查看 ControllerService 方法..."
echo "---"
grpcurl -plaintext $CONTROLLER_HOST list aria.agent.ControllerService
echo ""

echo "3️⃣  测试 Register API..."
echo "---"
if [ -n "$TOKEN" ]; then
    grpcurl -plaintext -d "{
  \"public_key\": \"$PUBLIC_KEY\",
  \"endpoint\": \":51820\",
  \"public_ip\": \"1.2.3.4\",
  \"hostname\": \"test-node-grpc\",
  \"registered_at\": $(date +%s),
  \"token\": \"$TOKEN\",
  \"region\": \"test\",
  \"runtime_mode\": \"kernel\",
  \"kernel_version\": \"5.15.0\",
  \"has_aesni\": true
}" $CONTROLLER_HOST aria.agent.ControllerService/Register
else
    echo "⚠️  跳过（需要 TOKEN 参数）"
    echo "用法: $0 <host:port> <token>"
fi
echo ""

echo "4️⃣  测试 Sync API..."
echo "---"
grpcurl -plaintext -d "{
  \"public_key\": \"$PUBLIC_KEY\"
}" $CONTROLLER_HOST aria.agent.ControllerService/Sync
echo ""

echo "✅ 测试完成！"
