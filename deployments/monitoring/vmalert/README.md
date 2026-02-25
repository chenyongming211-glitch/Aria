# Aria 告警配置

## 架构

Aria 使用 vmalert（VictoriaMetrics 的告警组件）进行告警管理：

```
┌─────────────────┐
│  VictoriaMetrics│  ← 数据存储
└────────┬────────┘
         │
    ┌────▼────┐
    │ vmalert │  ← 告警评估引擎
    └────┬────┘
         │
    ┌────▼────────────────┐
    │  通知渠道 (可选)     │
    │  - Webhook          │
    │  - Email (via      │
    │    AlertManager)    │
    │  - Slack           │
    └─────────────────────┘
```

## 告警规则

### 当前规则概览

| 告警名称 | 严重程度 | 触发条件 | 持续时间 |
|---------|---------|---------|---------|
| **WireGuard**
| WireGuardTunnelDown | critical | 5分钟未握手 | 5m |
| MultipleWireGuardTunnelsDown | critical | ≥2个隧道断开 | 3m |
| WireGuardLowTraffic | warning | 流量<100B/s | 10m |
| **链路质量**
| HighPacketLoss | warning | 丢包率>10% | 3m |
| SeverePacketLoss | critical | 丢包率>30% | 2m |
| HighLatency | warning | RTT>200ms | 5m |
| LinkUnhealthy | warning | 健康分=0 | 5m |
| **系统**
| HighCPUUsage | warning | CPU>80% | 5m |
| HighMemoryUsage | warning | 内存>500MB | 5m |
| GoroutineLeak | warning | Goroutines>1000 | 10m |
| **Controller**
| ManyNodesOffline | critical | 离线节点>30% | 5m |
| NodeNotReporting | warning | 10分钟未上报 | 立即 |

## 快速开始

### 1. 启动告警服务

```bash
cd deployments/monitoring
docker-compose up -d vmalert
```

### 2. 验证配置

```bash
# 查看 vmalert 日志
docker logs -f aria-vmalert

# 检查规则是否加载
curl http://localhost:8880/api/v1/rules

# 查看活跃告警
curl http://localhost:8880/api/v1/alerts
```

### 3. 访问 vmalert UI

打开浏览器访问：http://localhost:8880

可以看到：
- 当前告警规则
- 活跃告警列表
- 告警历史

## 配置通知渠道

### 方法 1: Webhook（推荐）

vmalert 支持直接发送 Webhook：

```yaml
# 在 docker-compose.yml 中添加
command:
  - '-datasource.url=http://victoria-metrics:8428'
  - '-remoteWrite.url=http://victoria-metrics:8428'
  - '-remoteRead.url=http://victoria-metrics:8428'
  - '-rule=/etc/vmalert/rules/*.yml'
  - '-httpListenAddr=:8880'
  - '-evaluationInterval=30s'
  - '-notifier.url=http://your-webhook-endpoint'  # 添加此行
```

Webhook 将接收 JSON 格式的告警数据：
```json
{
  "alerts": [
    {
      "labels": {
        "alertname": "WireGuardTunnelDown",
        "severity": "critical",
        "public_key": "abc123..."
      },
      "annotations": {
        "summary": "WireGuard tunnel to abc123... is down",
        "description": "Peer has not handshaked for 5m30s."
      },
      "startsAt": "2026-02-10T03:00:00Z",
      "endsAt": "0001-01-01T00:00:00Z",
      "status": "firing"
    }
  ]
}
```

### 方法 2: AlertManager（完整功能）

如需 Email、Slack、PagerDuty 等集成，使用 AlertManager：

1. 在 `docker-compose.yml` 中添加 AlertManager 服务：

```yaml
  alertmanager:
    image: prom/alertmanager:latest
    container_name: aria-alertmanager
    ports:
      - "9093:9093"
    volumes:
      - ./alertmanager:/etc/alertmanager
    command:
      - '--config.file=/etc/alertmanager/alertmanager.yml'
    restart: unless-stopped
    networks:
      - aria-monitoring
```

2. 创建 AlertManager 配置 (`alertmanager/alertmanager.yml`)：

```yaml
global:
  smtp_smarthost: 'smtp.gmail.com:587'
  smtp_from: 'alerts@example.com'
  smtp_auth_username: 'your-email@gmail.com'
  smtp_auth_password: 'your-app-password'

route:
  group_by: ['alertname', 'severity']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: 'email-notifications'
  routes:
    - match:
        severity: critical
      receiver: 'critical-alerts'

receivers:
  - name: 'email-notifications'
    email_configs:
      - to: 'team@example.com'
        headers:
          subject: '[Aria] {{ .GroupLabels.alertname }}'

  - name: 'critical-alerts'
    email_configs:
      - to: 'oncall@example.com'
        headers:
          subject: '[CRITICAL] {{ .GroupLabels.alertname }}'
    # Slack 集成（可选）
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/WEBHOOK/URL'
        channel: '#aria-alerts'
        title: '{{ .GroupLabels.alertname }}'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

3. 更新 vmalert 配置：

```yaml
vmalert:
  command:
    - '-datasource.url=http://victoria-metrics:8428'
    - '-remoteWrite.url=http://victoria-metrics:8428'
    - '-remoteRead.url=http://victoria-metrics:8428'
    - '-rule=/etc/vmalert/rules/*.yml'
    - '-httpListenAddr=:8880'
    - '-evaluationInterval=30s'
    - '-notifier.url=http://alertmanager:9093'  # 指向 AlertManager
