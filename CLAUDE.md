# Aria 开发规范

Aria 是一个去中心化组网系统，包含控制面（Controller）和边缘节点（Agent）。
**请严格遵守以下开发规范。**

---

## 目录结构总览

```
Aria/
├── cmd/                # 【源码】程序入口 (package main)
│   ├── controller/     # Controller 程序入口 → 编译成 bin/controller
│   └── agent/          # Agent 程序入口 → 编译成 bin/agent
├── pkg/                # 【源码】公共库代码 (可被外部引用)
├── internal/           # 【源码】私有业务逻辑 (不可被外部引用)
│   ├── protocol/       # 自定义协议处理
│   ├── routing/        # 路由选择算法
│   └── vpn/            # 隧道封装逻辑
├── configs/            # 配置文件 (yaml/json)
├── deployments/        # 部署配置 (systemd, docker)
├── scripts/            # 编译和管理脚本
├── bin/                # 【产物】开发编译产物 (临时)
├── dist/               # 【产物】打包临时产物 (临时)
├── releases/           # 【产物】正式发布包 (永久)
│   ├── deploy/         # 部署脚本和 Docker 镜像
│   └── <version>/      # 版本化归档
├── Makefile            # 核心构建入口
└── CLAUDE.md           # 本文件
```

---

## 输出风格

- **默认简洁输出**，只显示关键结果，需要详细信息时用户会主动询问
- 打包完成后只输出版本号和产物路径，不自动输出部署命令
- 部署命令、目录结构等详细信息，等用户询问时再提供
- **永远使用中文回复**

---

## 核心约束（必须遵守）

### 1. cmd/ 与 pkg/ 的区别

| 目录 | package 名 | 有 main() | 编译结果 | 用途 |
|------|-----------|-----------|----------|------|
| `cmd/` | `main` | ✅ 有 | 可执行文件 | 程序入口 |
| `pkg/` | 自定义 | ❌ 无 | 被导入使用 | 可复用库 |

**规则：**
- 新增可执行程序 → 在 `cmd/<程序名>/main.go` 创建
- 新增功能模块 → 在 `pkg/<模块名>/` 创建
- 项目内部专用逻辑 → 在 `internal/<模块名>/` 创建

### 2. 三层产物分离（禁止混放）

| 目录 | 用途 | 生成命令 | make clean | 可提交 Git |
|------|------|----------|------------|-----------|
| `bin/` | 开发调试 | `make build` | ✅ 删除 | ❌ 禁止 |
| `dist/` | CI/CD 打包 | `make package-*` | ✅ 删除 | ❌ 禁止 |
| `releases/` | 正式发布 | `make release` | ❌ 保留 | ❌ 禁止 |

**规则：**
- 编译产物只能放 `bin/`
- 打包产物只能放 `dist/`
- 正式发布只能放 `releases/`
- **禁止在根目录生成任何二进制文件或压缩包**

### 3. 编译规范

**❌ 禁止：**
```bash
go build ./cmd/agent              # 会在根目录生成二进制
go build ./cmd/controller         # 会在根目录生成二进制
```

**✅ 正确：**
```bash
make build                        # 推荐：自动输出到 bin/
go build -o bin/agent ./cmd/agent # 手动指定输出目录
```

### 4. Docker 镜像规范

Controller 使用容器部署，**默认打包 linux/amd64 架构**（除非用户明确指定其他架构）：

```bash
# 构建镜像（默认 amd64）
docker buildx build --platform linux/amd64 -t aria-controller:latest -f Dockerfile.controller . --load

# 导出镜像到 releases/
docker save aria-controller:latest -o releases/deploy/controller-web/images/aria-controller.tar
```

**禁止在根目录存放 .tar 镜像文件。**

---

## 版本管理（重要）

### 单一数据源原则

**版本号只在 VERSION 文件中维护**，其他位置通过自动化同步。

版本号存在的位置：
- `VERSION` 文件 ← **唯一修改点**
- `internal/cli/root.go` ← 自动同步
- `Dockerfile.controller` ← 自动同步
- `cmd/ariactl/main.go` ← 自动同步

