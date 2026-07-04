# 代码 Bug 追踪

**最后复核**: 2026-06-27
**复核方式**: 全量静态代码复核 + 本地 Go/Frontend 回归验证
**说明**: 本文档记录“当前状态”，不再把历史 OPEN 条目和后续 FIXED 结论混写在一起。
**AI 范围决定**: AI Agent 后续计划替换为 Hermes Agent。本轮不扩展旧 AI 写操作闭环，但已把旧 AI 写入口后端 fail-closed：聊天只暴露只读工具，`ExecuteTool` 即使 `confirmed=true` 也拒绝旧写工具；Hermes 接入时再重新设计 pending confirmation、policy delivery 和审计链路。

---

## BUG-25 到 BUG-37 闭合总览

| Bug | 优先级 | 归属闭环 | 批次 | 当前状态 |
| --- | --- | --- | --- | --- |
| BUG-25 | P1 | AI / 运维确认 | B5 | ✅ FIXED: 旧 AI 聊天只暴露只读工具，旧写工具后端拒绝执行 |
| BUG-26 | P1 | AI / 路由控制 | B5 | ✅ FIXED: 旧 AI 路由写入口被禁用，Hermes 未来必须复用 policy delivery |
| BUG-27 | P2 | 控制闭环 / legacy 路由 | B2 | ✅ FIXED: `/api/v2/agents/network` 保留兼容入口，但内部进入 policy delivery |
| BUG-28 | P1 | 接入闭环 / 租户停用 | B1 | ✅ FIXED: enrollment/runtime token 都校验 tenant active |
| BUG-29 | P2 | 权限闭环 / 租户停用 | B1 | ✅ FIXED: login/refresh/permissions 对 inactive tenant fail closed |
| BUG-30 | P2 | 权限闭环 / 前端权限 | B1 | ✅ FIXED: 权限刷新失败不再回退本地内置写权限 |
| BUG-31 | P3 | 监控闭环 / inactive 节点 | B4 | ✅ FIXED: suspended/banned 不再被在线态、拓扑、流量统计当作 eligible online |
| BUG-32 | P1 | 控制闭环 / 节点编辑 | B2 | ✅ FIXED: 通用节点编辑拒绝 `advertised_routes`，前端不再提交该字段 |
| BUG-33 | P1 | 接入闭环 / 节点生命周期 | B1 | ✅ FIXED: hostname 复用不再回收 suspended/banned 节点 |
| BUG-34 | P1 | 接入闭环 / 节点生命周期 | B1 | ✅ FIXED: unregister 拒绝 suspended/banned runtime token |
| BUG-35 | P1 | 控制闭环 / Agent 命令 | B3 | ✅ FIXED: Agent command status 增加枚举和合法流转校验 |
| BUG-36 | P1 | 控制闭环 / 节点 API 响应 | B2 | ✅ FIXED: 租户节点响应返回 `region` |
| BUG-37 | P2 | 控制闭环 / 节点 API 响应 | B2 | ✅ FIXED: 租户节点响应返回 `vpc_id` |

## 修复计划总览

详细执行计划见 `docs/superpowers/plans/2026-06-27-confirmed-bugfix-closure.md`。

1. **B1: 租户、权限、节点生命周期 fail-closed**
   覆盖 BUG-28、BUG-29、BUG-30、BUG-33、BUG-34。先封住 inactive tenant / node 的接入、登录、权限、注册复用和 unregister 边界。
2. **B2: 非 AI 路由写入统一进入 policy delivery**
   覆盖 BUG-27、BUG-32。统一页面/API 的路由变更入口，避免数据库状态、Agent 执行状态和前端回显分叉。
3. **B3: Agent 命令状态机硬化**
   覆盖 BUG-35。约束命令状态枚举和合法流转，确保控制闭环能超时、失败、重试或收敛。
4. **B4: 监控 inactive 节点语义修正**
   覆盖 BUG-31。最后修正监控展示与统计口径，避免被前面生命周期调整反复冲突。
5. **B5: 旧 AI 写入口 fail-closed**
   覆盖 BUG-25、BUG-26。不继续扩展旧 AI 写闭环，但旧入口必须后端封禁写工具；Hermes Agent 接入后再恢复 AI 建议、确认、执行和留痕。

每个批次都必须满足：先加回归测试，再改实现；本地通过相关 Go/Vitest 测试；涉及前端状态的批次补浏览器冒烟；部署类变更按当前低带宽本地 artifact 流程或正式 CI 闭环执行。

