# 代码 Bug 追踪

**最后复核**: 2026-06-27
**复核方式**: 全量静态代码复核 + 本地 Go/Frontend 回归验证
**说明**: 本文档记录“当前状态”，不再把历史 OPEN 条目和后续 FIXED 结论混写在一起。
**AI 范围决定**: AI Agent 后续计划替换为 Hermes Agent。当前阶段不继续开发旧 AI 写操作闭环；AI 相关缺陷保留为 Hermes 阶段输入，近期修复只处理非 AI 的 SaaS 边界、控制闭环和监控语义。

---

## 开放 Bug 总览

| Bug | 优先级 | 归属闭环 | 修复批次 | 当前处理原则 |
| --- | --- | --- | --- | --- |
| BUG-25 | P1 | Hermes 阶段 / AI 确认 | Deferred | 旧 AI 写操作闭环暂停开发；Hermes Agent 接入时重新设计 pending confirmation |
| BUG-26 | P1 | Hermes 阶段 / AI 路由 | Deferred | 旧 AI 路由写工具暂停开发；Hermes Agent 接入时必须复用标准 policy delivery |
| BUG-27 | P2 | 控制闭环 / legacy 路由 | B2 | `/api/v2/agents/network` 保留兼容入口，但内部迁入标准 route mutation |
| BUG-28 | P1 | 接入闭环 / 租户停用 | B1 | enrollment/runtime token 都必须校验 tenant active |
| BUG-29 | P2 | 权限闭环 / 租户停用 | B1 | login/refresh/permissions 对 inactive tenant fail closed |
| BUG-30 | P2 | 权限闭环 / 前端权限 | B1 | 生产登录态权限刷新失败不得回退内置写权限 |
| BUG-31 | P3 | 监控闭环 / inactive 节点 | B4 | 监控在线态、拓扑、流量统计要识别 suspended/banned |
| BUG-32 | P1 | 控制闭环 / 节点编辑 | B2 | 通用节点编辑不得绕过 `routes:write` 修改 `advertised_routes` |
| BUG-33 | P1 | 接入闭环 / 节点生命周期 | B1 | hostname 复用不得自动回收 suspended/banned 节点 |
| BUG-34 | P1 | 接入闭环 / 节点生命周期 | B1 | unregister 必须拒绝 suspended/banned runtime token |
| BUG-35 | P1 | 控制闭环 / Agent 命令 | B3 | Agent command status 必须枚举校验并限制合法状态流转 |

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
5. **Hermes 阶段暂缓项**
   BUG-25、BUG-26 不进入当前修复批次。Hermes Agent 接入前，旧 AI 写入口不作为 v0.1.0 闭环验收项；若旧入口继续暴露，应至少通过 feature flag 或权限配置保持只读/隐藏。

每个批次都必须满足：先加回归测试，再改实现；本地通过相关 Go/Vitest 测试；涉及前端状态的批次补浏览器冒烟；部署类变更按当前低带宽本地 artifact 流程或正式 CI 闭环执行。

---

## Hermes 阶段暂缓项

### BUG-25: AI 普通聊天阶段会直接执行写工具，确认弹窗未形成后端门禁

- **状态**: DEFERRED
- **优先级**: P1
- **文件**: `internal/agent/brain/agent.go`, `internal/service/ai_service.go`, `frontend/src/views/AIAssistant.vue`
- **说明**: AI chat 路径把所有工具交给 LLM，模型返回 `ToolCalls` 后会在普通聊天阶段直接 `runTool(...)`。当前工具列表包含 `add_route`、`remove_route`、`create_token` 等写工具，而前端确认弹窗依赖后端先返回 `needs_confirm/tool_calls`，实际后端不会在写工具前强制确认。
- **影响**: 拥有 `ai:use` 和对应写权限的用户，可能在普通对话中触发路由修改或注册 token 创建，绕过“人工确认 -> 执行留痕”的运维闭环。
- **当前处理**: 旧 AI 闭环暂停开发。Hermes Agent 接入时重新设计 chat、pending confirmation、confirm/cancel 和审计链路；在此之前旧 AI 写入口不进入当前验收范围。

### BUG-26: AI 路由写工具绕过控制闭环，并可能错误恢复 inactive 节点