### 版本更新流程

```bash
# 步骤 1：更新 VERSION 文件
echo "0.2.0" > VERSION

# 步骤 2：自动同步到所有文件
make sync-version

# 步骤 3：构建和发布
make release VERSION=0.2.0
```

### 自动同步工具

```bash
# 同步版本号
make sync-version

# 或直接运行脚本
./scripts/sync-version.sh
```

脚本会自动更新所有需要版本号的文件。

### 重要提醒

❌ **禁止手动修改**:
- `internal/cli/root.go` 中的 `version = "x.x.x"`
- `Dockerfile.controller` 中的 `ARG VERSION=x.x.x"`

✅ **正确做法**:
1. 只修改 `VERSION` 文件
2. 运行 `make sync-version`
3. 构建和发布

详细文档：`docs/VERSION-MANAGEMENT.md`

### 代码约束（必须遵守）

**❌ 禁止在代码中硬编码版本号**:
```go
// 错误示例
version = "0.2.15"  // 硬编码版本号
```

**✅ 正确做法：使用 ldflags 注入**:
```go
// 正确示例：使用默认值，通过编译时注入
var version = "dev"  // 默认开发版本
```

**版本号注入方式**:
```bash
# 通过 ldflags 注入版本
go build -ldflags "-X main.version=${VERSION} -X aria/pkg/mcp.version=${VERSION}" ./cmd/mcp-server
```

**需要注入版本的位置**:
- `cmd/*/main.go` → `main.version`
- `internal/cli/root.go` → `aria/internal/cli.Version`
- `pkg/mcp/host.go` → `aria/pkg/mcp.Version`

---

## 用户指令识别

当用户说以下内容时，按对应方式处理：

| 用户说 | 意图 | 版本号 | 执行命令 |
|--------|------|--------|----------|
| "编译" / "build" | 本地开发调试 | - | `make build` |
| "打包测试" / "上线测试" | 测试版本 | `x.x.x-test` | 完整打包流程 |
| "打包版本" / "正式发布" | 正式版本 | `x.x.x` | 完整打包流程（去掉 -test） |
| "打包 agent" | 只打包 Agent | 按上下文 | `make release-deploy` |
| "打包 controller" | 只打包 Controller | 按上下文 | `make release-deploy` + Docker |

**版本号规则：**
- "打包测试" → 版本号带 `-test` 后缀（如 `0.1.0-test`）
- "打包版本" → 正式版本号（如 `0.1.0`），通常在测试完成后执行

### 打包流程

1. 确认版本号（测试版带 `-test`，正式版不带）
2. 更新 `VERSION` 文件
3. 执行 `make build-linux-amd64` 编译 Agent
4. 构建 Docker 镜像（Controller）：`docker buildx build --platform linux/amd64 --no-cache -t aria-controller:<版本> -t aria-controller:latest -f Dockerfile.controller . --load`
5. **创建版本归档：**
   ```bash
   mkdir -p releases/<版本>
   cp bin/aria-linux-amd64 releases/<版本>/aria
   echo "<版本>" > releases/<版本>/VERSION
   ```
6. **同步部署包（必须）：**
   ```bash
   # Agent
   cp bin/aria-linux-amd64 releases/deploy/agent/aria
   # Controller 镜像
   docker save aria-controller:latest -o releases/deploy/controller-web/images/aria-controller.tar
   # UI 文件（从源码同步）
   cp -r deployments/controller-web/ui-dist/* releases/deploy/controller-web/ui-dist/
   ```
7. 线上部署使用 `releases/deploy/` 目录下的文件

### 版本管理

- 版本号存储在 `VERSION` 文件
- 可通过命令行覆盖：`make release VERSION=x.x.x`
- 自动保留最近 5 个历史版本 + 最新版本
- 旧版本在每次 release 时自动清理

---

## 常用命令速查

