# 运维闭环实施方案

本文定义 Aria SD-WAN 运维闭环的目标链路、事件模型、人为确认、执行结果和留痕方式。

运维闭环的目标不是“有监控页面”或“有聊天入口”，而是当系统发现异常后，能把监控、上下文、确认、执行和审计串成一条可追踪链路。

2026-06-27 范围调整：旧 AI Agent 暂停继续开发，后续计划替换为 Hermes Agent。v0.1.0 运维闭环先收口非 AI 链路：`Monitoring -> Alert -> 用户查看证据 -> Run Sync/Health Check -> Agent 回写 -> audit/event/resolve`。本文中涉及 AI Copilot、ActionPlan、Ask AI、IM 卡片确认的内容保留为 Hermes 阶段设计输入，不作为当前验收前置条件。

## 目标链路

```text
Monitoring 发现节点离线、同步失败、策略失败、证书风险或命令失败
-> Controller 生成 Alert / Event
-> Nodes / Monitoring 展示异常上下文
-> 用户在控制台或 IM 卡片确认
-> Controller 重新校验权限和当前状态
-> Controller 执行低风险命令或策略变更
-> Agent 执行并回传结果
-> Controller 写 audit / event / alert resolution
-> 前端和 IM 展示执行结果与恢复证据
```

第一阶段应复用现有 Monitoring API、`alerts`、`audit_events`、`agent_commands` 和 `policy_deliveries`，先把非 AI 闭环跑通，再在 Hermes 阶段扩展更复杂的自动化。

## v0.1.0 闭环终点

运维闭环第一阶段的终点是先跑通一条代表性链路，而不是做完整智能运维平台：

```text
Monitoring 发现 sync_failed、policy_failed 或 node_offline
-> Controller 生成 active alert
-> 用户点进 alert 看到证据
-> 用户在控制台确认 sync 或 health_check
-> Controller 执行命令
-> Agent 回写结果
-> alert / event / audit / node detail 都能看到处理结果
```

到这里即视为 v0.1.0 运维闭环完成，可以停止扩展。第一阶段优先用 `sync_failed` 跑通样板链路，再复制到 `policy_failed` 和 `node_offline`。Hermes AI 建议、IM 卡片确认、无人值守自动修复、复杂 ML 异常检测、多步骤 ActionPlan、自动回滚和熔断系统都不进入第一阶段终点。

停止规则：用户能从一个告警出发，完成一次“诊断 -> 确认 -> 执行 -> 留痕”，并且成功和失败都可见。

## 事件输入范围

第一阶段纳入运维闭环的异常来源：

| 来源 | 事件类型 | 检测依据 | 建议动作 |
| --- | --- | --- | --- |
| 节点状态 | `node_offline` | 节点心跳或状态为 offline | 查看节点详情、触发 health check、提示登录节点排查 |
| 同步状态 | `sync_failed` | `last_sync_error` 非空或 desired/applied 不一致 | 触发 `sync`，展示失败原因 |
| 策略投递 | `policy_failed` | `policy_deliveries.command_status=failed` | 查看策略、重试 sync、回滚或修正参数 |
| Agent 命令 | `command_failed` | `agent_commands.status=failed` | 展示命令结果、允许重试 |
| 证书 | `certificate_expiring` / `certificate_failed` | 证书即将过期、吊销或续期失败 | 提示续期、重新注册或人工处理 |
| 监控指标 | `high_latency` / `traffic_anomaly` | VictoriaMetrics 或 Prometheus 查询 | 展示节点流量、邻居和策略上下文 |

暂不把复杂 ML 异常检测作为第一阶段目标。第一阶段用确定性规则和可解释阈值即可。

## 统一事件模型

运维闭环建议统一以下对象：

| 对象 | 作用 | 当前基础 |
| --- | --- | --- |
| Alert | 需要处理的当前问题 | `alerts` |
| Event | 面向时间线展示的事件 | `alerts` + `audit_events` 合并查询 |
| AuditEvent | 谁在什么时候做了什么 | `audit_events` |
| AIAuditLog | AI 会话、工具调用、参数和结果 | `ai_audit_logs` |
| ActionPlan | AI 给出的待确认动作计划 | 第一阶段可先放在 AI 响应和 audit detail 中 |
| ApprovalRecord | 用户确认记录 | 第一阶段可先写入 `audit_events.detail`，后续再拆表 |
| ExecutionResult | Agent 或 Controller 执行结果 | `agent_commands.result`、`policy_deliveries.last_error` |

