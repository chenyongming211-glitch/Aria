# Aria 监控系统部署

## 快速启动

```bash
cd deployments/monitoring
docker-compose up -d
```

## 访问地址

- **VictoriaMetrics**: http://localhost:8428
- **Grafana**: http://localhost:3000 (默认账号: admin/admin)

## 配置 Agent 推送

编辑 `/etc/aria/agent.yaml`，添加或修改 metrics 配置：

```yaml
metrics:
  enabled: true
  listen_addr: ":9090"
  push_gateway: "http://localhost:8428/api/v1/import/prometheus"
  push_interval: 15s
  collect_interval: 10s
```

重启 Agent：
```bash
sudo systemctl restart aria-agent
```

## 验证数据

### 1. 检查 Agent /metrics 端点
```bash
curl http://localhost:9090/metrics | grep wireguard
```

预期输出：
```
wireguard_peer_last_handshake_seconds{endpoint="x.x.x.x:51820",public_key="abc..."} 1707554321
wireguard_peer_rx_bytes{endpoint="...",public_key="..."} 123456
```

### 2. 检查 VictoriaMetrics 数据
```bash
curl -s 'http://localhost:8428/api/v1/query?query=wireguard_peer_last_handshake_seconds'
```

### 3. 检查 Controller /metrics 端点
```bash
curl http://localhost:8080/metrics | grep aria_controller
```

## Grafana Dashboard 配置

### 添加数据源

1. 登录 Grafana (http://localhost:3000)
2. 左侧菜单 -> Configuration -> Data Sources
3. 点击 "Add data source"
4. 选择 "Prometheus"
5. 配置：
   - Name: `VictoriaMetrics`
   - URL: `http://victoria-metrics:8428`
   - Access: `Server (default)`
6. 点击 "Save & Test"

### 导入 Dashboard

示例 PromQL 查询：

**WireGuard 隧道健康**
```promql
time() - wireguard_peer_last_handshake_seconds < 180
```

**平均 RTT**
```promql
avg(aria_probe_rtt_milliseconds) by (peer_ip)
```

**丢包率**
```promql
aria_probe_loss_ratio * 100
```

**流量速率**
```promql
rate(wireguard_peer_rx_bytes[5m])
```

**节点总数（Controller）**
```promql
aria_controller_nodes_total
```

## 停止服务

```bash
docker-compose down
```

## 清理数据

```bash
docker-compose down -v  # 删除卷，清空所有数据
```

## 数据保留策略

- 默认保留 30 天
- 修改保留期：编辑 `docker-compose.yml` 中的 `--retentionPeriod=30d`

## 性能调优

### VictoriaMetrics 内存限制

添加到 `docker-compose.yml`：
```yaml
victoria-metrics:
  # ...
  mem_limit: 2g
  memswap_limit: 2g
```

### 磁盘空间监控

```bash
docker exec aria-victoria-metrics du -sh /victoria-metrics-data
```

## 故障排查

### Push 失败

检查 Agent 日志：
```bash
sudo journalctl -u aria-agent -f | grep metrics
```

检查 VictoriaMetrics 日志：
```bash
docker logs -f aria-victoria-metrics
```

### 数据缺失

1. 检查 Agent metrics server 是否启动：
   ```bash
   curl http://localhost:9090/health
   ```

2. 检查 VictoriaMetrics 是否接收数据：
   ```bash
   curl 'http://localhost:8428/api/v1/labels'
   ```

3. 检查时间同步（NTP）：
   ```bash
   timedatectl status
   ```

## 高级配置

### 启用 Grafana 告警

编辑 `docker-compose.yml`，添加 SMTP 配置：
```yaml
grafana:
  environment:
    - GF_SMTP_ENABLED=true
    - GF_SMTP_HOST=smtp.gmail.com:587
    - GF_SMTP_USER=your-email@gmail.com
    - GF_SMTP_PASSWORD=your-app-password
```

### 多 VictoriaMetrics 集群

生产环境建议使用 VictoriaMetrics 集群模式，参考官方文档：
https://docs.victoriametrics.com/Cluster-VictoriaMetrics.html