| 命令 | 说明 | 输出位置 |
|------|------|----------|
| `make build` | 编译所有组件 | `bin/` |
| `make build-linux-amd64` | 交叉编译 Linux | `bin/` |
| `make package-deb` | 生成 DEB 包 | `dist/` |
| `make package-tarball` | 生成 tarball | `dist/` |
| `make release` | 创建正式发布 | `releases/<version>/` |
| `make release-deploy` | 准备部署包 | `releases/deploy/` |
| `make clean` | 清理临时产物 | 删除 bin/ dist/ |
| `make test` | 运行测试 | - |
| `make help` | 查看所有命令 | - |

---

## CHANGELOG 管理规范

### 规则

1. **所有版本更新必须记录到 `CHANGELOG.md`**
2. **新版本内容添加到文件开头**（最新版本在最上面）
3. **不要创建单独的版本 CHANGELOG 文件**（如 `CHANGELOG-v0.2.0.md`）
4. **直接更新 `CHANGELOG.md`**

### 格式要求

```markdown
# CHANGELOG

## [版本号] - YYYY-MM-DD

### 🚀 性能优化
- 具体优化项
- 性能数据对比

### ✨ 新增功能
- 功能描述

### 🔧 技术改进
- 技术细节

### 🐛 Bug 修复
- 修复内容

### 📝 文件变更
- 修改的文件列表

---

## [上一个版本] - YYYY-MM-DD
...
```

### 示例

```markdown
# CHANGELOG

## [v0.2.0] - 2026-02-11

### 🚀 性能优化

#### 1. 无状态防火墙 + NOTRACK
- CPU 占用：-67% (15% → 5%)
- 延迟 P99：-75% (2ms → 0.5ms)
...

---

## [v0.1.0] - 2026-01-15
...
```

### 注意事项

- ✅ 包含性能数据对比
- ✅ 包含文件变更清单
- ✅ 包含向后兼容性说明
- ✅ 包含已知限制
- ❌ 不要创建临时 CHANGELOG 文件
- ❌ 不要分散在多个文件

---

## Git 规范

**.gitignore 已配置忽略：**
- `bin/` - 编译产物
- `dist/` - 打包产物
- `releases/` - 发布包
- `.env`, `*.pem`, `*.key` - 敏感文件
- `*.tar`, `*.tar.gz` - 压缩包

**提交前：**
```bash
make fmt     # 格式化代码
make lint    # 代码检查（需安装 golangci-lint）
```

---

## 代码风格

- 使用 `go fmt` 格式化
- 私有函数/变量使用小写开头
- 导出函数/变量使用大写开头
- 错误处理：`if err != nil { return err }`
- 注释使用中文或英文，保持一致

---

## 部署说明

### Controller（容器部署）
```bash
# 上传并部署
scp -r releases/deploy/controller root@<server>:/root/aria-controller/
ssh root@<server> "cd /root/aria-controller && ./deploy.sh primary"
```

### Agent（二进制部署）
```bash
# 上传并部署
scp -r releases/deploy/agent root@<server>:/root/aria-agent/
ssh root@<server> "cd /root/aria-agent && ./deploy.sh install http://<controller>:8080"
```

### 节点信息

| 角色 | IP | 地区 | 用途 |
|------|-----|------|------|
| Controller | 112.124.8.241 | - | 控制面 |
| Agent | 146.56.196.231 | sh | 边缘节点 |
| Agent | 118.195.135.16 | bj | 边缘节点 |

---

## 🚨 行为约束 - Claude Code 专用（必须遵守）

### 1. 📂 目录与文件结构规范

* **根目录整洁**：禁止在根目录创建临时文件或构建产物
* **二进制输出**：所有编译产物必须输出到 `./bin/`
  * 例如：`./bin/aria`, `./bin/aria-controller`
* **Docker Compose**：唯一合法的部署文件位于 `./deployments/docker-compose.yaml`
  * ❌ 禁止在根目录创建 `docker-compose.yml`
  * ❌ 禁止创建随意的 `deployment.yaml` 文件
* **源码目录**：
  * 入口点：`./cmd/`（如 `./cmd/agent`, `./cmd/controller`）
  * 业务逻辑：`./internal/`
  * 共享库：`./pkg/`

