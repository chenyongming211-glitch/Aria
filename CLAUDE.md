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

### 7. 🤖 Rust Agent 部署（重要）

当用户说"部署 Agent"或类似指令时，**必须**按以下流程执行：

#### 完整部署流程（每个节点单独编译）

**第一步：同步源码到所有节点**
```bash
# 同步修改的源码文件到所有 Agent 节点
# 示例：同步单个文件
rsync -avz agent-rust/agent/src/config.rs root@146.56.196.231:/root/agent-rust/agent/src/
rsync -avz agent-rust/agent/src/config.rs root@118.195.135.16:/root/agent-rust/agent/src/

# 或同步整个 src 目录
rsync -avz --delete agent-rust/agent/src/ root@146.56.196.231:/root/agent-rust/agent/src/
rsync -avz --delete agent-rust/agent/src/ root@118.195.135.16:/root/agent-rust/agent/src/
```

**第二步：在每个节点上完全重新编译**
```bash
# ⚠️ 重要：必须先清理，再编译，避免增量编译问题
# sh 节点编译
ssh root@146.56.196.231 "source ~/.cargo/env && cd /root/agent-rust && cargo clean && cargo build --release"

# bj 节点编译
ssh root@118.195.135.16 "source ~/.cargo/env && cd /root/agent-rust && cargo clean && cargo build --release"
```

**第三步：原子替换并重启**
```bash
# sh 节点
ssh root@146.56.196.231 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria && systemctl restart aria"

# bj 节点
ssh root@118.195.135.16 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria && systemctl restart aria"
```

**第四步：验证部署**
```bash
# sh 节点验证
ssh root@146.56.196.231 "aria --version && aria status && aria peers"

# bj 节点验证
ssh root@118.195.135.16 "aria --version && aria status && aria peers"
```

#### 为什么要每个节点单独编译？

**重要原因：**
- ✅ **glibc 版本不兼容**：不同虚拟机的 glibc 版本可能不同（如 Ubuntu 22.04 vs 24.04）
- ✅ **避免运行时错误**：高版本 glibc 编译的二进制无法在低版本系统运行
- ✅ **依赖库差异**：不同系统的系统库版本可能不同

**错误示例：**
```
/lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.39' not found
```

#### 为什么必须 cargo clean？

**重要原因：**
- ✅ **避免增量编译问题**：Rust 增量编译可能导致某些代码更改未生效
- ✅ **确保完全重新编译**：清理所有中间产物，保证编译结果正确
- ✅ **避免奇怪的运行时错误**：增量编译有时会导致不可预测的行为

**增量编译问题示例：**
- 新增的代码逻辑未生效
- 输出格式未更新
- 运行时出现莫名其妙的错误

**正确做法：**
```bash
# ❌ 错误：增量编译
cargo build --release

# ✅ 正确：完全重新编译
cargo clean && cargo build --release
```
- ✅ **依赖库差异**：不同系统的系统库版本可能不同

**错误示例：**
```
/lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.39' not found
```

#### 关键规则

* **必须在每个节点编译**：禁止跨节点复制二进制文件
* **必须先清理再编译**：使用 `cargo clean && cargo build --release`，避免增量编译问题
* **原子替换**：使用 `mv -f` 原子替换，无需停止服务
* **先替换后重启**：必须先替换二进制文件，再通过 systemctl 重启
* **验证部署**：每次部署后必须验证版本和状态

#### Agent 服务器信息

| 节点 | IP | Region | VPN IP | OS |
|------|-----|--------|--------|-----|
| sh | 146.56.196.231 | sh | 100.64.0.1 | Ubuntu 24.04 |
| bj | 118.195.135.16 | bj | 100.64.0.2 | Ubuntu 22.04 |

#### 快速部署脚本

