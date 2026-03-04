# QoS 规则同步功能实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 实现 Controller 到 Agent 的 QoS 规则自动同步，确保前端创建的带宽限制规则在 5 秒内生效。

**Architecture:** 使用现有 gRPC Sync() 轮询机制（每 5 秒），Controller 查询数据库中的 QoS 规则并通过 SyncResponse 下发给 Agent，Agent 解析规则并调用 QoS Manager 应用到 eBPF 程序。

**Tech Stack:** Go 1.25 (Controller), Rust (Agent), gRPC + Protobuf, PostgreSQL, eBPF + TC

---

## 前置条件检查

**在开始之前，确认以下条件：**
- ✅ Controller 已部署在 112.124.8.241
- ✅ Agent 已部署在 146.56.196.231 (sh) 和 118.195.135.16 (bj)
- ✅ gRPC 通信正常（端口 50051）
- ✅ 数据库 `bandwidth_limits` 表已创建
- ✅ Agent QoS Manager 已实现（limit_service/peer/ip）

---

## Task 1: Controller 端 - 修改结构体和构造函数

**Files:**
- Modify: `internal/controller/grpc/server.go:15-30`

**Step 1: 修改 ControllerServer 结构体**

在 `internal/controller/grpc/server.go` 的 `ControllerServer` 结构体中添加 `store` 字段：

```go
type ControllerServer struct {
    agentpb.UnimplementedControllerServiceServer
    registerHandler func(interface{}) (assignedIP, metricsGateway string, err error)
    syncHandler     func(publicKey string) (peers interface{}, assignedIP string, aclRules interface{}, metricsGateway string, err error)
    store           *controllerstorage.Storage  // 新增：用于查询 QoS 规则
}
```

**Step 2: 修改 NewControllerServer 构造函数**

修改 `NewControllerServer()` 函数签名和实现：

```go
func NewControllerServer(
    registerHandler func(interface{}) (string, string, error),
    syncHandler func(string) (interface{}, string, interface{}, string, error),
    store *controllerstorage.Storage,  // 新增参数
) *ControllerServer {
    return &ControllerServer{
        registerHandler: registerHandler,
        syncHandler:     syncHandler,
        store:           store,  // 新增
    }
}
```

**Step 3: 验证代码语法**

Run: `cd /Users/chen/Aria && go build ./internal/controller/grpc`
Expected: 编译成功，无错误

**Step 4: Commit**

```bash
git add internal/controller/grpc/server.go
git commit -m "feat(controller): add store field to ControllerServer for QoS queries"
```

---

## Task 2: Controller 端 - 实现 getQoSRules() 方法

**Files:**
- Modify: `internal/controller/grpc/server.go:200+`（文件末尾）

**Step 1: 添加 getQoSRules() 方法**

在 `internal/controller/grpc/server.go` 文件末尾添加新方法：

```go
// getQoSRules 查询适用于该 Agent 的 QoS 规则
func (s *ControllerServer) getQoSRules(ctx context.Context, publicKey string) ([]*agentpb.QoSRule, error) {
    var qosRules []*agentpb.QoSRule
    
    // 防御性编程：确保 store 存在
    if s.store == nil {
        return qosRules, nil  // 返回空列表
    }
    
    // 查询 tenant_id
    var tenantID string
    err := s.store.DB().QueryRowContext(ctx,
        "SELECT tenant_id FROM nodes WHERE public_key = $1",
        publicKey,
    ).Scan(&tenantID)
    
    if err != nil {
        if err == sql.ErrNoRows {
            // 节点不存在，返回空列表
            return qosRules, nil
        }
        // 记录错误但继续
        fmt.Printf("[WARN] Failed to query tenant_id for %s: %v\n", publicKey, err)
        return qosRules, nil
    }
    
    // 查询 QoS 规则
    query := `
        SELECT src_ip, dst_ip, src_port, dst_port, protocol, bandwidth_mbps
        FROM bandwidth_limits
        WHERE tenant_id = $1
        ORDER BY created_at DESC
    `
    rows, err := s.store.DB().QueryContext(ctx, query, tenantID)
    if err != nil {
        fmt.Printf("[WARN] Failed to query QoS rules: %v\n", err)
        return qosRules, nil
    }
    defer rows.Close()
    
    for rows.Next() {
        var rule agentpb.QoSRule
        err := rows.Scan(
            &rule.SrcIp,
            &rule.DstIp,
            &rule.SrcPort,
            &rule.DstPort,
            &rule.Protocol,
            &rule.BandwidthMbps,
        )
        if err != nil {
            fmt.Printf("[WARN] Failed to scan QoS rule: %v\n", err)
            continue
        }
        qosRules = append(qosRules, &rule)
    }
    
    fmt.Printf("[INFO] Retrieved %d QoS rules for tenant %s\n", len(qosRules), tenantID)
    return qosRules, nil
}
```

