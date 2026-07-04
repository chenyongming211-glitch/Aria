# 代码 Bug 追踪

**最后复核**: 2026-07-03
**复核方式**: 全量静态代码复核 + 多 Agent 并行 Review + 本地 Go/Frontend 回归验证
**说明**: 本文档记录“当前状态”，不再把历史 OPEN 条目和后续 FIXED 结论混写在一起。
**AI 范围决定**: AI Agent 后续计划替换为 Hermes Agent。本轮不扩展旧 AI 写操作闭环，但已把旧 AI 写入口后端 fail-closed：聊天只暴露只读工具，`ExecuteTool` 即使 `confirmed=true` 也拒绝旧写工具；Hermes 接入时再重新设计 pending confirmation、policy delivery 和审计链路。
**2026-07-03 复核结论**: BUG-58~61、BUG-63~82、BUG-84~89、BUG-91~93、BUG-95~99 均有当前代码证据；BUG-62、BUG-83 复核为不成立；BUG-90 原描述部分成立（ACL 缺复合索引，QoS 已有）；BUG-94、BUG-97、BUG-99 是优化债，不按 correctness bug 排序。新增 BUG-100~110。

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

## BUG-58 到 BUG-63（第二轮 Review 新发现）

**复核日期**: 2026-07-03
**复核方式**: 全量静态代码复核 + diff 分析 (fa9922c..HEAD)
**说明**: 第一轮 BUG-38~57 修复后，对新增/变更代码进行第二轮排查。

### 🔴 HIGH

#### BUG-58: Routing.vue 3 处裸 error.message

- **状态**: ✅ FIXED (B5: `codex/bugfix-b5-frontend-stability`)
- **严重度**: HIGH
- **文件**: `frontend/src/views/Routing.vue:304,373,394`
- **根因**: 同批改的 Policies/ACLRules/BandwidthControl 都加了 `errorMessage()` helper（`instanceof Error` 守卫），Routing.vue 遗漏。`error.message` 在非 Error 对象上抛 TypeError。
- **修复结果**: Routing 增加安全 `errorMessage()` helper，兼容 API response message、`Error`、字符串和未知值，避免 catch 块二次抛错。

#### BUG-59: IPGroups.vue 4 处裸 error.message

- **状态**: ✅ FIXED (B5: `codex/bugfix-b5-frontend-stability`)
- **严重度**: HIGH
- **文件**: `frontend/src/views/IPGroups.vue:273,305,355,383`
- **根因**: 同 BUG-58。IPGroups.vue 在本轮有大量修改但忘记了加 `instanceof Error` 守卫。
- **修复结果**: IPGroups 增加安全 `errorMessage()` helper，load/reference/delete/save 失败时不再依赖裸 `error.message`。

#### BUG-60: Tokens.vue 2 处未使用已有的 errorMessage helper

- **状态**: ✅ FIXED (B5: `codex/bugfix-b5-frontend-stability`)
- **严重度**: HIGH
- **文件**: `frontend/src/views/Tokens.vue:314,332`
- **根因**: 文件内 L188 已有 `errorMessage()` helper（已加 `instanceof Error` 守卫），但 `saveToken` 和 `revokeToken` 的 catch 块仍用裸 `error.message || error`。
- **修复结果**: token create/revoke catch 块改为复用已有 helper，非 `Error` reject 值显示 fallback，不再触发 TypeError。

### 🟡 MEDIUM

#### BUG-61: main.ts void bootstrap() 无人捕获 rejection

- **状态**: ✅ FIXED (B5: `codex/bugfix-b5-frontend-stability`)
- **严重度**: MEDIUM
- **文件**: `frontend/src/main.ts:466`
- **根因**: `void bootstrap()` 丢弃了 async 函数返回的 Promise。bootstrap 内任一步（loadSession、mount）失败时 rejection 无人处理，app 静默挂掉。
- **修复结果**: `bootstrap().catch(renderStartupFailure)` 统一捕获启动失败，记录错误并在 `#app` 渲染确定性的启动失败状态。

#### BUG-62: main.ts fetchVersion() fire-and-forget