第一阶段不强制新增大表。可以先用 `audit_events.detail` 存放 `action_plan_id`、`approval_actor`、`approved_at`、`tool_name`、`command_id`、`policy_delivery_id` 等字段。等 IM 卡片确认和多步骤 ActionPlan 稳定后，再拆出正式的 `action_plans` / `approval_records` 表。

## Monitoring 发现规则

Monitoring 需要从“展示数据”升级为“产生事件”。

建议先实现以下规则：

| 规则 | 触发条件 | 去重键 | 严重级别 |
| --- | --- | --- | --- |
| 节点离线 | 节点状态为 offline 超过阈值 | `tenant_id,node_id,node_offline` | warning / critical |
| 同步失败 | `last_sync_error` 非空，或 desired/applied 不一致超过阈值 | `tenant_id,node_id,sync_failed,desired_state_version` | warning |
| 策略失败 | 最近 policy delivery 失败 | `tenant_id,node_id,policy_failed,policy_ref` | warning |
| 命令失败 | 最近 command 失败 | `tenant_id,node_id,command_failed,command_id` | warning |
| 证书即将过期 | 证书剩余有效期低于阈值 | `tenant_id,node_id,certificate_expiring,serial_number` | warning |
| 延迟过高 | 节点 latency 超过阈值并持续 N 分钟 | `tenant_id,node_id,high_latency` | warning |

事件生成原则：

- 同一去重键的 active alert 不重复创建。
- 问题恢复后可以自动 resolve，但要记录恢复依据。
- 手动 resolve 不等于问题已修复。若检测条件仍成立，应允许再次创建 alert。
- 每条 alert 的 `context` 必须包含能复盘的证据，例如版本、错误、命令 ID、策略 ID、指标值。

## 上下文组装

AI 建议必须基于可追踪上下文，不应只根据用户一句话猜。

一次运维建议至少应组装：

| 上下文 | 内容 |
| --- | --- |
| 节点基础信息 | hostname、region、public_ip、assigned_ip、endpoint、status |
| 控制状态 | desired/applied/observed、last_sync_at、last_sync_error |
| 最近命令 | command、status、message、result、created_at |
| 最近策略投递 | policy_domain、policy_ref、action、command_status、last_error |
| 活跃告警 | alert_type、severity、title、message、context |
| 证书信息 | status、serial_number、not_after、revoke_reason |
| 指标 | 在线率、同步成功率、带宽、延迟、失败命令数 |
| 审计历史 | 最近 policy.changed、command.queued、alert_resolved |

建议在 Monitoring 和 Nodes 提供“Ask AI”入口，入口携带：

```json
{
  "tenant_id": "tenant ID",
  "node_id": "node ID",
  "alert_id": "alert ID",
  "focus": "sync_failed",
  "time_range": "1h"
}
```

AI 服务收到后，先调用只读工具获取上下文，再生成建议。

## AI 建议契约

AI 输出应分为两类：

1. 诊断解释：只读，不需要确认。
2. 待执行动作：写操作，必须用户确认。

建议 AI 返回结构化建议：

```json
{
  "summary": "节点 node-a 最近同步失败，原因是 ACL 快照应用失败。",
  "evidence": [
    "desired_state_version=ds-100",
    "applied_state_version=ds-099",
    "last_sync_error=invalid CIDR"
  ],
  "recommendations": [
    {
      "title": "检查 ACL 规则",
      "risk": "low",
      "action": "open_policy",
      "requires_confirmation": false
    },
    {
      "title": "重新触发 Agent sync",
      "risk": "low",
      "tool_name": "run_agent_command",
      "params": {
        "node_id": "node ID",
        "command": "sync"
      },
      "requires_confirmation": true
    }
  ]
}
```

工具分类建议：

| 类型 | 示例 | 是否需要确认 |
| --- | --- | --- |
| 只读查询 | list nodes、get node detail、get monitor stats、diagnose connectivity | 否 |
| 低风险执行 | run `sync`、run `health_check` | 是 |
| 配置变更 | add/remove route、enable/disable policy、renew certificate | 是 |
| 高风险操作 | delete node、revoke certificate、批量修改策略 | 是，并要求更高权限或二次确认 |

`confirmed=false` 时，AI 写工具不得真实执行。它只能返回待确认计划。

## 人工确认

确认动作可以先从控制台实现，后续扩展到 IM 卡片。

确认请求建议包含：

