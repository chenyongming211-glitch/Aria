# Agent 部署和更新流程

## 概述

Aria Agent 使用 Rust + eBPF 实现，支持高性能的 QoS 和 ACL 功能。本文档描述了从源码编译到部署的完整流程。

## 架构说明

- **Agent**: Rust 编写，包含 eBPF 程序，运行在边缘节点
- **Controller**: Go 编写，运行在中心节点（容器化部署）
- **通信**: gRPC over TLS，端口 50051

## 服务器信息

| 节点 | IP | Region | VPN IP | 系统版本 |
|------|-----|--------|--------|----------|
| Controller | 112.124.8.241 | - | - | Ubuntu 24.04 |
| Agent sh | 146.56.196.231 | sh | 100.64.0.1 | Ubuntu 24.04 |
| Agent bj | 118.195.135.16 | bj | 100.64.0.2 | Ubuntu 22.04 |

---

## 一、环境准备

### 1.1 编译环境要求

在编译节点（建议 sh 节点）上安装以下工具：

```bash
# 安装 Rust Nightly
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
source ~/.cargo/env

# 安装 Rust 组件
rustup component add rust-src --toolchain nightly
rustup component add clippy --toolchain nightly
rustup component add rustfmt --toolchain nightly

# 安装系统依赖
apt-get update
apt-get install -y \
    build-essential \
    clang \
    llvm \
    libelf-dev \
    libbpf-dev \
    pkg-config

# 安装 bpf-linker
cargo install bpf-linker
```

### 1.2 目标节点要求

- Linux 内核 5.4+（支持 eBPF 和 bpf_spin_lock）
- root 权限或 CAP_BPF、CAP_NET_ADMIN 权限
- systemd 服务管理器
- 开放端口：51820/udp (WireGuard)、9090/tcp (Metrics)

---

## 二、全新部署流程

### 2.1 准备源码

**从开发机同步到编译节点：**

```bash
# 在开发机上执行
rsync -avz --exclude='target' --exclude='.git' \
    agent-rust/ root@146.56.196.231:/root/agent-rust/
```

### 2.2 编译 Agent

**在编译节点上执行：**

```bash
# 进入源码目录
cd /root/agent-rust

# 清理旧产物
cargo clean

# 编译 Release 版本
cargo build --release

# 验证编译产物
ls -lh target/release/aria-agent
file target/release/aria-agent
```

**编译产物位置：**
- 用户态程序：`target/release/aria-agent`（约 6MB）
- eBPF 程序：已嵌入到用户态程序中

### 2.3 准备配置文件

**配置文件模板：** `/etc/aria/agent.yaml`

```yaml
controller_url: https://112.124.8.241:50051
ca_cert: /etc/aria/certs/ca/ca.crt
client_cert: /etc/aria/certs/agents/agent-<region>.crt
client_key: /etc/aria/certs/agents/agent-<region>.key
device_id: null
private_key: <自动生成或从Controller获取>
public_key: <自动生成或从Controller获取>
assigned_ip: null
address: null
interface_name: aria0
listen_port: 51820
mtu: 1360
region: <region>
customer_id: null
advertised_routes:
  - 2.2.2.0/24
  - 3.3.3.0/24
  - 8.8.8.0/24
hostname: null
sync_interval: 5
```

**重要字段说明：**
- `controller_url`: Controller 的 gRPC 地址（必须包含端口 50051）
- `ca_cert`: CA 证书路径
- `client_cert/client_key`: Agent 客户端证书
- `region`: 节点区域标识
- `advertised_routes`: 该节点通告的路由

### 2.4 创建 systemd 服务

**服务配置：** `/etc/systemd/system/aria.service`

```ini
[Unit]
Description=Aria SD-WAN Agent (Rust eBPF)
Documentation=https://github.com/anomaly/aria
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/aria up --interface=eth0 --config=/etc/aria/agent.yaml
Environment="ARIA_ENV=production"
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=aria

# Security hardening
NoNewPrivileges=false
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/aria /var/log/aria /etc/wireguard /etc/aria /run
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW CAP_BPF CAP_SYS_ADMIN
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW CAP_BPF CAP_SYS_ADMIN

[Install]
WantedBy=multi-user.target
```

### 2.5 部署到目标节点

**方案一：从编译节点直接部署**

```bash
# 复制二进制文件
ssh root@146.56.196.231 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria"
ssh root@146.56.196.231 "chmod +x /usr/local/bin/aria"

# 复制到其他节点（注意 glibc 版本兼容性）
ssh root@146.56.196.231 "cat /root/agent-rust/target/release/aria-agent" | \
    ssh root@118.195.135.16 "cat > /usr/local/bin/aria && chmod +x /usr/local/bin/aria"
```