---

## BUG-64 到 BUG-66 Rust Agent Runtime Durability

**复核日期**: 2026-07-04
**复核方式**: Rust Agent runtime 局部复核
**执行计划**: `docs/superpowers/plans/2026-07-04-b3-agent-runtime-durability.md`

### BUG-64: Agent 多接口 sync_peers 部分更新不回滚

- **状态**: ✅ FIXED (B3: `codex/bugfix-b3-agent-runtime`)
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/agent_runtime.rs`
- **根因**: `sync_peers` 逐接口读取、diff、删除、添加、更新 peer；若后续接口计划或执行失败，前面接口已经提交，缺少统一 preflight 和 rollback。
- **修复结果**: `sync_peers` 改为先采集所有接口快照，再构建所有接口的 peer sync plan；计划阶段失败不触碰接口。执行阶段失败会用快照尽力恢复所有接口到 sync 前状态。

### BUG-65: Agent state 文件损坏后启动失败

- **状态**: ✅ FIXED (B3: `codex/bugfix-b3-agent-runtime`)
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/config.rs`
- **根因**: runtime 启动路径通过 `load_state_opt()?` 读取 state YAML，解析错误会直接中止，不会 fallback 到 legacy config 或 fresh state。
- **修复结果**: 保留 `load_state_opt()` 的严格诊断语义；`load_or_migrate_state()` 在 state 读取/解析失败时记录 warning，并 fallback 到 legacy config 或 fresh state。

### BUG-66: 证书续期先覆盖文件后验证 gRPC 重连