**Step 2: 添加必要的 import**

在文件顶部的 import 区域添加 `database/sql`：

```go
import (
    "context"
    "database/sql"  // 新增
    "encoding/json"
    "fmt"
    "io"
    "time"

    "aria/pkg/grpc/agentpb"
)
```

**Step 3: 验证代码语法**

Run: `cd /Users/chen/Aria && go build ./internal/controller/grpc`
Expected: 编译成功，无错误

**Step 4: Commit**

```bash
git add internal/controller/grpc/server.go
git commit -m "feat(controller): implement getQoSRules() method"
```

---

## Task 3: Controller 端 - 修改 Sync() 方法

**Files:**
- Modify: `internal/controller/grpc/server.go:122-128`

**Step 1: 修改 Sync() 方法的返回部分**

在 `Sync()` 方法的返回语句之前添加 QoS 规则查询：

```go
// 查询 QoS 规则
qosRules, err := s.getQoSRules(ctx, req.PublicKey)
if err != nil {
    // 记录错误但继续
    fmt.Printf("[WARN] Failed to get QoS rules: %v\n", err)
    qosRules = []*agentpb.QoSRule{}  // 空列表
}

return &agentpb.SyncResponse{
    Peers:              peers,
    AssignedIp:         assignedIP,
    LastUpdate:         time.Now().Unix(),
    AclRules:           aclRules,
    MetricsPushGateway: metricsGateway,
    QosRules:           qosRules,  // 新增
}, nil
```

**Step 2: 验证代码语法**

Run: `cd /Users/chen/Aria && go build ./internal/controller/grpc`
Expected: 编译成功，无错误

**Step 3: Commit**

```bash
git add internal/controller/grpc/server.go
git commit -m "feat(controller): add QoS rules to SyncResponse"
```

---

## Task 4: Controller 端 - 更新 NewControllerServer 调用位置

**Files:**
- Modify: 查找 `NewControllerServer()` 的调用位置（可能在 `cmd/controller/main.go` 或 gRPC 初始化代码）

**Step 1: 查找调用位置**

Run: `cd /Users/chen/Aria && grep -rn "NewControllerServer" --include="*.go" | grep -v "server.go"`
Expected: 找到调用位置（假设在 `internal/controller/main.go` 或类似位置）

**Step 2: 添加 store 参数**

根据查找到的位置，修改调用代码：

```go
// 示例（具体路径可能不同）
controllerServer := grpc_server.NewControllerServer(
    registerHandler,
    syncHandler,
    store,  // 新增：传入 store 实例
)
```

**Step 3: 验证代码语法**

Run: `cd /Users/chen/Aria && go build ./cmd/controller`
Expected: 编译成功，无错误

**Step 4: Commit**

```bash
git add <修改的文件路径>
git commit -m "feat(controller): pass store to NewControllerServer"
```

---

## Task 5: Agent 端 - 新增 QoSRule 结构体

