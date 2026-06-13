# 已知问题状态追踪

**复核日期**: 2026-05-29
**复核方式**: 重新对照当前代码，而不是沿用历史修复结论

---

## 当前已确认完成的结构项

### 1. API 命名空间收敛

**状态**: ✅ 已完成

- 前端 API 配置已统一到 `/v2/...` 路径，见 `frontend/src/config/api.js`
- 后端已删除 `internal/api/v1`，当前北向入口为 `/api/v2/...`
- 当前文档把 `v2-only` 作为基线是成立的

### 2. 节点身份与运行期鉴权分离

**状态**: ✅ 已完成

- 注册与运行期凭据已拆分
- gRPC 运行期鉴权、Runtime Token 和自动刷新链路已经接入
- 当前把这部分写成已落地是准确的

### 3. Desired/Applied 版本化

**状态**: ✅ 已完成

- 节点控制状态已经有 `desired_state_version`
- ACL/QoS/Blacklist 等策略写操作会触发版本提升与同步派发
- 当前把 “Sync 即时拼装，未版本化” 视为已解决是准确的

---

## 已完成基础收敛，但不应继续写成“完全闭环”的项

### 4. 前端菜单与后端领域模型对齐

**状态**: ⚠️ 基础对齐已完成，产品闭环未完全收口

- 路由、菜单和权限模型已经基本对齐 v0.1.0 领域划分
- 但 `Nodes`、`Monitoring`、`Policy Center` 仍是分散页面，不是单一运维工作台
- 因此这里应表述为“基础对齐完成”，不应再写成“已完全修复”

### 5. ACL / Policies / Bandwidth 概念重叠

**状态**: 🟢 IP Group 主模型已落地，页面仍可继续统一体验

- 后端已经统一到节点级策略域，并具备版本提升与下发记录
- ACL 前端优先发送 `src_group_id/dst_group_id`，直接 CIDR 只作为 inline IP Group 快捷路径
- QoS 前端优先发送 `group_id`，直接 CIDR 只作为 inline IP Group 快捷路径
- Controller、Sync、Agent、eBPF runtime id 映射已经按 IP Group 模型收口
- 仍可继续优化的是前端体验：`IP Group`、`ACL`、`Bandwidth` 仍是分页面管理，后续可以做成统一策略工作台

---

## 当前仍应保留在文档里的现实问题

### Settings / Backup 已完成最小收口，但未完全产品化

**状态**: 🟢 最小真实能力已完成，仍需增强恢复安全性

- 后端 `internal/api/v2/settings.go` 已支持真实的备份 `create/list/download/delete/upload/restore`
- 备份导出和恢复是全局控制面能力，当前已收口为 `super_admin` 专用
- 前端下载改为通过鉴权 API 客户端拉取 blob，不再使用裸 `window.location.assign`
- 仍未完成的是恢复前 dry-run、选择性恢复、二次确认强约束等恢复安全增强；其他系统设置项继续隐藏，不展示 placeholder

### 运维闭环仍未完全打通

**状态**: 🟡 进行中，Nodes 工作台第一阶段已收口

- 监控、策略投递、命令历史、AI 入口都已经存在
- Nodes 详情已能集中展示在线状态、desired/applied version、最近同步、最近命令、最近策略投递、活跃告警和证书状态
- Nodes 详情已支持 `sync` / `health_check` 快速命令下发，并在后端刷新前先显示 queued/pending 状态
- Nodes 详情、Monitoring 详情和 Policy Center 已能通过 node/policy/command/focus 上下文互相跳转
- 但“发现问题 -> 确认动作 -> 执行 -> 审计回看”的全局统一体验还没完全收口，尤其是 AI/IM 主动确认链路仍待推进
- 这仍然应该是后续实现计划里的高优先级项

---

## 结论

当前代码支持以下更准确的判断：

- `v2-only`、运行期鉴权、状态版本化这些底层结构项已经落地
- `菜单/领域对齐` 与 `策略概念收拢` 只完成了基础层，不应继续表述为“完全闭环”
- `Settings / Backup` 已具备最小真实能力；后续重点是恢复安全增强，而不是补齐基础 upload/restore
