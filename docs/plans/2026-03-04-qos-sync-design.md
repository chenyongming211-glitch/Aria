# QoS 规则同步功能设计文档

**日期：** 2026-03-04  
**作者：** Aria Team  
**状态：** 已批准  
**优先级：** 高

---

## 1. 背景和目标

### 1.1 问题陈述

当前 Aria 系统中，用户可以通过前端 Web UI 创建和管理 QoS（带宽限制）规则，规则成功保存到 Controller 的数据库中。但是，这些规则**未能自动下发到 Agent 节点**，导致 eBPF 程序未加载，带宽限制功能无法生效。

### 1.2 现状分析

| 层级 | 状态 | 说明 |
|------|------|------|
| 前端 UI | ✅ 正常 | 可以创建和管理 QoS 规则 |
| Controller API | ✅ 正常 | 规则保存到数据库成功 |
| 数据库 | ✅ 正常 | `bandwidth_limits` 表存储规则 |
| Controller → Agent 同步 | ❌ **缺失** | `Sync()` 未包含 QoS 规则 |
| Agent 接收逻辑 | ❌ **缺失** | 未解析 `qos_rules` 字段 |
| Agent 应用逻辑 | ❌ **缺失** | 未调用 QoS Manager |
| eBPF 加载 | ✅ 已实现 | `limit_service/peer/ip` 方法已存在 |

### 1.3 目标

**核心目标：** 实现 Controller 到 Agent 的 QoS 规则自动同步，确保用户在前端创建的规则在 5 秒内生效。

**关键指标：**
- ✅ 规则同步延迟 ≤ 5 秒
- ✅ 支持 3 种 QoS 规则类型（应用级、对等体级、全局级）
- ✅ 规则变更自动检测和差异应用
- ✅ 错误处理不影响 Peer 和 ACL 同步

---

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                      前端 Web UI                             │
│  用户创建/删除 QoS 规则 → POST /api/v1/bandwidth/limits     │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                   Controller (Go)                            │
│  1. 保存规则到数据库 ✅ 已实现                                │
│  2. Sync() 时查询并返回 QoS 规则 ⚠️ 待实现                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ gRPC Sync() (每 5 秒)
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                    Agent (Rust)                              │
│  1. 接收 SyncResponse 中的 qos_rules ⚠️ 待实现               │
│  2. 对比本地状态（新增/删除/修改）⚠️ 待实现                    │
│  3. 调用 QoS Manager 应用规则 ⚠️ 待实现                       │
│  4. eBPF 程序加载和配置 ✅ 已实现                             │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 数据流

```
用户创建规则
    ↓
Controller API (bandwidth_management.go)
    ↓
PostgreSQL (bandwidth_limits 表)
    ↓
Agent 每 5 秒调用 Sync()
    ↓
Controller 查询该租户的所有 QoS 规则
    ↓
gRPC SyncResponse (qos_rules 字段)
    ↓
Agent 解析并对比本地状态
    ↓
调用 QoS Manager 的 limit_* 方法
    ↓
eBPF 程序加载 + TC 配置
    ↓
带宽限制生效
```

### 2.3 协议设计

**Proto 定义（已存在）：**
```protobuf
message QoSRule {
  string src_ip = 1;              // 源 IP
  string dst_ip = 2;              // 目标 IP
  uint32 src_port = 3;            // 源端口
  uint32 dst_port = 4;            // 目标端口
  uint32 protocol = 5;            // 协议（6=TCP, 17=UDP）
  uint64 bandwidth_mbps = 6;      // 带宽限制（Mbps）
}

message SyncResponse {
  repeated PeerInfo peers = 1;
  string assigned_ip = 2;
  int64 last_update = 3;
  repeated ACLRule acl_rules = 4;
  string metrics_push_gateway = 5;
  repeated QoSRule qos_rules = 6;  // 新增：QoS 规则列表
}
```

---