**Files:**
- Modify: `agent-rust/agent/src/grpc_client.rs:164+`

**Step 1: 添加 QoSRule 结构体**

在 `agent-rust/agent/src/grpc_client.rs` 文件末尾（在 `AclRule` 定义之后）添加：

```rust
/// QoS 规则
#[derive(Debug, Clone)]
pub struct QoSRule {
    pub src_ip: String,
    pub dst_ip: String,
    pub src_port: u32,
    pub dst_port: u32,
    pub protocol: u32,
    pub bandwidth_mbps: u64,
}
```

**Step 2: 验证代码语法**

Run: `cd /Users/chen/Aria/agent-rust && cargo check`
Expected: 编译成功，无错误

**Step 3: Commit**

```bash
git add agent-rust/agent/src/grpc_client.rs
git commit -m "feat(agent): add QoSRule struct"
```

---

## Task 6: Agent 端 - 修改 SyncResult 结构体

**Files:**
- Modify: `agent-rust/agent/src/grpc_client.rs:143-147`

**Step 1: 修改 SyncResult 结构体**

添加 `qos_rules` 字段：

```rust
/// 同步结果
pub struct SyncResult {
    pub peers: Vec<PeerInfo>,
    pub assigned_ip: String,
    pub acl_rules: Vec<AclRule>,
    pub qos_rules: Vec<QoSRule>,  // 新增
}
```

**Step 2: 验证代码语法**

Run: `cd /Users/chen/Aria/agent-rust && cargo check`
Expected: 编译成功，无错误

**Step 3: Commit**

```bash
git add agent-rust/agent/src/grpc_client.rs
git commit -m "feat(agent): add qos_rules to SyncResult"
```

---

## Task 7: Agent 端 - 修改 sync() 方法解析逻辑

**Files:**
- Modify: `agent-rust/agent/src/grpc_client.rs:116-138`

**Step 1: 修改 sync() 方法的返回部分**

在 `sync()` 方法中添加 `qos_rules` 的解析：

```rust
Ok(SyncResult {
    peers: resp.peers.into_iter().map(|p| PeerInfo {
        public_key: p.public_key,
        endpoint: p.endpoint,
        private_ip: p.private_ip,
        public_ip: p.public_ip,
        region: p.region,
        vpc_id: p.vpc_id,
        hostname: p.hostname,
        assigned_ip: p.assigned_ip,
        role: p.role,
        advertised_routes: p.advertised_routes,
    }).collect(),
    assigned_ip: resp.assigned_ip,
    acl_rules: resp.acl_rules.into_iter().map(|r| AclRule {
        src_net: r.src_net,
        dst_net: r.dst_net,
        protocol: r.protocol,
        min_port: r.min_port,
        max_port: r.max_port,
    }).collect(),
    qos_rules: resp.qos_rules.into_iter().map(|r| QoSRule {  // 新增
        src_ip: r.src_ip,
        dst_ip: r.dst_ip,
        src_port: r.src_port,
        dst_port: r.dst_port,
        protocol: r.protocol,
        bandwidth_mbps: r.bandwidth_mbps,
    }).collect(),
})
```

**Step 2: 验证代码语法**

Run: `cd /Users/chen/Aria/agent-rust && cargo check`
Expected: 编译成功，无错误

**Step 3: Commit**

```bash
git add agent-rust/agent/src/grpc_client.rs
git commit -m "feat(agent): parse qos_rules in sync()"
```

---

## Task 8: Agent 端 - 新增 sync_qos_rules() 方法

**Files:**
- Modify: `agent-rust/agent/src/unified_agent.rs:1037+`（在 `sync()` 方法之后）

**Step 1: 添加 sync_qos_rules() 方法**

在 `unified_agent.rs` 的 `sync()` 方法之后添加新方法：

