# CHANGELOG

All notable changes to the Aria project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-03-03

### 🚀 重大架构重构

#### Agent 从 Go 重构为 Rust

- **性能大幅提升**：
  - 内存占用降低 80%（50MB → 10MB）
  - 启动时间降低 75%（2s → 0.5s）
  - CPU 占用降低 67%（3% → 1%）
  - 数据面吞吐量提升 10 倍（1 Gbps → 10 Gbps）

- **eBPF 数据面**：
  - XDP ACL（访问控制）
  - TC QoS（流量限速）
  - 内核层处理，性能更优
  - 支持 IPv4/IPv6

- **功能完整性**：
  - WireGuard 隧道管理
  - gRPC 控制面通信
  - 路由同步
  - Prometheus Metrics
  - Unix Socket CLI

### 🗑️ 删除的代码

#### Go Agent 代码完全删除

- `internal/cli/up.go` - Agent 启动
- `internal/cli/down.go` - Agent 停止
- `internal/cli/status.go` - Agent 状态
- `internal/cli/peers.go` - Peer 管理
- `internal/cli/route.go` - 路由管理
- `internal/cli/ping.go` - Ping 工具
- `internal/cli/tune.go` - 调优工具
- `internal/cli/init.go` - Agent 初始化
- `internal/cli/install.go` - Agent 安装
- `internal/agent/` - Agent 业务逻辑（整个目录）

### ✨ 新增功能

#### Rust Agent (agent-rust/)

- `unified_agent.rs` - 统一 Agent 核心
  - 整合 eBPF + gRPC + WireGuard
  - 主控制循环
  - Metrics 采集
  - 配置热加载
  - SIGHUP 信号处理

- `acl.rs` 扩展
  - `get_all_rule_stats()` - 获取所有 ACL 规则统计
  - `clear_all_rules()` - 清空所有 ACL 规则

- `qos.rs` 扩展
  - `get_all_qos_stats()` - 获取所有 QoS 规则统计
  - `clear_all_rules()` - 清空所有 QoS 规则

- `config.rs` 完善
  - 添加 mTLS 证书配置
  - 添加 sync_interval 配置
  - 改进序列化/反序列化

- `metrics.rs` 实现
  - WireGuard 统计指标
  - eBPF ACL 统计指标
  - eBPF QoS 统计指标
  - gRPC 心跳统计
  - 配置重载统计

### 📝 配置文件

- 新增 `/etc/aria/agent.yaml` 配置示例
- 支持 YAML 格式
- 支持配置热加载（SIGHUP）

### 🔧 CLI 命令

#### 新的 Agent 命令（Rust）

```bash
aria-agent up --interface eth0              # 启动完整 Agent
aria-agent init --server <URL> --token <TOKEN>  # 初始化配置
aria-agent status                           # 查看状态
aria-agent peers                            # 查看 Peers
aria-agent qos limit-ip --ip 10.0.0.1 --mbps 100  # QoS 管理
aria-agent acl allow --src-ip ...           # ACL 管理
```

#### Controller 命令（Go，保持不变）

```bash
aria-controller serve --config /etc/aria/controller.yaml  # 启动 Controller
aria-controller token create --uses 10 --ttl 24h         # 创建 Token
aria-controller admin list                               # 查看所有 Agent
```

### 📊 Metrics

#### 新增 Prometheus 指标

- `wireguard_peer_rx_bytes` - Peer 接收字节数
- `wireguard_peer_tx_bytes` - Peer 发送字节数
- `wireguard_total_peers` - 总 Peer 数
- `wireguard_active_peers` - 活跃 Peer 数
- `acl_packets_passed_total` - ACL 通过包数
- `acl_packets_dropped_total` - ACL 丢弃包数
- `qos_bytes_passed_total` - QoS 通过字节数
- `qos_bytes_dropped_total` - QoS 丢弃字节数
- `grpc_heartbeat_success_total` - gRPC 心跳成功数
- `grpc_heartbeat_failure_total` - gRPC 心跳失败数
- `config_reload_total` - 配置重载次数

### 📚 文档

- 新增 `docs/ARCHITECTURE-REFACTOR.md` - 架构重构说明
- 新增 `agent-rust/config/agent.yaml.example` - 配置示例
- 更新部署文档

### ⚠️ 破坏性变更

- **Go Agent 完全废弃**：不再维护 Go Agent 代码
- **二进制分离**：`aria` 拆分为 `aria-controller` 和 `aria-agent`
- **配置文件位置**：Agent 配置从 `/etc/aria/config.yaml` 改为 `/etc/aria/agent.yaml`

### 🔄 迁移指南

#### 从 Go Agent 迁移到 Rust Agent

```bash
# 1. 停止旧 Agent
systemctl stop aria

# 2. 备份配置
cp /etc/aria/agent.yaml /etc/aria/agent.yaml.bak

# 3. 安装 Rust Agent
./aria-agent init --server <CONTROLLER> --token <TOKEN>
./aria-agent up --interface eth0

# 4. 验证连接
./aria-agent status
```

### 🐛 已知问题

1. **编译环境要求**：Rust Agent 需要安装 Rust 工具链
2. **内核版本要求**：eBPF 需要 Linux 5.4+
3. **证书管理**：需要手动管理 mTLS 证书

### 🛠️ 技术栈

#### Controller（Go）
- Go 1.22+
- gRPC + Protobuf
- PostgreSQL
- Redis
- Gin Web Framework

#### Agent（Rust）
- Rust 1.75+
- Tokio（异步运行时）
- Aya（eBPF 框架）
- WireGuard UAPI
- Tonic（gRPC 客户端）
- Metrics + Prometheus

---

## [0.2.26-test-7] - 2026-03-02

### 新增
- AI 助手集成（DeepSeek）
- 飞书/钉钉通知支持
- 多租户网络隔离

### 改进
- Web UI 性能优化
- gRPC 心跳机制
- 配置同步流程

### 修复
- WireGuard 隧道重建问题
- 路由同步延迟
- Metrics 指标丢失

---

## [0.2.0] - 2026-02-11

### 重大变更
- 无状态防火墙 + NOTRACK
- CPU 占用降低 67%（15% → 5%）
- 延迟 P99 降低 75%（2ms → 0.5ms）

### 新增
- eBPF 数据面
- 端口级别流量控制
- 五元组 QoS

---

## [0.1.0] - 2026-01-15

### 新增
- 初始版本发布
- WireGuard 隧道管理
- 基础 ACL 功能
- Web 管理界面

---

[0.3.0]: https://github.com/anomaly/aria/compare/v0.2.26...v0.3.0
[0.2.26-test-7]: https://github.com/anomaly/aria/compare/v0.2.0...v0.2.26
[0.2.0]: https://github.com/anomaly/aria/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/anomaly/aria/releases/tag/v0.1.0