```

## 自定义告警规则

### 添加新规则

编辑 `vmalert/rules/aria-alerts.yml`：

```yaml
groups:
  - name: my_custom_alerts
    interval: 30s
    rules:
      - alert: MyCustomAlert
        expr: your_promql_expression > threshold
        for: duration
        labels:
          severity: warning/critical
          component: your_component
        annotations:
          summary: "Short description"
          description: "Detailed description with {{ $value }}"
```

### 重载规则

```bash
# 方法 1：重启 vmalert
docker restart aria-vmalert

# 方法 2：发送 SIGHUP（热重载）
docker kill -s SIGHUP aria-vmalert
```

## 告警测试

### 手动触发告警

#### 1. 模拟隧道断开
```bash
# 停止某个 Agent
sudo systemctl stop aria-agent

# 5分钟后将触发 WireGuardTunnelDown 告警
```

#### 2. 模拟高丢包
```bash
# 使用 tc 添加丢包
sudo tc qdisc add dev aria0 root netem loss 15%

# 3分钟后将触发 HighPacketLoss 告警

# 恢复
sudo tc qdisc del dev aria0 root netem
```

#### 3. 模拟高 CPU
```bash
# 运行 stress 工具
stress --cpu 4 --timeout 300s

# 5分钟后将触发 HighCPUUsage 告警
```

### 验证告警

```bash
# 查看活跃告警
curl http://localhost:8880/api/v1/alerts

# 查看告警历史
curl 'http://localhost:8428/api/v1/query?query=ALERTS'

# 检查 AlertManager（如果使用）
curl http://localhost:9093/api/v2/alerts
```

## 告警抑制和静默

### 临时静默告警（维护窗口）

访问 vmalert UI (http://localhost:8880)：
1. 点击告警规则
2. 点击 "Silence"
3. 设置静默时长和原因

或使用 API：
```bash
curl -X POST 'http://localhost:8880/api/v1/admin/silence?alertname=WireGuardTunnelDown&duration=1h'
```

### 告警分组

在 AlertManager 中配置 `group_by`：
```yaml
route:
  group_by: ['alertname']  # 按告警名称分组
  group_wait: 30s          # 等待30秒收集同组告警
  group_interval: 5m       # 每5分钟发送一次组内新告警
```

## 性能调优

### 减少评估频率

```yaml
vmalert:
  command:
    - '-evaluationInterval=1m'  # 从30秒改为1分钟
```

### 限制告警数量

在规则中添加 `limit`：
```yaml
- alert: HighLatency
  expr: count(aria_probe_rtt_milliseconds > 200) > 5  # 仅当超过5个peer高延迟时告警
```

## 故障排查

### vmalert 无法连接 VictoriaMetrics
```bash
# 检查网络连通性
docker exec aria-vmalert curl http://victoria-metrics:8428/metrics

# 检查 vmalert 日志
docker logs aria-vmalert | grep ERROR
```

### 规则未加载
```bash
# 验证 YAML 语法
yamllint vmalert/rules/aria-alerts.yml

# 检查规则
curl http://localhost:8880/api/v1/rules
```

### 告警未触发
```bash
# 手动执行 PromQL 查询
curl 'http://localhost:8428/api/v1/query?query=time()-wireguard_peer_last_handshake_seconds'

# 检查告警评估历史
curl 'http://localhost:8880/api/v1/alerts?active=true'
```

## 最佳实践

1. **分级告警**: 使用 warning/critical 区分严重程度
2. **合理阈值**: 根据实际业务调整阈值，避免告警疲劳
3. **持续时间**: 设置合理的 `for` 时长，避免瞬时波动误报
4. **描述清晰**: annotations 包含足够的上下文信息
5. **定期测试**: 定期测试告警是否正常工作
6. **告警分组**: 使用 group_by 避免告警风暴
7. **静默机制**: 维护期间使用静默功能

## 扩展阅读

- [vmalert Documentation](https://docs.victoriametrics.com/vmalert.html)
- [AlertManager Documentation](https://prometheus.io/docs/alerting/latest/alertmanager/)
- [PromQL for Alerting](https://prometheus.io/docs/prometheus/latest/querying/basics/)