- **状态**: ✅ FIXED (B3: `codex/bugfix-b3-agent-runtime`)
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/certificate_client.rs`, `agent-rust/agent/src/agent_runtime.rs`
- **根因**: 续期拿到新证书后直接覆盖 CA/client cert/client key，再执行 gRPC reconnect；重连失败时旧文件已经丢失。
- **修复结果**: 覆盖前先备份旧证书文件；写入失败或 gRPC 重连失败会恢复旧 CA/client cert/client key，避免 Agent 下次启动只剩失败的新证书组合。

---

## BUG-25 到 BUG-37 本轮修复详情

### BUG-25: AI 普通聊天阶段会直接执行写工具，确认弹窗未形成后端门禁

- **状态**: ✅ FIXED
- **文件**: `internal/service/ai_service.go`, `internal/service/ai_service_test.go`
- **修复结果**: `Chat` 和 `ChatWithContext` 只传入 `scopedReadOnlyTools(...)`；旧写工具不会再暴露给 LLM tool call。`ExecuteTool` 在执行前统一拒绝 `RequiredPermission` 以 `:write` 结尾的旧 AI 工具，即使 `confirmed=true` 也不会执行。
- **Hermes 后续要求**: Hermes 恢复 AI 写能力时必须使用后端持有的 pending action、confirm/cancel 和审计链路，不能复用旧 chat 直接执行模型 tool call 的方式。

### BUG-26: AI 路由写工具绕过控制闭环，并可能错误恢复 inactive 节点

- **状态**: ✅ FIXED
- **文件**: `internal/service/ai_service.go`, `internal/service/ai_service_test.go`
- **修复结果**: 旧 AI 服务入口已禁止 `add_route` / `remove_route` 等写工具执行，避免继续通过 `SaveNode(...)` 绕过 `desired_state_version`、`agent_commands` 和 `policy_deliveries`。
- **Hermes 后续要求**: Hermes 任何路由写入都必须调用标准 route mutation / policy delivery 链路，不允许直接修改 `nodes.advertised_routes` 或调用 `SaveNode(...)`。

### BUG-27: `/api/v2/agents/network` legacy 路由入口仍绕过 policy delivery

- **状态**: ✅ FIXED
- **文件**: `internal/cli/controller_serve.go`, `internal/cli/controller_southbound_auth_test.go`
- **修复结果**: 兼容入口不再直接 `UpdateTenantNodeAdvertisedRoutes(...)`；路由变更现在通过事务更新 desired state、写入 `agent_commands`、创建 `policy_deliveries`，响应中返回 command/delivery 状态。

### BUG-28: suspended/deleted 租户仍可能完成 Agent 接入和南向运行

- **状态**: ✅ FIXED
- **文件**: `internal/cli/controller_serve.go`, `internal/controller/grpc/server.go`, `internal/api/v2/platform.go`, `internal/cli/token_create.go`
- **修复结果**: enrollment token、runtime gRPC、证书/注册路径、CLI/API token 创建都增加 tenant active 校验；inactive tenant fail closed。

### BUG-29: suspended/deleted 租户用户仍可登录、刷新 token、获取权限

- **状态**: ✅ FIXED
- **文件**: `internal/api/handlers/auth.go`, `internal/api/handlers/auth_login_test.go`, `internal/api/handlers/auth_refresh_test.go`, `internal/api/handlers/auth_permissions_test.go`
- **修复结果**: 非 `super_admin` 的 login、refresh 和 permissions 都校验租户 active；inactive tenant 返回认证/租户错误，不再签发或刷新业务会话。

### BUG-30: 前端权限刷新失败时会回退到本地内置权限

- **状态**: ✅ FIXED
- **文件**: `frontend/src/stores/user.js`, `frontend/src/router/index.js`, `frontend/tests/unit/userSession.test.js`, `frontend/tests/unit/routerPermissions.test.js`
- **修复结果**: 非 `super_admin` 权限接口失败时清空权限并 fail closed；路由守卫不再从本地角色默认值重新生成写权限。

### BUG-31: 监控在线状态忽略 suspended/banned 节点状态

- **状态**: ✅ FIXED
- **文件**: `internal/api/v2/operations.go`, `internal/api/v2/monitoring.go`, `pkg/controllerstorage/monitoring_queries.go`, `internal/api/v2/nodes_monitoring_behavior_test.go`, `pkg/controllerstorage/monitoring_queries_test.go`
- **修复结果**: availability 先识别 `deleted` / `suspended` / `banned`，只有 eligible active 节点能按 `last_seen` 进入 online；traffic/topology/learned routes 过滤 inactive 节点。

### BUG-32: 节点普通编辑接口可绕过 `routes:write` 和 policy delivery 修改 `advertised_routes`

- **状态**: ✅ FIXED
- **文件**: `internal/api/v2/setup.go`, `frontend/src/views/Nodes.vue`, `frontend/src/stores/node.js`, `internal/api/v2/nodes_monitoring_behavior_test.go`, `frontend/tests/unit/nodeStore.test.js`
- **修复结果**: 通用节点编辑接口收到 `advertised_routes` 会返回 400，并提示使用 route API；前端节点编辑和 node store 不再把 `advertised_routes` 提交到通用节点更新 API。

### BUG-33: hostname 复用会把 suspended/banned 节点软删除并替换成 online 新节点

- **状态**: ✅ FIXED
- **文件**: `pkg/controllerstorage/postgres.go`, `pkg/controllerstorage/node_lifecycle_test.go`
- **修复结果**: `ReuseHostnameIP` 查询旧 hostname 状态并拒绝 suspended/banned，避免软删除封禁或暂停节点。

### BUG-34: `/api/v2/agents/unregister` 未拒绝 suspended/banned 节点的 runtime token

- **状态**: ✅ FIXED
- **文件**: `internal/cli/controller_serve.go`, `internal/cli/controller_southbound_auth_test.go`
- **修复结果**: runtime token 绑定节点会校验当前节点状态；suspended/banned 节点不能通过 unregister 把自身改为 deleted。

### BUG-35: Agent 命令回报状态未做枚举和状态流转校验，可能卡死控制闭环

- **状态**: ✅ FIXED
- **文件**: `pkg/controllerstorage/agent_commands.go`, `pkg/controllerstorage/agent_commands_test.go`, `internal/controller/grpc/command_stream_test.go`
- **修复结果**: Agent command status 增加支持状态枚举、行锁读取当前状态、合法流转校验和 terminal 状态保护；未知状态或 terminal downgrade 不会写入 `agent_commands` / `policy_deliveries`。

### BUG-36: 节点 API 响应未返回 `region`，编辑保存后前端显示仍为 `unknown`

- **状态**: ✅ FIXED
- **文件**: `internal/api/v2/setup.go`, `internal/api/v2/node_response_security_test.go`
- **修复结果**: `buildTenantNodeResponse()` 返回 `region`，节点列表和详情能拿到 DB 中真实区域；新增回归测试覆盖响应字段合同。

### BUG-37: 节点 API 响应未返回 `vpc_id`

- **状态**: ✅ FIXED
- **文件**: `internal/api/v2/setup.go`, `internal/api/v2/node_response_security_test.go`
- **修复结果**: `buildTenantNodeResponse()` 返回 `vpc_id`，避免节点详情响应与存储字段不一致；新增回归测试与 BUG-36 共用响应合同覆盖。

## 本轮验证命令

```bash
go test ./internal/cli ./internal/controller/grpc ./internal/api/handlers ./internal/api/v2 ./internal/agent/... ./internal/service/... ./pkg/controllerstorage -count=1
cd frontend && npm test -- --run
cd frontend && npm run build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/aria-controller-linux-amd64 ./cmd
```

新的风险项应先进入 `known-issues-status.md`，确认可复现代码缺陷后再进入本文档。

---

## 已重新验证为已修复的 Bug

### BUG-21: 租户 `suspended` / `deleted` 状态未进入后端授权链路

- **状态**: ✅ FIXED
- **文件**: `internal/api/v2/setup.go`, `pkg/controllerstorage/postgres.go`
- **说明**: 非 `super_admin` 的租户作用域授权现在会读取 `tenants.status`，只有 `active` 租户可继续访问；`super_admin` 保留平台管理能力。

### BUG-22: `tenants.code` 为 NULL 时租户详情误报 404

- **状态**: ✅ FIXED
- **文件**: `internal/api/handlers/tenant.go`, `pkg/controllerstorage/postgres.go`, `internal/api/middleware/tenant_storage.go`
- **说明**: 租户信息扫描改为 `sql.NullString`，历史或默认租户的 NULL `code` 会返回空字符串，不再误报 404。

### BUG-23: `tokens.tag` 为 NULL 时 token 查询 / 列表扫描失败

- **状态**: ✅ FIXED
- **文件**: `internal/token/store.go`, `internal/api/v2/platform.go`
- **说明**: token store 和租户 token 列表都改为 `sql.NullString` 扫描 tag，NULL tag 会规范化为空字符串。

### BUG-24: 创建租户缺少后端 `name` / `code` 业务校验

- **状态**: ✅ FIXED
- **文件**: `internal/api/handlers/tenant.go`, `internal/api/apibase/responses.go`
- **说明**: 后端创建租户现在会校验 `name` / `code` 长度和格式，重复 code 的唯一约束冲突返回 409。

### 构建 / 开发环境风险: Rust Agent 缺少 macOS 平台门禁

- **状态**: ✅ FIXED
- **文件**: `agent-rust/agent/build.rs`
- **说明**: Agent build script 在非 Linux target 上会提前给出明确错误，避免失败下沉到 Linux-only 依赖层。

### BUG-9: Settings 上传鉴权来源错误

- **状态**: ✅ FIXED
- **文件**: `frontend/src/views/Settings.vue`
- **说明**: 上传头已改为从 `localStorage.getItem('aria_token')` 读取，不再使用错误的 `sessionStorage`

### BUG-12: `acl_rules.id` 类型不匹配

- **状态**: ✅ FIXED
- **文件**: `pkg/controllerstorage/postgres.go`, `pkg/controllerstorage/network_policy.go`
- **说明**: 当前 `acl_rules.id` 为 UUID，存储层记录结构和 CRUD 参数也已统一为 `uuid.UUID`

### BUG-13: 前后端 ACL 字段名不匹配

- **状态**: ✅ FIXED
- **前端**: `frontend/src/composables/useAclApi.js`
- **后端**: `internal/api/v2/security.go`
- **说明**: 前端创建/更新已标准化发送 `src_cidr`、`dst_cidr`、`dst_port`；后端仍兼容旧字段，避免历史调用方中断。

### BUG-14: AI `maxTokens` 未初始化

- **状态**: ✅ FIXED
- **文件**: `internal/service/ai_service.go`
- **说明**: `NewAIService(...)` 现在显式设置 `maxTokens: 20`

### BUG-15: CIDR 路由 ID 中的 `/` 导致路径解析失败

- **状态**: ✅ FIXED
- **文件**: `internal/api/v2/setup.go`
- **说明**: 节点路由处理已改为将 `parts[7:]` 重新 `Join` 回路由标识符

### BUG-16: 监控流量时间戳与下载数据不对齐

- **状态**: ✅ FIXED
- **文件**: `internal/api/v2/monitoring.go`
- **说明**: 当上传流量为空时，时间戳会从 RX 结果补齐，并补全缺失上传数据点

### BUG-17: CreateTenant 丢弃 `email` / `phone`

- **状态**: ✅ FIXED
- **文件**: `internal/api/handlers/tenant.go`
- **说明**: 插入 SQL 现已包含 `email` 和 `phone` 字段

### BUG-18: `sync_failed` / `policy_failed` 告警重复创建

- **状态**: ✅ FIXED
- **文件**: `pkg/controllerstorage/alert_generator.go`
- **说明**: 现在已先检查活跃告警，再决定是否创建新告警

### BUG-19: QoS 通用 API 仍允许 `bandwidth_mbps=0`

- **状态**: ✅ FIXED
- **前端**: `frontend/src/composables/useQosApi.js`
- **后端**: `internal/api/v2/security.go`
- **说明**: 前端 API 组合层和后端创建/更新接口均拒绝 `bandwidth_mbps <= 0`。

### BUG-20: Rust Agent 硬编码 `eth0` 物理接口

- **状态**: ✅ FIXED
- **文件**: `agent-rust/agent/src/agent_runtime.rs`
- **说明**: 当前会先探测非回环、非 `aria*` 的物理接口，再传给系统优化模块

---

## BUG-38 到 BUG-57（已复核收口）

**复核日期**: 2026-06-29
**复核方式**: 全量静态代码复核 + 行级验证
**说明**: 本轮为全仓库 bug review，覆盖 Go Controller、Rust Agent、Vue 前端。BUG-38~48、50~54、56~57 已修复；BUG-49、BUG-55 复核为当前产品语义下非 bug。

### 🔴 Critical / High（线上可能崩）

#### BUG-38: 前端 learned routes 表格 region 崩溃

- **状态**: ✅ FIXED
- **严重度**: CRITICAL
- **文件**: `frontend/src/views/Nodes.vue:447`
- **根因**: `row.region.toUpperCase()` 作用于 learned routes 原始 API 数据，`region` 未归一化，为 `undefined` 时抛 TypeError，表格白屏。

#### BUG-39: Rust Agent 端口溢出导致 Agent 崩溃或静默误配

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/agent_runtime.rs:1869`
- **根因**: `u16 + offset` 多隧道端口计算溢出。debug 构建 panic 崩 Agent，release 静默绕回错误端口，WireGuard 连接中断。