**方案二：在目标节点上编译（推荐用于不同系统版本）**

如果编译节点和目标节点的 glibc 版本不同（如 Ubuntu 24.04 vs 22.04），需要在目标节点上单独编译：

```bash
# 在目标节点上安装编译环境（参考 1.1）
# 然后同步源码并编译
rsync -avz --exclude='target' --exclude='.git' \
    agent-rust/ root@118.195.135.16:/root/agent-rust/

ssh root@118.195.135.16 "source ~/.cargo/env && cd /root/agent-rust && cargo build --release"
ssh root@118.195.135.16 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria"
```

### 2.6 启动服务

```bash
# 重载 systemd 配置
systemctl daemon-reload

# 启动服务
systemctl start aria

# 查看状态
systemctl status aria

# 查看日志
journalctl -u aria -f

# 设置开机自启
systemctl enable aria
```

---

## 三、更新流程（原子替换）

### 3.1 本地编译（如果使用开发机编译）

```bash
# 在开发机上
cd /path/to/Aria

# 同步源码到编译节点
rsync -avz --exclude='target' --exclude='.git' \
    agent-rust/ root@146.56.196.231:/root/agent-rust/

# 在编译节点上编译
ssh root@146.56.196.231 "source ~/.cargo/env && cd /root/agent-rust && cargo build --release"
```

### 3.2 原子替换（推荐）

使用原子替换方式，避免服务中断：

```bash
# sh 节点
rsync -avz root@146.56.196.231:/root/agent-rust/target/release/aria-agent \
    root@146.56.196.231:/usr/local/bin/aria.new
ssh root@146.56.196.231 "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"

# bj 节点（需要先在该节点编译，或使用兼容的二进制）
rsync -avz root@118.195.135.16:/root/agent-rust/target/release/aria-agent \
    root@118.195.135.16:/usr/local/bin/aria.new
ssh root@118.195.135.16 "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"
```

### 3.3 重启服务

```bash
# 重启两个节点的服务
ssh root@146.56.196.231 "systemctl restart aria"
ssh root@118.195.135.16 "systemctl restart aria"

# 验证服务状态
ssh root@146.56.196.231 "systemctl status aria"
ssh root@118.195.135.16 "systemctl status aria"

# 验证版本
ssh root@146.56.196.231 "aria --help"
ssh root@118.195.135.16 "aria --help"
```

---

## 四、常见问题排查

### 4.1 glibc 版本不兼容

**症状：**
```
/usr/local/bin/aria: /lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.39' not found
```

**原因：** 编译节点的 glibc 版本高于目标节点

**解决方案：**
- 方案一：在目标节点上单独编译
- 方案二：使用静态链接（修改 Cargo.toml）
- 方案三：使用 musl 目标编译

### 4.2 端口被占用

**症状：**
```
Error: Failed to initialize metrics
Caused by: failed to create HTTP listener: Address already in use (os error 98)
```

**解决方案：**
```bash
# 查找占用端口的进程
netstat -tlnp | grep :9090

# 停止旧进程
kill -9 <PID>

# 或修改配置使用其他端口
# 在 agent.yaml 中修改 metrics.listen_addr
```

### 4.3 配置文件字段缺失

**症状：**
```
Error: Failed to parse config YAML
Caused by: missing field `ca_cert`
```

**解决方案：** 检查配置文件，确保包含所有必需字段：
- `controller_url`（必须包含端口 :50051）
- `ca_cert`
- `client_cert`（如果使用 mTLS）
- `client_key`（如果使用 mTLS）
- `region`
- `interface_name`

### 4.4 无法连接到 Controller

**症状：**
```
Error: Failed to connect to Controller
Caused by: transport error
```

或

```
Error: peer closed connection without sending TLS close_notify
```

**排查步骤：**
1. 检查 Controller 是否运行：`docker ps | grep controller`
2. 检查 Controller 端口监听：`docker exec aria_controller netstat -tlnp | grep 50051`
3. 检查容器端口映射：`docker port aria_controller`
4. 检查防火墙规则：`iptables -L -n | grep 50051`
5. 测试网络连通性：`telnet 112.124.8.241 50051`
6. 测试TLS连接：
   ```bash
   openssl s_client -connect 112.124.8.241:50051 \
     -CAfile /etc/aria/certs/ca/ca.crt \
     -cert /etc/aria/certs/agents/agent-<region>.crt \
     -key /etc/aria/certs/agents/agent-<region>.key
   ```

**解决方案：** 
- 确保 Controller 的 gRPC 端口可以从 Agent 节点访问
- 检查证书是否正确：
  - CA证书一致
  - 客户端证书和私钥匹配
  - 证书未过期
  - 证书CN名称正确
- 如果TLS握手失败，检查Controller日志：
  ```bash
  docker logs aria_controller 2>&1 | grep -E '(TLS|certificate|client)'
  ```

