# 已知问题状态追踪

**复核日期**: 2026-06-27
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

### 运维闭环非 AI 链路已完成第一阶段验收

**状态**: ✅ v0.1.0 非 AI 闭环已完成；Hermes / IM / 自动化留到后续阶段

- Monitoring 已能展示 active alert，并从 alert 跳转到 Node Detail / Policy Center。
- Nodes 详情已能集中展示在线状态、desired/applied version、最近同步、最近命令、最近策略投递、活跃告警和证书状态。
- Monitoring 和 Node Detail 已支持用户确认执行 `sync` / `health_check`，并能携带 `alert_id`、`event_type`、`policy_ref`、`policy_domain`、`command_id` 等上下文。
- Controller 已能创建 `agent_commands`、记录 `command.queued` audit，Agent 回写后在 command/event/audit 里留下执行结果。
- Resolve Alert 已支持 `reason/source/command_id` 处理证据，并保留 `alert_resolved` 事件。
- 2026-06-27 线上已完成代表性非 AI 告警处置链路验收：active `policy_failed` / `sync_failed` alert 触发 `sync`，Agent 回写 `completed` / `sync completed`，最终 active alerts 为 `0`。
- 不再把旧 Ask AI 链路作为 v0.1.0 验收项；AI 建议、IM 卡片确认、无人值守自愈、多步骤 ActionPlan 等后续由 Hermes Agent 阶段重新设计。

---

## 结论

当前代码支持以下更准确的判断：

- `v2-only`、运行期鉴权、状态版本化这些底层结构项已经落地
- `菜单/领域对齐` 与 `策略概念收拢` 只完成了基础层，不应继续表述为“完全闭环”
- `Settings / Backup` 已具备最小真实能力；后续重点是恢复安全增强，而不是补齐基础 upload/restore
- `运维闭环` 的 v0.1.0 非 AI 链路已经完成线上验收；剩余工作是 Hermes/IM/自动化增强，不再作为当前阻塞项