- **状态**: DEFERRED
- **优先级**: P1
- **文件**: `internal/agent/tools/route_management.go`, `pkg/controllerstorage/postgres.go`, `pkg/controllerstorage/policy_sync.go`, `internal/api/v2/setup.go`
- **说明**: `add_route` / `remove_route` 直接修改 `targetNode.AdvertisedRoutes` 后调用 `store.SaveNode(...)`。`SaveNode` 的 upsert 会强制 `status = 'online'`、`offline_since = NULL`。同时 AI 工具查询节点使用 `GetAllNodes()`，只排除 `deleted`，不排除 `suspended` / `banned`。
- **影响**: AI 修改路由不会创建 `desired_state_version`、`agent_commands`、`policy_deliveries`，前端无法回显真实下发状态；如果命中 suspended/banned 节点，还可能把节点状态错误改回 online。
- **当前处理**: 旧 AI 路由写工具暂停开发。Hermes Agent 接入后，任何路由写入都必须调用标准 route mutation / policy delivery 链路，不允许直接 `SaveNode`。

## 当前仍未闭合的 Bug

### BUG-27: `/api/v2/agents/network` legacy 路由入口仍绕过 policy delivery

- **状态**: OPEN
- **优先级**: P2
- **文件**: `internal/cli/controller_serve.go`, `pkg/controllerstorage/policy_sync.go`
- **说明**: `/api/v2/agents/network` 已挂载并具备 JWT、`routes:write`、租户边界和 inactive 节点保护，但最终调用 `UpdateTenantNodeAdvertisedRoutes(...)` 直接更新 `nodes.advertised_routes`，不会进入 `MutatePolicyAndQueueSync`。
- **影响**: 这条 legacy 入口修改路由后不会生成 `agent_commands`、`policy_deliveries`、`desired_state_version`，控制台监控和 Agent 下发状态会与 v2 标准路由 API 不一致。
- **建议修复**: 废弃该入口，或改造为复用标准 v2 route mutation/service，确保所有路由变更统一进入控制闭环。

### BUG-28: suspended/deleted 租户仍可能完成 Agent 接入和南向运行

- **状态**: OPEN
- **优先级**: P1
- **文件**: `internal/cli/controller_serve.go`, `internal/token/validator.go`, `pkg/controllerstorage/postgres.go`, `internal/controller/grpc/auth_interceptor.go`, `internal/controller/grpc/server.go`, `internal/cli/token_create.go`
- **说明**: `validateEnrollmentTokenTenant` 只调用 token validator 和 `GetTenantIDByToken`，没有校验租户 `status='active'`；`GetTenantIDByToken` 只从 `tokens` 表读取 `tenant_id`，没有 join/check `tenants.status`。注册路径中的 `SaveNode` 会插入或更新节点，并将 `status` 置为 `online`。gRPC runtime token 校验和拦截器只校验 runtime token 与节点状态，没有校验节点所属租户状态。CLI 显式 `--tenant-id` 创建 token 只排除 `deleted`，仍允许 `suspended`；API super_admin 创建 token 时也会绕过租户 active 状态判断。
- **影响**: suspended/deleted 租户下已有 active enrollment token 仍可能注册节点；已有 runtime token 的 Agent 也可能继续 metrics/command stream，只要节点本身未被 suspended/deleted/banned。这破坏了接入闭环和租户停用边界。
- **建议修复**: enrollment token 校验和 runtime token 请求路径都要校验租户 active 状态；租户 suspend/delete 时撤销 active token、失败化 pending commands，并按产品规则将租户节点标记为 suspended/disconnected；补 suspended/deleted 租户注册和 runtime gRPC 回归测试。

### BUG-29: suspended/deleted 租户用户仍可登录、刷新 token、获取权限

- **状态**: OPEN
- **优先级**: P2
- **文件**: `internal/api/handlers/auth.go`, `internal/api/v2/setup.go`, `frontend/src/stores/user.js`
- **说明**: 登录逻辑只查询 `users` 并签发 JWT，没有校验用户所属租户 active 状态；refresh 逻辑只重新加载 `users` 并签发新 token，同样没有校验租户状态；`HandlePermissions` 只按 tenant/role 读取权限，没有校验 tenant 是否 suspended/deleted。后续 tenant-scoped API 又会通过 `authorizeTenant` 拒绝 inactive tenant，形成登录成功但业务 API 失败的不一致状态。
- **影响**: suspended/deleted 租户用户仍能拿到有效前端会话和权限数据，UI 可进入系统并展示功能入口，但实际业务请求失败，权限和租户边界表现不一致。
- **建议修复**: 非 super_admin 的 login/refresh/permissions 都应 join/check `tenants.status='active'`，缺失、deleted、suspended 都拒绝；补 suspended/deleted 租户 login、refresh、permissions 测试，super_admin 例外路径单独覆盖。

