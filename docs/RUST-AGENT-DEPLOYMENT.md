# Rust Agent 部署文档

## 快速开始

### 1. 环境准备

```bash
# 检查系统要求
- Linux 内核 5.4+ (支持 eBPF)
- root 权限
- ethtool 工具（用于硬件卸载配置）
```

### 2. 编译 Agent

**在开发机上（有 cargo 环境）：**
```bash
# 本地编译
cd /path/to/Aria/agent-rust
cargo build --release

# 检查二进制
ls -lh target/release/aria-agent
```

**或在 Agent 节点上编译：**
```bash
# 登录到 agent 节点
ssh root@<agent-ip>

# 进入代码目录
cd /root/aria-agent-rust

# 编译 release 版本
/root/.cargo/bin/cargo build --release
```

### 3. 部署新版本

```bash
# 3.1 停止现有服务
systemctl stop aria

# 3.2 备份旧版本（可选）
cp /usr/local/bin/aria /usr/local/bin/aria.bak

# 3.3 部署新二进制
cp /root/aria-agent-rust/target/release/aria-agent /usr/local/bin/aria
chmod +x /usr/local/bin/aria

# 3.4 验证版本
aria --version
```

### 4. 配置文件

配置文件位置：`/etc/aria/agent.yaml`

```yaml
controller_url: https://<controller-ip>:50051
ca_cert: /etc/aria/certs/ca/ca.crt
client_cert: /etc/aria/certs/agents/agent-sh.crt
client_key: /etc/aria/certs/agents/agent-sh.key
private_key: <generated-by-aria-init>
public_key: <generated-by-aria-init>
interface_name: aria0
listen_port: 51820
mtu: 1360
region: sh
advertised_routes:
  - 2.2.2.0/24
  - 3.3.3.0/24
  - 8.8.8.0/24
sync_interval: 5
```

### 5. Systemd 服务管理

**启动服务：**
```bash
systemctl daemon-reload
systemctl start aria
systemctl status aria
```

**停止服务：**
```bash
systemctl stop aria
```

**重启服务：**
```bash
systemctl restart aria
```

**查看日志：**
```bash
# 查看最新日志
journalctl -u aria -n 100 --no-pager

# 实时查看日志
journalctl -u aria -f

# 过滤特定内容
journalctl -u aria --no-pager | grep -E 'offload|NOTRACK|BBR'
```

### 6. 验证系统优化

**检查 NOTRACK 规则：**
```bash
iptables -t raw -L -n -v | grep 51820
```

**检查 BBR：**
```bash
sysctl net.ipv4.tcp_congestion_control
# 应该输出: net.ipv4.tcp_congestion_control = bbr
```

**检查 TCP 缓冲区：**
```bash
sysctl net.core.rmem_max net.core.wmem_max
# 应该输出: 26214400 (25MB)
```

**检查 eth0 硬件卸载：**
```bash
ethtool -k eth0 | grep -E 'gso|gro|tso'
# 应该都是 on
```

**检查 aria0 硬件卸载（接口创建后）：**
```bash
# 先确认 aria0 接口存在
ip link show aria0

# 检查卸载状态
ethtool -k aria0 | grep -E 'gso|gro'
```

**检查 Ring Buffer：**
```bash
ethtool -g eth0 | grep -A 2 'Current hardware'
# RX/TX 应该增大到 4096
```

**检查持久化配置：**
```bash
cat /etc/sysctl.d/99-aria.conf
```

## 完整部署流程（从零开始）

### 步骤 1: 初始化 Agent

```bash
# 在 Controller 上创建 token
ssh root@<controller-ip> "docker exec aria_controller aria token create --tag=agent-sh"

# 在 Agent 节点上初始化
aria init \
  --server=https://<controller-ip>:50051 \
  --token=<token> \
  --region=sh \
  --advertise-routes=2.2.2.0/24,3.3.3.0/24,8.8.8.0/24 \
  --interface=eth0
```

### 步骤 2: 配置 Systemd 服务

```bash
# 创建服务文件
cat > /etc/systemd/system/aria.service << 'SERVICE'
[Unit]
Description=Aria SD-WAN Agent (Rust eBPF)
Documentation=https://github.com/anomaly/aria
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/aria up --interface=eth0 --config=/etc/aria/agent.yaml
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

Environment=RUST_LOG=info

LimitNOFILE=65535
LimitNPROC=4096

NoNewPrivileges=false
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_BPF CAP_PERFMON

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable aria
```

