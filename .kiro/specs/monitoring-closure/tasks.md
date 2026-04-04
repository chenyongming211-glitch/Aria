# 实施计划：Monitoring 闭环

## 概述

基于 desired / applied / observed 三态模型，实现租户级监控闭环。按依赖关系排序：数据库 migration → 存储层 → 告警生成 → API handlers → 离线检测集成 → 前端重构。后端使用 Go，前端使用 Vue 3 + Composition API。

## 任务

- [x] 1. 数据库 Migration 与基础数据模型
  - [x] 1.1 在 `pkg/controllerstorage/postgres.go` 的 `Migrate()` 方法中添加 `alerts` 和 `audit_events` 两张表的 CREATE TABLE 语句及索引
    - alerts 表：id, tenant_id, node_id, alert_type, severity, title, message, context(JSONB), status, created_at, resolved_at
    - audit_events 表：id, tenant_id, node_id, event_type, actor, summary, detail(JSONB), created_at
    - 添加索引：idx_alerts_tenant_status, idx_alerts_tenant_node_type, idx_alerts_created_at, idx_audit_events_tenant, idx_audit_events_tenant_node, idx_audit_events_type, idx_audit_events_created_at
    - _需求: 3.1, 4.1_

- [x] 2. Alert 存储层实现
  - [x] 2.1 创建 `pkg/controllerstorage/alerts.go`，定义 Alert 结构体、AlertFilter 结构体，实现 CRUD 方法
    - CreateAlert：插入新告警记录
    - ResolveAlert：将告警 status 更新为 resolved，记录 resolved_at
    - GetActiveAlertByNodeAndType：按 tenant_id + node_id + alert_type 查询 active 告警（用于幂等检查）
    - ListAlerts：按 AlertFilter 查询告警列表，支持 status/alert_type/node_id/limit/offset 过滤，返回列表和 total count
    - CountActiveAlerts：统计租户下 active 告警数量
    - _需求: 3.1, 6.1, 6.2_
  - [ ]* 2.2 编写 Alert 存储层属性测试
    - **Property 1: Alert 存储往返一致性**
    - **验证: 需求 3.1**

- [x] 3. AuditEvent 存储层实现
  - [x] 3.1 创建 `pkg/controllerstorage/audit_events.go`，定义 AuditEvent 结构体、AuditEventFilter 结构体，实现 CRUD 方法
    - CreateAuditEvent：插入新审计事件
    - ListAuditEvents：按 AuditEventFilter 查询审计事件列表，支持 node_id/event_type/since/limit/offset 过滤，返回列表和 total count
    - _需求: 4.1_
  - [ ]* 3.2 编写 AuditEvent 存储层属性测试
    - **Property 1: AuditEvent 存储往返一致性**
    - **验证: 需求 4.1**

- [x] 4. 告警生成逻辑实现
  - [x] 4.1 创建 `pkg/controllerstorage/alert_generator.go`，实现告警生成与自动解除方法
    - GenerateNodeOfflineAlert：检查是否已存在 active 的 node_offline 告警（幂等），若无则创建 severity=critical 的告警，并创建 alert_created 审计事件
    - ResolveNodeOfflineAlert：查找并解除 active 的 node_offline 告警，创建 alert_resolved 审计事件
    - GenerateSyncFailedAlert：创建 severity=warning 的 sync_failed 告警，context 包含 command_id 和 error message，创建 alert_created 审计事件
    - GeneratePolicyFailedAlert：创建 severity=warning 的 policy_failed 告警，context 包含 policy_domain、policy_ref、last_error，创建 alert_created 审计事件
    - 所有告警生成失败时仅记录日志，不阻塞主流程
    - _需求: 3.2, 3.3, 3.4, 3.5, 4.4_
  - [ ]* 4.2 编写告警幂等性属性测试
    - **Property 7: 告警生成幂等性**
    - **验证: 需求 9.4**

- [x] 5. 检查点 - 确保存储层编译通过
  - 确保所有测试通过，如有问题请向用户确认。