创建一键部署脚本 `scripts/deploy-rust-agent.sh`：
```bash
#!/bin/bash
# 快速部署 Rust Agent 到所有节点

echo "=== Step 1: Syncing source code ==="
rsync -avz --delete agent-rust/agent/src/ root@146.56.196.231:/root/agent-rust/agent/src/
rsync -avz --delete agent-rust/agent/src/ root@118.195.135.16:/root/agent-rust/agent/src/

echo "=== Step 2: Building on sh node ==="
ssh root@146.56.196.231 "source ~/.cargo/env && cd /root/agent-rust && cargo clean && cargo build --release"

echo "=== Step 3: Building on bj node ==="
ssh root@118.195.135.16 "source ~/.cargo/env && cd /root/agent-rust && cargo clean && cargo build --release"

echo "=== Step 4: Deploying to sh node ==="
ssh root@146.56.196.231 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria && systemctl restart aria"

echo "=== Step 5: Deploying to bj node ==="
ssh root@118.195.135.16 "cp /root/agent-rust/target/release/aria-agent /usr/local/bin/aria.new && chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria && systemctl restart aria"

echo "=== Step 6: Verifying deployment ==="
ssh root@146.56.196.231 "aria --version && aria peers"
ssh root@118.195.135.16 "aria --version && aria peers"

echo "✅ Deployment complete!"
```

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

---

## 13. 🚨 Agent 部署避坑指南（重要）

### 13.1 核心原则

**⚠️ 禁止随意重建容器**
- Controller 容器更新时，**不要删除重建**，会导致配置丢失
- 使用 `docker restart` 重启容器
- 只有镜像本身需要更新时，才使用 `docker rm` + `docker run`
- 更新时确保环境变量和卷挂载与之前完全一致

---

### 13.2 Controller gRPC 配置（必坑）

#### 问题 1: 端口未映射

**症状：** Agent 报错 "Connection refused" 或 "Failed to connect to Controller"

**原因：** Controller 容器未映射 50051 端口到宿主机

**解决方案：**
```bash
# 启动 Controller 时必须添加端口映射
docker run -d \
    --name aria_controller \
    --network aria_shared_net \
    -p 50051:50051 \  # ← 必须添加
    ...
```

#### 问题 2: gRPC 证书配置错误（最常见）

**症状：** 
- "invalid peer certificate: UnknownIssuer"
- "certificate not valid for name"
- "peer closed connection without sending TLS close_notify"

**原因：** Controller 默认使用 `server.crt`（公网 HTTPS 证书），但 gRPC 应该使用 `grpc-server.crt`

**解决方案：**
```bash
# ❌ 错误：不指定环境变量，会使用默认的 server.crt
docker run -d --name aria_controller ...

# ✅ 正确：通过环境变量指定 gRPC 专用证书
docker run -d \
    --name aria_controller \
    --network aria_shared_net \
    -p 50051:50051 \
    -v /root/aria-controller/certs:/etc/aria/certs:ro \
    -e ARIA_GRPC_SERVER_CERT=/etc/aria/certs/grpc-server.crt \
    -e ARIA_GRPC_SERVER_KEY=/etc/aria/certs/grpc-server.key \
    -e ARIA_GRPC_CA_CERT=/etc/aria/certs/ca.crt \
    ...
```

**证书说明：**
- `server.crt` / `server.key`: 公网 HTTPS 证书（用于 Web UI）
- `grpc-server.crt` / `grpc-server.key`: gRPC mTLS 证书（用于 Agent 通信）
- `ca.crt`: CA 根证书（用于验证客户端证书）

---

### 13.3 Agent 证书配置（必坑）

#### 问题 1: 证书路径配置缺失

**症状：** "missing field `ca_cert`" 或 "missing field `client_cert`"

**原因：** Rust Agent 配置文件必须包含完整的证书路径