- **状态**: ⚪ NOT CONFIRMED
- **严重度**: N/A
- **文件**: `frontend/src/main.ts:36`、`frontend/src/stores/app.ts:50-71`
- **复核结论**: 不成立。`fetchVersion()` 虽然在 main.ts 里未 await，但函数内部已经 `try/catch` 并记录错误，请求失败不会产生 unhandled rejection。

### 🟢 LOW

#### BUG-63: DingTalk handler 未同步 Feishu 的重构

- **状态**: ✅ FIXED (B2: `codex/bugfix-b1-security`)
- **严重度**: LOW
- **文件**: `internal/im/dingtalk.go`
- **修复结果**: 新增 `writeDingTalkJSON`，统一设置 JSON 响应、状态码和编码错误日志；DingTalk webhook 的空消息、错误响应和正常回复都进入 checked encoder path。

---

## BUG-64 到 BUG-79（第三轮 Deep Review 新发现）

**复核日期**: 2026-07-03
**复核方式**: Rust Agent / Go Controller / 前端业务逻辑深度审查
**说明**: 第三轮聚焦状态机完整性、事务边界、并发安全、故障恢复路径。

### 🔴 HIGH（6）

#### BUG-64: Agent 多隧道 sync_peers 部分更新不回滚

- **状态**: 🔴 OPEN
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/agent_runtime.rs:1704-1814`
- **根因**: sync_peers 遍历多隧道接口（最多 4 个），逐个 apply peer 变更。若 aria2 失败，aria0/aria1 已提交但不回滚，peer 状态不一致。

#### BUG-65: Agent state 文件损坏后永久启动失败

- **状态**: 🔴 OPEN
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/config.rs:397-417`
- **根因**: `load_state_opt()` 解析失败直接 `?` 传播错误，不 fallback 到 legacy config 或 fresh state。磁盘满/崩溃截断后永久无法启动。

#### BUG-66: 证书续期先写磁盘后验证 gRPC 重连

- **状态**: 🔴 OPEN
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/agent_runtime.rs:2265-2271`
- **根因**: `write_renewed_certificate_files` → 磁盘写入成功 → `reconnect_grpc` 失败。旧证书已被覆盖，重连失败时 agent 重启即砖。

#### BUG-67: Restore 全量替换生产数据库无 pre-flight 检查

- **状态**: ✅ FIXED (B2: `codex/bugfix-b1-security`)
- **严重度**: HIGH
- **文件**: `internal/api/v2/settings.go:693-729`
- **修复结果**: Restore dry-run 和真实恢复都会执行 preflight；真实恢复在事务前阻断在线 Agent、未完成 Agent command、未完成 policy delivery，并返回 `409 CONFLICT` 与可读 warning。

#### BUG-68: 暂停租户不会级联暂停节点/撤销 Token/取消命令

- **状态**: ✅ FIXED (B1: `codex/bugfix-b1-security`)
- **严重度**: HIGH
- **文件**: `internal/api/v2/setup.go`、`internal/controller/grpc/server.go`、`pkg/controllerstorage/node_lifecycle.go`
- **修复结果**: `PUT /api/v2/tenants/{id}` 将租户改为 `suspended` 时进入 storage lifecycle 事务，更新租户状态、bump 租户用户 JWT version、暂停 active 节点、bump 节点 runtime token version、撤销节点证书、失败未完成命令并写审计；已建立的 CommandStream 每次轮询/回报都会重新校验 tenant active。

#### BUG-69: Runtime Token 无吊销机制

- **状态**: ✅ FIXED (B1: `codex/bugfix-b1-security`)
- **严重度**: HIGH
- **文件**: `internal/auth/runtime_token.go`、`internal/controller/grpc/auth_interceptor.go`、`pkg/controllerstorage/auth_state.go`
- **修复结果**: Runtime Token 增加 `ver` claim；gRPC interceptor 读取节点 runtime auth state，校验 node_id、tenant_id、node status、tenant status 和 runtime token version。租户暂停/删除会 bump 节点 version，使旧 runtime token 立即失效。

### 🟡 MEDIUM / 局部 HIGH（8）

#### BUG-70: IP Group 删除 TOCTOU 竞态

- **状态**: ✅ FIXED (B4: `codex/bugfix-b4-controller-correctness`)
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/ip_groups.go:271-292`
- **修复结果**: `DeleteIPGroup` 将 reference check 和 DELETE 放入同一事务；被引用时仍返回友好 conflict，避免并发策略创建导致 raw FK/DB 错误暴露给 API。