- [x] 6. AgentCommand / PolicyDelivery 状态变更集成告警与审计
  - [x] 6.1 修改 `pkg/controllerstorage/agent_commands.go` 中的 `UpdateAgentCommandStatus` 方法
    - 在事务中，当 status 变为 failed 时调用 GenerateSyncFailedAlert
    - 当 status 变为 completed 时创建 command_completed 审计事件
    - 当 status 变为 failed 时创建 command_failed 审计事件
    - 当 PolicyDelivery 级联更新为 failed 时调用 GeneratePolicyFailedAlert
    - 当 PolicyDelivery 级联更新为 completed 时创建 policy_delivered 审计事件
    - 当 PolicyDelivery 级联更新为 failed 时创建 policy_failed 审计事件
    - 告警/审计写入失败不阻塞主流程，记录 error 日志
    - _需求: 3.3, 3.4, 4.2, 4.3_
  - [ ]* 6.2 编写状态变更审计事件生成属性测试
    - **Property 5: 失败操作告警生成**
    - **Property 8: 状态变更审计事件生成**
    - **验证: 需求 3.3, 3.4, 4.2, 4.3**

- [x] 7. 离线检测与告警集成
  - [x] 7.1 修改 `internal/cli/controller_serve.go` 中的 `cleanupStaleNodes` 方法，集成离线检测告警逻辑
    - 在 MarkOfflineNodes 之后，查询新标记为 offline 的节点，对每个节点调用 GenerateNodeOfflineAlert
    - 查询从 offline 恢复为 online 的节点（last_seen 恢复到 60 秒以内且 status 为 offline），调用 ResolveNodeOfflineAlert
    - 为节点在线状态变化创建 node_offline / node_online 审计事件
    - 错误仅记录日志，不中断定时器
    - _需求: 9.1, 9.2, 9.3, 9.4, 3.2, 3.5, 4.5_
  - [ ]* 7.2 编写离线检测属性测试
    - **Property 3: 节点在线/离线分类正确性**
    - **Property 4: 离线告警生成**
    - **Property 6: 节点恢复在线时告警自动解除**
    - **验证: 需求 1.3, 3.2, 3.5, 9.2, 9.3**

- [x] 8. 检查点 - 确保后端核心逻辑编译通过
  - 确保所有测试通过，如有问题请向用户确认。

- [x] 9. Monitoring API Handlers 实现
  - [x] 9.1 创建 `internal/api/v2/monitoring.go`，实现 stats 端点
    - 重构 `handleTenantMonitoringStats`：查询 total_nodes、online_nodes、offline_nodes（基于 last_seen 60 秒阈值）、sync_success_rate（基于 NodeControlState）、total_peers、total_acl_rules（查 acl_rules 表）、total_qos_rules（查 qos_rules 表）、failed_commands_count（查 agent_commands 表 status=failed）、active_alerts_count（查 alerts 表）
    - 在 `pkg/controllerstorage/` 中添加必要的聚合查询方法：CountACLRulesByTenant、CountQoSRulesByTenant、CountFailedCommandsByTenant
    - 租户无节点时返回所有计数为 0、sync_success_rate 为 100
    - _需求: 1.1, 1.2, 1.3, 1.4_
  - [ ]* 9.2 编写 sync_success_rate 计算属性测试
    - **Property 2: sync_success_rate 计算正确性**
    - **验证: 需求 1.2, 1.4**
  - [ ]* 9.3 编写统计响应字段完整性属性测试
    - **Property 16: 统计响应字段完整性**
    - **验证: 需求 1.1**
  - [x] 9.4 实现 node detail 端点
    - 重构 `handleTenantMonitoringNodeDetail`：返回 availability_status、desired_state_version、applied_state_version、observed_state、observed_message、state_convergence、last_sync_at、last_sync_error
    - 同时返回最近 20 条 AgentCommand 和最近 20 条 PolicyDelivery，按 created_at 倒序
    - node_id 不属于租户时返回 404 NODE_NOT_FOUND
    - 无 NodeControlState 记录时返回 state_convergence="idle"、版本为空字符串
    - _需求: 2.1, 2.2, 2.3, 2.4_
  - [ ]* 9.5 编写节点详情响应完整性属性测试
    - **Property 9: 节点详情响应完整性**
    - **Property 10: 历史记录数量限制**
    - **Property 11: 租户隔离**
    - **验证: 需求 2.1, 2.2, 2.3**
  - [x] 9.6 实现 events 端点（事件流 API）
    - 新增 `handleTenantMonitoringEvents`：UNION 查询 alerts 和 audit_events，按 created_at 倒序合并
    - 支持查询参数：limit（默认50，最大200）、offset（默认0）、node_id、event_type、severity、since
    - 返回统一格式 EventFeedItem：id, source(alert/audit), event_type, severity, node_id, title, detail, created_at
    - 返回 total count 用于分页
    - _需求: 5.1, 5.2, 5.3, 5.4_
  - [ ]* 9.7 编写事件流排序与过滤属性测试
    - **Property 12: 事件流时间排序**
    - **Property 13: 事件流过滤正确性**
    - **验证: 需求 5.1, 5.2**
  - [x] 9.8 实现 alerts 端点（告警列表与手动解除 API）
    - 新增 `handleTenantMonitoringAlerts`：GET 返回告警列表，默认 status=active，支持 status/alert_type/node_id/limit/offset 过滤
    - 新增 `handleTenantMonitoringAlertResolve`：POST resolve 端点，更新告警状态并创建 alert_resolved 审计事件
    - 告警不存在返回 404 ALERT_NOT_FOUND，已解除返回 400 ALERT_ALREADY_RESOLVED
    - _需求: 6.1, 6.2, 6.3_
  - [ ]* 9.9 编写告警列表过滤与手动解除属性测试
    - **Property 14: 告警列表默认过滤与过滤正确性**
    - **Property 15: 告警手动解除**
    - **验证: 需求 6.1, 6.2, 6.3**

