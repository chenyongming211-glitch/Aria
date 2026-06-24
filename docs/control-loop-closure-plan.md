# 控制闭环实施方案

本文定义 Aria SD-WAN 控制闭环的目标链路、状态模型、落地步骤和验收标准。

控制闭环的目标不是“前端调用接口成功”，而是让用户下发的每一次策略或命令，都能被 Controller 校验、持久化、投递到 Agent、由 Agent 执行，并在前端看到真实执行结果。

## 目标链路

```text
前端创建 / 更新 / 删除策略或命令
-> Controller 校验租户、权限、节点、参数和冲突
-> Controller 写入业务记录和 desired state
-> Controller 生成 desired_state_version
-> Controller 创建 agent_command 和 policy_delivery
-> Agent 通过 Sync / CommandStream 获取变更
-> Agent 应用 WireGuard / route / ACL / QoS / runtime command
-> Agent 回传 applied_state / observed_state / command result
-> Controller 更新 node_control_states / policy_deliveries / audit_events
-> 前端回显 pending / sent / applied / failed 以及失败原因
```

第一阶段重点是把现有 `desired/applied/observed`、`agent_commands`、`policy_deliveries`、`audit_events` 和前端 Nodes / Policy Center / Monitoring 贯通。

## v0.1.0 闭环终点

控制闭环第一阶段的终点是：

```text
前端创建 / 更新 / 删除 ACL、QoS 或 Route
-> Controller 校验并写 desired_state_version
-> Controller 生成 command_id / policy_delivery
-> Agent 执行
-> Agent 回写 applied_state_version 或 failed reason
-> Policy Center / Nodes / Monitoring 看到同一个结果
```

到这里即视为 v0.1.0 控制闭环完成，可以停止扩展。第一阶段不继续追求多节点强事务、策略历史回滚 UI、AI 自动生成并自动下发策略、ACE 大重构或所有策略模型重写。

停止规则：HTTP 成功不再被当作执行成功；用户能看到每次策略或命令是否真的被 Agent 应用，失败时能看到同一个失败原因。

## 控制对象范围

第一阶段纳入闭环的对象：

| 对象 | 用户入口 | Controller 记录 | Agent 执行 |
| --- | --- | --- | --- |
| Route | Policy Center / Nodes | route 业务表、desired state、policy delivery | 更新路由配置 |
| ACL | Policy Center / ACL Rules | ACL 业务表、desired state、policy delivery | 更新 ACL dataplane |
| QoS | Policy Center / QoS Rules | QoS 业务表、desired state、policy delivery | 更新 QoS dataplane |
| Blacklist / security | Security / Policy Center | security policy、desired state、policy delivery | 更新安全规则 |
| Agent command | Nodes / Monitoring / AI | agent command、audit event | `sync`、`health_check`、`config_reload` 等命令 |

暂不纳入第一阶段的对象：

- 多节点强事务。批量下发可以有逐节点结果，但不要求全部节点原子成功。
- 完全自动策略生成。AI 可以建议，写操作仍需要用户确认。
- ACE 或更大的控制平面重构。第一阶段复用当前 Controller 存储和 API。
- 历史策略版本回滚界面。可以先保留版本和审计，为后续回滚做基础。

## 状态模型

控制闭环需要区分三类状态：

| 状态层 | 含义 | 典型字段 |
| --- | --- | --- |
| desired | Controller 已接受并希望 Agent 达到的目标状态 | `desired_state_version`、`desired_state_metadata`、`desired_state_updated_at` |
| applied | Agent 已确认应用的目标版本 | `applied_state_version`、`applied_state_updated_at` |
| observed | Agent 或监控看到的运行态结果 | `observed_state`、`observed_message`、`last_sync_at`、`last_sync_error` |

策略或命令的生命周期建议统一为：

| 状态 | 含义 | 前端展示 |
| --- | --- | --- |
| `accepted` | Controller 已校验并写入业务记录 | 已接受 |
| `pending` | 已创建命令，等待 Agent 获取 | 待下发 |
| `sent` | 命令已发送到 Agent | 已发送 |
| `acknowledged` | Agent 已收到并开始处理 | 执行中 |
| `applied` / `completed` | Agent 执行成功，目标版本收敛 | 已应用 |
| `failed` | Agent 执行失败或 Controller 投递失败 | 失败 |
| `stale` | 已被更新的 desired version 覆盖 | 已过期 |
| `cancelled` | 用户或系统取消未执行任务 | 已取消 |

页面不要只看 HTTP 返回值判断成功。HTTP 成功只能说明 Controller 接受了请求，不能说明 Agent 已应用。

## 前端提交契约

前端提交策略或命令时，必须带上明确上下文：