### BUG-30: 前端权限刷新失败时会回退到本地内置权限

- **状态**: OPEN
- **优先级**: P2
- **文件**: `frontend/src/router/index.js`, `frontend/src/stores/user.js`
- **说明**: 路由守卫在权限刷新失败、store 缺失或租户缺失时，会调用 `permissionsForRole` 回退，并写入 `aria_permissions`。`userStore.loadPermissions` 捕获 `/v2/auth/permissions` 失败后调用 `loadFallbackPermissions`，该逻辑可能使用缓存权限或内置默认权限。
- **影响**: 后端仍会做最终权限校验，所以不是直接后端越权；但当前端权限接口失败、租户被停用或角色异常时，UI 可能展示用户不应看到的路由和操作按钮，造成错误操作预期和失败写入。
- **建议修复**: 生产登录态下权限刷新失败应 fail closed，使用空权限或展示会话/权限错误；如保留缓存，应按 user+tenant+role 分区并加 TTL，且后端 403/5xx 后不得回退到内置写权限。

### BUG-31: 监控在线状态忽略 suspended/banned 节点状态

- **状态**: OPEN
- **优先级**: P3
- **文件**: `internal/api/v2/operations.go`, `pkg/controllerstorage/monitoring_queries.go`, `pkg/controllerstorage/postgres.go`, `internal/api/v2/monitoring.go`
- **说明**: `nodeAvailabilityStatus` 只特殊处理 `deleted`，随后按 `last_seen` 判断 online/offline；`CountNodesByTenantAndStatus` 只排除 `deleted`；`GetNodesByTenant` 仍会返回 suspended/banned 节点，导致 topology 和 traffic 过滤也会包含 inactive 节点。
- **影响**: 最近上报过的 suspended/banned 节点可能在监控页显示为 online，Monitoring 卡片、拓扑链路和流量筛选会给出错误运行状态。
- **建议修复**: availability 计算先处理 deleted/suspended/banned；统计中区分非 deleted 总量与 eligible online，或在 online/peers/traffic/topology 中排除 inactive 节点；补 suspended/banned 节点最近 `last_seen` 的回归测试。

### BUG-32: 节点普通编辑接口可绕过 `routes:write` 和 policy delivery 修改 `advertised_routes`

- **状态**: OPEN
- **优先级**: P1
- **文件**: `internal/api/v2/setup.go`, `frontend/src/views/Nodes.vue`, `frontend/src/stores/node.js`, `frontend/src/composables/useRouteApi.js`
- **说明**: Nodes 页面编辑节点时会把 `advertised_routes` 放进 `PUT /api/v2/tenants/{tenant_id}/nodes/{node_id}`。后端该路径只要求 `nodes:write`，`updateTenantNode` 直接执行 `UPDATE nodes SET ... advertised_routes = $8`。这条路径没有进入 `/nodes/{id}/routes` 的 `routes:write` 权限、`writeTransactionalPolicyMutationSuccess`、`MutatePolicyAndQueueSync`、`agent_commands` 和 `policy_deliveries` 链路。
- **影响**: 拥有 `nodes:write` 但没有 `routes:write` 的用户可以通过节点编辑修改路由策略；写入成功后只更新数据库，Agent 不会收到策略同步命令，desired/applied/policy delivery 回显也不会更新，控制闭环会和真实运行态分叉。
- **建议修复**: 从通用节点编辑接口移除 `advertised_routes`，或在检测到该字段时强制要求 `routes:write` 并复用标准 route mutation/service；前端 Nodes 编辑应停止直接提交路由数组，改为调用 `useRouteApi` 的增删改接口并展示 dispatch/delivery 状态。

### BUG-33: hostname 复用会把 suspended/banned 节点软删除并替换成 online 新节点