### 2. 🔨 构建与运行规范

* **Makefile 是法律**：所有构建和运维任务必须使用 `Makefile`，禁止自己发明原始命令
  * 编译：运行 `make build`（确保二进制输出到 `./bin/`）
  * 部署：运行 `make up`（确保使用正确的 docker-compose 文件）
  * 清理：运行 `make clean`
* **Go 版本**：目标 Go 1.22+

### 3. 🐳 部署与网络规范

* **统一网络**：所有容器（Web, Controller, Grafana, VM）必须加入外部桥接网络 `aria-shared-net`
* **端口暴露（安全）**：
  * ✅ **公共**：只有 `aria-web` (Nginx) 允许暴露端口 `80` 和 `443` 到宿主机
  * 🔒 **内部**：Controller (8080)、Grafana (3000)、VictoriaMetrics (8428) **禁止**映射端口到宿主机（`-p`），必须通过 Nginx 代理在 Docker 网络内访问
* **容器命名**：在 docker-compose 中使用固定容器名（如 `container_name: aria-controller`）

### 4. 💻 编码规范

* **防火墙实现**：使用 `nft` CLI 封装模式（exec 命令）。禁止使用原始 `google/nftables` Go 库处理复杂 sets/maps，以避免 Netlink 对齐问题
* **配置文件**：应用配置文件严格位于 `./config.yaml`
* **日志**：使用结构化日志

### 5. 🚨 行为约束

* **一致性**：在建议命令之前，先检查 Makefile 是否有对应的 target
* **环境假设**：始终假设用户在 `VM-0-4-ubuntu` Linux 环境中工作

### 6. 📦 跨机器部署规则（Controller 部署）

当用户说"部署新版本到 Controller"或类似指令时，**必须**按以下流程执行：

#### 完整部署流程

**第一步：本地构建**
```bash
# 1. 更新版本号（如 0.2.13-5）
echo "0.2.13-5" > VERSION

# 2. 同步版本号到各文件
make sync-version && make sync-ui

# 3. 构建 Controller 镜像
docker buildx build --platform linux/amd64 --no-cache \
  -t aria-controller:0.2.13-5 \
  -t aria-controller:latest \
  --build-arg VERSION=0.2.13-5 \
  -f Dockerfile.controller . --load

# 4. 验证版本
docker run --rm --platform linux/amd64 aria-controller:latest --version

# 5. 保存镜像到 bin/images/
make save-image
```

**第二步：上传到服务器**
```bash
# 传输镜像到服务器
rsync -avz bin/images/ root@112.124.8.241:/root/aria-controller/bin/images/
```

**第三步：服务器部署**
```bash
# 在服务器上执行
ssh root@112.124.8.241 "cd /root/aria-controller && ./deploy-controller.sh deploy"
```

#### 关键规则

* **镜像保存**：必须使用 `make save-image`，输出到 `bin/images/aria-controller-latest.tar`
* **镜像部署**：服务器上**必须**使用 `./deploy-controller.sh deploy`，禁止直接用 `docker compose up`
* **原因**：`docker compose up` 不会重新加载新镜像，`deploy-controller.sh` 会自动加载镜像并用 `--force-recreate` 重建容器

#### 服务器信息

| 角色 | IP | 部署目录 |
|------|-----|----------|
| Controller | 112.124.8.241 | /root/aria-controller |

### 7. 🤖 Agent 部署

当用户说"部署 Agent"或类似指令时，**必须**按以下流程执行：

#### 完整部署流程

**第一步：本地构建**
```bash
# 1. 编译 Agent（必须！）
make build-linux-amd64

# 2. 复制到部署目录
cp bin/aria-linux-amd64 releases/deploy/agent/aria
```

**第二步：上传到服务器**
```bash
# 上传到所有 Agent 服务器
rsync -avz releases/deploy/agent/ root@146.56.196.231:/root/aria-agent/
rsync -avz releases/deploy/agent/ root@118.195.135.16:/root/aria-agent/
```