#### BUG-40: Rust Agent hot sync 路径内联 panic

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/agent_runtime.rs:1705`
- **根因**: `panic!("WireGuardManager for {} not found", iface)` 在 `spawn_blocking` 的 sync 热路径中，接口管理丢失时直接崩。

#### BUG-41: Rust Agent Mutex 毒化导致管理员命令崩 Agent

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/agent_runtime.rs:1188,1201,1226`
- **根因**: `StdMutex::lock().unwrap()` 毒化时 panic。操作员执行 `aria-agent log` 等管理命令可能把 Agent 进程打挂。

#### BUG-42: 前端 6 处 `error.message` 二次崩溃 + loading 永久挂死

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `frontend/src/views/Settings.vue:251,264,286,302,319,346`
- **根因**: `catch (error)` 中 `error.message` 在非 Error 对象上抛 TypeError。同时 `finally` 中的 `loading.value = false` 永远不会执行，UI 按钮永久转圈。

### 🟠 HIGH（内存泄漏 / 竞态）

#### BUG-43: document 事件监听器内存泄漏

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `frontend/src/composables/useApi.ts:245-251`
- **根因**: 4 个 document 事件监听器（mousedown/keydown/scroll/touchstart）模块级注册，永不移除。

#### BUG-44: tenantChanged 监听器内存泄漏

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `frontend/src/composables/useAclApi.ts:164-168`
- **根因**: `window.addEventListener('tenantChanged', ...)` 模块级注册，HMR 时重复累积。