- **状态**: OPEN
- **优先级**: P1
- **文件**: `internal/cli/controller_serve.go`, `pkg/controllerstorage/postgres.go`, `internal/cli/controller_registration_test.go`
- **说明**: 新公钥注册时，`processRegistration` 会调用 `ReuseHostnameIP(req.Hostname, requestedTenantID)` 复用同租户同 hostname 的旧节点 IP。`ReuseHostnameIP` 的查询条件是 `status != 'deleted'`，包含 `suspended` 和 `banned` 节点，随后直接把旧节点 `status` 更新为 `deleted` 并返回其 `assigned_ip/ip_offset` 给新节点。当前 `nodeRegistrationForbidden` 只检查同 public key 的 existing node，不能覆盖同 hostname、新 public key 的绕过路径。
- **影响**: 持有租户注册 token 的人可以用相同 hostname 注册新节点，把被暂停或封禁的旧节点变成 deleted，并生成一个 online 新节点复用原 IP，破坏节点生命周期和封禁边界。
- **建议修复**: `ReuseHostnameIP` 只允许复用 deleted 或明确可回收状态，不能自动回收 suspended/banned；如命中 suspended/banned hostname，应返回 409 并保留原节点状态。补同租户同 hostname、旧节点 suspended/banned、新公钥注册的回归测试。

### BUG-34: `/api/v2/agents/unregister` 未拒绝 suspended/banned 节点的 runtime token

- **状态**: OPEN
- **优先级**: P1
- **文件**: `internal/cli/controller_serve.go`, `internal/controller/grpc/auth_interceptor.go`, `internal/controller/grpc/server.go`, `internal/cli/controller_southbound_auth_test.go`
- **说明**: HTTP unregister 路径只通过 `authorizeRuntimeNodeByPublicKey` 校验 runtime token 的 node/tenant 绑定，没有检查节点当前 `status`。相比之下，证书发放和 gRPC runtime 路径已经通过 `nodeCertificateRequestForbidden` / `isInactiveNodeStatus` 拒绝 `deleted`、`suspended`、`banned`。因此 suspended/banned 节点仍可调用 unregister，把自己转换为 `deleted`。
- **影响**: 被暂停或封禁的 Agent 仍可用未过期 runtime token 自行删除生命周期状态；删除后又会落入“deleted 节点可 fresh enrollment”的路径，和 BUG-33 叠加时会削弱封禁和暂停的执行效果。
- **建议修复**: `authorizeRuntimeNodeByPublicKey` 或 `HandleUnregister` 增加 inactive 状态检查，至少拒绝 `suspended` / `banned`；如产品允许 online/offline 节点自注销，应为 suspended/banned 返回 403，并补 unregister 对 inactive 节点的回归测试。

### BUG-35: Agent 命令回报状态未做枚举和状态流转校验，可能卡死控制闭环

- **状态**: OPEN
- **优先级**: P1
- **文件**: `internal/controller/grpc/server.go`, `pkg/controllerstorage/agent_commands.go`, `pkg/controllerstorage/postgres.go`, `pkg/controllerstorage/node_control_states.go`, `frontend/src/utils/controlLoopStatus.js`
- **说明**: `CommandStream` 收到 Agent `CommandResponse` 后把 `resp.Status` 直接传给 `UpdateAgentCommandStatusForNode(...)`。存储层只校验 `commandID` 和 `status` 非空，然后把原始 `$2` 写入 `agent_commands.status` 和 `policy_deliveries.command_status`；建表语句中这两个字段只是 `VARCHAR(20)`，没有 CHECK 约束。当前也没有状态流转校验，已完成/失败的命令仍可能被后续响应改回非终态，未知状态也会被持久化。
- **影响**: 异常或恶意 Agent 可以把命令状态写成 `ready`、`running` 之外的任意短字符串，或把 terminal command 回退成非终态。后端超时清理和未完成计数只处理 `pending` / `sent` / `acknowledged`，`completed_at` 也只在 `completed` / `failed` 时设置，导致 `agent_commands`、`policy_deliveries`、`node_control_states` 与前端回显可能长期停在无法自动失败、无法重试、也不显示为 pending 的不一致状态，控制闭环无法可靠收敛。
- **建议修复**: 在 gRPC 或 storage 层统一校验状态枚举，只允许 `sent` / `acknowledged` / `completed` / `failed` 等受支持状态；增加合法流转约束，例如 `sent -> acknowledged -> completed|failed`，terminal 状态不可回退；对未知状态返回错误并记录审计/告警。补充测试覆盖未知状态拒绝、terminal 状态不可降级、policy delivery 不接受未知 `command_status`。

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

## 历史说明

- 较早一批问题（如 BUG-1 ~ BUG-11）已经在当前代码中维持修复状态，本次未逐条重写，只保留仍需关注的现实风险
- 旧版本文档里“未修复”与“已全部修复”同时存在的冲突表述已移除
- 后续如果继续维护这份文档，建议只保留“当前仍开放的问题”和“本轮新确认关闭的问题”