- [x] 10. 路由注册
  - [x] 10.1 修改 `internal/api/v2/setup.go` 中的 `handleTenantMonitoring` 方法，注册 events、alerts、alerts/{id}/resolve 路由
    - 扩展路由匹配逻辑，将新端点路由到 monitoring.go 中的 handler
    - _需求: 5.1, 6.1, 6.3_

- [x] 11. 检查点 - 确保后端 API 编译通过
  - 确保所有测试通过，如有问题请向用户确认。

- [x] 12. 前端 API 层扩展
  - [x] 12.1 修改 `frontend-refactor/src/config/api.js`，在 MONITOR 对象中添加新端点
    - EVENTS: (tenantId) => buildTenantPath(tenantId, '/monitoring/events')
    - ALERTS: (tenantId) => buildTenantPath(tenantId, '/monitoring/alerts')
    - ALERT_RESOLVE: (tenantId, alertId) => buildTenantPath(tenantId, `/monitoring/alerts/${alertId}/resolve`)
    - _需求: 5.1, 6.1, 6.3_
  - [x] 12.2 修改 `frontend-refactor/src/composables/useMonitorApi.js`，添加新 API 方法
    - getEvents(params)：GET /monitoring/events，支持 limit/offset/node_id/event_type/severity/since 参数
    - getAlerts(params)：GET /monitoring/alerts，支持 status/alert_type/node_id/limit/offset 参数
    - resolveAlert(alertId)：POST /monitoring/alerts/{alertId}/resolve
    - getNodeMonitorDetail(nodeId)：已有，确认接口兼容
    - _需求: 7.7_

- [x] 13. 前端 Monitoring.vue 重构
  - [x] 13.1 重构 `frontend-refactor/src/views/Monitoring.vue`，移除所有 mock 数据，接入真实 API
    - 页面顶部统计卡片区域：在线节点数/总节点数、同步成功率、Peer 数、ACL 规则数、QoS 规则数、命令失败数、活跃告警数，数据来源 stats API
    - 统计卡片下方展示事件流时间线：按时间倒序展示 Alert 和 AuditEvent 混合流，数据来源 events API
    - 每条事件展示：时间戳、事件类型图标/标签、severity 标签（仅 Alert）、事件摘要文本、关联节点名称（可点击跳转节点详情）
    - Alert 类型事件提供"标记已解决"按钮，调用 resolve API
    - 支持按事件类型和告警级别筛选
    - 页面加载时自动请求 stats 和 events API，支持手动刷新
    - _需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7_

- [x] 14. 前端节点监控详情面板
  - [x] 14.1 创建 `frontend-refactor/src/views/NodeMonitorDetail.vue`，实现节点监控详情面板
    - 展示节点 desired / applied / observed 三态信息和 state_convergence 状态
    - 展示最近 AgentCommand 和 PolicyDelivery 历史记录，按时间倒序
    - desired_state_version 与 applied_state_version 不一致时以醒目样式标示 diverged/pending
    - 存在 last_sync_error 时展示错误信息
    - 数据来源 GET /monitoring/nodes/{nodeId} API
    - _需求: 8.1, 8.2, 8.3, 8.4_

- [-] 15. 最终检查点 - 全面验证
  - 确保所有测试通过，如有问题请向用户确认。

## 备注

- 标记 `*` 的任务为可选任务，可跳过以加速 MVP 交付
- 每个任务引用了具体的需求编号以确保可追溯性
- 检查点确保增量验证
- 属性测试验证通用正确性属性，单元测试验证具体示例和边界条件