- `tenant_id`
- `node_id` 或目标节点集合
- `policy_domain`，例如 `route`、`acl`、`qos`、`security`
- `action`，例如 `create`、`update`、`delete`、`sync`
- 业务参数，例如 CIDR、IP Group、端口、协议、优先级、限速值、方向
- 可选的 `policy_ref`，用于把投递记录和业务对象关联起来

前端提交后应显示两段结果：

1. Controller 接受结果：业务记录 ID、`desired_state_version`、`command_id`、`policy_delivery.id`。
2. Agent 执行结果：`pending/sent/acknowledged/applied/failed`、时间、错误原因。

建议页面行为：

- 表单本地校验只处理必填项、格式和明显范围错误。
- 权限、租户、节点状态、策略冲突必须由 Controller 再校验。
- 创建或更新成功后，不要只弹“保存成功”。应把行状态置为 `pending` 或 `queued`，直到后端返回应用结果。
- 删除策略也要进入闭环。删除不是前端移除一行，而是 Controller 记录 desired state 并投递给 Agent。

## Controller 校验和事务

Controller 接到前端请求后，应在一个明确的事务边界内完成以下步骤：

1. 解析 `tenant_id`，确认当前用户有目标租户权限。
2. 校验 RBAC 权限，例如 `policy:write`、`nodes:write` 或具体域权限。
3. 加载目标节点，拒绝 `deleted`、`suspended`、`banned` 等不可变更状态。
4. 校验业务参数：
   - CIDR 格式必须合法。
   - IP Group 引用必须存在且属于同一租户。
   - ACL/QoS 方向、协议、端口范围、优先级、动作必须合法。
   - Route 的目标网段和下一跳不能与现有策略产生不可解释冲突。
5. 写入业务表。
6. 生成新的 `desired_state_version`。
7. 更新 `node_control_states.desired_state_version` 和 `desired_state_metadata`。
8. 创建 `agent_commands`，通常为 `sync` 或具体 runtime command。
9. 创建 `policy_deliveries`，关联 `command_id`、`policy_domain`、`policy_ref`、`action`。
10. 写入 `audit_events`，事件类型建议使用 `policy.changed` 或 `command.queued`。
11. 返回结构化 dispatch 信息给前端。

返回结构建议包含：

```json
{
  "policy_id": "业务对象 ID",
  "dispatch": {
    "command_id": "agent command ID",
    "status": "pending",
    "desired_state_version": "ds-...",
    "desired_state_updated_at": "2026-06-23T10:00:00Z",
    "last_delivery": {
      "id": "policy delivery ID",
      "policy_domain": "acl",
      "policy_ref": "policy ID",
      "action": "update",
      "command_status": "pending"
    }
  }
}
```

## Agent 投递和执行

Agent 执行侧需要保持以下原则：

- `Sync` 拉取的是 Controller 当前目标快照，不是增量猜测。
- `CommandStream` 用于下发命令和触发快速同步。
- Agent 执行前应校验本地身份、runtime token、节点状态和快照完整性。
- ACL/QoS/Route 应尽量按一次快照应用，避免半新半旧状态。
- 执行失败时，应保留旧的已应用状态，并上报失败原因。
- 执行成功后，应回传 `applied_state_version`、执行结果和观察信息。

Agent 回传结果至少应包含：

| 字段 | 含义 |
| --- | --- |
| `command_id` | 对应 `agent_commands.id` |
| `status` | `acknowledged`、`completed`、`failed` |
| `applied_state_version` | 成功应用的 desired version |
| `observed_state` | `idle`、`syncing`、`degraded`、`error` 等 |
| `message` | 人可读说明 |
| `result` | 结构化执行结果 |
| `error` | 失败原因 |

## Controller 结果处理

Controller 收到 Agent 回传后，应更新以下数据：

| 数据表 / 对象 | 更新内容 |
| --- | --- |
| `agent_commands` | `status`、`message`、`result`、`sent_at`、`acknowledged_at`、`completed_at` |
| `policy_deliveries` | `command_status`、`last_error`、`completed_at` |
| `node_control_states` | `applied_state_version`、`observed_state`、`observed_message`、`last_sync_at`、`last_sync_error` |
| `audit_events` | 成功、失败、人工触发来源和关键上下文 |
| `alerts` | 必要时创建 `sync_failed`、`policy_failed` 或恢复事件 |

结果处理必须做到幂等：

- Agent 重试同一 `command_id` 不应重复创建多条完成记录。
- 同一个 desired version 的重复上报不应导致状态回退。
- 旧版本失败不应覆盖新版本成功状态。
- `failed` 状态必须保留可诊断错误，不要只写“执行失败”。

## 前端回显

控制闭环的前端回显至少要覆盖三个入口。

### Policy Center

每条策略行展示：

