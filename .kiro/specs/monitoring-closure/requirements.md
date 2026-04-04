# 需求文档：Monitoring 闭环

## 简介

v0.1.0 产品蓝图第七步（最后一步）：实现租户级监控闭环。将现有 Monitoring 页面从 mock 数据驱动的统计卡片模式，重构为基于 desired / applied / observed 三态模型的事件流/审计式监控视图。优先覆盖控制闭环相关指标（节点在线、同步成功率、策略下发状态），并实现节点离线、同步失败、策略失败三类基础告警。

本需求不追求大而全的监控平台，聚焦于"发现异常 → 展示上下文 → 支撑运维决策"的最小闭环。

## 术语表

- **Controller**：Aria SD-WAN 控制面服务，Go 实现，提供 REST API 和 gRPC 南向接口
- **Agent**：运行在边缘节点上的 Rust 代理程序，通过 gRPC 与 Controller 通信
- **Monitoring_API**：Controller 中 `/api/v2/tenants/{tenant_id}/monitoring/` 命名空间下的 REST 端点集合
- **Monitoring_View**：前端 Vue 3 监控页面组件
- **NodeControlState**：节点控制状态模型，包含 desired_state_version、applied_state_version、observed_state 三态字段
- **PolicyDelivery**：策略下发记录，跟踪单条策略从下发到完成/失败的全生命周期
- **AgentCommand**：Agent 命令队列记录，跟踪命令从 pending → sent → acknowledged → completed/failed 的状态流转
- **Alert**：系统生成的告警记录，描述一个需要关注的异常事件（节点离线、同步失败、策略失败）
- **AuditEvent**：审计事件记录，描述系统中发生的一个可追溯操作或状态变更
- **Event_Feed**：按时间倒序排列的 AuditEvent 和 Alert 混合流，用于事件流式展示
- **State_Convergence**：desired 与 applied 版本是否一致的收敛状态，取值为 converged / pending / diverged / idle

## 需求

### 需求 1：租户级监控统计 API

**用户故事：** 作为租户管理员，我希望获取当前租户下所有节点的控制闭环统计摘要，以便快速了解整体网络健康状况。

#### 验收标准

1. WHEN 租户管理员请求 `GET /api/v2/tenants/{tenant_id}/monitoring/stats` 时，THE Monitoring_API SHALL 返回包含以下字段的 JSON 响应：total_nodes（总节点数）、online_nodes（在线节点数）、offline_nodes（离线节点数）、sync_success_rate（最近同步成功率，百分比）、total_peers（Peer 总数）、total_acl_rules（ACL 规则总数）、total_qos_rules（QoS 规则总数）、failed_commands_count（命令执行失败数）、active_alerts_count（活跃告警数）
2. WHEN 计算 sync_success_rate 时，THE Monitoring_API SHALL 基于 NodeControlState 表中 desired_state_version 与 applied_state_version 的匹配情况计算：sync_success_rate = (desired == applied 的节点数) / (desired 非空的节点数) × 100
3. WHEN 判断节点在线状态时，THE Monitoring_API SHALL 使用 nodes 表的 last_seen 字段，将 last_seen 距当前时间超过 60 秒的节点判定为离线
4. IF 租户下无任何节点，THEN THE Monitoring_API SHALL 返回所有计数字段为 0、sync_success_rate 为 100 的响应

### 需求 2：节点控制状态详情 API

**用户故事：** 作为租户管理员，我希望查看单个节点的 desired / applied / observed 三态详情及最近操作历史，以便诊断节点同步问题。

#### 验收标准

1. WHEN 租户管理员请求 `GET /api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}` 时，THE Monitoring_API SHALL 返回该节点的完整控制状态，包含：availability_status（online/offline）、desired_state_version、applied_state_version、observed_state、observed_message、state_convergence（converged/pending/diverged/idle）、last_sync_at、last_sync_error
2. WHEN 租户管理员请求节点详情时，THE Monitoring_API SHALL 同时返回该节点最近 20 条 AgentCommand 记录和最近 20 条 PolicyDelivery 记录，按 created_at 倒序排列
3. IF 请求的 node_id 不属于该租户，THEN THE Monitoring_API SHALL 返回 HTTP 404 和错误码 NODE_NOT_FOUND
4. IF 该节点尚无 NodeControlState 记录，THEN THE Monitoring_API SHALL 返回 state_convergence 为 "idle"、desired_state_version 和 applied_state_version 为空字符串的默认响应

