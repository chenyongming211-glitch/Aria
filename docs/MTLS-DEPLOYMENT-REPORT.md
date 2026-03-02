# Aria gRPC mTLS 部署报告

## 🎉 部署状态

**部署时间**: 2026-03-02
**版本**: 0.2.26-mtls
**状态**: ✅ 成功

---

## 📊 部署详情

### 1. Controller 服务

```
容器 ID: d9ddf760d2ff
镜像版本: aria-controller:0.2.26-mtls
网络: aria_shared_net
端口映射: 0.0.0.0:50051->50051/tcp
```

**启动命令:**
```bash
docker run -d \
    --name aria_controller \
    --network aria_shared_net \
    -p 50051:50051 \
    -v /root/aria-controller/config/controller.yaml:/etc/aria/controller.yaml:ro \
    -v /etc/aria/certs:/etc/aria/certs:ro \
    -v /var/log/aria:/var/log/aria \
    -e ARIA_GRPC_MTLS_ENABLED=true \
    -e ARIA_GRPC_SERVER_CERT=/etc/aria/certs/controller/server.crt \
    -e ARIA_GRPC_SERVER_KEY=/etc/aria/certs/controller/server.key \
    -e ARIA_GRPC_CA_CERT=/etc/aria/certs/ca/ca.crt \
    aria-controller:latest
```

**服务状态:**
```
✅ HTTP API: 监听 :8080 (容器内)
✅ gRPC API: 监听 :50051 (外部可访问)
✅ mTLS: 已启用
✅ 证书: 已挂载
```

---

### 2. mTLS 配置

**证书体系:**
```
/etc/aria/certs/
├── ca/
│   ├── ca.crt    ← CA 证书（分发所有节点）
│   └── ca.key    ← CA 私钥（保密）
├── controller/
│   ├── server.crt ← Controller 证书
│   └── server.key ← Controller 私钥
└── agents/
    ├── agent-sh.crt ← 上海节点证书
    ├── agent-sh.key ← 上海节点私钥
    ├── agent-bj.crt ← 北京节点证书
    └── agent-bj.key ← 北京节点私钥
```

**证书指纹:**
```
CA:          7D:6D:A0:D3:4D:EB:83:5A:...:D5:64:DA
Server:      ED:2A:C1:E2:EE:70:B5:0F:...:22:4C:49:F9
Agent-sh:    （查看 certs/agents/agent-sh.crt）
Agent-bj:    03:81:95:E5:D3:E8:28:36:...:CD:D0:BB
```

**环境变量:**
```bash
ARIA_GRPC_MTLS_ENABLED=true
ARIA_GRPC_SERVER_CERT=/etc/aria/certs/controller/server.crt
ARIA_GRPC_SERVER_KEY=/etc/aria/certs/controller/server.key
ARIA_GRPC_CA_CERT=/etc/aria/certs/ca/ca.crt
```

---

## 🔒 安全特性

### 双向认证流程

```
1. Agent 连接 Controller:50051
2. Controller 发送 server.crt
3. Agent 验证:
   - 用 CA 证书验证 server.crt
   - 检查证书是否过期
   - 检查 CN/SAN 是否匹配
4. Agent 发送 client.crt
5. Controller 验证:
   - 用 CA 证书验证 client.crt
   - 检查证书是否过期
   - 检查 Agent 身份
6. 建立 TLS 1.2+ 加密通道
7. 应用层认证（public_key）
```

### 加密参数

- **协议**: TLS 1.2+
- **证书类型**: X.509
- **密钥长度**: RSA 4096 bit
- **有效期**: 365 天
- **签名算法**: SHA-256

---

## 🧪 测试方法

### 1. 从本地测试（需要 grpcurl）

```bash
# 安装 grpcurl
brew install grpcurl  # macOS
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest  # Linux

# 测试连接（不提供证书，应该失败）
grpcurl 112.124.8.241:50051 list
# 预期: 连接失败（需要客户端证书）

# 测试连接（提供证书）
grpcurl -cacert certs/ca/ca.crt \
    -cert certs/agents/agent-sh.crt \
    -key certs/agents/agent-sh.key \
    112.124.8.241:50051 list
# 预期: 列出 gRPC 服务
```

### 2. 从服务器测试（使用 openssl）

```bash
ssh root@112.124.8.241

# 测试 TLS 握手
openssl s_client -connect localhost:50051 \
    -CAfile /etc/aria/certs/ca/ca.crt \
    -cert /etc/aria/certs/agents/agent-sh.crt \
    -key /etc/aria/certs/agents/agent-sh.key

# 查看证书信息
echo | openssl s_client -connect localhost:50051 \
    -CAfile /etc/aria/certs/ca/ca.crt \
    -cert /etc/aria/certs/agents/agent-sh.crt \
    -key /etc/aria/certs/agents/agent-sh.key 2>&1 | \
    grep "Verify return code"
# 预期: Verify return code: 0 (ok)
```

### 3. 查看 Controller 日志

```bash
# 查看 mTLS 相关日志
docker logs aria_controller 2>&1 | grep -E '(mTLS|gRPC)'

# 实时查看日志
docker logs -f aria_controller
```

---

## 📝 客户端连接示例

### Go 客户端（Rust Agent 参考实现）

