# 需求文档：Dashboard 真实数据接入

## 简介

Aria SD-WAN 管理平台的 Dashboard、Nodes 详情和 VPN Topology 三个核心页面目前使用 mock 数据或空壳占位符。本需求旨在将这三个功能模块接入真实后端 API 数据，包括：Dashboard 流量图表与系统健康指标、节点带宽/延迟统计、以及基于真实节点和 peer 关系的 VPN 拓扑可视化。

## 术语表

- **Controller**：Aria SD-WAN 的中心控制器，Go 语言实现，负责节点管理、策略下发和 API 服务
- **Agent**：部署在各节点上的 Rust 代理程序，负责 WireGuard 隧道管理、eBPF ACL/QoS 执行和 metrics 采集
- **Dashboard**：前端仪表盘页面（Dashboard.vue），展示全局流量图表、统计卡片和系统健康指标
- **Nodes_Detail_Panel**：节点详情弹窗中的网络统计区域，展示单节点的上传/下载带宽和延迟
- **Topology_View**：VPN 拓扑可视化页面（VpnTopology.vue），展示节点间的网络拓扑关系
- **Traffic_API**：Controller 侧新增的流量聚合 API，提供租户级别的历史流量时序数据
- **Health_API**：Controller 侧新增的系统健康 API，提供 Controller 自身和节点聚合的健康指标
- **Node_Metrics_API**：Controller 侧新增的单节点 metrics API，提供从 Agent 上报的带宽和延迟数据
- **Topology_API**：Controller 侧新增的拓扑数据 API，提供节点列表和 peer 连接关系
- **VictoriaMetrics**：时序数据库，用于存储 Agent 上报的 Prometheus metrics
- **ECharts**：前端图表库，已在项目中使用

## 需求

### 需求 1：Dashboard 流量图表接入真实数据

**用户故事：** 作为网络管理员，我希望在 Dashboard 看到真实的网络流量趋势图，以便了解网络的实际使用情况。

#### 验收标准

1. WHEN 用户访问 Dashboard 页面, THE Traffic_API SHALL 返回指定时间范围内的流量时序数据，包含时间戳、上传字节数和下载字节数
2. THE Traffic_API SHALL 支持 1h、24h、7d、30d 四种时间范围查询参数
3. WHEN 用户切换时间范围选择器, THE Dashboard SHALL 重新请求对应时间范围的流量数据并更新 ECharts 图表
4. THE Traffic_API SHALL 从 VictoriaMetrics 查询 `wireguard_total_rx_bytes` 和 `wireguard_total_tx_bytes` 指标，按租户下所有节点聚合
5. IF Traffic_API 返回错误或超时, THEN THE Dashboard SHALL 显示错误提示信息并保留上次成功加载的数据
6. THE Traffic_API 的响应时间 SHALL 在 2 秒以内完成
7. THE Traffic_API SHALL 遵循 `/api/v2/tenants/{tenant_id}/monitoring/traffic` 路径规范，使用统一响应格式

### 需求 2：Dashboard 系统健康指标接入真实数据

**用户故事：** 作为网络管理员，我希望在 Dashboard 看到真实的系统健康状态，以便及时发现和处理系统异常。

#### 验收标准

1. THE Health_API SHALL 返回以下聚合健康指标：节点在线率、同步成功率、活跃告警数量和失败命令数量
2. WHEN 用户访问 Dashboard 页面, THE Dashboard SHALL 从 Health_API 获取健康数据并渲染到 System Health 卡片的进度条中
3. THE Health_API SHALL 从已有的 monitoring/stats 数据和 VictoriaMetrics 中聚合计算健康指标
4. WHILE Dashboard 页面处于活跃状态, THE Dashboard SHALL 每 60 秒自动刷新健康指标数据
5. IF Health_API 返回错误, THEN THE Dashboard SHALL 在 System Health 卡片中显示"数据不可用"状态
6. THE Health_API SHALL 遵循 `/api/v2/tenants/{tenant_id}/monitoring/health` 路径规范，使用统一响应格式

### 需求 3：Dashboard 统计卡片接入真实数据

**用户故事：** 作为网络管理员，我希望 Dashboard 顶部的统计卡片显示真实的趋势变化数据，而非硬编码的百分比。

#### 验收标准

1. THE Dashboard SHALL 从已有的节点列表 API 和 monitoring/stats API 获取统计卡片所需的数据
2. THE Dashboard SHALL 计算并显示真实的节点总数、在线节点数、路由总数
3. THE Dashboard SHALL 从 Traffic_API 获取当前峰值带宽并显示在 Bandwidth 卡片中
4. WHEN 统计数据加载完成, THE Dashboard SHALL 移除硬编码的趋势百分比（如 "+12%"、"+5%"），替换为基于历史数据计算的真实趋势值，或在无历史数据时隐藏趋势标签
5. IF 任一统计 API 请求失败, THEN THE Dashboard SHALL 在对应卡片中显示 "N/A" 而非 0