```rust
/// 同步 QoS 规则
async fn sync_qos_rules(&mut self, new_rules: &[GrpcQoSRule]) -> Result<()> {
    use crate::qos::QoSManager;
    
    tracing::info!("Syncing {} QoS rules", new_rules.len());
    
    // 在阻塞任务中执行 QoS 操作（因为 QoS Manager 不是异步的）
    let new_rules = new_rules.to_vec();
    let result = tokio::task::spawn_blocking(move || -> Result<()> {
        let mut qos_mgr = match QoSManager::new("eth0") {
            Ok(mgr) => mgr,
            Err(e) => {
                tracing::error!("Failed to create QoS manager: {:?}", e);
                return Err(e);
            }
        };
        
        let mut success_count = 0;
        let mut fail_count = 0;
        
        for rule in &new_rules {
            // 根据规则特征判断类型并应用
            let result = if rule.src_ip.is_empty() && rule.dst_ip.is_empty() {
                // 端口级规则（已弃用，忽略）
                tracing::warn!("Port-level rules are deprecated, skipping");
                continue;
            } else if rule.src_ip.is_empty() || rule.dst_ip.is_empty() {
                // IP 级规则
                let ip = if !rule.src_ip.is_empty() {
                    &rule.src_ip
                } else {
                    &rule.dst_ip
                };
                qos_mgr.limit_ip(ip, rule.bandwidth_mbps)
            } else if rule.src_port == 0 && rule.dst_port == 0 {
                // Peer 级规则（只有 IP 对）
                qos_mgr.limit_peer_pair(&rule.src_ip, &rule.dst_ip, rule.bandwidth_mbps)
            } else {
                // 服务级规则（五元组）
                qos_mgr.limit_service(
                    &rule.src_ip,
                    &rule.dst_ip,
                    rule.src_port,
                    rule.dst_port,
                    rule.protocol,
                    rule.bandwidth_mbps,
                )
            };
            
            match result {
                Ok(_) => {
                    success_count += 1;
                    tracing::debug!(
                        "Applied QoS rule: {}:{}:{}:{}:{} -> {} Mbps",
                        rule.src_ip, rule.dst_ip, rule.src_port,
                        rule.dst_port, rule.protocol, rule.bandwidth_mbps
                    );
                }
                Err(e) => {
                    fail_count += 1;
                    tracing::error!(
                        "Failed to apply QoS rule: {}:{}:{}:{}:{} -> {:?}",
                        rule.src_ip, rule.dst_ip, rule.src_port,
                        rule.dst_port, rule.protocol, e
                    );
                }
            }
        }
        
        tracing::info!(
            "QoS sync completed: {} success, {} failed",
            success_count,
            fail_count
        );
        
        Ok(())
    }).await?;
    
    result
}
```

**Step 2: 添加必要的 use 声明**

在文件顶部添加（如果还没有）：

```rust
use crate::grpc_client::QoSRule as GrpcQoSRule;
```

**Step 3: 验证代码语法**

Run: `cd /Users/chen/Aria/agent-rust && cargo check`
Expected: 编译成功，无错误

**Step 4: Commit**

```bash
git add agent-rust/agent/src/unified_agent.rs
git commit -m "feat(agent): implement sync_qos_rules() method"
```

---

## Task 9: Agent 端 - 修改 sync() 方法调用 QoS 同步

**Files:**
- Modify: `agent-rust/agent/src/unified_agent.rs:1015-1037`

**Step 1: 修改 sync() 方法**

在 ACL 同步之后添加 QoS 同步调用：