### 步骤 3: 启动并验证

```bash
# 启动服务
systemctl start aria

# 等待启动完成
sleep 5

# 检查状态
systemctl status aria

# 检查日志中的系统优化
journalctl -u aria -n 100 --no-pager | grep -E 'Step 2|System optim|offload|NOTRACK|BBR'

# 检查 aria0 接口
ip link show aria0

# 验证系统优化
ethtool -k eth0 | grep -E 'gso|gro|tso'
ethtool -k aria0 | grep -E 'gso|gro'
sysctl net.ipv4.tcp_congestion_control
iptables -t raw -L -n -v | grep 51820
```

## 更新流程

### 快速更新（推荐）

```bash
# 1. 编译新版本
cd /root/aria-agent-rust
/root/.cargo/bin/cargo build --release

# 2. 停止服务
systemctl stop aria

# 3. 原子替换二进制
cp /root/aria-agent-rust/target/release/aria-agent /usr/local/bin/aria.new
chmod +x /usr/local/bin/aria.new
mv -f /usr/local/bin/aria.new /usr/local/bin/aria

# 4. 启动服务
systemctl start aria

# 5. 验证
systemctl status aria
journalctl -u aria -n 50 --no-pager
```

### 从开发机推送更新

```bash
# 在开发机上执行

# 1. 编译
cd /path/to/Aria/agent-rust
cargo build --release

# 2. 推送到所有 Agent 节点
for node in root@146.56.196.231 root@118.195.135.16; do
  echo "Updating $node..."
  ssh $node "systemctl stop aria"
  scp target/release/aria-agent $node:/usr/local/bin/aria.new
  ssh $node "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"
  ssh $node "systemctl start aria"
  echo "$node updated"
done

# 3. 验证所有节点
for node in root@146.56.196.231 root@118.195.135.16; do
  echo "Checking $node..."
  ssh $node "systemctl status aria --no-pager | head -10"
done
```

## 故障排查

### Agent 无法启动

```bash
# 1. 检查日志
journalctl -u aria -n 100 --no-pager

# 2. 检查配置文件
cat /etc/aria/agent.yaml

# 3. 检查证书文件
ls -la /etc/aria/certs/

# 4. 手动启动测试
/usr/local/bin/aria up --interface=eth0 --config=/etc/aria/agent.yaml
```

### aria0 接口未创建

```bash
# 检查 WireGuard 配置
wg show

# 检查 eBPF 程序
bpftool prog list | grep aria

# 手动创建接口（测试）
ip link add dev aria0 type wireguard
```

### 系统优化未生效

```bash
# 检查日志中是否有 "System optimizations"
journalctl -u aria --no-pager | grep -i "system optim"

# 手动验证各项优化
iptables -t raw -L -n -v
sysctl net.ipv4.tcp_congestion_control
ethtool -k eth0 | grep -E 'gso|gro|tso'
ethtool -g eth0
```

### 连接 Controller 失败

```bash
# 1. 检查网络连通性
ping <controller-ip>
telnet <controller-ip> 50051

# 2. 检查证书
openssl x509 -in /etc/aria/certs/ca/ca.crt -text -noout

# 3. 检查时间同步
timedatectl status
```

## 性能验证

### 压力测试

```bash
# 在另一个节点上运行 iperf3 服务端
iperf3 -s

# 在本节点测试
iperf3 -c <peer-ip> -P 10 -t 60

# 观察 CPU、内存、网络使用率
top -p $(pgrep aria)
```

### 监控指标

```bash
# 访问 Prometheus 指标
curl http://localhost:9090/metrics
```

## 常见问题

**Q: aria0 的 offload 失败怎么办？**
A: aria0 offload 需要在接口创建后执行。检查日志中是否有 "Creating WireGuard interface" 和 "System optimizations" 的顺序。

**Q: 如何回滚到旧版本？**
A: 
```bash
systemctl stop aria
mv /usr/local/bin/aria.bak /usr/local/bin/aria
systemctl start aria
```

**Q: 系统优化会影响其他程序吗？**
A: 不会。NOTRACK 只影响 WireGuard 端口，sysctl 优化是全局的但都是性能提升，不会有负面影响。

## 参考资料

- eBPF 文档：`docs/EBPF-QOS-DESIGN.md`
- 系统优化详细说明：`docs/SYSTEM-OPTIMIZATION.md`
- API 文档：`docs/API-REFERENCE.md`