## 3. 实现细节

### 3.1 Controller 端实现

#### 3.1.1 修改 `ControllerServer` 结构体

**文件：** `internal/controller/grpc/server.go`

**变更：** 添加 `store` 字段用于数据库访问

```go
type ControllerServer struct {
    agentpb.UnimplementedControllerServiceServer
    registerHandler func(interface{}) (assignedIP, metricsGateway string, err error)
    syncHandler     func(publicKey string) (peers interface{}, assignedIP string, aclRules interface{}, metricsGateway string, err error)
    store           *controllerstorage.Storage  // 新增
}
```

#### 3.1.2 新增 `getQoSRules()` 方法

**功能：** 查询指定 Agent 所属租户的所有 QoS 规则

**实现逻辑：**
1. 根据 `public_key` 查询 `nodes` 表获取 `tenant_id`
2. 根据 `tenant_id` 查询 `bandwidth_limits` 表
3. 转换为 `[]*agentpb.QoSRule` 格式

**SQL 查询：**
```sql
-- Step 1: 查询 tenant_id
SELECT tenant_id FROM nodes WHERE public_key = $1;

-- Step 2: 查询 QoS 规则
SELECT src_ip, dst_ip, src_port, dst_port, protocol, bandwidth_mbps
FROM bandwidth_limits
WHERE tenant_id = $1
ORDER BY created_at DESC;
```

**错误处理：**
- 节点不存在：返回空列表（不影响同步）
- 查询失败：记录日志并返回空列表
- 扫描失败：跳过该规则，继续处理其他规则

#### 3.1.3 修改 `Sync()` 方法

**位置：** `internal/controller/grpc/server.go` 的 `Sync()` 方法

**变更：** 在返回前调用 `getQoSRules()` 并填充到响应中

```go
func (s *ControllerServer) Sync(ctx context.Context, req *agentpb.SyncRequest) (*agentpb.SyncResponse, error) {
    // ... 现有的 Peer 和 ACL 同步逻辑
    
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
}
```

### 3.2 Agent 端实现

#### 3.2.1 新增 `QoSRule` 结构体

**文件：** `agent-rust/agent/src/grpc_client.rs`