**常见TLS错误：**

1. **certificate not valid for name**
   - 原因：domain_name配置与证书CN/SAN不匹配
   - 解决：检查`grpc_client.rs`中的`domain_name`配置，应该与服务器证书的SAN匹配
   
2. **peer closed connection without sending TLS close_notify**
   - 原因：Controller拒绝了客户端证书或mTLS验证失败
   - 解决：检查Controller是否信任该客户端证书，检查证书CN是否在白名单中

3. **Permission denied (os error 13)**
   - 可能是TLS握手失败的误导性错误
   - 使用`strace`跟踪详细错误：
     ```bash
     strace -s 500 -e trace=network,openat /usr/local/bin/aria up --interface=eth0 --config=/etc/aria/agent.yaml 2>&1 | grep -E '(connect|cert|Permission|TLS)'
     ```

### 4.5 eBPF 程序加载失败

**症状：**
```
Error: Failed to load eBPF program
```

**排查步骤：**
1. 检查内核版本：`uname -r`（需要 5.4+）
2. 检查 eBPF 支持：`bpftool feature`
3. 检查权限：确保有 CAP_BPF、CAP_NET_ADMIN 权限
4. 查看详细日志：`journalctl -u aria -n 100`

---

## 五、快速部署脚本

### 5.1 完整部署脚本

```bash
#!/bin/bash
set -e

# 配置变量
COMPILE_NODE="146.56.196.231"
AGENTS=("146.56.196.231" "118.195.135.16")
REGIONS=("sh" "bj")

echo "=== 1. 同步源码到编译节点 ==="
rsync -avz --exclude='target' --exclude='.git' \
    agent-rust/ root@${COMPILE_NODE}:/root/agent-rust/

echo "=== 2. 在编译节点上编译 ==="
ssh root@${COMPILE_NODE} "source ~/.cargo/env && cd /root/agent-rust && cargo build --release"

echo "=== 3. 部署到所有 Agent 节点 ==="
for i in "${!AGENTS[@]}"; do
    AGENT="${AGENTS[$i]}"
    REGION="${REGIONS[$i]}"
    
    echo "部署到 ${REGION} 节点 (${AGENT})..."
    
    # 原子替换
    ssh root@${COMPILE_NODE} "cat /root/agent-rust/target/release/aria-agent" | \
        ssh root@${AGENT} "cat > /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"
    
    # 重启服务
    ssh root@${AGENT} "systemctl restart aria"
    
    # 验证
    echo "验证 ${REGION} 节点..."
    ssh root@${AGENT} "systemctl status aria --no-pager | head -10"
done

echo "=== 部署完成 ==="
```

### 5.2 仅更新二进制脚本

```bash
#!/bin/bash
set -e

COMPILE_NODE="146.56.196.231"
AGENTS=("146.56.196.231" "118.195.135.16")

echo "=== 重新编译 ==="
ssh root@${COMPILE_NODE} "source ~/.cargo/env && cd /root/agent-rust && cargo build --release"

echo "=== 原子替换 ==="
for AGENT in "${AGENTS[@]}"; do
    echo "更新 ${AGENT}..."
    ssh root@${COMPILE_NODE} "cat /root/agent-rust/target/release/aria-agent" | \
        ssh root@${AGENT} "cat > /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"
done

echo "=== 重启服务 ==="
for AGENT in "${AGENTS[@]}"; do
    ssh root@${AGENT} "systemctl restart aria"
done

echo "=== 验证 ==="
for AGENT in "${AGENTS[@]}"; do
    echo "${AGENT}:"
    ssh root@${AGENT} "aria --help | head -5"
done
```

---

## 六、命令速查

```bash
# 编译
cargo build --release

# 部署
cp target/release/aria-agent /usr/local/bin/aria

# 服务管理
systemctl start aria
systemctl stop aria
systemctl restart aria
systemctl status aria

# 查看日志
journalctl -u aria -f
journalctl -u aria -n 100

# 验证
aria --help
aria status

# 查看接口
ip link show | grep aria
```

---

## 七、注意事项

1. **glibc 兼容性**：不同系统版本的节点需要在本地编译
2. **原子替换**：使用 `mv` 命令在同一文件系统下是原子操作，无需停止服务
3. **配置文件**：确保所有必需字段都已填写，特别是 `controller_url` 和证书路径
4. **端口冲突**：部署前检查 9090 端口是否被占用
5. **权限**：eBPF 程序需要 CAP_BPF、CAP_NET_ADMIN 等权限
6. **网络**：确保 Agent 可以访问 Controller 的 gRPC 端口（50051）

---

**最后更新**: 2026-03-04  
**版本**: 1.0.0  
**维护者**: Aria Team