**第三步：服务器部署**
```bash
# 在每个 Agent 服务器上执行
# 关键：必须先替换 /usr/local/bin/aria，再重启服务

# sh 节点
ssh root@146.56.196.231 "cp /root/aria-agent/aria /usr/local/bin/aria && systemctl restart aria"

# bj 节点
ssh root@118.195.135.16 "cp /root/aria-agent/aria /usr/local/bin/aria && systemctl restart aria"

# 验证版本
ssh root@146.56.196.231 "aria --version"
ssh root@118.195.135.16 "aria --version"
```

#### 优雅升级流程（原子替换，不中断业务）

```bash
# ========== 1. 本地编译 ==========
make build-linux-amd64
cp bin/aria-linux-amd64 releases/deploy/agent/aria

# ========== 2. 原子替换（推荐方式）==========
# 原理：mv 在同一文件系统下是原子操作，无需停止服务

# sh 节点
rsync -avz releases/deploy/agent/aria root@146.56.196.231:/usr/local/bin/aria.new
ssh root@146.56.196.231 "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"

# bj 节点
rsync -avz releases/deploy/agent/aria root@118.195.135.16:/usr/local/bin/aria.new
ssh root@118.195.135.16 "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"

# ========== 3. 重启服务 ==========
ssh root@146.56.196.231 "systemctl restart aria"
ssh root@118.195.135.16 "systemctl restart aria"

# ========== 4. 验证 ==========
ssh root@146.56.196.231 "aria --version && aria status"
ssh root@118.195.135.16 "aria --version && aria status"
```

#### 关键规则

* **必须先编译**：`make build-linux-amd64`，不能直接用旧二进制
* **原子替换**：使用 `mv -f` 在同一目录下操作，原子替换无需停机
* **同一目录**：临时文件和目标文件必须在同一挂载点，确保 mv 是原子操作
* **通过 systemctl 重启**：使用 `systemctl restart aria`，不要直接 `aria up`（会绕过服务管理）
* **数据面不断**：升级时 WireGuard tunnel 保持连接，只有控制面短暂中断

#### Agent 服务器信息

| 节点 | IP | Region | VPN IP |
|------|-----|--------|--------|
| sh | 146.56.196.231 | sh | 100.64.0.1 |
| bj | 118.195.135.16 | bj | 100.64.0.2 |

#### 创建 Token（首次部署或新增节点）

```bash
# 在 Controller 服务器上创建 Token
ssh root@112.124.8.241 "docker exec aria_controller aria token create --tag=agent-sh"
ssh root@112.124.8.241 "docker exec aria_controller aria token create --tag=agent-bj"

# 首次初始化 Agent
ssh root@146.56.196.231 "aria init --server=https://112.124.8.241 --token=<token> --region=sh --advertise-routes=2.2.2.0/24,3.3.3.0/24,8.8.8.0/24"
ssh root@118.195.135.16 "aria init --server=https://112.124.8.241 --token=<token> --region=bj --advertise-routes=2.2.2.0/24,3.3.3.0/24,8.8.8.0/24"
```

---

## 8. 🔄 无缝重启（避免重复隧道）

为避免 Agent 重启时 WireGuard 隧道重建导致产生重复隧道，代码实现了接口复用机制：

* **`ensureKernelInterface()` 优化**：当检测到接口已存在时，自动采用（adopt）现有接口并更新配置，而非重建
* **`GetAllAriaInterfaces()`**：获取所有现有 aria* 接口
* **多隧道模式**：支持 aria0, aria1, aria2, aria3 多个隧道同时存在

**验证无重复隧道**：
```bash
# 查看现有接口
ip link show | grep aria

# 应该看到类似输出（每个节点最多 4 个）
# aria0: <POINTOPOINT,MULTICAST,NOARP,UP,LOWER_UP>
# aria1: <POINTOPOINT,MULTICAST,NOARP,UP,LOWER_UP>
# ...
```

---

**最后更新：2026-02-17**
**版本：0.2.22-test-10**

---

## 9. 🤖 AI 与飞书集成（重要）

### DeepSeek 环境变量问题总结