**正确的配置文件：** `/etc/aria/agent.yaml`
```yaml
controller_url: https://112.124.8.241:50051  # ← 必须包含端口号
ca_cert: /etc/aria/certs/ca/ca.crt            # ← CA 证书
client_cert: /etc/aria/certs/agents/agent-<region>.crt  # ← 客户端证书
client_key: /etc/aria/certs/agents/agent-<region>.key   # ← 客户端私钥
device_id: null
private_key: <WireGuard 私钥>
public_key: <WireGuard 公钥>
assigned_ip: null
interface_name: aria0
listen_port: 51820
mtu: 1360
region: <region>
advertised_routes:
  - 2.2.2.0/24
  - 3.3.3.0/24
  - 8.8.8.0/24
sync_interval: 5
```

#### 问题 2: TLS 域名验证失败

**症状：** "certificate not valid for name"

**原因：** Agent 代码中的 `domain_name` 与 Controller 服务器证书的 CN/SAN 不匹配

**解决方案：**
检查 `agent-rust/agent/src/grpc_client.rs` 中的配置：
```rust
let tls_config = ClientTlsConfig::new()
    .ca_certificate(ca)
    .identity(identity)
    .domain_name("aria-controller");  // ← 必须与服务器证书匹配
```

**验证证书：**
```bash
# 查看服务器证书的 CN 和 SAN
openssl x509 -in /etc/aria/certs/grpc-server.crt -text -noout | grep -E '(Subject:|DNS:)'

# 测试 TLS 连接
openssl s_client -connect 112.124.8.241:50051 \
    -CAfile /etc/aria/certs/ca/ca.crt \
    -cert /etc/aria/certs/agents/agent-sh.crt \
    -key /etc/aria/certs/agents/agent-sh.key
```

#### 问题 3: 客户端证书被拒绝

**症状：** "peer closed connection without sending TLS close_notify"

**可能原因：**
1. 客户端证书 CN 不在 Controller 的白名单中
2. Controller 不信任该客户端证书
3. 证书过期或无效

**排查方法：**
```bash
# 查看 Controller 日志
docker logs aria_controller 2>&1 | grep -E '(TLS|certificate|client)'

# 使用 strace 跟踪 Agent
strace -s 500 -e trace=network,openat \
    /usr/local/bin/aria up --interface=eth0 --config=/etc/aria/agent.yaml 2>&1 \
    | grep -E '(connect|cert|TLS)'
```

---

### 13.4 编译和部署问题

#### 问题 1: glibc 版本不兼容

**症状：** 
```
/lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.39' not found
```

**原因：** 编译节点的 glibc 版本高于目标节点（如 Ubuntu 24.04 → 22.04）

**解决方案：**
- **方案一（推荐）：** 在目标节点上编译
  ```bash
  # 在每个目标节点上编译
  ssh root@<node> "source ~/.cargo/env && cd /root/agent-rust && cargo build --release"
  ```

- **方案二：** 使用静态链接
  ```bash
  # 修改 Cargo.toml
  [target.x86_64-unknown-linux-musl]
  linker = "rust-lld"
  
  # 编译
  cargo build --release --target x86_64-unknown-linux-musl
  ```

#### 问题 2: 端口冲突

**症状：** 
```
Error: Failed to initialize metrics
Caused by: Address already in use (os error 98)
```

**原因：** 9090 端口被其他 aria 服务占用

**解决方案：**
```bash
# 查找占用进程
netstat -tlnp | grep :9090

# 停止冲突服务
systemctl stop aria-rust aria-ebpf
systemctl disable aria-rust aria-ebpf

# 或杀掉占用进程
pkill -9 -f 'aria-rust'
```

#### 问题 3: 数据库脏数据导致启动失败（⚠️ 最常见）

**症状：**
```
Error: Failed to connect to Controller
Caused by: Permission denied (os error 13)
```

或

```
Error: status: Unknown, message: "sync failed: node not found: sql: Scan error on column index 8, name \"vpc_id\": converting NULL to string is unsupported"
```