```rust
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

#### 3.2.2 修改 `SyncResult` 结构体

**文件：** `agent-rust/agent/src/grpc_client.rs`

```rust
pub struct SyncResult {
    pub peers: Vec<PeerInfo>,
    pub assigned_ip: String,
    pub acl_rules: Vec<AclRule>,
    pub qos_rules: Vec<QoSRule>,  // 新增
}
```

#### 3.2.3 修改 `sync()` 方法

**文件：** `agent-rust/agent/src/grpc_client.rs`

**变更：** 解析 `resp.qos_rules` 字段

```rust
Ok(SyncResult {
    peers: resp.peers.into_iter().map(|p| PeerInfo { /* ... */ }).collect(),
    assigned_ip: resp.assigned_ip,
    acl_rules: resp.acl_rules.into_iter().map(|r| AclRule { /* ... */ }).collect(),
    qos_rules: resp.qos_rules.into_iter().map(|r| QoSRule {
        src_ip: r.src_ip,
        dst_ip: r.dst_ip,
        src_port: r.src_port,
        dst_port: r.dst_port,
        protocol: r.protocol,
        bandwidth_mbps: r.bandwidth_mbps,
    }).collect(),
})
```

#### 3.2.4 新增 `sync_qos_rules()` 方法

**文件：** `agent-rust/agent/src/unified_agent.rs`

**功能：** 同步 QoS 规则到本地 eBPF 程序

**实现逻辑：**
1. 创建 `QoSManager` 实例
2. 构建新规则的唯一标识集合（`src_ip:dst_ip:src_port:dst_port:protocol`）
3. 对比当前已应用的规则（查询 eBPF maps）
4. 删除不再需要的规则
5. 添加/更新新规则

**规则类型判断：**
```rust
// 根据 QoS 规则字段判断类型
if src_ip.is_empty() && dst_ip.is_empty() {
    // 端口级规则（已弃用，忽略）
} else if src_ip.is_empty() || dst_ip.is_empty() {
    // IP 级规则 → qos_mgr.limit_ip()
} else if src_port == 0 && dst_port == 0 {
    // Peer 级规则 → qos_mgr.limit_peer_pair()
} else {
    // 服务级规则 → qos_mgr.limit_service()
}
```

#### 3.2.5 修改 `sync()` 方法

**文件：** `agent-rust/agent/src/unified_agent.rs`

**变更：** 在 ACL 同步后添加 QoS 同步

```rust
pub async fn sync(&mut self) -> Result<()> {
    let sync_result = self.grpc_client.sync(self.config.public_key.clone()).await?;
    
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
    Ok(())
}
```

---

## 4. QoS 规则类型映射

### 4.1 三层 QoS 架构

Aria 支持三层 QoS 优先级，从高到低：

| 优先级 | 类型 | 数据库字段特征 | Agent 方法 |
|--------|------|---------------|-----------|
| **第一优先级** | 应用级 | src_ip ✓, dst_ip ✓, (src_port ✓ 或 dst_port ✓) | `limit_service()` |
| **第二优先级** | 对等体级 | src_ip ✓, dst_ip ✓, src_port=0, dst_port=0 | `limit_peer_pair()` |
| **第三优先级** | 全局级 | src_ip ✓ 或 dst_ip ✓ (另一个为空) | `limit_ip()` |

### 4.2 规则类型判断逻辑

```rust
fn classify_qos_rule(rule: &QoSRule) -> QoSRuleType {
    // 端口级规则（已弃用）
    if rule.src_ip.is_empty() && rule.dst_ip.is_empty() {
        return QoSRuleType::Port;  // 忽略
    }
    
    // IP 级规则（只有一端有 IP）
    if rule.src_ip.is_empty() || rule.dst_ip.is_empty() {
        return QoSRuleType::IP;
    }
    
    // Peer 级规则（两端都有 IP，但无端口）
    if rule.src_port == 0 && rule.dst_port == 0 {
        return QoSRuleType::Peer;
    }
    
    // 服务级规则（五元组完整）
    return QoSRuleType::Service;
}
```

---

## 5. 错误处理策略

### 5.1 Controller 端

| 错误场景 | 处理策略 | 影响 |
|---------|---------|------|
| 节点不存在 | 返回空列表 | 不影响同步 |
| 查询 tenant_id 失败 | 记录日志，返回空列表 | 不影响同步 |
| 查询 QoS 规则失败 | 记录日志，返回空列表 | 不影响同步 |
| 扫描规则失败 | 跳过该规则，继续其他 | 部分规则丢失 |

### 5.2 Agent 端

| 错误场景 | 处理策略 | 影响 |
|---------|---------|------|
| QoSManager 创建失败 | 记录错误并返回 | QoS 同步失败 |
| 单条规则应用失败 | 记录错误，继续其他规则 | 部分规则未生效 |
| eBPF 加载失败 | 记录错误，规则标记为失败 | 规则未生效 |
| 同步整体失败 | 记录错误，不影响 Peer/ACL 同步 | 下次重试 |

### 5.3 重试机制

- **Agent 端：** 每 5 秒自动重试，无需额外重试逻辑
- **Controller 端：** 无状态查询，失败直接返回空列表

---

## 6. 性能考虑

### 6.1 数据库查询优化

**索引建议：**
```sql
-- 为 tenant_id 添加索引
CREATE INDEX idx_bandwidth_limits_tenant_id ON bandwidth_limits(tenant_id);

