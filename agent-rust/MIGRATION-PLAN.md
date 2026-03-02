# Go Agent → Rust Agent 迁移计划

## 当前架构

```
┌─────────────────────────────────────┐
│   Go Agent (aria) - 45,000+ 行      │
│  ┌──────────┐  ┌─────────────────┐  │
│  │ WireGuard│  │ gRPC/Controller │  │
│  │  隧道管理 │  │     通信        │  │
│  └──────────┘  └─────────────────┘  │
│  ┌──────────┐  ┌─────────────────┐  │
│  │  路由管理 │  │   防火墙/ACL   │  │
│  │  系统路由 │  │  (已迁移Rust)  │  │
│  └──────────┘  └─────────────────┘  │
│  ┌──────────┐  ┌─────────────────┐  │
│  │ 健康检查  │  │  指标/监控     │  │
│  │  延迟/丢包│  │  Prometheus    │  │
│  └──────────┘  └─────────────────┘  │
└─────────────────────────────────────┘
```

## Go Agent 功能清单

### 核心功能（必须迁移）

| 功能 | 复杂度 | 优先级 | 说明 |
|------|--------|--------|------|
| **WireGuard 隧道** | ⭐⭐⭐⭐⭐ | P0 | 多隧道管理，接口创建/配置 |
| **gRPC 通信** | ⭐⭐⭐⭐ | P0 | 与 Controller 同步配置 |
| **系统路由** | ⭐⭐⭐ | P0 | Linux routing tables |
| **eBPF 防火墙** | ⭐⭐⭐⭐ | ✅ 已完成 | QoS + ACL |
| **配置管理** | ⭐⭐ | P1 | YAML 配置文件 |
| **健康检查** | ⭐⭐⭐ | P1 | Peer 延迟、丢包检测 |
| **指标导出** | ⭐⭐ | P2 | Prometheus metrics |

### CLI 命令

| 命令 | 功能 | 优先级 |
|------|------|--------|
| `init` | 初始化配置，注册到 Controller | P0 |
| `up` | 启动 agent daemon | P0 |
| `down` | 停止 agent | P0 |
| `status` | 查看状态 | P1 |
| `peers` | 查看 peer 列表 | P1 |
| `route` | 路由管理 | P1 |
| `sync` | 手动同步配置 | P1 |
| `token` | Token 管理 | P2 |
| `tune` | 系统调优 | P2 |
| `install` | 安装 systemd 服务 | P2 |

### 依赖库（Rust 替代方案）

| Go 库 | 功能 | Rust 替代 |
|-------|------|----------|
| `golang.zx2c4.com/wireguard` | WireGuard | `wireguard-uapi` + `netlink` |
| `google.golang.org/grpc` | gRPC | `tonic` |
| `github.com/spf13/cobra` | CLI | `clap` |
| `github.com/vishvananda/netlink` | 网络配置 | `rtnetlink` / `netlink-packet-route` |
| `gopkg.in/yaml.v3` | YAML | `serde_yaml` |
| `github.com/prometheus/client_golang` | Metrics | `prometheus` |

## 迁移策略

### 方案 A：渐进式迁移（推荐）

```
Phase 1: Rust eBPF Agent 独立运行 ✅ 已完成
  ├─ QoS 限速功能
  ├─ ACL 规则管理
  └─ Unix Socket 接口

Phase 2: 添加 WireGuard 管理到 Rust
  ├─ 集成 wireguard-uapi
  ├─ 接口创建/配置
  └─ Peer 管理

Phase 3: 添加 gRPC 通信
  ├─ Controller 同步协议
  ├─ 配置下发
  └─ 状态上报

Phase 4: 添加路由管理
  ├─ Linux routing tables
  └─ ECMP 支持

Phase 5: 替换 Go agent
  ├─ 所有功能迁移完成
  └─ 停止维护 Go 版本
```

**优点：**
- 风险可控，随时可回退
- 可以逐步验证每个功能
- Go agent 继续服务，不影响生产

**时间估计：** 4-6 周

### 方案 B：一次性重写

直接用 Rust 重写所有功能，然后一次性切换。

**优点：**
- 代码架构更统一
- 不需要维护两套代码

**缺点：**
- 风险高
- 时间长（8-12 周）
- 难以调试和验证

## Phase 2 详细计划：WireGuard 管理

### 需要的 Rust crates

```toml
[dependencies]
# WireGuard
wireguard-uapi = "3.0"          # WireGuard netlink API
netlink-packet-route = "0.19"   # Netlink 协议
netlink-sys = "0.8"             # Netlink 系统

# 网络
socket2 = "0.5"                 # Socket 操作
interfaces = "0.0"              # 网络接口管理

# 已有依赖
tokio = { version = "1", features = ["full"] }
serde = { version = "1", features = ["derive"] }
clap = { version = "4", features = ["derive"] }
```

### 核心结构

```rust
// WireGuard 管理器
pub struct WireGuardManager {
    interface_name: String,
    config: InterfaceConfig,
}

// Peer 配置
pub struct PeerConfig {
    public_key: String,
    endpoint: Option<String>,
    allowed_ips: Vec<String>,
    persistent_keepalive: u16,
}

// 主要方法
impl WireGuardManager {
    pub fn create_interface(&mut self) -> Result<()>;
    pub fn add_peer(&mut self, peer: &PeerConfig) -> Result<()>;
    pub fn remove_peer(&mut self, public_key: &str) -> Result<()>;
    pub fn list_peers(&self) -> Result<Vec<PeerInfo>>;
    pub fn get_stats(&self) -> Result<InterfaceStats>;
}
```

### 工作量估计

| 任务 | 时间 |
|------|------|
| WireGuard 接口管理 | 2-3 天 |
| Peer 配置与管理 | 2-3 天 |
| 集成到现有架构 | 1-2 天 |
| 测试与验证 | 1-2 天 |
| **总计** | **6-10 天** |

## 下一步行动

### 立即执行（推荐）

**开始 Phase 2：添加 WireGuard 到 Rust Agent**

1. 添加依赖到 `agent-rust/agent/Cargo.toml`
2. 创建 `wireguard.rs` 模块
3. 实现接口创建/配置
4. 实现 Peer 管理
5. 添加 CLI 命令
6. 测试验证

### 或者

**等待决策：**
- 选择迁移方案（渐进式 vs 一次性）
- 确定优先级
- 分配开发时间

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| WireGuard API 复杂 | 中 | 使用成熟 crate，参考 Go 实现 |
| gRPC 协议兼容性 | 高 | 使用 protobuf 定义，保证兼容 |
| 生产环境故障 | 高 | 渐进式迁移，保留回退能力 |
| 性能问题 | 中 | Rust 性能更好，测试验证 |

## 成功标准

- [ ] 所有 Go agent 功能在 Rust 中实现
- [ ] 通过所有现有测试
- [ ] 生产环境稳定运行 1 周
- [ ] 性能不低于 Go 版本
- [ ] 二进制体积合理（< 20MB）

---

**创建时间：** 2026-03-02
**最后更新：** 2026-03-02
**负责人：** 暂定