### 需求 3：告警模型与生成

**用户故事：** 作为租户管理员，我希望系统自动检测节点离线、同步失败、策略失败三类异常并生成告警，以便及时发现问题。

#### 验收标准

1. THE Controller SHALL 在数据库中维护 alerts 表，每条 Alert 包含：id、tenant_id、node_id、alert_type（node_offline / sync_failed / policy_failed）、severity（critical / warning / info）、title、message、context（JSONB，存储相关上下文如 command_id、policy_ref 等）、status（active / resolved）、created_at、resolved_at
2. WHEN 节点的 last_seen 距当前时间超过 60 秒且该节点当前无 active 状态的 node_offline 告警时，THE Controller SHALL 创建一条 severity 为 critical 的 node_offline 告警
3. WHEN AgentCommand 状态变更为 failed 时，THE Controller SHALL 创建一条 severity 为 warning 的 sync_failed 告警，context 中包含 command_id 和 error message
4. WHEN PolicyDelivery 的 command_status 变更为 failed 时，THE Controller SHALL 创建一条 severity 为 warning 的 policy_failed 告警，context 中包含 policy_domain、policy_ref 和 last_error
5. WHEN 节点重新上线（last_seen 恢复到 60 秒以内）且存在该节点的 active node_offline 告警时，THE Controller SHALL 将该告警 status 更新为 resolved 并记录 resolved_at

### 需求 4：审计事件模型与记录

**用户故事：** 作为租户管理员，我希望系统记录所有关键操作和状态变更为审计事件，以便追溯问题根因和操作历史。

#### 验收标准

1. THE Controller SHALL 在数据库中维护 audit_events 表，每条 AuditEvent 包含：id、tenant_id、node_id（可为空）、event_type（node_registered / node_offline / node_online / command_queued / command_completed / command_failed / policy_delivered / policy_failed / alert_created / alert_resolved）、actor（system / user:{username}）、summary（人类可读的事件摘要）、detail（JSONB，存储事件详细数据）、created_at
2. WHEN AgentCommand 状态变更为 completed 或 failed 时，THE Controller SHALL 创建对应的 command_completed 或 command_failed 审计事件
3. WHEN PolicyDelivery 的 command_status 变更为 completed 或 failed 时，THE Controller SHALL 创建对应的 policy_delivered 或 policy_failed 审计事件
4. WHEN Alert 被创建或 resolved 时，THE Controller SHALL 创建对应的 alert_created 或 alert_resolved 审计事件
5. WHEN 节点在线状态发生变化（online → offline 或 offline → online）时，THE Controller SHALL 创建对应的 node_offline 或 node_online 审计事件

### 需求 5：事件流 API

**用户故事：** 作为租户管理员，我希望获取按时间排序的事件流（包含告警和审计事件），以便在统一视图中了解系统发生了什么。

#### 验收标准

1. WHEN 租户管理员请求 `GET /api/v2/tenants/{tenant_id}/monitoring/events` 时，THE Monitoring_API SHALL 返回该租户下 AuditEvent 和 Alert 按 created_at 倒序合并的事件流列表
2. THE Monitoring_API SHALL 支持以下查询参数：limit（默认 50，最大 200）、offset（默认 0）、node_id（可选，按节点过滤）、event_type（可选，按事件类型过滤）、severity（可选，按告警级别过滤，仅对 Alert 类型生效）、since（可选，ISO 8601 时间戳，仅返回该时间之后的事件）
3. THE Monitoring_API SHALL 对每条事件返回统一格式：id、source（alert / audit）、event_type、severity（Alert 有值，AuditEvent 为空字符串）、node_id、title/summary、detail/context、created_at
4. IF 未提供任何过滤参数，THEN THE Monitoring_API SHALL 返回最近 50 条事件

### 需求 6：告警列表 API