**根本原因：**
1. **数据库中存储了错误的公钥**：之前的部署在数据库中留下了旧的公钥记录，与配置文件中的新公钥不匹配
2. **NULL字段导致扫描失败**：`vpc_id` 或 `tenant_id` 等字段为NULL，Controller的Go代码无法正确扫描

**检查方法：**
```bash
# 1. 检查数据库中的公钥是否与配置文件一致
docker exec aria_postgres psql -U aria -d aria -c \
  "SELECT region, public_key, assigned_ip FROM nodes;"

# 2. 检查配置文件中的公钥
cat /etc/aria/agent.yaml | grep public_key

# 3. 比较两者是否一致
```

**解决方案：**
```bash
# 1. 删除脏数据
docker exec aria_postgres psql -U aria -d aria -c \
  "DELETE FROM nodes WHERE region = '<region>' AND public_key = '<错误的公钥>';"

# 2. 使用正确的公钥插入新记录
docker exec aria_postgres psql -U aria -d aria -c \
  "INSERT INTO nodes (id, public_key, machine_id, region, assigned_ip, endpoint, private_ip, public_ip, hostname, last_seen, registered_at, role, runtime_mode, status, advertised_routes, vpc_id, tenant_id) VALUES (gen_random_uuid(), '<正确的公钥>', '<region>-node', '<region>', '<IP>', '<endpoint>', '<IP>', '<IP>', '<hostname>', EXTRACT(EPOCH FROM NOW())::bigint, EXTRACT(EPOCH FROM NOW())::bigint, 'agent', 'kernel', 'online', ARRAY['2.2.2.0/24'], 'default', '00000000-0000-0000-0000-000000000001');"

# 3. 验证插入结果
docker exec aria_postgres psql -U aria -d aria -c \
  "SELECT region, public_key, vpc_id, tenant_id FROM nodes;"
```

**预防措施：**
- ✅ 每次重新部署前，先检查数据库中是否有该节点的旧记录
- ✅ 如果有旧记录，先删除再部署
- ✅ 确保所有必需字段（`vpc_id`, `tenant_id`）都有有效值
- ❌ 不要直接修改配置文件中的密钥而不更新数据库

#### 问题 4: systemd 安全限制导致证书访问失败（⚠️ 必坑）

**症状：**
- 手动运行 `aria up` 成功
- systemd 启动服务失败：`Permission denied (os error 13)`
- 日志显示在 gRPC 连接阶段失败

**根本原因：**
systemd 的安全限制（`ProtectSystem=strict`, `ProtectHome=true`）过于严格，导致进程无法读取 `/etc/aria/certs/` 下的证书文件

**❌ 错误配置：**
```ini
[Service]
# 过于严格的安全限制
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/aria /var/log/aria /etc/wireguard /etc/aria /run
```

**✅ 正确配置：**
```ini
[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/aria up --interface=eth0 --config=/etc/aria/agent.yaml
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# 环境变量
Environment=RUST_LOG=info

# 资源限制
LimitNOFILE=65535
LimitNPROC=4096

# 安全加固 - 放宽限制以允许证书访问
NoNewPrivileges=false
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_BPF CAP_PERFMON
```

**验证方法：**
```bash
# 如果手动运行成功但服务启动失败，说明是 systemd 配置问题
# 1. 手动运行测试
/usr/local/bin/aria up --interface=eth0 --config=/etc/aria/agent.yaml

# 2. 如果手动成功，对比 systemd 配置
cat /etc/systemd/system/aria.service

# 3. 使用参考节点的配置
# 从正常运行的节点复制 systemd 配置
```

**关键要点：**
- ✅ 参考 sh 节点（146.56.196.231）的 systemd 配置
- ❌ 不要使用 `ProtectSystem=strict` 和 `ProtectHome=true`
- ✅ 只使用必要的 `AmbientCapabilities`

#### 问题 5: 手动测试留下残留进程

**症状：**
```
Error: Failed to initialize metrics
Caused by: failed to create HTTP listener: Address already in use (os error 98)
```