-- 为 nodes.public_key 添加索引
CREATE INDEX idx_nodes_public_key ON nodes(public_key);
```

**查询性能：**
- 单次查询：`O(n)`，n = 租户的规则数量
- 典型场景：n < 100，查询时间 < 10ms

### 6.2 网络传输优化

**数据量估算：**
- 单条 QoS 规则：约 50 bytes
- 100 条规则：约 5 KB
- 传输时间：< 1ms（内网）

**压缩：** gRPC 默认使用 Protobuf，已高效压缩，无需额外压缩。

### 6.3 Agent 端性能

**规则应用性能：**
- 单条规则应用：< 10ms（eBPF map 更新）
- 100 条规则：< 1s
- 差异应用：只处理变更部分

**并发控制：**
- QoS 操作在 `spawn_blocking` 中执行
- 不阻塞异步运行时
- 与 Peer/ACL 同步串行执行

---

## 7. 测试计划

### 7.1 单元测试

#### Controller 端

```go
func TestGetQoSRules(t *testing.T) {
    // 测试正常场景
    t.Run("valid_tenant", func(t *testing.T) {
        // 插入测试数据
        // 调用 getQoSRules()
        // 验证返回结果
    })
    
    // 测试节点不存在
    t.Run("node_not_found", func(t *testing.T) {
        // 验证返回空列表
    })
    
    // 测试空规则列表
    t.Run("empty_rules", func(t *testing.T) {
        // 验证返回空列表
    })
}
```

#### Agent 端

```rust
#[test]
fn test_qos_rule_classification() {
    // 测试应用级规则
    let service_rule = QoSRule {
        src_ip: "192.168.1.1".to_string(),
        dst_ip: "10.0.0.1".to_string(),
        src_port: 8080,
        dst_port: 80,
        protocol: 6,
        bandwidth_mbps: 100,
    };
    assert_eq!(classify_qos_rule(&service_rule), QoSRuleType::Service);
    
    // 测试 Peer 级规则
    // 测试 IP 级规则
}
```

### 7.2 集成测试

**测试环境：**
- Controller: 112.124.8.241
- Agent (sh): 146.56.196.231
- Agent (bj): 118.195.135.16

**测试步骤：**

1. **创建 QoS 规则**
   ```bash
   curl -X POST https://aria.yun/api/v1/bandwidth/limits \
     -H "Authorization: Bearer <token>" \
     -d '{
       "src_ip": "100.64.0.1",
       "dst_ip": "100.64.0.2",
       "bandwidth_mbps": 50
     }'
   ```

2. **等待同步（5 秒）**
   ```bash
   sleep 6
   ```

3. **检查 Controller 日志**
   ```bash
   docker logs aria_controller 2>&1 | grep -i qos
   # 预期输出: [INFO] Retrieved 1 QoS rules for tenant xxx
   ```

4. **检查 Agent 日志**
   ```bash
   ssh root@146.56.196.231 "journalctl -u aria -n 100 --no-pager | grep -i qos"
   # 预期输出: QoS sync completed: 1 success, 0 failed
   ```

5. **验证 eBPF 程序**
   ```bash
   ssh root@146.56.196.231 "bpftool prog list | grep -i qos"
   # 预期输出: 应该看到 QoS 相关的 eBPF 程序
   ```

6. **验证 TC 配置**
   ```bash
   ssh root@146.56.196.231 "tc filter show dev eth0 egress"
   # 预期输出: 应该看到 QoS 过滤器
   ```

7. **测试带宽限制**
   ```bash
   # 在 sh 节点启动 iperf3 服务器
   ssh root@146.56.196.231 "iperf3 -s"
   
   # 在 bj 节点测试带宽
   ssh root@118.195.135.16 "iperf3 -c 100.64.0.1 -t 10"
   # 预期结果: 带宽应限制在 50 Mbps 左右
   ```

### 7.3 边界情况测试

| 测试场景 | 输入 | 预期结果 |
|---------|------|---------|
| 空规则列表 | 删除所有规则 | Agent 清空所有 QoS 规则 |
| 重复规则 | 相同的五元组 | 只应用一次 |
| 无效 IP | `256.256.256.256` | 记录错误，跳过该规则 |
| Agent 离线 | Agent 重启 | 自动同步最新规则 |
| Controller 重启 | Controller 重启 | 规则持久化，Agent 重连后正常同步 |
| 大量规则 | 1000 条规则 | 同步时间 < 5s |

---

## 8. 部署计划

### 8.1 代码变更清单

**Controller (Go):**
- ✏️ `internal/controller/grpc/server.go`
  - 添加 `store` 字段到 `ControllerServer`
  - 新增 `getQoSRules()` 方法
  - 修改 `Sync()` 方法
  - 修改 `NewControllerServer()` 构造函数

- ✏️ `cmd/controller/main.go` (或 gRPC 初始化位置)
  - 更新 `NewControllerServer()` 调用，传入 `store` 参数

**Agent (Rust):**
- ✏️ `agent-rust/agent/src/grpc_client.rs`
  - 新增 `QoSRule` 结构体
  - 修改 `SyncResult` 结构体
  - 修改 `sync()` 方法

- ✏️ `agent-rust/agent/src/unified_agent.rs`
  - 新增 `sync_qos_rules()` 方法
  - 修改 `sync()` 方法

### 8.2 部署步骤

#### 步骤 1: Controller 更新

```bash
# 1. 修改代码
vim internal/controller/grpc/server.go