#### BUG-71: 部分表恢复可产生孤儿引用

- **状态**: ✅ FIXED (B2: `codex/bugfix-b1-security`)
- **严重度**: MEDIUM
- **文件**: `internal/api/v2/settings.go:472-503,731-747`
- **修复结果**: 选择性恢复会先校验表依赖闭包，并按 `backupRestoreTables` 的固定顺序执行，不再按请求顺序插入；`ip_group_members`、`ip_groups` 等关联表缺依赖时在 dry-run 阶段返回可读 `BAD_REQUEST`。

#### BUG-72: 无 SuspendTenant 统一函数

- **状态**: ✅ FIXED (B1: `codex/bugfix-b1-security`)
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/node_lifecycle.go`、`setup.go`
- **修复结果**: 新增 `ApplyTenantLifecycleTransition(...)`，将 tenant 停止态、用户 session version、节点 runtime token version、证书撤销、命令失败和审计写入放入同一事务。

#### BUG-73: 过期证书排除在续期候选外

- **状态**: ✅ FIXED (B4: `codex/bugfix-b4-controller-correctness`)
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/certificate_lifecycle.go:42-76`
- **修复结果**: `ListCertificatesExpiringBefore` 不再用 `not_after >= NOW()` 排除已过期但仍为 issued 的证书，续期/对账候选能覆盖调度延迟窗口。

#### BUG-74: eBPF map 关闭清理不完整

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `agent-rust/agent/src/agent_runtime.rs:2424-2447`、`agent-rust/agent/src/agent_runtime.rs:489-519`
- **根因**: 运行时关闭 cleanup 只删 4 个 identity map；启动前 cleanup 会删 12 个 map，但正常停机后仍会留下 POLICY/QOS/STATS/CONFIG 等 pinned map，造成 bpffs 残留和运维误判。

#### BUG-75: Agent state YAML 非原子写入

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `agent-rust/agent/src/config.rs:464-489`
- **根因**: `write_yaml_file` 直接 `std::fs::write` 无 tmp+rename。崩溃截断 → BUG-65 永久启动失败。

#### BUG-76: 命令流断开时在途命令结果丢失

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `agent-rust/agent/src/agent_runtime.rs:942-948`
- **根因**: 命令流重连后创建新 mpsc channel，旧命令的最终 completed/failed 响应无处投递。Controller 收到 ack 但无终态。

#### BUG-77: Nodes.vue 保存分两步无回滚

- **状态**: ✅ FIXED (B5: `codex/bugfix-b5-frontend-stability`)
- **严重度**: HIGH
- **文件**: `frontend/src/views/Nodes.vue:1453-1472`
- **根因**: updateNodeRemote 成功 → syncEditedAdvertisedRoutes 失败。hostname/region 已持久化但 UI 显示"更新失败"，重试 diff 基线仍是编辑前状态。
- **修复结果**: 元数据保存成功但路由同步失败时，前端刷新节点列表和节点详情，重置编辑基线为后端最新状态，保留弹窗并显示部分成功提示。

### 🟡 前端逻辑（2 个 MEDIUM）

#### BUG-78: 聚焦轮询 500 错误无限重试无退避

- **状态**: ✅ FIXED (B5: `codex/bugfix-b5-frontend-stability`)
- **严重度**: MEDIUM
- **文件**: `frontend/src/composables/useFocusedPolling.ts:32-51`
- **根因**: poll 失败只记 warn 不降频。每 3s 锤一次 500 端点的服务器。
- **修复结果**: 轮询从固定 `setInterval` 改为 timeout 调度，失败后指数退避，连续失败达到阈值后停止。

#### BUG-79: CIDR 正则接受无效 octet/prefix