**问题根源**：每次部署 Controller 时，环境变量没有正确传递到容器中，导致 AI 服务无法初始化。

**症状**：
- 飞书返回："AI 服务未正确配置，请检查 DEEPSEEK_API_KEY 环境变量"
- 日志显示：`[Feishu] AI 回复: AI 服务未正确配置，请检查 DEEPSEEK_API_KEY 环境变量`

**原因**：
部署脚本 `deploy-controller.sh` 使用了环境变量引用格式 `${VAR:-}`，但服务器 shell 中没有设置这些变量。

**解决方案**：
在部署脚本中**硬编码**所有 AI 和飞书相关的环境变量，而不是引用 shell 变量。

### 部署脚本环境变量配置

**文件位置**：`releases/deploy/controller-web/deploy-controller.sh` 和服务器上的 `/root/aria-controller/deploy-controller.sh`

**必须硬编码的环境变量**：

```bash
docker run -d \
    --name aria_controller \
    --network aria_shared_net \
    -v /root/aria-controller/config/controller.yaml:/etc/aria/controller.yaml:ro \
    -e POSTGRES_HOST=aria-postgres \
    -e POSTGRES_PORT=5432 \
    -e POSTGRES_USER=aria \
    -e POSTGRES_PASSWORD=aria-local-password \
    -e POSTGRES_DATABASE=aria \
    -e REDIS_ADDR=aria-redis:6379 \
    -e DEEPSEEK_API_KEY="sk-44fc7def9839422390f8d2018967081f" \
    -e DEEPSEEK_BASE_URL="https://api.deepseek.com" \
    -e DEEPSEEK_MODEL="deepseek-chat" \
    -e DEEPSEEK_SYSTEM_PROMPT='You are Aria'\''s intelligent operations assistant.' \
    -e FEISHU_APP_ID="cli_a9f4b69ae5b8dbb3" \
    -e FEISHU_APP_SECRET="TSbiUWpalWbbIZ7t6Je3IfUgcqtWOBvF" \
    -e FEISHU_ENCRYPT_KEY="" \
    -e FEISHU_VERIFY_TOKEN="X12NiE0nKEQltttec2p7ynlNHrBkjDvx" \
    --restart unless-stopped \
    aria-controller:latest \
    controller serve --config=/etc/aria/controller.yaml
```

### 错误示例（禁止使用）

```bash
# ❌ 错误：引用 shell 变量，服务器可能没有设置
-e DEEPSEEK_API_KEY="${DEEPSEEK_API_KEY:-}" \
-e FEISHU_APP_ID="${FEISHU_APP_ID:-}" \
```

### 正确示例（必须使用）

```bash
# ✅ 正确：硬编码值
-e DEEPSEEK_API_KEY="sk-44fc7def9839422390f8d2018967081f" \
-e FEISHU_APP_ID="cli_a9f4b69ae5b8dbb3" \
```

### 更新部署脚本的步骤

当需要更新 AI 或飞书配置时：

1. 修改 `releases/deploy/controller-web/deploy-controller.sh`
2. 上传到服务器：`scp releases/deploy/controller-web/deploy-controller.sh root@112.124.8.241:/root/aria-controller/`
3. 在服务器重新部署：`ssh root@112.124.8.241 "cd /root/aria-controller && ./deploy-controller.sh deploy"`

### 验证 AI 服务正常

```bash
# 查看容器环境变量
docker exec aria_controller env | grep -E '(DEEPSEEK|FEISHU)'

# 查看日志确认 AI 初始化
docker logs aria_controller 2>&1 | grep -E '(AI|DeepSeek|Agent)'
```

---

## 10. 🚀 新功能部署信息

### 端口级别控制功能（eBPF QoS）

**功能概述**：新增的端口级别流量控制功能，允许基于五元组（源IP、目标IP、源端口、目标端口、协议）进行精确的带宽限制。

**技术架构**：
- eBPF程序在XDP/TC层处理数据包
- 三级限速架构：服务级（最高优先级）→ Peer级 → IP级（最低优先级）
- 支持TCP/UDP头部解析，实现真正的五元组控制