```rust
pub async fn sync(&mut self) -> Result<()> {
    tracing::debug!("Syncing with Controller...");
    
    let sync_result = self.grpc_client
        .sync(self.config.public_key.clone())
        .await?;
    
    tracing::debug!("Sync received: {} peers, {} ACL rules, {} QoS rules", 
        sync_result.peers.len(), 
        sync_result.acl_rules.len(),
        sync_result.qos_rules.len());  // 修改日志
    
    self.sync_peers(&sync_result.peers).await?;
    self.sync_advertised_routes(&sync_result.peers).await?;
    
    if !sync_result.acl_rules.is_empty() {
        if let Err(e) = self.sync_acl_rules(&sync_result.acl_rules).await {
            tracing::error!("Failed to sync ACL rules: {:?}", e);
        }
    }
    
    // 新增：同步 QoS 规则
    if let Err(e) = self.sync_qos_rules(&sync_result.qos_rules).await {
        tracing::error!("Failed to sync QoS rules: {:?}", e);
    }
    
    *self.last_sync_peers.lock().unwrap() = sync_result.peers;
    tracing::debug!("Sync completed");
    Ok(())
}
```

**Step 2: 验证代码语法**

Run: `cd /Users/chen/Aria/agent-rust && cargo check`
Expected: 编译成功，无错误

**Step 3: Commit**

```bash
git add agent-rust/agent/src/unified_agent.rs
git commit -m "feat(agent): call sync_qos_rules() in sync() method"
```

---

## Task 10: Controller 端 - 本地测试

**Files:**
- 无文件修改

**Step 1: 编译 Controller**

Run: `cd /Users/chen/Aria && make build`
Expected: 编译成功，生成 `bin/controller`

**Step 2: 检查数据库连接**

Run: 
```bash
ssh root@112.124.8.241 "docker exec aria_postgres psql -U aria -d aria -c 'SELECT COUNT(*) FROM bandwidth_limits;'"
```
Expected: 返回数据库中的规则数量

**Step 3: 提交最终版本**

```bash
git add -A
git commit -m "feat: complete QoS sync implementation"
```

---

## Task 11: Controller 端 - 构建和部署

**Files:**
- 无文件修改

**Step 1: 构建 Docker 镜像**

Run:
```bash
cd /Users/chen/Aria
docker buildx build --platform linux/amd64 --no-cache \
  -t aria-controller:latest \
  -f Dockerfile.controller . --load
```
Expected: 镜像构建成功

**Step 2: 保存镜像**

Run:
```bash
mkdir -p bin/images
docker save aria-controller:latest -o bin/images/aria-controller-latest.tar
```
Expected: 镜像保存成功

**Step 3: 上传到服务器**

Run:
```bash
rsync -avz bin/images/ root@112.124.8.241:/root/aria-controller/bin/images/
```
Expected: 上传成功

**Step 4: 部署到服务器**

Run:
```bash
ssh root@112.124.8.241 "cd /root/aria-controller && ./deploy-controller.sh deploy"
```
Expected: 部署成功，容器运行

**Step 5: 验证部署**

Run:
```bash
ssh root@112.124.8.241 "docker logs aria_controller 2>&1 | tail -20"
```
Expected: 无错误日志

**Step 6: Commit**

```bash
git add -A
git commit -m "deploy: Controller with QoS sync support"
```

---

## Task 12: Agent 端 - 构建和部署（sh 节点）

**Files:**
- 无文件修改

**Step 1: 同步源码到 sh 节点**

Run:
```bash
rsync -avz --delete agent-rust/agent/src/ root@146.56.196.231:/root/agent-rust/agent/src/
```
Expected: 同步成功

**Step 2: 在 sh 节点编译**

Run:
```bash
ssh root@146.56.196.231 "source ~/.cargo/env && cd /root/agent-rust && cargo clean && cargo build --release"
```
Expected: 编译成功（约 2-3 分钟）

**Step 3: 原子替换二进制**

Run:
```bash
ssh root@146.56.196.231 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"
```
Expected: 替换成功

**Step 4: 重启服务**

Run:
```bash
ssh root@146.56.196.231 "systemctl restart aria && sleep 3 && systemctl status aria"
```
Expected: 服务运行正常

**Step 5: 验证日志**

