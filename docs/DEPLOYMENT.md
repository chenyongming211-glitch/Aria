# Aria 部署文档

## 当前架构

```
┌─────────────────────────────────────────────────────────────┐
│                     112.124.8.241 (Controller)             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐    │
│  │ aria_web    │  │ aria_       │  │ aria_postgres  │    │
│  │ (nginx)     │──│ controller  │──│ (16-alpine)    │    │
│  │ :80/:443    │  │ :8080/:50051│  │ :5432          │    │
│  └─────────────┘  └─────────────┘  └─────────────────┘    │
│         │                │                  │              │
│         └────────────────┼──────────────────┘              │
│                    aria_shared_net (172.30.0.0/16)       │
└─────────────────────────────────────────────────────────────┘
         │
         │ gRPC (:50051)
         ▼
┌─────────────────┐    ┌─────────────────┐
│ 146.56.196.231  │    │ 118.195.135.16 │
│ (Agent sh)      │◀──▶│ (Agent bj)     │
│ WireGuard :51820│    │ WireGuard :51820│
└─────────────────┘    └─────────────────┘
```

## 1. Controller 部署（使用 docker compose）

### 1.1 目录结构

```
/root/aria-controller/
├── docker-compose.yml    # 容器编排配置
├── .env                  # 环境变量
├── config/
│   └── controller.yaml   # Controller配置
├── certs/               # 证书目录
│   ├── grpc-server.crt
│   ├── grpc-server.key
│   └── ca.crt
└── bin/images/          # Docker镜像
    └── aria-controller-latest.tar
```

### 1.2 docker-compose.yml

```yaml
# 模板位置: deployments/ansible/roles/controller/templates/docker-compose.yml.j2

version: '3.8'

services:
  aria-controller:
    image: aria-controller:latest
    container_name: aria_controller
    restart: unless-stopped
    ports:
      - "50051:50051"
    volumes:
      - ./config/controller.yaml:/etc/aria/controller.yaml:ro
      - ./certs:/etc/aria/certs:ro
    env_file:
      - .env
    networks:
      - aria_shared_net

networks:
  aria_shared_net:
    external: true
```

### 1.3 .env 文件

```bash
# 模板位置: deployments/ansible/roles/controller/templates/controller.env.j2

# 数据库
POSTGRES_HOST=aria-postgres
POSTGRES_PORT=5432
POSTGRES_USER=aria
POSTGRES_PASSWORD=aria-local-password
POSTGRES_DATABASE=aria

# Redis
REDIS_ADDR=aria-redis:6379

# gRPC TLS
ARIA_GRPC_TLS_MODE=disabled
ARIA_GRPC_SERVER_CERT=/etc/aria/certs/grpc-server.crt
ARIA_GRPC_SERVER_KEY=/etc/aria/certs/grpc-server.key
ARIA_GRPC_CA_CERT=/etc/aria/certs/ca.crt

# DeepSeek AI
DEEPSEEK_API_KEY=sk-xxx
DEEPSEEK_BASE_URL=https://api.deepseek.com
DEEPSEEK_MODEL=deepseek-chat

# 飞书
FEISHU_APP_ID=cli_xxx
FEISHU_APP_SECRET=xxx
FEISHU_VERIFY_TOKEN=xxx
```

### 1.4 部署命令

```bash
# 创建网络
docker network create --subnet=172.30.0.0/16 aria_shared_net 2>/dev/null || true

# 启动服务
cd /root/aria-controller
docker compose up -d
```

### 1.5 TLS模式说明

| 模式 | ARIA_GRPC_TLS_MODE | 说明 |
|------|-------------------|------|
| 禁用 | `disabled` | HTTP明文传输（测试用） |
| 单向TLS | `server` | 只验证服务端证书（推荐） |
| 双向mTLS | `mutual` | 双向证书验证 |

---

## 2. Agent 部署

### 2.1 配置文件 `/etc/aria/agent.yaml`

```yaml
controller_url: https://aria.yun:50051
ca_cert: /etc/aria/certs/ca/ca.crt
client_cert: /etc/aria/certs/agents/agent-sh.crt
client_key: /etc/aria/certs/agents/agent-sh.key
private_key: <WireGuard私钥>
public_key: <WireGuard公钥>
assigned_ip: "100.64.0.1"
address: "100.64.0.1/32"
interface_name: aria0
listen_port: 51820
mtu: 1360
region: sh
sync_interval: 5
multi_tunnel: true
```

### 2.2 systemd 服务

```ini
[Unit]
Description=Aria SD-WAN Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/aria up --interface=eth0 --config=/etc/aria/agent.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 2.3 启动

```bash
systemctl daemon-reload
systemctl enable aria
systemctl start aria
```

---

## 3. 更新流程

### 3.1 更新 Controller

```bash
# 1. 本地构建
cd /root/aria-project
make build
docker buildx build --platform linux/amd64 -t aria-controller:latest -f Dockerfile.controller . --load

# 2. 保存镜像
docker save aria-controller:latest -o bin/images/aria-controller-latest.tar

# 3. 上传到服务器
rsync -avz bin/images/aria-controller-latest.tar root@112.124.8.241:/root/aria-controller/bin/images/

# 4. 服务器部署（使用 docker compose）
ssh root@112.124.8.241 "cd /root/aria-controller && \
    docker compose down && \
    docker load -i bin/images/aria-controller-latest.tar && \
    docker compose up -d"
```

### 3.2 使用 Ansible（推荐）

```bash
cd deployments/ansible
ansible-playbook -i inventory.yaml playbooks/deploy-controller.yml
```

---

## 4. 节点信息

| 角色 | IP | Region | VPN IP |
|------|-----|--------|--------|
| Controller | 112.124.8.241 | - | - |
| Agent | 146.56.196.231 | sh | 100.64.0.1 |
| Agent | 118.195.135.16 | bj | 100.64.0.2 |

---

## 5. 常用命令

```bash
# Controller
docker compose logs -f aria_controller
docker compose restart aria_controller

# Agent
systemctl status aria-agent
systemctl restart aria-agent
aria-agent peers
```
--

## 5. 常用命令

```bash
# Controller
docker compose logs -f aria_controller
docker compose restart aria_controller

# Agent
systemctl status aria
systemctl restart aria
aria peers
```