```go
package main

import (
    "context"
    "crypto/tls"
    "crypto/x509"
    "io/ioutil"
    "log"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
    
    pb "aria/pkg/grpc/agentpb"
)

func main() {
    // 加载客户端证书
    cert, err := tls.LoadX509KeyPair(
        "/etc/aria/certs/agents/agent-sh.crt",
        "/etc/aria/certs/agents/agent-sh.key",
    )
    if err != nil {
        log.Fatalf("Failed to load client cert: %v", err)
    }
    
    // 加载 CA 证书
    caCert, err := ioutil.ReadFile("/etc/aria/certs/ca/ca.crt")
    if err != nil {
        log.Fatalf("Failed to read CA cert: %v", err)
    }
    
    caCertPool := x509.NewCertPool()
    caCertPool.AppendCertsFromPEM(caCert)
    
    // 创建 TLS 配置
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        RootCAs:      caCertPool,
        MinVersion:   tls.VersionTLS12,
    }
    
    // 创建 gRPC 连接
    creds := credentials.NewTLS(tlsConfig)
    conn, err := grpc.Dial("112.124.8.241:50051", grpc.WithTransportCredentials(creds))
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    // 创建客户端
    client := pb.NewControllerServiceClient(conn)
    
    // 调用 Sync API
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    resp, err := client.Sync(ctx, &pb.SyncRequest{
        PublicKey: "your-public-key-here",
    })
    if err != nil {
        log.Fatalf("Sync failed: %v", err)
    }
    
    log.Printf("Synced %d peers", len(resp.Peers))
    log.Printf("Assigned IP: %s", resp.AssignedIp)
}
```

### Rust 客户端（Phase 1 实现）

```rust
use tonic::transport::{Certificate, Channel, ClientTlsConfig, Identity};
use std::fs;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 加载证书
    let ca_cert = fs::read_to_string("/etc/aria/certs/ca/ca.crt")?;
    let client_cert = fs::read_to_string("/etc/aria/certs/agents/agent-sh.crt")?;
    let client_key = fs::read_to_string("/etc/aria/certs/agents/agent-sh.key")?;
    
    // 创建 TLS 配置
    let ca = Certificate::from_pem(ca_cert);
    let identity = Identity::from_pem(client_cert, client_key);
    
    let tls_config = ClientTlsConfig::new()
        .ca_certificate(ca)
        .identity(identity)
        .domain_name("aria-controller");
    
    // 连接服务器
    let channel = Channel::from_static("112.124.8.241:50051")
        .tls_config(tls_config)?
        .connect()
        .await?;
    
    // 创建客户端
    let mut client = ControllerServiceClient::new(channel);
    
    // 调用 Sync API
    let request = tonic::Request::new(SyncRequest {
        public_key: "your-public-key-here".to_string(),
    });
    
    let response = client.sync(request).await?;
    let sync_resp = response.into_inner();
    
    println!("Synced {} peers", sync_resp.peers.len());
    println!("Assigned IP: {}", sync_resp.assigned_ip);
    
    Ok(())
}
```

---

## 🔄 证书轮换

### 重新生成所有证书

```bash
cd /Users/chen/Aria
./scripts/certs/generate-certs.sh ./certs 90  # 90 天有效期

# 上传到服务器
rsync -avz certs/ root@112.124.8.241:/etc/aria/certs/

# 重启 Controller
ssh root@112.124.8.241 "docker restart aria_controller"
```

### 更新单个 Agent 证书

```bash
./scripts/certs/generate-agent-cert.sh agent-sh 90

# 上传并分发
rsync -avz certs/agents/agent-sh.* root@146.56.196.231:/etc/aria/certs/
```

---

## 🚨 故障排查

### 问题 1: 连接被拒绝

```bash
# 检查端口监听
netstat -tlnp | grep 50051

# 检查防火墙
ufw status
iptables -L -n | grep 50051

# 查看日志
docker logs aria_controller --tail 50
```

### 问题 2: 证书验证失败

```bash
# 验证证书链
openssl verify -CAfile /etc/aria/certs/ca/ca.crt \
    /etc/aria/certs/controller/server.crt

# 查看证书详情
openssl x509 -in /etc/aria/certs/controller/server.crt -noout -text

# 检查证书过期时间
openssl x509 -in /etc/aria/certs/ca/ca.crt -noout -dates
```

### 问题 3: 客户端连接超时

```bash
# 测试网络连通性
ping 112.124.8.241
nc -zv 112.124.8.241 50051

# 检查证书路径
ls -la /etc/aria/certs/agents/agent-sh.*

# 查看 Agent 日志
journalctl -u aria -f
```

---

## 📈 监控指标

### Controller 状态

```bash
# 容器状态
docker ps | grep aria_controller

# 资源使用
docker stats aria_controller --no-stream

# gRPC 连接数（未来实现）
# docker exec aria_controller netstat -an | grep :50051 | grep ESTABLISHED | wc -l
```

### 证书过期检查

```bash
# 检查 CA 证书过期时间
openssl x509 -in /etc/aria/certs/ca/ca.crt -noout -dates

# 检查 Server 证书过期时间
openssl x509 -in /etc/aria/certs/controller/server.crt -noout -dates

# 自动化检查（推荐添加到监控）
for cert in /etc/aria/certs/*/*.crt; do
    echo "$cert:"
    openssl x509 -in "$cert" -noout -subject -dates
    echo ""
done
```

---

## ✅ 部署检查清单

- [x] Controller 镜像构建（0.2.26-mtls）
- [x] 证书生成（CA + Server + 2 Agents）
- [x] 证书上传到服务器（/etc/aria/certs/）
- [x] Controller 容器启动
- [x] mTLS 配置启用
- [x] gRPC 端口映射（50051）
- [x] 环境变量配置
- [x] 服务日志确认
- [ ] Agent 证书分发（待 Phase 1）
- [ ] Rust Agent gRPC 客户端（待 Phase 1）

---

## 📞 支持信息

**服务器地址**: 112.124.8.241
**gRPC 端口**: 50051
**CA 证书**: certs/ca/ca.crt
**技术文档**: docs/GRPC-TESTING.md

---

**部署完成时间**: 2026-03-02 12:22
**下次证书轮换**: 2027-03-02（365 天后）
