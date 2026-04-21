# Aria 架构重构说明

## 文档状态

这份文档描述的是当前仓库的真实拆分形态，而不是早期迁移计划。它侧重回答 3 个问题：

- Controller 和 Agent 的职责现在怎么分
- 当前南北向接口如何连接
- 哪些能力已经迁走，哪些仍然保留在 Go 控制面中

## 当前架构

Aria 当前是一个单仓库、双运行时的系统：

```text
aria-controller (Go)                 aria-agent (Rust)
├── HTTP northbound API              ├── WireGuard interface / peers
├── HTTP southbound fixed endpoints  ├── eBPF ACL + QoS
├── gRPC ControllerService           ├── gRPC client + command stream
├── PostgreSQL / Redis               ├── routing / sync loop
├── Monitoring / RBAC / AI / IM      ├── local unix socket / local commands
└── Frontend static assets           └── runtime state persistence
```

## 二进制与入口

### Controller（Go）

- 根入口是 `cmd/main.go`
- 实际 CLI 由 `internal/cli/root.go` 提供，主二进制名为 `aria-controller`
- `controller serve` 会启动：
  - HTTP API（默认 `:8080`）
  - gRPC 服务（默认 `:50051`）
  - PostgreSQL / Redis / Monitoring / AI / IM 初始化

### Agent（Rust）

- 工作区位于 `agent-rust/`
- 统一运行期核心在 `agent-rust/agent/src/unified_agent.rs`
- 负责：
  - WireGuard 接口创建与 peer 管理
  - eBPF ACL / QoS 挂载与更新
  - gRPC `Register / Sync / CommandStream / ReportMetrics`
  - 本地状态持久化与 unix socket 命令

## 接口分层

### Northbound

提供给前端和管理员，统一走 `/api/v2/...`：

- `auth`
- `tenants / users / tokens / roles`
- `nodes / routes / policies`
- `security / qos`
- `monitoring`
- `ai`
- `settings`

### Southbound

当前采用“固定 HTTP 入口 + gRPC 运行期通道”的组合：

- 固定 HTTP 入口：
  - `/api/v2/agents/register`
  - `/api/v2/agents/unregister`
  - `/api/v2/agents/network`
  - `/api/v2/agents/certificates/{issue,renew}`
- gRPC `ControllerService`：
  - `Register`
  - `Sync`
  - `CommandStream`
  - `ReportMetrics`

## 代码边界

### 已明确迁到 Rust Agent 的能力

- WireGuard 运行期配置
- eBPF ACL / QoS 执行
- Agent 同步循环
- 命令流消费与本地执行
- 节点运行时状态文件

### 仍在 Go 控制面的能力

- 多租户模型与 RBAC
- Token / Runtime Token / 证书签发
- 策略存储、版本化与交付记录
- 命令队列
- Monitoring / Alerts / Audit
- AI 与飞书 / 钉钉集成

### 仍保留在 Go 中、但不是旧 Agent 运行时的部分

仓库里仍有 `internal/agent/` 目录，但它当前承担的是 AI 工具与控制面辅助逻辑，不是旧的 Go Agent 数据面。

## 当前实现与旧记录的差异

下面这些旧描述已经不再准确：

- Controller 当前使用标准库 `net/http` / `ServeMux`，不是 Gin。
- `internal/api/v1` 已删除，北向业务 API 已收敛到 `v2-only`。
- `internal/agent/` 并没有整体消失，只是职责已经不再是旧 Agent 数据面。
- 架构文档中的性能数字与季度计划不是当前代码事实，不应再作为基线说明。

## 配置与运行期状态

### Controller

Controller 配置主要覆盖：

- PostgreSQL
- Redis
- Network base CIDR
- JWT / Runtime Token secret
- gRPC TLS / mTLS
- AI / Feishu / DingTalk

### Agent

Agent 配置与运行时状态已经逐步拆分：

- bootstrap 信息：
  - controller URL
  - enrollment token
  - interface / region / TLS 材料
- runtime state：
  - node_id
  - assigned_ip
  - current credential
  - last desired/applied version
  - last sync observation

## 当前产品阶段的重点

当前架构层面已经不再缺“Controller/Agent 是否分离”的说明，后续重点是：

1. 把 Platform / Settings 中仍是 placeholder 的能力做真
2. 把 `Nodes + Monitoring + Policy Center` 收成连续工作流
3. 把 Monitoring / AI / IM 串成主动告警与确认执行闭环
4. 把证书生命周期从“可用”推进到“可部署”

---

**更新日期**: 2026-04-21  
**状态**: current snapshot
