#!/bin/bash
# 为新 Agent 生成客户端证书
# 用法：./generate-agent-cert.sh <agent-name>

set -e

if [ -z "$1" ]; then
    echo "用法: $0 <agent-name>"
    echo "示例: $0 agent-bj"
    exit 1
fi

AGENT_NAME="$1"
DAYS="${2:-365}"

# 证书目录（项目根目录下的 certs）
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
CERT_DIR="$PROJECT_ROOT/certs"

if [ ! -d "$CERT_DIR/agents" ]; then
    echo "❌ 证书目录不存在: $CERT_DIR/agents"
    echo "项目根目录: $PROJECT_ROOT"
    echo "请先运行: ./scripts/certs/generate-certs.sh"
    exit 1
fi

echo "生成 Agent 证书: $AGENT_NAME"
echo "证书目录: $CERT_DIR"
echo "有效期: $DAYS 天"
echo ""

cd "$CERT_DIR/agents"

# 生成私钥
openssl genrsa -out ${AGENT_NAME}.key 4096

# 生成 CSR
cat > ${AGENT_NAME}.cnf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = ${AGENT_NAME}
O = Aria Network
C = CN

[v3_req]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = clientAuth
EOF

openssl req -new -key ${AGENT_NAME}.key -out ${AGENT_NAME}.csr -config ${AGENT_NAME}.cnf

# 用 CA 签发
openssl x509 -req -days $DAYS -in ${AGENT_NAME}.csr \
    -CA ../ca/ca.crt -CAkey ../ca/ca.key -CAcreateserial \
    -out ${AGENT_NAME}.crt -extfile ${AGENT_NAME}.cnf

# 验证
openssl verify -CAfile ../ca/ca.crt ${AGENT_NAME}.crt

echo ""
echo "✅ 证书已生成："
echo "   - agents/${AGENT_NAME}.key"
echo "   - agents/${AGENT_NAME}.crt"
echo ""
echo "指纹:"
openssl x509 -in ${AGENT_NAME}.crt -noout -fingerprint -sha256