#### BUG-45: ACLRules 页面 searchTimer 未清理

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `frontend/src/views/ACLRules.vue:663-669`
- **根因**: debounce `setTimeout` 无 `onUnmounted` 清理，离开页面后定时器仍在执行，调用已销毁组件上的 API。

#### BUG-46: versionWatcher interval 永不停止

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `frontend/src/stores/app.ts:50-57`
- **根因**: `setInterval` 每分钟轮询版本号，跨页面持续运行，store 未暴露 stop 方法。

#### BUG-47: loadSession 返回 true 时权限仍在异步加载

- **状态**: ✅ FIXED
- **严重度**: HIGH
- **文件**: `frontend/src/stores/user.ts:364-367`
- **根因**: `loadSession()` 同步返回 `true`，`loadCurrentPermissions()` 异步飞过。刚登录后 `hasPermission()` 检查不可靠，UI 可能渲染无权限按钮。

### 🟡 MEDIUM

#### BUG-48: 飞书 Webhook 无认证缺口

- **状态**: ✅ FIXED
- **严重度**: MEDIUM
- **文件**: `internal/im/feishu.go`
- **根因**: 配置了 `verifyToken` 时，旧逻辑只在请求携带 header token 时才校验；缺失 token 的请求会绕过认证。

#### BUG-49: UpdateNodePublicIdentity 无条件清空 private_ip