Run:
```bash
ssh root@146.56.196.231 "journalctl -u aria -n 50 --no-pager | grep -i qos"
```
Expected: 看到类似 "Syncing X QoS rules" 的日志

---

## Task 13: Agent 端 - 构建和部署（bj 节点）

**Files:**
- 无文件修改

**Step 1: 同步源码到 bj 节点**

Run:
```bash
rsync -avz --delete agent-rust/agent/src/ root@118.195.135.16:/root/agent-rust/agent/src/
```
Expected: 同步成功

**Step 2: 在 bj 节点编译**

Run:
```bash
ssh root@118.195.135.16 "source ~/.cargo/env && cd /root/agent-rust && cargo clean && cargo build --release"
```
Expected: 编译成功（约 2-3 分钟）

**Step 3: 原子替换二进制**

Run:
```bash
ssh root@118.195.135.16 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"
```
Expected: 替换成功

**Step 4: 重启服务**

Run:
```bash
ssh root@118.195.135.16 "systemctl restart aria && sleep 3 && systemctl status aria"
```
Expected: 服务运行正常

**Step 5: 验证日志**

Run:
```bash
ssh root@118.195.135.16 "journalctl -u aria -n 50 --no-pager | grep -i qos"
```
Expected: 看到类似 "Syncing X QoS rules" 的日志

---

## Task 14: 集成测试 - 创建测试规则

**Files:**
- 无文件修改

**Step 1: 创建 Peer 级 QoS 规则**

Run:
```bash
curl -X POST https://aria.yun/api/v1/bandwidth/limits \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "src_ip": "100.64.0.1",
    "dst_ip": "100.64.0.2",
    "bandwidth_mbps": 50
  }'
```
Expected: 返回成功，包含规则 ID

**Step 2: 查询数据库确认**

Run:
```bash
ssh root@112.124.8.241 "docker exec aria_postgres psql -U aria -d aria -c 'SELECT * FROM bandwidth_limits ORDER BY created_at DESC LIMIT 5;'"
```
Expected: 看到新创建的规则

**Step 3: 等待同步（6 秒）**

Run: `sleep 6`

**Step 4: 检查 Controller 日志**

Run:
```bash
ssh root@112.124.8.241 "docker logs aria_controller 2>&1 | grep -i qos | tail -10"
```
Expected: 看到 "[INFO] Retrieved X QoS rules for tenant xxx"

**Step 5: 检查 Agent 日志**

Run:
```bash
ssh root@146.56.196.231 "journalctl -u aria -n 100 --no-pager | grep -i qos"
```
Expected: 看到 "Syncing 1 QoS rules" 和 "QoS sync completed: 1 success"

---

## Task 15: 集成测试 - 验证 eBPF 程序

**Files:**
- 无文件修改

**Step 1: 检查 eBPF 程序列表**

Run:
```bash
ssh root@146.56.196.231 "bpftool prog list | grep -A 5 -i qos"
```
Expected: 看到 QoS 相关的 eBPF 程序

**Step 2: 检查 TC 过滤器**

Run:
```bash
ssh root@146.56.196.231 "tc filter show dev eth0 egress"
```
Expected: 看到 QoS 过滤器配置

**Step 3: 测试带宽限制（可选）**

在 sh 节点启动 iperf3 服务器：
```bash
ssh root@146.56.196.231 "iperf3 -s &"
```

在 bj 节点测试带宽：
```bash
ssh root@118.195.135.16 "iperf3 -c 100.64.0.1 -t 10"
```
Expected: 带宽应限制在 50 Mbps 左右

---

## Task 16: 创建服务级规则测试

**Files:**
- 无文件修改

**Step 1: 创建服务级 QoS 规则**

Run:
```bash
curl -X POST https://aria.yun/api/v1/bandwidth/limits \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "src_ip": "192.168.1.100",
    "dst_ip": "10.1.1.10",
    "dst_port": 443,
    "protocol": 6,
    "bandwidth_mbps": 100
  }'
```
Expected: 返回成功