**原因：** 手动运行 `aria up` 测试后，进程没有正确停止，占用了 9090 端口

**解决方案：**
```bash
# 1. 查找所有 aria 进程
ps aux | grep aria

# 2. 杀掉所有 aria 进程
pkill -9 -f 'aria'

# 3. 验证端口已释放
netstat -tlnp | grep :9090

# 4. 重启服务
systemctl restart aria
```

**预防措施：**
- ✅ 手动测试完成后，务必用 `pkill` 清理进程
- ✅ 在重启 systemd 服务前，先检查是否有残留进程
- ❌ 不要用 `Ctrl+C` 后直接重启服务（可能留下僵尸进程）

---

### 13.5 Agent 更新注意事项（⚠️ 重要）

**在更新 Agent 前，必须按以下步骤检查，避免部署失败：**

#### 步骤 1: 检查数据库中的节点记录

```bash
# 查询数据库中的节点信息
docker exec aria_postgres psql -U aria -d aria -c \
  "SELECT region, public_key, assigned_ip, vpc_id, tenant_id FROM nodes;"
```

**检查要点：**
- ✅ 公钥是否与配置文件一致
- ✅ `vpc_id` 和 `tenant_id` 是否为有效值（不是NULL）
- ✅ `assigned_ip` 是否正确

#### 步骤 2: 对比配置文件中的公钥

```bash
# 检查配置文件中的公钥
cat /etc/aria/agent.yaml | grep public_key

# 比较数据库和配置文件
# 如果不一致，需要更新数据库
```

#### 步骤 3: 如果公钥不匹配，更新数据库

```bash
# 1. 删除旧记录
docker exec aria_postgres psql -U aria -d aria -c \
  "DELETE FROM nodes WHERE region = '<region>' AND public_key = '<旧的错误公钥>';"

# 2. 插入新记录（使用配置文件中的正确公钥）
docker exec aria_postgres psql -U aria -d aria -c \
  "INSERT INTO nodes (id, public_key, machine_id, region, assigned_ip, endpoint, private_ip, public_ip, hostname, last_seen, registered_at, role, runtime_mode, status, advertised_routes, vpc_id, tenant_id) VALUES (gen_random_uuid(), '<配置文件中的公钥>', '<region>-node', '<region>', '<IP>', '<endpoint>', '<IP>', '<IP>', '<hostname>', EXTRACT(EPOCH FROM NOW())::bigint, EXTRACT(EPOCH FROM NOW())::bigint, 'agent', 'kernel', 'online', ARRAY['2.2.2.0/24'], 'default', '00000000-0000-0000-0000-000000000001');"
```

#### 步骤 4: 清理残留进程

```bash
# 在更新前，清理所有aria进程
pkill -9 -f 'aria'

# 验证端口已释放
netstat -tlnp | grep :9090
```

#### 步骤 5: 验证 systemd 配置

```bash
# 确保 systemd 配置正确（不要使用过于严格的安全限制）
cat /etc/systemd/system/aria.service

# 参考 sh 节点的配置
# 如果不一致，从 sh 节点复制配置
```

#### 步骤 6: 更新并重启

```bash
# 原子替换二进制
rsync -avz /path/to/aria-agent root@<node>:/usr/local/bin/aria.new
ssh root@<node> "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"

# 重启服务
systemctl restart aria

# 验证服务状态
systemctl status aria

# 查看日志
journalctl -u aria -n 50
```

**常见更新失败原因：**
1. ❌ 数据库中有旧的错误公钥记录
2. ❌ `vpc_id` 或 `tenant_id` 为NULL
3. ❌ systemd 安全限制太严格（`ProtectSystem=strict`）
4. ❌ 手动测试留下的进程占用端口