- **状态**: ✅ FIXED (B5: `codex/bugfix-b5-frontend-stability`)
- **严重度**: MEDIUM
- **文件**: `frontend/src/views/Routing.vue:347-348`、`Nodes.vue:1412`
- **根因**: `/^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/` 接受 `999.999.999.999/99`。客户端验证不可信但导致迷惑性 UI。
- **修复结果**: 新增共享 `isValidCidrOrIp()`，Routing 和 Nodes 复用真实 IPv4/IPv6 CIDR/IP 解析边界，拒绝非法 octet 和 prefix。

---

## BUG-80 到 BUG-99（第四轮 Review：安全 + 性能）

**复核日期**: 2026-07-03
**复核方式**: 安全审计 + 性能分析
**说明**: 第四轮聚焦认证安全、数据泄露、索引缺失、N+1 查询、大响应体。

### 🔴 CRITICAL（1）

#### BUG-80: Backup 明文导出全部 password hash + enrollment token

- **状态**: ✅ FIXED (B2: `codex/bugfix-b1-security`)
- **严重度**: CRITICAL
- **文件**: `internal/api/v2/settings.go:35-38,86-91`
- **修复结果**: 默认备份写入 `sensitive_redacted=true`，并将 `users.password_hash`、`tokens.token`、`nodes.enrolled_with_token` 替换为 `<redacted>`；只有显式传 `include_sensitive=true` 且确认短语为 `EXPORT SENSITIVE ARIA BACKUP` 时才导出可恢复敏感字段。红acted 备份禁止 restore/upload 作为可恢复备份。

### 🔴 HIGH / 🟡 MEDIUM / ⚪ 不成立

#### BUG-81: JWT Token 刷新/登出后旧 token 不撤销

- **状态**: ✅ FIXED (B1: `codex/bugfix-b1-security`)
- **严重度**: HIGH
- **文件**: `internal/auth/jwt.go`、`internal/api/handlers/auth.go`、`internal/api/middleware/jwt_auth.go`
- **修复结果**: JWT 增加 `ver` claim；真实路由使用带 store 的 JWT middleware 校验 `users.token_version`。refresh 成功会 bump version 后签发新 token，logout 会 bump version 吊销当前 token，旧 token 再访问受保护接口或刷新都会被拒绝。

#### BUG-82: DingTalk webhook 未配 secret 时完全无认证

- **状态**: ✅ FIXED (B2: `codex/bugfix-b1-security`)
- **严重度**: HIGH
- **文件**: `internal/im/dingtalk.go:67,182`
- **修复结果**: `verifySign` 在 `secret` 为空时返回 false，`HandleWebhook` 统一要求签名验证通过；未配置或签名错误都会返回 `401 Unauthorized`。

#### BUG-83: 零 CORS 配置

- **状态**: ⚪ NOT CONFIRMED
- **严重度**: N/A
- **文件**: `internal/cli/controller_serve.go:538`
- **复核结论**: 不成立。没有 CORS header 不是“裸 API 无保护”，浏览器跨域请求默认会被拦截；CORS 不是服务端安全边界。若产品需要跨域前端访问，应作为部署/功能配置任务处理，不应按安全 bug 修。

#### BUG-84: Node 详情页 8+ 次独立 DB 查询

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `internal/api/v2/monitoring.go:77-231`
- **根因**: `handleMonitoringNodeDetail` 对节点详情、控制状态、策略统计、证书、证书审计、命令、策略下发、告警、同租户节点等逐项查询；这是确认存在的 N+1/多查询性能风险，但不是 correctness bug。

#### BUG-85: 批量命令每节点一次独立 DB 写入

- **状态**: 🔴 OPEN
- **严重度**: HIGH
- **文件**: `internal/api/v2/operations.go:226-261`
- **根因**: `handleTenantBatchAgentCommand` for 循环内每节点一次 QueueAgentCommand + audit。N=100 节点 = 100 次独立写入。

#### BUG-86: CommandStream 每 agent 每 2s 空轮询

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `internal/controller/grpc/server.go:275,310`
- **根因**: 无命令时仍每 2s 轮询一次。100 agent = 50 次/秒无用数据库查询。

### 🟡 MEDIUM / 🟢 LOW

#### BUG-87: Feishu webhook verifyToken 为空时认证跳过