**Step 2: 等待同步**

Run: `sleep 6`

**Step 3: 检查 Agent 日志**

Run:
```bash
ssh root@146.56.196.231 "journalctl -u aria -n 50 --no-pager | grep -i qos"
```
Expected: 看到新规则应用成功

---

## Task 17: 删除规则测试

**Files:**
- 无文件修改

**Step 1: 删除刚才创建的规则**

通过前端 UI 或 API 删除一条规则

**Step 2: 等待同步**

Run: `sleep 6`

**Step 3: 检查 Agent 行为**

Run:
```bash
ssh root@146.56.196.231 "journalctl -u aria -n 50 --no-pager | grep -i qos"
```
Expected: 看到规则被移除的日志（如果实现了删除逻辑）

---

## Task 18: 最终验证和清理

**Files:**
- 无文件修改

**Step 1: 检查所有节点状态**

Run:
```bash
ssh root@146.56.196.231 "aria status"
ssh root@118.195.135.16 "aria status"
```
Expected: 所有节点在线，同步正常

**Step 2: 检查 Controller 状态**

Run:
```bash
ssh root@112.124.8.241 "docker ps | grep aria_controller"
```
Expected: 容器运行正常

**Step 3: 清理测试规则（可选）**

通过前端 UI 或数据库删除测试规则

**Step 4: 更新 CHANGELOG**

在 `CHANGELOG.md` 添加本次更新：

```markdown
## [v0.2.28] - 2026-03-04

### ✨ 新增功能

#### QoS 规则自动同步
- Controller 通过 gRPC Sync() 自动下发 QoS 规则到 Agent
- Agent 每 5 秒同步一次，自动应用带宽限制
- 支持三层 QoS 架构：应用级、对等体级、全局级
- 前端创建规则后 5 秒内生效

### 🔧 技术改进
- Controller: 添加 `getQoSRules()` 方法查询数据库
- Agent: 新增 `sync_qos_rules()` 自动应用规则
- Agent: 支持 QoS 规则类型智能识别

### 📝 文件变更
- `internal/controller/grpc/server.go`
- `agent-rust/agent/src/grpc_client.rs`
- `agent-rust/agent/src/unified_agent.rs`
```

**Step 5: 最终提交**

```bash
git add CHANGELOG.md
git commit -m "chore: update CHANGELOG for QoS sync feature"
git push origin feature/qos-sync
```

---

## 完成标准

**功能完整性：**
- ✅ 前端创建规则后 5 秒内生效
- ✅ 支持 3 种 QoS 规则类型
- ✅ Controller 和 Agent 日志清晰
- ✅ eBPF 程序正确加载
- ✅ 带宽限制实际生效

**代码质量：**
- ✅ 所有代码编译通过
- ✅ 无明显性能问题
- ✅ 错误处理完善
- ✅ 日志记录详细

**部署质量：**
- ✅ 所有节点部署成功
- ✅ 服务运行稳定
- ✅ 无内存泄漏
- ✅ 无异常重启

---

## 预计时间

- Task 1-4 (Controller): 30 分钟
- Task 5-9 (Agent): 40 分钟
- Task 10-13 (部署): 30 分钟
- Task 14-18 (测试): 30 分钟

**总计：约 2.5 小时**

---

## 注意事项

1. **编译顺序：** 先完成 Controller 端所有修改，再修改 Agent 端
2. **测试依赖：** 需要先部署 Controller 才能测试 Agent
3. **错误容忍：** QoS 同步失败不应影响 Peer 和 ACL 同步
4. **日志监控：** 部署后密切关注 Controller 和 Agent 日志
5. **回滚准备：** 保留旧版本二进制文件以便快速回滚

---

**计划版本：** v1.0  
**创建日期：** 2026-03-04  
**预计完成：** 2026-03-04  
**下一步：** 开始执行 Task 1