### 需求 4：节点带宽和延迟统计接入真实数据

**用户故事：** 作为网络管理员，我希望在节点详情弹窗中看到真实的带宽和延迟数据，以便评估节点的网络性能。

#### 验收标准

1. THE Node_Metrics_API SHALL 返回指定节点的上传带宽（Mbps）、下载带宽（Mbps）和延迟（ms）
2. THE Node_Metrics_API SHALL 从 VictoriaMetrics 查询该节点 Agent 上报的 `wireguard_peer_rx_bytes`、`wireguard_peer_tx_bytes` 指标，并计算为带宽速率
3. THE Node_Metrics_API SHALL 从 VictoriaMetrics 查询该节点 Agent 上报的 `wireguard_peer_last_handshake_secs` 指标，作为延迟参考值
4. WHEN 用户打开节点详情弹窗, THE Nodes_Detail_Panel SHALL 从 Node_Metrics_API 获取真实数据并替换当前硬编码的 `bandwidth: { upload: 0, download: 0 }` 和 `latency: 0`
5. IF Node_Metrics_API 返回错误或该节点无 metrics 数据, THEN THE Nodes_Detail_Panel SHALL 显示 "N/A" 而非 0
6. THE Node_Metrics_API SHALL 遵循 `/api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}/metrics` 路径规范，使用统一响应格式

### 需求 5：VPN 拓扑可视化实现

**用户故事：** 作为网络管理员，我希望在 VPN Topology 页面看到真实的网络拓扑图，以便直观了解节点间的连接关系和网络结构。

#### 验收标准

1. THE Topology_API SHALL 返回拓扑数据，包含节点列表（id、hostname、region、status、assigned_ip）和节点间的 peer 连接关系
2. THE Topology_API SHALL 基于节点的 WireGuard peer 配置构建连接关系，每条连接包含源节点、目标节点和连接状态
3. WHEN 用户访问 VPN Topology 页面, THE Topology_View SHALL 使用 ECharts graph 类型渲染力导向拓扑图，替换当前的 el-empty 占位符
4. THE Topology_View SHALL 使用不同颜色区分节点状态：在线节点为绿色，离线节点为红色
5. THE Topology_View SHALL 使用不同颜色区分连接状态：活跃连接为绿色实线，非活跃连接为灰色虚线
6. WHEN 用户将鼠标悬停在节点上, THE Topology_View SHALL 显示 tooltip，包含节点的 hostname、region、assigned_ip 和 status
7. WHEN 用户将鼠标悬停在连接线上, THE Topology_View SHALL 显示 tooltip，包含连接的源节点、目标节点和连接状态
8. WHEN 用户点击 Refresh 按钮, THE Topology_View SHALL 重新从 Topology_API 获取数据并更新拓扑图
9. IF Topology_API 返回错误, THEN THE Topology_View SHALL 显示错误提示信息
10. IF 租户下无节点数据, THEN THE Topology_View SHALL 显示 "暂无节点数据" 的空状态提示
11. THE Topology_API SHALL 遵循 `/api/v2/tenants/{tenant_id}/monitoring/topology` 路径规范，使用统一响应格式

### 需求 6：Controller 查询 VictoriaMetrics 的能力

**用户故事：** 作为系统开发者，我希望 Controller 能够查询 VictoriaMetrics 中的时序数据，以便为前端提供流量、带宽和延迟等 metrics 数据。

#### 验收标准

1. THE Controller SHALL 提供一个 VictoriaMetrics 查询客户端，支持 range query 和 instant query 两种查询方式
2. THE Controller SHALL 通过配置文件指定 VictoriaMetrics 的地址（默认 `http://localhost:8428`）
3. WHEN Controller 执行 range query, THE Controller SHALL 向 VictoriaMetrics 的 `/api/v1/query_range` 端点发送请求，并解析 Prometheus 格式的响应
4. WHEN Controller 执行 instant query, THE Controller SHALL 向 VictoriaMetrics 的 `/api/v1/query` 端点发送请求，并解析 Prometheus 格式的响应
5. IF VictoriaMetrics 不可用或查询超时, THEN THE Controller SHALL 返回空数据集并记录错误日志，查询超时时间为 10 秒
6. THE Controller SHALL 在查询时按租户的节点列表过滤 metrics 数据，确保租户隔离
