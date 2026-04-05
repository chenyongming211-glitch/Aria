# 实现计划：Dashboard 真实数据接入

## 概述

按依赖关系从后端到前端逐步实现：先创建 VictoriaMetrics 查询客户端，再新增 4 个 API handler，最后改造前端 3 个页面。每个任务构建在前一个任务之上，确保无孤立代码。

## 任务

- [x] 1. 创建 VictoriaMetrics 查询客户端
  - [x] 1.1 创建 `pkg/victoriametrics/types.go`，定义 QueryRangeResult、QueryInstantResult 等响应类型
    - 包含 QueryRangeData、RangeResultItem、QueryInstantData、InstantResultItem
    - _需求: 6.3, 6.4_
  - [x] 1.2 创建 `pkg/victoriametrics/client.go`，实现 VM 查询客户端
    - NewClient(baseURL string) 构造函数
    - QueryRange(ctx, query, start, end, step) 方法，调用 `/api/v1/query_range`
    - QueryInstant(ctx, query) 方法，调用 `/api/v1/query`
    - 使用标准库 net/http，查询超时 10 秒通过 context.WithTimeout 控制
    - VM 不可用时返回空数据集并记录日志
    - _需求: 6.1, 6.2, 6.3, 6.4, 6.5_
  - [ ]* 1.3 编写 VM Client 单元测试 `pkg/victoriametrics/client_test.go`
    - 使用 httptest.Server 模拟 VM 响应
    - 测试 QueryRange 正常路径和超时路径
    - 测试 QueryInstant 正常路径和错误路径
    - _需求: 6.5_
  - [ ]* 1.4 编写属性测试：Prometheus 响应解析正确性
    - **Property 9: Prometheus 响应解析正确性**
    - **验证需求: 6.3, 6.4**

- [x] 2. 将 VM Client 注入 Router 并扩展路由
  - [x] 2.1 修改 `internal/api/v2/setup.go`，在 Router 结构体中添加 `vmClient` 字段
    - 修改 SetupRoutes 函数签名，接受可选的 VM Client 参数
    - _需求: 6.1_
  - [x] 2.2 修改 `internal/cli/controller_serve.go`，初始化 VM Client 并传递给 Router
    - 从 `metrics.push_gateway` 配置推导 VM 查询地址（截取到 host:port）
    - 未配置时默认使用 `http://localhost:8428`
    - _需求: 6.2_
  - [x] 2.3 修改 `internal/api/v2/operations.go` 中的 `handleTenantMonitoring`，新增 4 个路由分发
    - GET `/monitoring/traffic` → handleMonitoringTraffic
    - GET `/monitoring/health` → handleMonitoringHealth
    - GET `/monitoring/nodes/{node_id}/metrics` → handleMonitoringNodeMetrics
    - GET `/monitoring/topology` → handleMonitoringTopology
    - _需求: 1.7, 2.6, 4.6, 5.11_

- [x] 3. 实现 Traffic API handler
  - [x] 3.1 在 `internal/api/v2/monitoring.go` 中实现 `handleMonitoringTraffic`
    - 解析 `range` 查询参数（1h/24h/7d/30d），无效值返回 400
    - 获取租户下所有节点的 public_key 列表
    - 构建 PromQL 查询 `wireguard_total_rx_bytes` 和 `wireguard_total_tx_bytes`，按节点 instance 过滤
    - 调用 VMClient.QueryRange，按 range 选择对应 step（1h→60s, 24h→300s, 7d→1800s, 30d→7200s）
    - 计算 peak_bandwidth_mbps（时序最大值）
    - VM 不可用时返回空数据集（200 状态码）
    - 返回 { timestamps, upload_bytes, download_bytes, peak_bandwidth_mbps }
    - _需求: 1.1, 1.2, 1.4, 1.6, 1.7_
  - [ ]* 3.2 编写属性测试：时间范围参数验证
    - **Property 2: 时间范围参数验证**
    - **验证需求: 1.2**
  - [ ]* 3.3 编写属性测试：PromQL 查询构建正确性
    - **Property 3: PromQL 查询构建正确性**
    - **验证需求: 1.4**
  - [ ]* 3.4 编写属性测试：峰值带宽为时序最大值
    - **Property 6: 峰值带宽为时序最大值**
    - **验证需求: 3.3**

- [x] 4. 实现 Health API handler
  - [x] 4.1 在 `internal/api/v2/monitoring.go` 中实现 `handleMonitoringHealth`
    - 调用已有的 CountNodesByTenantAndStatus 计算 node_online_rate（total 为 0 时返回 100）
    - 调用已有的 CalcSyncSuccessRate 获取 sync_success_rate
    - 调用已有的 CountActiveAlerts 获取 active_alerts_count
    - 调用已有的 CountFailedCommandsByTenant 获取 failed_commands_count
    - 返回 { node_online_rate, sync_success_rate, active_alerts_count, failed_commands_count }
    - _需求: 2.1, 2.3, 2.6_
  - [ ]* 4.2 编写属性测试：健康指标计算正确性
    - **Property 4: 健康指标计算正确性**
    - **验证需求: 2.1, 2.3**