- **状态**: ✅ FIXED (B2: `codex/bugfix-b1-security`)
- **严重度**: MEDIUM
- **文件**: `internal/im/feishu.go:129-131`
- **修复结果**: `verifyIncomingToken` 在 `verifyToken` 为空时返回 false，未配置 verify token 的 Feishu webhook 请求会返回 `401 Unauthorized`。

#### BUG-88: Token tag 无长度/内容校验

- **状态**: ✅ FIXED (B4: `codex/bugfix-b4-controller-correctness`)
- **严重度**: MEDIUM
- **文件**: `internal/api/v2/platform.go:561-603`、`internal/token/generator.go:53`
- **修复结果**: 创建 token 时先规范化 tag，限制最多 128 字符并拒绝控制字符；非法 tag 在租户状态查询和 DB 写入前返回 400。

#### BUG-89: getNodes 字符串拼接 SQL（当前安全但脆弱）

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/postgres.go:783-812`
- **根因**: `query += " " + extraWhere` 动态拼接。当前调用方只传硬编码常量，但未来开发者可传入用户输入。

#### BUG-90: acl_rules/qos_rules 缺 (tenant_id, node_id) 复合索引

- **状态**: 🟡 PARTIAL
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/postgres.go:379-389`、`pkg/controllerstorage/network_policy.go:625`
- **复核结论**: 部分成立。`qos_rules` 已有 `idx_qos_rules_tenant_node`，原描述中 QoS 部分不成立；`acl_rules` 只有 `tenant_id` 和 `node_id` 单列索引，`GetEnabledTenantNodeACLRules(tenant_id,node_id)` 缺复合索引。

#### BUG-91: nodes.hostname 缺索引

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/postgres.go` DDL
- **根因**: `GetNodeByHostname` 查询 `WHERE hostname = $1` 无索引，每次注册全表扫描。

#### BUG-92: GetNodesByTenant 无分页全量加载到内存

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/postgres.go:639`
- **根因**: 所有节点列表查询无 LIMIT/OFFSET。千级租户时全量加载。

#### BUG-93: Topology O(n²) 链路

- **状态**: 🔴 OPEN
- **严重度**: MEDIUM
- **文件**: `internal/api/v2/monitoring.go:884-906`
- **根因**: 双重 for 循环构造全网状 link。200 节点 = 19,900 个 link 对象，响应体巨大。

#### BUG-94: Rust metric label 用 format!() 创建每 30s ~1400 次分配

- **状态**: 🔴 OPEN
- **严重度**: LOW
- **文件**: `agent-rust/agent/src/agent_runtime.rs:2122-2149`
- **根因**: 每个 ACL/QoS rule stat 都用 `format!()` 新建 String。它是确认存在的性能优化点，但按 30s 周期看不应列为当前 correctness bug。

### 🟢 LOW / 优化债

#### BUG-95: JWT issuer 未强制校验

- **状态**: ✅ FIXED (B1: `codex/bugfix-b1-security`)
- **严重度**: LOW
- **文件**: `internal/auth/jwt.go:60-96`
- **修复结果**: `ValidateToken` 使用 `jwt.WithIssuer("aria-controller")` 强制校验 issuer，并补充 wrong issuer 回归测试。

#### BUG-96: Node update 缺少 hostname/region/vpc_id 长度限制

- **状态**: ✅ FIXED (B4: `codex/bugfix-b4-controller-correctness`)
- **严重度**: LOW
- **文件**: `internal/api/v2/setup.go:1099-1157`、`pkg/controllerstorage/postgres.go:184-186`
- **修复结果**: Node update 在 DB 写入前校验 `hostname <= 100`、`region <= 50`、`vpc_id <= 50`，并拒绝控制字符；非法输入返回 400。

#### BUG-97: Rust sync_peers 每次 clone 全部 PeerInfo

- **状态**: 🔴 OPEN
- **严重度**: LOW
- **文件**: `agent-rust/agent/src/agent_runtime.rs:1688-1690,1908-1911`
- **根因**: `sync_peers` 入口先 `to_vec()`，diff 时又 clone desired peer；这是确认存在的性能优化点，不是功能错误。

#### BUG-98: agent_commands deadline 表达式阻索引使用