| 字段 | 含义 |
| --- | --- |
| `session_id` | AI 会话 ID |
| `alert_id` | 可选，关联当前告警 |
| `tool_name` | 待执行工具 |
| `params` | 工具参数 |
| `confirmed` | 必须为 true |
| `approval_actor` | 确认人 |
| `approval_reason` | 可选确认原因 |
| `expires_at` | 建议过期时间 |

Controller 收到确认后必须重新校验：

1. 当前用户仍然有租户权限。
2. 当前用户仍然有工具所需权限。
3. 目标节点、策略或告警仍然存在。
4. ActionPlan 没有过期。
5. 当前状态与 AI 建议时没有发生关键变化。

如果状态已经变化，例如告警已恢复、节点已删除、desired version 已更新，应拒绝执行或要求用户重新生成建议。

## 执行和结果回写

确认后，Controller 应把动作落到现有控制闭环中：

- 触发 `sync` / `health_check`：创建 `agent_commands`，并写 `audit_events`。
- 修改策略：走策略写接口，创建 desired state、`agent_commands`、`policy_deliveries` 和 audit。
- 解决告警：调用 alert resolve，并写入解决说明和关联执行结果。

执行结果应回写到以下位置：

| 位置 | 内容 |
| --- | --- |
| AI 面板 | 工具执行结果、命令 ID、下一步提示 |
| Monitoring alert | 是否 resolved、resolved_at、处理人 |
| Event timeline | AI 建议、人工确认、命令执行、结果 |
| Node detail | 最近命令、最近策略投递、当前收敛状态 |
| Audit | `ai.suggested`、`ai.action_confirmed`、`command.queued`、`alert_resolved` 等事件 |
| IM 卡片 | 执行中、成功、失败和失败原因 |

## 告警解决规则

告警可以通过两种方式解决：

| 方式 | 适用场景 | 要求 |
| --- | --- | --- |
| 自动恢复 | 节点重新上线、desired/applied 收敛、命令失败恢复 | 必须写恢复证据 |
| 人工 resolve | 用户判断无需继续处理 | 必须写操作人和原因 |

自动恢复示例：

- `node_offline`：节点心跳恢复并持续在线超过阈值。
- `sync_failed`：`last_sync_error` 清空且 `desired_state_version=applied_state_version`。
- `policy_failed`：同一 `policy_ref` 的新 delivery 成功。
- `command_failed`：用户重试后新 command 成功。旧失败命令不改写，只解决对应 alert。

人工 resolve 不应删除历史。它只是把 alert 状态改为 `resolved`，事件时间线和审计必须保留。

## 前端工作流

### Monitoring

Monitoring 应从指标页变成运维入口：

1. 展示租户级健康指标。
2. 展示 active alerts。
3. 点击 alert 进入详情。
4. 详情展示节点、控制状态、最近命令、最近投递和错误。
5. 提供 Ask AI 按钮。
6. 展示 AI 建议。
7. 对写操作显示确认弹窗。
8. 执行后刷新 alert、node detail 和 event timeline。

### Nodes

Nodes 详情继续作为单节点运维工作台：

- 显示 active alerts。
- 显示最近命令和策略投递。
- 显示证书状态。
- 支持快速 `sync` 和 `health_check`。
- 可跳转到 Monitoring 并保留 `node_id` 和 focus。
- 可从当前节点上下文打开 AI 建议。

### AI Copilot

AI Copilot 不应只是聊天框。它需要显示：

- 当前上下文来源，例如 alert、node、policy。
- 证据列表。
- 建议列表。
- 风险等级。
- 需要确认的动作按钮。
- 执行结果。
- 关联命令、策略投递和审计记录。

### IM

IM 联动作为第二阶段：

1. Controller 把 active alert 推送到飞书或钉钉。
2. 卡片展示问题、节点、严重级别和 AI 摘要。
3. 用户点击“查看建议”或“确认执行”。
4. Controller 验证用户身份和权限。
5. 执行动作并更新卡片状态。
6. 回写 audit 和 event timeline。

## 异常场景

| 场景 | 处理方式 | 前端 / IM 表现 |
| --- | --- | --- |
| AI 建议过期 | 拒绝确认，要求重新生成建议 | 显示“建议已过期” |
| 用户权限变化 | 重新鉴权失败，不执行 | 显示权限不足 |
| 节点已删除 | 不执行命令或策略变更 | 显示目标不存在 |
| 告警已恢复 | 不执行无意义动作，提示刷新 | 显示告警已恢复 |
| Agent 离线 | 可创建 pending command，但提示无法立即执行 | 显示等待 Agent 上线 |
| 命令超时 | command failed，alert 继续 active | 显示超时和重试入口 |
| AI 建议高风险动作 | 必须二次确认或禁止 | 显示风险原因 |
| IM 确认链接过期 | 不执行，要求回控制台重新确认 | 卡片显示已过期 |
| 批量处置部分失败 | 逐节点记录结果 | 展示成功/失败明细 |
| 重复告警 | 用去重键复用 active alert | 只更新 context 和 last seen 信息 |