# 2. 本地测试
make test

# 3. 构建 Docker 镜像
docker buildx build --platform linux/amd64 \
  -t aria-controller:latest \
  -f Dockerfile.controller . --load

# 4. 保存镜像
docker save aria-controller:latest -o bin/images/aria-controller-latest.tar

# 5. 上传并部署
rsync -avz bin/images/ root@112.124.8.241:/root/aria-controller/bin/images/
ssh root@112.124.8.241 "cd /root/aria-controller && ./deploy-controller.sh deploy"

# 6. 验证
docker logs aria_controller 2>&1 | grep -i "qos"
```

#### 步骤 2: Agent 更新

```bash
# 1. 修改代码
vim agent-rust/agent/src/grpc_client.rs
vim agent-rust/agent/src/unified_agent.rs

# 2. 同步源码到所有节点
rsync -avz --delete agent-rust/agent/src/ root@146.56.196.231:/root/agent-rust/agent/src/
rsync -avz --delete agent-rust/agent/src/ root@118.195.135.16:/root/agent-rust/agent/src/

# 3. 在每个节点编译
ssh root@146.56.196.231 "source ~/.cargo/env && cd /root/agent-rust && cargo clean && cargo build --release"
ssh root@118.195.135.16 "source ~/.cargo/env && cd /root/agent-rust && cargo clean && cargo build --release"

# 4. 原子替换并重启
ssh root@146.56.196.231 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria && systemctl restart aria"

ssh root@118.195.135.16 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria && systemctl restart aria"

# 5. 验证
ssh root@146.56.196.231 "journalctl -u aria -n 50 --no-pager | grep -i qos"
ssh root@118.195.135.16 "journalctl -u aria -n 50 --no-pager | grep -i qos"
```

#### 步骤 3: 功能验证

```bash
# 1. 创建测试规则
curl -X POST https://aria.yun/api/v1/bandwidth/limits \
  -H "Authorization: Bearer <token>" \
  -d '{"src_ip":"100.64.0.1","dst_ip":"100.64.0.2","bandwidth_mbps":50}'

# 2. 等待同步
sleep 6

# 3. 检查 Agent 日志
ssh root@146.56.196.231 "journalctl -u aria -n 100 | grep -i qos"