**部署要求**：
- **仅需部署到Agent节点**，Controller无需更新
- Agent必须运行在支持eBPF的Linux内核上（推荐5.4+）
- 需要适当权限加载eBPF程序

**线上节点信息**：
- Controller: 112.124.8.241
- Agent sh节点: 146.56.196.231
- Agent bj节点: 118.195.135.16

**部署命令**：
```bash
# ========== 1. 本地编译（包含端口级别控制功能） ==========
make build-linux-amd64
cp bin/aria-linux-amd64 releases/deploy/agent/aria

# ========== 2. 原子替换到所有Agent节点 ==========
# sh 节点
rsync -avz releases/deploy/agent/aria root@146.56.196.231:/usr/local/bin/aria.new
ssh root@146.56.196.231 "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"

# bj 节点
rsync -avz releases/deploy/agent/aria root@118.195.135.16:/usr/local/bin/aria.new
ssh root@118.195.135.16 "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"

# ========== 3. 重启服务 ==========
ssh root@146.56.196.231 "systemctl restart aria && systemctl status aria"
ssh root@118.195.135.16 "systemctl restart aria && systemctl status aria"

# ========== 4. 验证新功能 ==========
ssh root@146.56.196.231 "aria ebpf status && aria --version"
ssh root@118.195.135.16 "aria ebpf status && aria --version"
```

**新CLI命令**：
- `aria ebpf qos limit-port [PORT] --mbps [bandwidth]`
- `aria ebpf qos limit-service [SRC_IP] [DST_IP] [SRC_PORT] [DST_PORT] --mbps [bandwidth] --protocol [protocol]`
- `aria ebpf qos limit-ip-port [IP] [PORT] --mbps [bandwidth] --direction [direction]`

**验证命令**：
```bash
# 检查eBPF支持
aria ebpf status

# 测试端口限速功能
aria ebpf qos limit-port 80 --mbps 100

# 测试五元组限速功能
aria ebpf qos limit-service 192.168.1.100 192.168.1.200 12345 80 --mbps 20 --protocol 6
```

---

## 11. 🌐 前端更新流程（重要）

当需要更新前端界面（如菜单排序、UI组件、样式等）时，请严格按照以下流程操作，**这是专门的前端更新方法，不需要更新整个 Controller 镜像**。

### 前端更新流程

**第一步：修改前端代码**
- 前端代码位于 `./frontend-refactor/` 目录
- 修改 Vue 组件、样式、路由等相关文件
- 例如修改菜单顺序在 `frontend-refactor/src/components/Layout/Layout.vue`

**第二步：构建前端项目**
```bash
# 进入前端目录
cd frontend-refactor

# 构建项目
npm run build

# 构建产物会输出到上级目录的 temp-dist/
```

**第三步：部署前端文件到服务器**
```bash
# 将构建产物同步到服务器的 UI 目录（注意：只同步内容，不是整个目录）
rsync -avz temp-dist/ root@112.124.8.241:/root/aria-controller/ui-dist/
```

**第四步：刷新 Web 服务（不需要重启 Controller）**
```bash
# 重启 Web 容器以加载新的前端文件
ssh root@112.124.8.241 "docker restart aria_web"
```

### 重要注意事项

- ✅ **前端更新不需要重新构建 Controller 镜像**
- ✅ **前端更新不需要重启 Controller 服务**
- ✅ **只需替换 UI 静态文件并通过重启 aria_web 容器使其生效**
- ❌ **不要通过更新整个 Controller 镜像来更新前端**

### 验证前端更新

访问 Aria 控制台，确认前端更改已生效：
- 检查界面元素是否按预期变化
- 确认浏览器没有使用旧的缓存版本
- 如有问题，可尝试清除浏览器缓存或强制刷新

### 故障排除

如果出现页面无法访问或其他问题：
1. 检查服务器上 `/root/aria-controller/ui-dist/` 目录是否有正确的前端文件
2. 确认 `aria_web` 容器运行正常：`docker ps | grep aria_web`
3. 必要时检查控制器和数据库连接状态（如果影响后端功能）