## 验收标准

运维闭环完成后，至少应能通过以下验收：

### 2026-06-25 当前收口状态

- Monitoring 已能从 active alert 跳 Node Detail / Policy Center，并能直接下发 `sync` 或 `health_check`。
- Node Detail 已能携带 alert、policy、command 上下文执行 `sync` / `health_check`，并能人工 resolve 当前 alert。
- Monitoring 和 Node Detail 已新增 Ask AI 入口；AI 页面会用告警上下文预填诊断提示，但不会自动执行写操作。
- 本阶段剩余工作是线上制造或复用 `sync_failed / policy_failed / node_offline`，验证“告警 -> Ask AI -> 人工确认命令 -> Agent 回写 -> resolve/失败留痕”。

1. 人为制造 `sync_failed` 后，Monitoring 出现 active alert。
2. 该 alert 的 `context` 包含 `node_id`、`desired_state_version`、`applied_state_version` 和错误原因。
3. 从 Monitoring 点击 Ask AI，AI 能读取节点、控制状态、最近命令和策略投递。
4. AI 能给出只读解释和至少一个待确认动作。
5. 未确认时，写工具不会执行。
6. 用户确认 `sync` 后，Controller 创建 `agent_commands`，并写入确认审计。
7. Agent 执行成功后，Nodes 展示最近命令成功，Monitoring 事件流展示执行结果。
8. 若执行后 desired/applied 收敛，`sync_failed` 告警自动或人工 resolved。
9. 若执行失败，alert 保持 active，并显示新的失败原因。
10. 跨租户用户无法看到或确认其他租户的 alert 和 AI 动作。
11. IM 卡片确认时，过期、权限不足、状态变化都能被拒绝。
12. 回归测试覆盖告警创建、AI 建议、确认执行、失败留痕和 resolve。

## 实施顺序

建议按以下顺序推进：

1. 统一 Alert / Event / Audit 关系
   - 明确哪些问题进入 `alerts`。
   - 明确哪些用户动作进入 `audit_events`。
   - 让 Monitoring event timeline 合并展示两类事件。

2. 补告警生成器
   - 从节点状态、控制状态、命令和策略投递生成 active alert。
   - 加入去重键和恢复判断。

3. 打通 Monitoring -> Node / Command 上下文
   - Alert 详情携带 `tenant_id`、`node_id`、`alert_id`、`focus`。
   - Monitoring 和 Node Detail 展示节点状态、控制状态、最近命令、最近策略投递和失败原因。

4. 规范人工处置动作
   - 第一阶段只提供 `sync`、`health_check`、查看策略投递和 resolve alert 等明确动作。
   - 写操作必须重新鉴权、重新校验当前状态，并写入 audit。
   - Hermes 阶段再恢复 AI ActionPlan，并要求写操作 `requires_confirmation=true`。

5. 实现控制台确认执行
   - 确认时重新鉴权和重新校验状态。
   - 执行动作进入控制闭环。
   - 写入命令确认和后续执行审计。

6. 回写执行结果
   - Monitoring、Nodes 同步展示 command / delivery 结果。
   - Alert resolve 关联执行证据。

7. 接 IM
   - 告警推送卡片。
   - 卡片确认。
   - 卡片状态回写。

8. 补端到端测试
   - `sync_failed -> 用户确认 sync -> Agent 执行 -> alert resolved`
   - `policy_failed -> 用户查看失败原因 -> 修正策略 -> delivery applied`
   - `node_offline -> 用户确认 health_check -> 节点恢复或失败留痕`

## 非目标

- 不在第一阶段实现无人值守自动修网。
- 不在旧 AI Agent 上继续扩展写操作；Hermes Agent 阶段重新设计 AI 建议与确认链路。
- 不实现复杂 ML 异常检测。
- 不要求 IM 成为唯一操作入口，控制台仍是主入口。
- 不把 Monitoring 重做成独立 BI 大屏。

## 相关文档

- [节点接入闭环实施方案](node-onboarding-closure-plan.md)
- [控制闭环实施方案](control-loop-closure-plan.md)
- [v0.1.0 产品蓝图](v0.1.0-product-blueprint.md)
- [API v2 白皮书](api-v2-whitepaper.md)