**最佳实践：**
- ✅ 每次更新前，先查询数据库确认数据正确
- ✅ 如果修改了配置文件中的密钥，必须同步更新数据库
- ✅ 更新前清理所有残留进程
- ✅ 使用经过验证的 systemd 配置（参考 sh 节点）
- ✅ 更新后立即验证服务状态和日志

---

### 13.6 更新流程（避免重建）

#### Controller 更新（只更新二进制，不重建容器）

```bash
# ❌ 错误：删除重建容器
docker rm -f aria_controller
docker run -d --name aria_controller ...  # 会导致配置丢失

# ✅ 正确：只更新镜像并重启
docker buildx build --platform linux/amd64 \
    -t aria-controller:latest \
    -f Dockerfile.controller . --load

docker restart aria_controller  # 重启容器，配置保持不变
```

**只有在以下情况才重建容器：**
1. 需要修改环境变量
2. 需要修改卷挂载
3. 需要修改端口映射
4. 需要修改网络配置

#### Agent 更新（原子替换）

```bash
# ✅ 推荐：原子替换，无需停止服务
rsync -avz /path/to/aria-agent root@<node>:/usr/local/bin/aria.new
ssh root@<node> "chmod +x /usr/local/bin/aria.new && mv -f /usr/local/bin/aria.new /usr/local/bin/aria"

# 重启服务
ssh root@<node> "systemctl restart aria"
```

---

### 13.7 完整部署检查清单

**Controller 部署前：**
- [ ] gRPC 端口已映射（50051:50051）
- [ ] gRPC 证书环境变量已设置：
  - `ARIA_GRPC_SERVER_CERT=/etc/aria/certs/grpc-server.crt`
  - `ARIA_GRPC_SERVER_KEY=/etc/aria/certs/grpc-server.key`
  - `ARIA_GRPC_CA_CERT=/etc/aria/certs/ca.crt`
- [ ] 证书文件存在且可读
- [ ] 容器已加入 `aria_shared_net` 网络

**Agent 部署前：**
- [ ] 配置文件完整（包含所有必需字段）
- [ ] 证书路径正确（ca_cert, client_cert, client_key）
- [ ] controller_url 包含端口号（:50051）
- [ ] 证书文件存在且可读
- [ ] 9090 端口未被占用
- [ ] glibc 版本兼容（或本地编译）

**部署后验证：**
- [ ] 服务状态正常：`systemctl status aria`
- [ ] 日志无错误：`journalctl -u aria -n 50`
- [ ] WireGuard 接口存在：`ip addr show aria0`
- [ ] 可以 ping 通其他节点：`ping 100.64.0.x`

---

### 13.8 快速故障排查

**Agent 启动失败：**
```bash
# 1. 查看详细日志
journalctl -u aria -n 100 --no-pager

# 2. 检查配置文件
cat /etc/aria/agent.yaml | grep -E '(controller_url|ca_cert|client_cert)'

# 3. 测试网络连接
telnet 112.124.8.241 50051

# 4. 测试 TLS 连接
openssl s_client -connect 112.124.8.241:50051 \
    -CAfile /etc/aria/certs/ca/ca.crt \
    -cert /etc/aria/certs/agents/agent-<region>.crt \
    -key /etc/aria/certs/agents/agent-<region>.key

# 5. 验证证书
openssl verify -CAfile /etc/aria/certs/ca/ca.crt \
    /etc/aria/certs/agents/agent-<region>.crt
```

**Controller 连接失败：**
```bash
# 1. 检查容器状态
docker ps | grep aria_controller

# 2. 检查端口映射
docker port aria_controller

# 3. 查看 Controller 日志
docker logs aria_controller 2>&1 | grep -E '(gRPC|TLS|50051)'

# 4. 进入容器检查
docker exec -it aria_controller sh
netstat -tlnp | grep 50051
cat /etc/aria/certs/grpc-server.crt | openssl x509 -text -noout | grep Subject
```

---

**最后更新**: 2026-03-04  
**版本**: 0.2.26-test-11  
**维护者**: Aria Team