- 策略状态：启用 / 禁用 / 删除中
- 投递状态：待下发 / 执行中 / 已应用 / 失败
- 最近 `desired_state_version`
- 最近 `command_id`
- 最近失败原因
- 跳转到节点详情或 Monitoring 的入口

### Nodes

节点详情应展示：

- `desired_state_version`
- `applied_state_version`
- `state_convergence`
- `last_sync_at`
- `last_sync_error`
- 最近命令
- 最近策略投递
- 活跃告警

这会把 Nodes 变成单节点运维工作台，而不是只展示静态节点列表。

### Monitoring

Monitoring 应展示：

- 租户级同步成功率。
- 失败命令数。
- 活跃告警数。
- 单节点最近命令和策略投递。
- `sync_failed`、`policy_failed`、`command_failed` 的事件流。

Monitoring 的事件应能反向跳转到策略、节点和命令详情。

## 异常场景

| 场景 | 处理方式 | 前端表现 |
| --- | --- | --- |
| 租户不存在或无权限 | Controller 拒绝请求 | 403 / 404，不创建 desired state |
| 节点已删除 | Controller 视为不存在 | 404 |
| 节点被暂停或封禁 | Controller 拒绝变更 | 409，并说明节点状态 |
| 节点离线 | 可创建 desired state 和 pending command，但不能标记应用成功 | 显示待下发，超时后显示失败或等待上线 |
| 参数冲突 | Controller 拒绝业务写入 | 409，返回冲突对象 |
| Agent 收到但执行失败 | 保留旧 applied state，更新失败结果 | 策略行和节点详情显示失败原因 |
| Agent 长时间无响应 | command 超时，policy delivery 标记失败 | 显示超时，保留重试入口 |
| 批量下发部分失败 | 每个节点独立记录结果 | 批量视图显示成功/失败明细 |
| 新版本覆盖旧 pending | 旧 delivery 标记 `stale` 或保持历史，但不再作为当前状态 | 当前行只展示最新 desired version |

## 验收标准

控制闭环完成后，至少应能通过以下验收：

1. 创建 ACL 规则后，Controller 返回 `desired_state_version`、`command_id` 和 `policy_delivery`。
2. 创建 QoS 规则后，`node_control_states.desired_state_version` 发生变化。
3. 删除 Route 后，前端不立即假定删除完成，而是展示投递状态。
4. Agent 成功执行后，`applied_state_version` 等于最新 `desired_state_version`。
5. Agent 执行失败后，Policy Center、Nodes 和 Monitoring 都能看到同一失败原因。
6. 节点离线时，策略变更不会被标记为已应用。
7. `agent_commands`、`policy_deliveries`、`audit_events` 可以串起一次完整变更。
8. 同一租户只能看到自己的策略、命令和投递状态。
9. 普通只读用户无法触发写操作或 AI 写工具。
10. 回归测试覆盖成功、失败、离线、无权限、跨租户和超时场景。

## 实施顺序

建议按以下顺序推进：

1. 统一状态词表
   - 明确前端、API、存储中的 `pending/sent/acknowledged/applied/failed/stale` 映射。
   - 避免同一状态在不同页面显示成不同含义。

2. 统一策略变更返回结构
   - 所有 ACL/QoS/Route/Security 写接口都返回 `dispatch`。
   - `dispatch` 必须包含 `desired_state_version`、`command_id` 和最近 delivery。

3. 收口 Controller 事务
   - 所有策略写操作都走同一套 desired state、command、delivery、audit 写入路径。
   - 修正绕过投递链路的旧接口。

4. 强化 Agent 结果回写
   - CommandStream 和 Sync 都能回写 `applied_state_version`。
   - 失败时写入结构化错误。

5. 改造前端回显
   - Policy Center 展示投递状态。
   - Nodes 展示收敛状态和最近命令。
   - Monitoring 展示失败事件和跳转。

6. 增加超时和 stale 处理
   - pending 超过阈值后标记超时。
   - 新 desired version 覆盖旧 pending 时，前端不再把旧投递当成当前状态。

7. 补测试
   - 后端单测覆盖策略写入、投递、失败回写。
   - 前端单测覆盖状态映射和失败展示。
   - 浏览器冒烟覆盖 Policy Center -> Agent -> Monitoring 回显。

## 非目标

- 不在第一阶段实现全自动策略优化。
- 不把 Monitoring 当作独立大屏重做。
- 不把所有策略模型迁移到新框架。
- 不要求批量下发具备跨节点事务。

## 相关文档

- [节点接入闭环实施方案](node-onboarding-closure-plan.md)
- [运维闭环实施方案](operations-loop-closure-plan.md)
- [v0.1.0 产品蓝图](v0.1.0-product-blueprint.md)
- [API v2 白皮书](api-v2-whitepaper.md)