- **状态**: ⚪ BY DESIGN
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/postgres.go:742`
- **根因**: `private_ip = ''` 无条件下清零，即使节点正确上报了内网 IP 也会被清掉。
- **复核结论**: 当前产品定义只记录真实公网 IP 与 VPN IP，本地接口/private IP 不入库。

#### BUG-50: Rust crypto provider 初始化 expect 直接崩

- **状态**: ✅ FIXED
- **严重度**: MEDIUM
- **文件**: `agent-rust/agent/src/main.rs:122`
- **根因**: `expect()` 在 `main()` 最前面，aws-lc-rs 安装失败时 Agent 无法启动，无任何错误处理。

#### BUG-51: Rust YAML 解析错误静默吞掉，Agent 以全默认值启动

- **状态**: ✅ FIXED
- **严重度**: MEDIUM
- **文件**: `agent-rust/agent/src/config.rs:427`
- **根因**: `serde_yaml::from_str(...).ok()` 丢弃解析错误，Agent 用全默认配置（空 controller_url、空密钥）启动。

### 🟢 LOW

#### BUG-52: rows.Err() 未检查

- **状态**: ✅ FIXED
- **严重度**: LOW
- **文件**: `internal/api/v2/setup.go`、`pkg/controllerstorage/postgres.go`、`pkg/controllerstorage/rbac.go`
- **根因**: `sql.Rows` 迭代后未检查 `rows.Err()`，连接中断/数据损坏时静默返回不完整数据。

#### BUG-53: json.Encode 错误丢弃 (多处)

- **状态**: ✅ FIXED
- **严重度**: LOW
- **文件**: `internal/im/feishu.go:126,135,149,164,174,182` 等
- **根因**: `_ = json.NewEncoder(w).Encode(...)` 丢弃编码错误，客户端收到空/截断响应 + HTTP 200。

#### BUG-54: tx.Rollback 错误丢弃 (多处)

- **状态**: ✅ FIXED
- **严重度**: LOW
- **文件**: `pkg/controllerstorage/*`、`internal/api/v2/settings.go`、`internal/api/handlers/tenant.go`
- **根因**: deferred `tx.Rollback()` 错误被丢弃。

#### BUG-55: nullableInt(0) 导致 port 0 / proto 0 不可存

- **状态**: ⚪ BY DESIGN
- **严重度**: LOW
- **文件**: `pkg/controllerstorage/network_policy.go:1110-1114`
- **根因**: `0` 被映射为 SQL NULL，实际值为 `0` 的合法字段无法存储。
- **复核结论**: 当前 ACL/QoS 语义里 `0` 表示 any/未指定，读路径会用 `COALESCE(..., 0)` 恢复页面语义。

#### BUG-56: Token 列表 API 失败时静默清空

- **状态**: ✅ FIXED
- **严重度**: LOW
- **文件**: `frontend/src/views/Tokens.vue:225-229`
- **根因**: 网络故障时静默清空 token 列表，用户看不到错误提示。

#### BUG-57: Node loadDetail allSettled 双失败时 throw 非 Error 值

- **状态**: ✅ FIXED
- **严重度**: LOW
- **文件**: `frontend/src/stores/node.ts:182-189`
- **根因**: `throw detailResult.reason || monitorResult.reason` 可能抛非 Error 值，上层 catch 依赖 `error.message` 的代码会二次崩溃。

---

## 历史说明

- 较早一批问题（如 BUG-1 ~ BUG-11）已经在当前代码中维持修复状态，本次未逐条重写，只保留仍需关注的现实风险
- 旧版本文档里“未修复”与“已全部修复”同时存在的冲突表述已移除
- 后续如果继续维护这份文档，建议只保留“当前仍开放的问题”和“本轮新确认关闭的问题”
