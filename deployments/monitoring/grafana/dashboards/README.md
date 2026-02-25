# Grafana Dashboards for Aria

## 自动加载

Dashboard 会在 Grafana 启动时自动加载到 "Aria" 文件夹中。

## 可用 Dashboards

### 1. Aria SD-WAN Overview
**文件**: `aria-overview.json`

**包含面板**:
- Online Peers - 在线节点数量
- Average RTT - 平均往返时间
- Packet Loss - 平均丢包率
- Traffic Rate (5m) - 流量速率（5分钟窗口）
- RTT by Peer - 各节点 RTT 趋势
- Peer Status Table - 节点状态表

**访问**: Grafana -> Dashboards -> Aria -> Aria SD-WAN Overview

## 手动导入

如果自动加载失败，可以手动导入：

1. 登录 Grafana (http://localhost:3000)
2. 左侧菜单 -> Dashboards -> Import
3. 上传 JSON 文件或粘贴内容
4. 选择 VictoriaMetrics 数据源
5. 点击 Import

## 自定义面板

所有 dashboard 都可以编辑。常用操作：

### 添加面板
1. 点击右上角 "Add panel"
2. 选择 "Add a new panel"
3. 输入 PromQL 查询
4. 配置可视化选项
5. 保存

### 常用 PromQL 查询

**隧道健康（最近 3 分钟内握手）**:
```promql
time() - wireguard_peer_last_handshake_seconds < 180
```

**平均 RTT**:
```promql
avg(aria_probe_rtt_milliseconds) by (peer_ip)
```

**丢包率（百分比）**:
```promql
aria_probe_loss_ratio * 100
```

**流量速率（5 分钟窗口）**:
```promql
rate(wireguard_peer_rx_bytes[5m])
rate(wireguard_peer_tx_bytes[5m])
```

**节点总数（按状态）**:
```promql
aria_controller_nodes_total
```

**CPU 使用率**:
```promql
aria_cpu_usage_percent
```

**内存使用（MB）**:
```promql
aria_memory_bytes{type="alloc"} / 1024 / 1024
```

## 告警配置

在面板的 "Alert" 选项卡中配置告警规则：

### 示例：隧道断开告警
1. 编辑 "Online Peers" 面板
2. 切换到 "Alert" 选项卡
3. 添加告警规则：
   - Query: `count(time() - wireguard_peer_last_handshake_seconds < 180)`
   - Condition: `WHEN last() OF query(A) IS BELOW 1`
   - Evaluate every: `1m`
   - For: `5m`
4. 配置通知渠道（Email/Slack/Webhook）

### 示例：高丢包率告警
1. 编辑 "Packet Loss" 面板
2. 添加告警规则：
   - Query: `avg(aria_probe_loss_ratio) * 100`
   - Condition: `WHEN avg() OF query(A) IS ABOVE 10`
   - Evaluate every: `1m`
   - For: `3m`

## 故障排查

### Dashboard 未自动加载
检查：
1. Grafana 日志：`docker logs aria-grafana`
2. Provisioning 配置：`/etc/grafana/provisioning/dashboards/aria.yml`
3. Dashboard 文件权限

### 数据源连接失败
检查：
1. VictoriaMetrics 是否运行：`curl http://localhost:8428/metrics`
2. 网络连接：Grafana 和 VictoriaMetrics 在同一 Docker 网络中
3. 数据源配置：Grafana -> Configuration -> Data Sources

### 无数据显示
检查：
1. Agent 是否推送数据：查看 Agent 日志
2. VictoriaMetrics 是否接收数据：
   ```bash
   curl 'http://localhost:8428/api/v1/label/__name__/values'
   ```
3. 时间范围是否正确：Dashboard 右上角时间选择器

## 性能优化

### 减少查询负载
- 增加刷新间隔（默认 10秒）
- 使用更长的时间聚合窗口
- 限制显示的 peer 数量

### 降低内存使用
- 减少保留时间（默认 30 天）
- 减少面板数量
- 使用 downsampling（降采样）

## 扩展阅读

- [Grafana Documentation](https://grafana.com/docs/)
- [PromQL Basics](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [VictoriaMetrics Query API](https://docs.victoriametrics.com/Single-server-VictoriaMetrics.html#prometheus-querying-api-usage)