---

## 12. 🌐 API 规范（重要）

为确保API的一致性和可维护性，所有API开发必须遵循以下统一规范。**带宽管理和策略管理API除外，这两个模块将在后续重构中应用此规范。**

### 12.1 API基础路径规范
```
/api/v1/{resource}
```

### 12.2 统一响应格式
所有API响应必须使用以下标准格式：

```go
// 标准响应格式
type APIResponse struct {
    Success  bool                   `json:"success"`
    Data     interface{}           `json:"data,omitempty"`
    Message  string                `json:"message,omitempty"`
    Error    *APIError             `json:"error,omitempty"`
    Meta     *APIMeta              `json:"meta,omitempty"`
    Code     string                `json:"code,omitempty"` // 业务状态码
}

type APIError struct {
    Code    string            `json:"code"`     // 错误代码
    Message string            `json:"message"`  // 错误消息
    Details map[string]string `json:"details,omitempty"` // 详细错误信息
}

type APIMeta struct {
    Total    int `json:"total,omitempty"`     // 总数
    Page     int `json:"page,omitempty"`      // 页码
    PageSize int `json:"page_size,omitempty"` // 页大小
    Next     string `json:"next,omitempty"`   // 下一页链接
    Prev     string `json:"prev,omitempty"`   // 上一页链接
}
```

### 12.3 统一请求格式
```go
// 标准请求格式
type APIRequest struct {
    Data     interface{} `json:"data"`
    Metadata interface{} `json:"metadata,omitempty"`
}
```

### 12.4 HTTP方法使用规范
```
GET    /api/v1/resources     # 获取资源列表
POST   /api/v1/resources     # 创建资源
GET    /api/v1/resources/:id # 获取单个资源
PUT    /api/v1/resources/:id # 完整更新资源
PATCH  /api/v1/resources/:id # 部分更新资源
DELETE /api/v1/resources/:id # 删除资源
```

### 12.5 认证规范
- 统一使用`Authorization: Bearer <token>`头部
- 租户隔离通过中间件在请求上下文中处理
- 所有需要认证的API都应该有统一的认证中间件

### 12.6 错误码规范
```go
// 通用错误码
const (
    CodeOK                   = "SUCCESS"
    CodeBadRequest           = "BAD_REQUEST"
    CodeUnauthorized         = "UNAUTHORIZED"
    CodeForbidden            = "FORBIDDEN"
    CodeNotFound             = "NOT_FOUND"
    CodeInternalServerError  = "INTERNAL_ERROR"
    CodeValidationFailed     = "VALIDATION_FAILED"
    CodeRateLimitExceeded    = "RATE_LIMIT_EXCEEDED"
)
```

### 12.7 统一响应辅助函数
```go
// 统一响应辅助函数
func WriteSuccess(w http.ResponseWriter, data interface{}, message string) {
    resp := APIResponse{
        Success: true,
        Data:    data,
        Message: message,
        Code:    "SUCCESS",
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(resp)
}

func WriteError(w http.ResponseWriter, statusCode int, errorCode, message string, details map[string]string) {
    resp := APIResponse{
        Success: false,
        Message: message,
        Error: &APIError{
            Code:    errorCode,
            Message: message,
            Details: details,
        },
        Code: errorCode,
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(resp)
}

func WriteValidationError(w http.ResponseWriter, fieldErrors map[string]string) {
    WriteError(w, http.StatusBadRequest, "VALIDATION_FAILED", "请求参数验证失败", fieldErrors)
}
```

### 12.8 遵循规范的模块范围
当前需要遵循此API规范的模块包括：
- 租户管理API (`tenant_api.go`, `tenant_management.go`)
- AI聊天API (`chat.go`)
- 其他新开发的API模块

**不包括：**
- 带宽管理API (`qos.go`, `tenant_qos.go`) - 等待重构
- 策略管理API - 等待重构

### 12.9 实施要求
1. 新增API必须严格遵循此规范
2. 现有API应逐步按此规范进行改造
3. 所有API都需要包含适当的参数验证
4. 错误处理需保持一致