# 4. 检查 eBPF 程序
ssh root@146.56.196.231 "bpftool prog list | grep -i qos"
```

### 8.3 回滚计划

**如果出现问题，回滚步骤：**

1. **Controller 回滚：**
   ```bash
   # 加载旧镜像
   docker load -i /root/aria-controller/bin/images/aria-controller-old.tar
   docker restart aria_controller
   ```

2. **Agent 回滚：**
   ```bash
   # 恢复旧二进制
   ssh root@146.56.196.231 "mv -f /usr/local/bin/aria.bak /usr/local/bin/aria && systemctl restart aria"
   ssh root@118.195.135.16 "mv -f /usr/local/bin/aria.bak /usr/local/bin/aria && systemctl restart aria"
   ```

---

## 9. 监控和告警

### 9.1 关键指标

| 指标 | 描述 | 告警阈值 |
|------|------|---------|
| `qos_sync_duration_seconds` | QoS 同步耗时 | > 5s |
| `qos_rules_applied_total` | 成功应用的规则数 | - |
| `qos_rules_failed_total` | 应用失败的规则数 | > 0 |
| `qos_sync_errors_total` | 同步错误总数 | > 10/min |

### 9.2 日志格式

**Controller 端：**
```
[INFO] Retrieved 3 QoS rules for tenant 00000000-0000-0000-0000-000000000001
[WARN] Failed to query QoS rules: connection refused
```

**Agent 端：**
```
INFO aria_agent::unified_agent: Syncing 3 QoS rules
INFO aria_agent::unified_agent: Applied QoS rule: 100.64.0.1:0:100.64.0.2:0:6
INFO aria_agent::unified_agent: QoS sync completed: 3 success, 0 failed
ERROR aria_agent::unified_agent: Failed to apply QoS rule: Invalid IP address
```

---

## 10. 未来优化方向

### 10.1 短期优化（1-2 周）

- [ ] 实现规则变更通知（通过 CommandStream）
- [ ] 添加规则状态反馈（Agent → Controller）
- [ ] 优化规则查询性能（缓存）

### 10.2 中期优化（1-2 月）

- [ ] 支持规则优先级动态调整
- [ ] 实现规则冲突检测
- [ ] 添加带宽统计和可视化

### 10.3 长期优化（3-6 月）

- [ ] 支持 QoS 规则模板
- [ ] 实现 AI 自动调优
- [ ] 多租户 QoS 隔离

---

## 11. 附录

### 11.1 相关文件列表

**Controller 端：**
- `internal/controller/grpc/server.go` - gRPC 服务端实现
- `internal/api/v1/bandwidth_management.go` - 带宽管理 API
- `pkg/controllerstorage/storage.go` - 数据库访问层
- `pkg/grpc/agentpb/aria-agent.proto` - Protobuf 定义

**Agent 端：**
- `agent-rust/agent/src/grpc_client.rs` - gRPC 客户端
- `agent-rust/agent/src/unified_agent.rs` - Agent 主逻辑
- `agent-rust/agent/src/qos.rs` - QoS Manager 实现
- `agent-rust/proto/aria-agent.proto` - Protobuf 定义

### 11.2 数据库表结构

```sql
CREATE TABLE bandwidth_limits (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    src_ip VARCHAR(50),
    dst_ip VARCHAR(50),
    src_port INTEGER DEFAULT 0,
    dst_port INTEGER DEFAULT 0,
    protocol INTEGER DEFAULT 6,
    bandwidth_mbps INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_bandwidth_limits_tenant_id ON bandwidth_limits(tenant_id);
```

### 11.3 参考资料

- [Aria 架构文档](../ARCHITECTURE-REFACTOR.md)
- [gRPC 测试文档](../GRPC-TESTING.md)
- [Agent 部署文档](../RUST-AGENT-DEPLOYMENT.md)
- [WireGuard 官方文档](https://www.wireguard.com/)
- [eBPF 官方文档](https://ebpf.io/)

---

**文档版本：** v1.0  
**最后更新：** 2026-03-04  
**审批状态：** ✅ 已批准  
**下一步：** 创建详细实施计划