**用户故事：** 作为租户管理员，我希望单独查看和管理告警列表，以便专注处理需要关注的异常。

#### 验收标准

1. WHEN 租户管理员请求 `GET /api/v2/tenants/{tenant_id}/monitoring/alerts` 时，THE Monitoring_API SHALL 返回该租户下的告警列表，默认仅返回 status 为 active 的告警，按 created_at 倒序排列
2. THE Monitoring_API SHALL 支持查询参数：status（active / resolved / all，默认 active）、alert_type（可选，按告警类型过滤）、node_id（可选，按节点过滤）、limit（默认 50，最大 200）、offset（默认 0）
3. WHEN 租户管理员请求 `POST /api/v2/tenants/{tenant_id}/monitoring/alerts/{alert_id}/resolve` 时，THE Monitoring_API SHALL 将该告警 status 更新为 resolved，记录 resolved_at，并创建 alert_resolved 审计事件

### 需求 7：前端监控视图重构

**用户故事：** 作为租户管理员，我希望在前端看到基于真实数据的监控视图，包含控制闭环统计卡片和事件流时间线，以替代当前的 mock 数据展示。

#### 验收标准

1. THE Monitoring_View SHALL 在页面顶部展示统计卡片区域，包含：在线节点数/总节点数、同步成功率、Peer 数、ACL 规则数、QoS 规则数、命令失败数、活跃告警数，数据来源为需求 1 的 stats API
2. THE Monitoring_View SHALL 在统计卡片下方展示事件流时间线，按时间倒序展示 Alert 和 AuditEvent 混合流，数据来源为需求 5 的 events API
3. THE Monitoring_View SHALL 对事件流中的每条事件展示：时间戳、事件类型图标/标签、severity 标签（仅 Alert）、事件摘要文本、关联节点名称（可点击跳转到节点详情）
4. THE Monitoring_View SHALL 对 Alert 类型事件提供"标记已解决"操作按钮，点击后调用需求 6 的 resolve API
5. THE Monitoring_View SHALL 支持按事件类型和告警级别进行筛选
6. THE Monitoring_View SHALL 移除所有 mock 数据，所有展示数据来源于后端 API 调用
7. WHEN 页面加载时，THE Monitoring_View SHALL 自动请求 stats API 和 events API，并支持手动刷新

### 需求 8：节点详情监控面板

**用户故事：** 作为租户管理员，我希望在节点详情页看到该节点的三态控制状态和操作历史，以便诊断单节点问题。

#### 验收标准

1. THE Monitoring_View SHALL 提供节点监控详情面板（可从事件流中的节点链接或节点列表进入），展示该节点的 desired / applied / observed 三态信息和 state_convergence 状态
2. THE Monitoring_View SHALL 在节点详情面板中展示该节点最近的 AgentCommand 和 PolicyDelivery 历史记录，按时间倒序排列
3. WHEN desired_state_version 与 applied_state_version 不一致时，THE Monitoring_View SHALL 以醒目样式（如红色/橙色标签）标示 state_convergence 为 diverged 或 pending
4. IF 节点存在 last_sync_error，THEN THE Monitoring_View SHALL 在面板中展示错误信息

### 需求 9：节点离线检测

**用户故事：** 作为系统运维人员，我希望 Controller 能定期检测节点离线状态并触发告警，以便及时发现网络中断。

#### 验收标准

1. THE Controller SHALL 提供一个定期执行的离线检测任务（检测间隔可配置，默认 30 秒），扫描所有节点的 last_seen 字段
2. WHEN 检测到节点 last_seen 距当前时间超过 60 秒时，THE Controller SHALL 将该节点 status 更新为 offline，记录 offline_since 时间戳，并触发需求 3 中的 node_offline 告警生成逻辑
3. WHEN 检测到节点 last_seen 恢复到 60 秒以内且当前 status 为 offline 时，THE Controller SHALL 将该节点 status 更新为 online，清除 offline_since，并触发需求 3 中的告警自动解除逻辑
4. THE Controller SHALL 确保同一节点的同一类型告警不会重复创建（即节点持续离线期间只生成一条 active 的 node_offline 告警）