- [x] 5. 实现 Node Metrics API 和 Topology API handler
  - [x] 5.1 在 `internal/api/v2/monitoring.go` 中实现 `handleMonitoringNodeMetrics`
    - 验证 node_id 属于该租户，不属于返回 404
    - 获取节点 public_key 前 16 字符作为 peer_id
    - 查询 `wireguard_peer_rx_bytes` 和 `wireguard_peer_tx_bytes`，使用 rate() 计算 5 分钟速率
    - 转换为 Mbps（rate * 8 / 1_000_000）
    - 查询 `wireguard_peer_last_handshake_secs` 作为延迟参考
    - VM 不可用时返回零值
    - 返回 { upload_mbps, download_mbps, latency_ms }
    - _需求: 4.1, 4.2, 4.3, 4.6_
  - [x] 5.2 在 `internal/api/v2/monitoring.go` 中实现 `handleMonitoringTopology`
    - 获取租户下所有非 deleted 节点
    - 构建全连接 peer 关系（N*(N-1)/2 条 link）
    - 连接状态：两端都 online → "active"，否则 → "inactive"
    - 返回 { nodes: [{id, hostname, region, status, assigned_ip}], links: [{source, target, status}] }
    - _需求: 5.1, 5.2, 5.11_
  - [ ]* 5.3 编写属性测试：字节速率到 Mbps 转换正确性
    - **Property 7: 字节速率到 Mbps 转换正确性**
    - **验证需求: 4.2**
  - [ ]* 5.4 编写属性测试：拓扑图连接数量正确性
    - **Property 8: 拓扑图连接数量正确性**
    - **验证需求: 5.1, 5.2**
  - [ ]* 5.5 编写属性测试：租户隔离
    - **Property 10: 租户隔离——查询仅包含本租户节点**
    - **验证需求: 6.6**

- [x] 6. 检查点 - 后端完成验证
  - 确保所有测试通过，如有疑问请询问用户。

- [x] 7. 前端 API 层扩展
  - [x] 7.1 修改 `frontend/src/config/api.js`，在 MONITOR 对象中新增 4 个端点
    - TRAFFIC: (tenantId) => buildTenantPath(tenantId, '/monitoring/traffic')
    - HEALTH: (tenantId) => buildTenantPath(tenantId, '/monitoring/health')
    - NODE_METRICS: (tenantId, nodeId) => buildTenantPath(tenantId, `/monitoring/nodes/${nodeId}/metrics`)
    - TOPOLOGY: (tenantId) => buildTenantPath(tenantId, '/monitoring/topology')
    - _需求: 1.7, 2.6, 4.6, 5.11_
  - [x] 7.2 修改 `frontend/src/composables/useMonitorApi.js`，新增 4 个方法
    - getTraffic(range = '24h')：调用 TRAFFIC 端点，传递 range 查询参数
    - getHealth()：调用 HEALTH 端点（覆盖已有的指向 /health 的 getHealth）
    - getNodeMetrics(nodeId)：调用 NODE_METRICS 端点
    - getTopology()：调用 TOPOLOGY 端点
    - _需求: 1.1, 2.1, 4.1, 5.1_

- [x] 8. Dashboard.vue 接入真实数据
  - [x] 8.1 改造 `frontend/src/views/Dashboard.vue` 统计卡片和流量图表
    - 统计卡片：从节点列表 API 获取 total/online/routes，从 Traffic API 获取 peak_bandwidth_mbps
    - 移除硬编码的趋势百分比（"+12%"、"+5%"、"-2%"、"+8%"），无历史数据时隐藏趋势标签
    - 流量图表：调用 getTraffic(timeRange) 替换 generateChartData()，按 timestamps/upload_bytes/download_bytes 渲染
    - 监听 timeRange 变化时重新请求 Traffic API
    - API 失败时统计卡片显示 "N/A"，流量图表显示错误提示并保留上次数据
    - _需求: 1.1, 1.2, 1.3, 1.5, 3.1, 3.2, 3.3, 3.4, 3.5_
  - [x] 8.2 改造 `frontend/src/views/Dashboard.vue` 健康指标和活动列表
    - System Health 卡片：调用 getHealth() 替换硬编码的 CPU/Memory/Disk/Latency
    - 渲染 node_online_rate、sync_success_rate、active_alerts_count、failed_commands_count 到进度条
    - 60 秒定时器自动刷新健康指标
    - Health API 失败时显示"数据不可用"
    - 活动列表：调用已有的 getEvents() 替换硬编码的 activities 数组
    - 区域分布：从节点列表按 region 聚合计算，替换硬编码的 regions 数组
    - _需求: 2.1, 2.2, 2.4, 2.5, 3.1_

- [x] 9. Nodes.vue 详情弹窗接入真实带宽/延迟
  - [x] 9.1 修改 `frontend/src/views/Nodes.vue` 和 `frontend/src/stores/node.js`
    - 在 viewNodeDetails 或 loadNodeDetail 中额外调用 getNodeMetrics(nodeId)
    - 将返回的 upload_mbps、download_mbps、latency_ms 填充到 selectedNode 的 bandwidth 和 latency 字段
    - API 失败时显示 "N/A" 而非 0
    - _需求: 4.1, 4.4, 4.5_

- [x] 10. VpnTopology.vue 完整实现
  - [x] 10.1 重写 `frontend/src/views/VpnTopology.vue`，替换当前的 el-empty 占位符
    - 调用 getTopology() 获取节点和连接数据
    - 使用 ECharts graph 类型渲染力导向拓扑图
    - 节点颜色：online → 绿色（#22C55E），offline → 红色（#EF4444）
    - 连接线：active → 绿色实线，inactive → 灰色虚线
    - 节点 tooltip：显示 hostname、region、assigned_ip、status
    - 连接 tooltip：显示源节点、目标节点、连接状态
    - Refresh 按钮重新获取数据并更新图表
    - 空状态：无节点时显示"暂无节点数据"
    - API 失败时显示错误提示信息
    - _需求: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 5.9, 5.10_

- [x] 11. 最终检查点 - 全部完成验证
  - 确保所有测试通过，如有疑问请询问用户。

## 备注

- 标记 `*` 的任务为可选，可跳过以加速 MVP 交付
- 每个任务引用了具体的需求编号以确保可追溯性
- 检查点确保增量验证
- 属性测试验证通用正确性属性，单元测试验证具体示例和边界条件
