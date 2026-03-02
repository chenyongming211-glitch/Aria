#!/bin/bash
# Aria gRPC mTLS 证书生成工具
# 生成 CA、Controller Server 和 Agent Client 证书

set -e

CERT_DIR="${1:-./certs}"
DAYS="${2:-365}"

# 转换为绝对路径
CERT_DIR="$(cd "$(dirname "$CERT_DIR")" 2>/dev/null && pwd)/$(basename "$CERT_DIR")"

echo "=================================="
echo "Aria gRPC mTLS 证书生成"
echo "=================================="
echo "证书目录: $CERT_DIR"
echo "有效期: $DAYS 天"
echo ""

# 创建目录
mkdir -p "$CERT_DIR/ca"
mkdir -p "$CERT_DIR/controller"
mkdir -p "$CERT_DIR/agents"

# 确保目录创建成功
if [ ! -d "$CERT_DIR" ]; then
    echo "❌ 无法创建证书目录: $CERT_DIR"
    exit 1
fi

echo "1️⃣  生成 CA 证书（证书颁发机构）..."
echo "---"
cd "$CERT_DIR/ca"

# 生成 CA 私钥
openssl genrsa -out ca.key 4096

# 生成 CA 证书
cat > ca.cnf << EOF
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_ca
prompt = no

[req_distinguished_name]
CN = Aria gRPC CA
O = Aria Network
C = CN

[v3_ca]
basicConstraints = critical,CA:true
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
EOF

openssl req -new -x509 -days $DAYS -key ca.key -out ca.crt -config ca.cnf

echo "✅ CA 证书已生成"
echo "   - ca.key (CA 私钥，请妥善保管)"
echo "   - ca.crt (CA 证书，分发给所有节点)"

echo ""
echo "2️⃣  生成 Controller Server 证书..."
echo "---"
cd "$CERT_DIR/controller"

# 生成 Controller 私钥
openssl genrsa -out server.key 4096

# 生成 CSR
cat > server.cnf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = aria-controller
O = Aria Network
C = CN

[v3_req]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = aria-controller
DNS.3 = aria_controller
DNS.4 = *.aria-controller.local
IP.1 = 127.0.0.1
IP.2 = 172.16.0.211
IP.3 = 112.124.8.241
EOF

openssl req -new -key server.key -out server.csr -config server.cnf

# 用 CA 签发证书
cat > server_ext.cnf << EOF
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = aria-controller
DNS.3 = aria_controller
DNS.4 = *.aria-controller.local
IP.1 = 127.0.0.1
IP.2 = 172.16.0.211
IP.3 = 112.124.8.241
EOF

openssl x509 -req -days $DAYS -in server.csr \
    -CA ../ca/ca.crt -CAkey ../ca/ca.key -CAcreateserial \
    -out server.crt -extfile server_ext.cnf

# 验证
openssl verify -CAfile ../ca/ca.crt server.crt

echo "✅ Controller Server 证书已生成"
echo "   - server.key (Server 私钥)"
echo "   - server.crt (Server 证书)"

echo ""
echo "3️⃣  生成 Agent Client 证书模板..."
echo "---"
cd "$CERT_DIR/agents"

# 生成示例 Agent 证书（agent-sh）
AGENT_NAME="agent-sh"

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

echo "✅ Agent Client 证书已生成 (${AGENT_NAME})"
echo "   - ${AGENT_NAME}.key (Client 私钥)"
echo "   - ${AGENT_NAME}.crt (Client 证书)"

echo ""
echo "4️⃣  生成证书指纹（用于验证）..."
echo "---"
cd "$CERT_DIR"
echo "CA 证书指纹:"
openssl x509 -in ca/ca.crt -noout -fingerprint -sha256

echo ""
echo "Controller Server 证书指纹:"
openssl x509 -in controller/server.crt -noout -fingerprint -sha256

echo ""
echo "=================================="
echo "证书生成完成！"
echo "=================================="
echo ""
echo "目录结构："
echo "$CERT_DIR/"
echo "├── ca/"
echo "│   ├── ca.key  ← CA 私钥（保密）"
echo "│   └── ca.crt  ← CA 证书（分发给所有节点）"
echo "├── controller/"
echo "│   ├── server.key  ← Controller 私钥"
echo "│   └── server.crt  ← Controller 证书"
echo "└── agents/"
echo "    ├── agent-sh.key  ← Agent 私钥"
echo "    └── agent-sh.crt  ← Agent 证书"
echo ""
echo "下一步："
echo "1. 部署 CA 证书到所有节点"
echo "2. 部署 Server 证书到 Controller"
echo "3. 部署 Client 证书到各 Agent"
echo "4. 为其他 Agent 生成证书："
echo "   cd $CERT_DIR/agents"
echo "   ../generate-agent-cert.sh agent-bj"
echo ""