- **状态**: 🔴 OPEN
- **严重度**: LOW
- **文件**: `pkg/controllerstorage/agent_commands.go:119-129,241-267`
- **根因**: 超时判断使用 `sent_at + timeout_seconds * interval` / `COALESCE(...) + timeout_seconds * interval` 表达式，普通时间索引无法直接服务该条件；大量命令时会影响扫描效率。

#### BUG-99: sync_with_state 6+ 中间 Vec 分配

- **状态**: 🔴 OPEN
- **严重度**: LOW
- **文件**: `agent-rust/agent/src/grpc_client.rs:306-360`
- **根因**: gRPC SyncResponse 转内部 SyncResult 时对 peers/ip_groups/acl/qos/blacklist 等集合逐个 map+collect；这是确认存在的内存分配优化点，不是 correctness bug。

---

## BUG-100 到 BUG-110（第五轮多 Agent Review 新确认）

**复核日期**: 2026-07-03
**复核方式**: 4 个子 Agent 并行 Review + 主线程代码链路复核
**说明**: 第五轮聚焦接入闭环、控制闭环、前端策略上下文、CI/CD 交付风险。

### 🔴 HIGH（2）

#### BUG-100: Agent 重新 bootstrap 时不能使用已有 runtime credential

- **状态**: 🔴 OPEN
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/main.rs:830-855`、`internal/cli/controller_serve.go:1003-1048`
- **根因**: Agent 只要 `assigned_ip` 缺失或 runtime token 60s 内过期，就进入 `bootstrap_register()` 并强制要求 `bootstrap.enrollment_token`。但 Controller 已支持已有节点用 runtime token re-registration，Agent 端没有利用，导致清理 enrollment token 后的重注册/恢复路径失败。

#### BUG-101: Sync 下发的新 runtime token 在 peer/route apply 失败时不会落盘

- **状态**: 🔴 OPEN
- **严重度**: HIGH
- **文件**: `agent-rust/agent/src/agent_runtime.rs:1493-1505,1547`、`internal/controller/grpc/server.go:209-226`
- **根因**: Controller 每次 Sync 刷新 runtime token；Agent 先把 token 写进内存，再执行 `sync_peers` / `sync_advertised_routes`。如果这两步失败，函数直接返回，`persist_runtime_state()` 不执行，新 token 不落盘，重启后可能继续使用旧 token。

### 🟡 MEDIUM（7）

#### BUG-102: PUT route 空 body 会静默删除旧路由

- **状态**: ✅ FIXED (B4: `codex/bugfix-b4-controller-correctness`)
- **严重度**: MEDIUM
- **文件**: `internal/api/v2/setup.go:1303-1357,2068-2090,2211-2219`
- **修复结果**: `decodeRouteBody()` 对缺少 `cidr` / `route` 的请求直接返回 400，空 PUT 不再进入 replace 流程。

#### BUG-103: 裸 IPv6 路由会被错误补成 `/32`

- **状态**: ✅ FIXED (B4: `codex/bugfix-b4-controller-correctness`)
- **严重度**: MEDIUM
- **文件**: `internal/api/v2/setup.go:2068-2082`
- **修复结果**: `normalizeRoutes()` 区分 IPv4/IPv6，裸 IPv4 仍补 `/32`，裸 IPv6 补 `/128`。

#### BUG-104: auth refresh / force-change-password 的 Bearer 解析大小写不一致

- **状态**: ✅ FIXED (B4: `codex/bugfix-b4-controller-correctness`)
- **严重度**: MEDIUM
- **文件**: `internal/api/handlers/auth.go:178,322`、`internal/api/middleware/jwt_auth.go:35-40`
- **修复结果**: `HandleForceChangePassword` 复用 auth handler 的 Bearer parser，和 refresh/logout/middleware 一样接受大小写不敏感的 Bearer scheme。

#### BUG-105: Agent restart 命令被后端允许但 Agent 永远返回未实现

- **状态**: ✅ FIXED (B4: `codex/bugfix-b4-controller-correctness`)
- **严重度**: MEDIUM
- **文件**: `pkg/controllerstorage/agent_commands.go:24-29`、`agent-rust/agent/src/agent_runtime.rs:1322-1326`
- **修复结果**: Controller command allowlist 移除 `restart`，直到 Agent 真实实现该命令前不允许排队投递。

#### BUG-106: Policy Center 顶部 IP Groups 按钮把 MouseEvent 当成 policy

- **状态**: ✅ FIXED (B5: `codex/bugfix-b5-frontend-stability`)
- **严重度**: MEDIUM
- **文件**: `frontend/src/views/Policies.vue:8,729-731`
- **根因**: 模板 `@click="goToIpGroups"` 会把 MouseEvent 作为第一个参数传入；函数签名把第一个参数当 `NormalizedPolicy`，导致上下文推导使用事件对象，可能跳转到错误或空的 IP Groups 上下文。
- **修复结果**: 顶部按钮改为显式调用 `goToIpGroups()`，函数同时防御 `Event` 参数，确保跳转保留当前 policy 上下文。

#### BUG-107: GitHub artifact 上传失败会被 continue-on-error 吞掉

- **状态**: ✅ FIXED (B6: `codex/bugfix-b6-delivery-guardrails`)
- **严重度**: MEDIUM
- **文件**: `.github/workflows/build.yml:49-59,149-155,187-193`
- **根因**: Go/Rust/Frontend artifact upload 同时配置 `if-no-files-found: error` 和 `continue-on-error: true`。artifact 缺失或上传失败时 job 仍可能为 green，后续部署拿不到产物。
- **修复结果**: Go、Rust Agent、Frontend artifact upload 移除 `continue-on-error: true`，缺失或上传失败会直接使对应 job 失败。

#### BUG-108: Docker publish 只依赖 Go build，且 workflow_dispatch 任意分支可推 latest

- **状态**: ✅ FIXED (B6: `codex/bugfix-b6-delivery-guardrails`)
- **严重度**: MEDIUM
- **文件**: `.github/workflows/build.yml:61-91`
- **根因**: `docker-build` 只 `needs: go-build`，不等待 Rust Agent 和 Frontend 构建；任意手动触发分支都可推 `latest` 和 `${VERSION}`，容易把未完整验证的镜像发布成线上候选。
- **修复结果**: Docker publish 改为依赖 Go、Rust Agent、Frontend 三个 job，并只允许手动从 `master` 或 `v*` release tag 发布。

### 🟢 LOW（2）

#### BUG-109: Ansible Controller 部署仍固定旧版本镜像

- **状态**: ✅ FIXED (B6: `codex/bugfix-b6-delivery-guardrails`)
- **严重度**: LOW
- **文件**: `deployments/ansible/playbooks/deploy-controller.yml:10-13`、`deployments/ansible/group_vars/all.yml:69-75`
- **根因**: Ansible Controller playbook / group vars 仍固定 `0.2.35-test`，当前项目版本已远高于该值；如果误用这套 playbook，会回滚到旧镜像。
- **修复结果**: Controller Ansible 使用 `ARIA_CONTROLLER_VERSION`、`ARIA_CONTROLLER_IMAGE` 或仓库 `VERSION` 推导镜像，不再固定旧 tag。

#### BUG-110: Ansible Agent 部署脚本使用旧路径、旧二进制名和旧 service

- **状态**: ✅ FIXED (B6: `codex/bugfix-b6-delivery-guardrails`)
- **严重度**: LOW
- **文件**: `deployments/ansible/playbooks/deploy-agent.yml:10-13,43-47`、`deployments/scripts/deploy-agent.sh:33,39,244`
- **根因**: Ansible Agent playbook 仍从 `/Users/chen/Aria/agent-rust` 同步源码、部署到 `/usr/local/bin/aria`、重启 `aria` service；当前脚本使用 `/usr/local/bin/aria-agent` 和 `aria-agent` service。误用会部署/重启错误目标。
- **修复结果**: Agent Ansible 改为部署预构建 `aria-agent` artifact 到 `/usr/local/bin/aria-agent`，并重启 `aria-agent` service。

---

## 历史说明

- 较早一批问题（如 BUG-1 ~ BUG-11）已经在当前代码中维持修复状态，本次未逐条重写，只保留仍需关注的现实风险
- 旧版本文档里“未修复”与“已全部修复”同时存在的冲突表述已移除
- 后续如果继续维护这份文档，建议只保留“当前仍开放的问题”和“本轮新确认关闭的问题”
