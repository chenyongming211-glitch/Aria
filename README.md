# Aria SD-WAN

企业级去中心化组网系统，基于 WireGuard 构建，支持零接触部署、eBPF 防火墙、QoS 流量控制和 AI 智能策略。

## 核心特性

### 组网能力
- **WireGuard Mesh** - 全互联 VPN 隧道，端到端加密
- **零接触部署** - Agent 自动注册，无需手动配置
- **多云支持** - 自动探测阿里云、腾讯云、华为云元数据
- **多隧道模式** - 支持 aria0, aria1, aria2, aria3 多个隧道

### 网络控制
- **eBPF 防火墙** - 基于五元组的高性能包过滤
- **eBPF QoS** - 端口级/服务级/IP 级流量限速
- **无状态防火墙** - 高性能 ACL 规则，不追踪连接状态
- **ECMP 路由** - 多隧道负载均衡

### 管理能力
- **多租户** - 租户隔离，独立的策略和网络视图
- **Token 管理** - 细粒度的节点准入控制
- **CLI 管理** - 完整的命令行工具
- **Web UI** - 现代化管理界面

### AI 集成
- **智能策略** - AI 自动生成网络策略
- **飞书告警** - 实时推送到飞书群机器人
- **自然语言交互** - 通过对话管理网络

### 监控运维
- **Metrics 暴露** - Prometheus 格式指标
- **Grafana 仪表盘** - 预置可视化面板
- **告警规则** - VictoriaMetrics 告警配置
- **链路探测** - 实时网络质量监控

## 架构

```
                    ┌─────────────────────────────────────┐
                    │           Controller                │
                    │  ┌─────────┐  ┌─────────┐          │
                    │  │PostgreSQL│  │  Redis  │          │
                    │  └─────────┘  └─────────┘          │
                    │         HTTP API :8080             │
                    └──────────────┬──────────────────────┘
                                   │
          ┌────────────────────────┼────────────────────────┐
          │                        │                        │
          ▼                        ▼                        ▼
   ┌────────────┐          ┌────────────┐          ┌────────────┐
   │  Agent A   │◄────────►│  Agent B   │◄────────►│  Agent C   │
   │ 100.64.0.1 │   Mesh   │ 100.64.0.2 │   Mesh   │ 100.64.0.3 │
   │  阿里云    │          │  腾讯云    │          │  华为云    │
   └────────────┘          └────────────┘          └────────────┘
          │                        │                        │
   ┌────────────┐          ┌────────────┐          ┌────────────┐
   │  eBPF      │          │  eBPF      │          │  eBPF      │
   │  Firewall  │          │  Firewall  │          │  Firewall  │
   │  QoS       │          │  QoS       │          │  QoS       │
   └────────────┘          └────────────┘          └────────────┘
```

## 快速开始

### 部署 Controller

```bash
# 上传部署包
scp -r releases/deploy/controller root@<server>:/root/aria-controller/

# SSH 登录并部署
ssh root@<server>
cd /root/aria-controller
./deploy.sh primary
```

### 部署 Agent

```bash
# 上传到目标节点
scp -r releases/deploy/agent root@<node>:/root/aria-agent/

# 安装并连接
ssh root@<node>
cd /root/aria-agent
./deploy.sh install http://<controller>:8080
```

### 使用 CLI

```bash
# 查看状态
aria status

# 查看节点
aria peers

# 查看路由
aria route list

# 启用 eBPF QoS（需要 root）
sudo aria ebpf status
sudo aria ebpf qos limit-port 80 --mbps 100
```

## CLI 命令

| 命令 | 说明 |
|------|------|
| `aria status` | 查看 Agent 状态 |
| `aria up` | 启动 Agent |
| `aria down` | 停止 Agent |
| `aria peers` | 查看对等节点 |
| `aria route` | 管理路由 |
| `aria token` | 管理 Token |
| `aria metrics` | 查看 Metrics |
| `aria ebpf` | eBPF 防火墙/QoS |
| `aria init` | 初始化 Agent |

## API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/nodes` | GET | 节点列表 |
| `/api/v1/nodes/:id` | GET | 节点详情 |
| `/api/v1/policies` | GET/POST | 策略管理 |
| `/api/v1/bandwidth` | GET/POST | 带宽控制 |
| `/api/v1/tenants` | GET/POST | 租户管理 |
| `/api/v1/tokens` | GET/POST | Token 管理 |
| `/api/v1/chat` | POST | AI 对话 |

## 目录结构

```
Aria/
├── cmd/                    # Go 入口程序（controller、ariactl）
├── internal/               # Controller 私有模块（API、CLI、认证、租户、AI、IM）
├── pkg/                    # Go 公共库（存储、gRPC、监控、网络能力）
├── agent-rust/             # Rust Agent 与 eBPF 数据面
├── frontend-refactor/      # Vue 3 管理后台
├── deployments/            # 部署与监控配置（Ansible、systemd、monitoring）
├── configs/                # Redis/PostgreSQL 等配置样例
├── scripts/                # 证书、测试、部署辅助脚本
├── docs/                   # 关键架构、部署、gRPC 测试文档
└── Makefile                # 构建入口
```

## 文档索引

- `docs/README.md` - 主要技术文档导航
- `agent-rust/README.md` - Rust Agent 架构说明
- `agent-rust/BUILD-GUIDE.md` - Rust Agent 编译指南
- `frontend-refactor/DESIGN-SYSTEM.md` - 前端设计规范

## 监控

访问 Grafana: `http://<controller>/grafana`

预置仪表盘：
- Aria Overview - 系统总览
- Bandwidth Control - 带宽监控
- Firewall - 防火墙统计

## 系统要求

### Controller
- Docker 20.10+
- PostgreSQL 12+
- Redis 6+
- 2GB+ 内存

### Agent
- Linux 5.4+（支持 eBPF）
- WireGuard
- root 权限

## License

MIT
