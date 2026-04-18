# 已知问题状态追踪

**更新日期**: 2026-04-18
**基准文档**: `docs/V0.1.0-PRODUCT-BLUEPRINT.md` 第 13 节

---

## 1. API 命名空间不统一（v1/v2/裸路径并存）

**状态**: 大部分已修复

### 已完成
- 前端 100% 迁移到 `/api/v2/...`，共 37 个 v2 端点
- 后端北向 API 全部统一到 `/api/v2/` 路径
- 旧路由（`/routing`、`/acl-rules`、`/bandwidth-control` 等）均有重定向到新路径
- gRPC 南向 API 独立于 REST 路径体系（设计如此）

### 遗留问题

#### 1a. MCP 白名单路径错误（功能性 Bug）
- `internal/api/middleware/jwt_auth.go:25` 白名单引用 `/v1/auth/*`
- 实际注册路径为 `/api/v2/auth/*`
- **影响**: `must_change_password=true` 的用户永远无法访问改密端点，因为白名单不匹配
- **修复**: 将白名单更新为 `/api/v2/auth/force-change-password`

#### 1b. IM webhook 路径
- `/v1/im/dingtalk` 和 `/v1/im/feishu` 仍为 v1 路径
- 这些是外部平台回调，迁移需同步更新钉钉/飞书平台配置
- 建议低优先级处理

#### 1c. 重复 `/version` 端点
- `/version` 和 `/api/version` 注册同一 handler
- 前端实际调用 `/api/version`，裸 `/version` 无使用者
- 可安全移除 `/version` 注册

#### 1d. v1 死代码
- `internal/api/v1/` 目录中 `auth_api.go`、`tenant_api.go`、`tenant_management.go`、`chat.go` 全部为死代码
- 仅 `v1/common.go` 仍被 `v2/operations.go` 和 `v2/security.go` 引用
- `controller_serve.go` 中约 400 行未注册的 handler 方法可删除：
  `HandleConfig`、`HandleListNodes`、`HandleTokens`、`HandleTokenRevoke`、`HandleTokenDetail`
- 未使用的类型和函数：`PolicyRequest`、`PolicyResponse`、`CreateTokenRequest`、
  `TokenDetailResponse`、`generateTokenID`、`parseTTL`、`parsePortRange`、`formatPortRange`、`ipInRoutes`

#### 1e. 三套重复工具包
- `v1/common.go`、`apibase/responses.go`、`handlers/*` 提供相同功能
- `operations.go` 和 `security.go` 仍引用 `v1`，应迁移到 `apibase`

### 分析详情
见 `docs/API-VERSION-AUDIT.md`

---

## 2. 节点身份与运行鉴权未分离

**状态**: 部分修复

### 已完成
- `node_id` (UUID) 已引入为数据库主键，区别于 `public_key`
- `machine_id` 已采集并存储
- gRPC `SyncRequest` 优先使用 `node_id`，`public_key` 降级为 fallback
- 注册阶段 Enrollment Token 验证正常工作

### 遗留问题
- `SaveNode()` 仍使用 `ON CONFLICT (public_key)` 做 upsert
- `GetNode()`、`GetNodeByTenant()` 等核心查询仍按 `public_key` 查找
- `agent_commands` 表仍用 `node_public_key` 做外键关联
- **运行期零鉴权**: Sync/Metrics/CommandStream 不验证任何凭据，agent 只发送 node_id + public_key
- `current_credential` 字段已声明但从未使用（死代码）
- mTLS 配置存在 bug: 使用 `RequestClientCert` 而非 `RequireAndVerifyClientCert`，且未设置 `ClientCAs`

### 安全风险
任何知道节点 `public_key` 或 `node_id` 的攻击者可伪造 Sync/Metrics 请求。

---

## 3. Sync 即时拼装，未版本化

**状态**: 已修复

### 已实现
- Protobuf: `SyncRequest.applied_state_version` + `SyncResponse.desired_state_version`
- 数据库: `node_control_states` 表，含 desired/applied 版本列及收敛状态计算
- Controller: 版本创建（`dsv-{timestamp}-{uuid}` 格式）、比较、通过 gRPC 报告
- Rust Agent: 发送 applied 版本、接收 desired 版本、持久化到 `agent-state.yaml`
- API v2: `monitoring/nodes/{nid}` 返回收敛状态（converged/pending/diverged/offline）
- 前端: `NodeMonitorDetail.vue` 展示收敛状态卡片，支持状态着色

---

## 4. 前端菜单与后端领域模型未对齐

**状态**: 已修复

### 当前导航结构（与蓝图完全对齐）
- Dashboard（顶级 `/dashboard`）
- Nodes（顶级 `/nodes`）
- Connectivity（子菜单：路由管理 + VPN 拓扑）
- Policy Center（子菜单：统一概览 + ACL 管理 + 带宽控制）
- Monitoring（顶级 `/monitoring`）
- AI Copilot（顶级 `/ai-copilot`）
- Platform（子菜单：Token + 租户 + 设置）

### 遗留小问题
- Policy Center 子菜单首项与父级同名（`nav.policyCenter`）
- 路由策略同时出现在 Connectivity（可编辑）和 Policy Center（只读）中

---

## 5. ACL / Policies / Bandwidth 概念重叠

**状态**: 部分修复

### 已完成
- 统一读 API: `GET /v2/tenants/{tid}/policies?kind=acl|qos|route`
- 统一前端 Policy Center 只读概览页
- 策略交付追踪统一: `policy_deliveries` 表支持 `policy_domain = acl|qos|route`

### 遗留问题
- 无统一 Policy 数据表: 仍是 3 张独立表（`acl_rules`、`qos_rules`、`blacklist_rules`）
- 无统一 Policy Go struct: 5 个独立 struct（`ACLRule`、`ACLRuleRecord`、`QoSRuleRecord`、`BlacklistRuleRecord`、`PolicyDelivery`）
- 两套 ACL 模型共存: 旧版 `ACLRule`（全局 scope，AI 工具使用）与 v2 `ACLRuleRecord`（node-scoped）写入同一张表但字段不兼容
- **Schema 不一致（严重）**: v2 ACL 代码使用 `node_id`、`src_cidr`、`dst_cidr` 列，但数据库 migration 仅创建了 `src_net`、`dst_net` 列，未添加 v2 所需列。v2 ACL 创建操作可能在运行时失败
- 无统一写 API: 各子域仍使用独立 CRUD 端点
- Agent 端未统一: `AclManager` 和 `QoSManager` 完全独立，使用不同 eBPF Map

### 数据一致性风险
两套 ACL 模型写入同一 `acl_rules` 表但使用不同字段集，可能导致统一查询遗漏 v2 创建的规则。

---

## 6. 代码审查发现的 Bug（2026-04-18 新增）

### Critical

| # | 文件 | 问题 |
|---|------|------|
| B1 | `pkg/controllerstorage/postgres.go:388` + `network_policy.go:466` | `node_control_states` 表 PRIMARY KEY 为 `node_id`，但 UPSERT 用 `ON CONFLICT (tenant_id, node_id)` |
| B2 | `pkg/controllerstorage/redis.go:360` | RateLimiter 存储已取消的 context，所有限流检查立即失败 |
| B3 | `internal/api/v2/security.go` + `pkg/controllerstorage/postgres.go` | v2 ACL 写入引用不存在的列（`node_id`、`src_cidr`、`dst_cidr`），migration 未创建这些列 |

### High

| # | 文件 | 问题 |
|---|------|------|
| B4 | `internal/api/v1/tenant_management.go:92` | `SuperAdminOnly` 检查 `role != "admin"` 应为 `role != "super_admin"` |
| B5 | `internal/cli/controller_serve.go:905` | syncNode 正常路径获取全局 ACL，泄露跨租户规则 |
| B6 | `internal/token/validator.go:58` | ConsumeToken TOCTOU 竞态，可双花 |
| B7 | `internal/api/v2/platform.go:62` | createTenantToken 创建零值 token 且前缀不一致 |
| B8 | `internal/cli/controller_serve.go:675` | 空 PublicKey 导致 panic |
| B9 | `internal/cli/controller_serve.go:430` | mTLS 用 RequestClientCert 且未设 ClientCAs |
| B10 | `internal/api/middleware/jwt_auth.go:25` | MCP 白名单路径错误，阻止改密流程 |
| B11 | `agent-rust/agent/src/unified_agent.rs:1641` | 硬编码接口名 `["aria0","aria1","aria2","aria3"]` |
| B12 | `agent-rust/agent/src/grpc_client.rs:95` | gRPC 连接无超时 |
| B13 | `agent-rust/agent/src/unified_agent.rs:1765` | adjust_endpoint_port 丢弃原始端口 |

### Medium

| # | 文件 | 问题 |
|---|------|------|
| B14 | `internal/auth/jwt.go:17` | JWTSecret 竞态读写 |
| B15 | `internal/cli/controller_serve.go:2096` | ensureSuperAdmin 吞掉数据库错误 |
| B16 | `internal/im/feishu.go:142` | 事件处理 goroutine 无限泄漏 |
| B17 | `internal/api/middleware/jwt_auth.go` + `tenant.go` | context 中 tenant_id 类型不一致（string vs UUID） |
| B18 | `agent-rust/agent/src/unified_agent.rs:2058` | config reload 不更新 sync_interval timer |
| B19 | `agent-rust/agent/src/qos.rs:698` | QoS mbps=0 导致永久丢弃所有流量 |
| B20 | `frontend/src/stores/user.js:25` | token 在 sessionStorage 中跨 tab 丢失 |
| B21 | `frontend/src/router/index.js:144` | 路由守卫不检查 token 过期时间 |
| B22 | `frontend/src/views/AIAssistant.vue:73` | v-html XSS 风险 |
| B23 | `frontend/src/views/Settings.vue:312` | uploadHeaders 从 localStorage 读 token（实际在 sessionStorage） |
| B24 | `frontend/src/stores/node.js:226` | updateNodeRemote 未导出 |
| B25 | `frontend/src/views/Dashboard.vue:396` | WarningIcon 未定义 |
| B26 | `frontend/src/views/Settings.vue:401` | ElMessageBox 未导入 |
| B27 | `frontend/src/views/Routing.vue:261` | deleteRoute 传 CIDR 当 routeId |

### Dead Code（可安全删除）

| 位置 | 内容 | 行数 |
|------|------|------|
| `controller_serve.go` | `HandleConfig`、`HandleListNodes`、`HandleTokens`、`HandleTokenRevoke`、`HandleTokenDetail` | ~400 行 |
| `controller_serve.go` | `PolicyRequest`、`PolicyResponse`、`CreateTokenRequest`、`TokenDetailResponse` struct | ~50 行 |
| `controller_serve.go` | `generateTokenID`、`parseTTL`、`parsePortRange`、`formatPortRange`、`ipInRoutes` 函数 | ~100 行 |
| `internal/api/v1/auth_api.go` | 整个文件 | ~200 行 |
| `internal/api/v1/tenant_api.go` | 整个文件 | ~300 行 |
| `internal/api/v1/tenant_management.go` | 整个文件（`SuperAdminOnly` 除外，需迁到 v2） | ~200 行 |
| `internal/api/v1/chat.go` | 整个文件 | ~150 行 |
| `agent-rust/agent/src/config.rs` | `current_credential` 字段 | ~10 行 |
| `frontend/src/config/api.js` | `VERSION: '/version'` 与实际使用不一致 | 配置 |

**预计可删除总行数**: ~1400 行

---

## 总结

| # | 问题 | 状态 | 剩余工作量 |
|---|------|------|-----------|
| 1 | API 命名空间 | 大部分修复 | 清理死代码 + MCP 白名单修复 |
| 2 | 节点身份与鉴权 | 部分修复 | 需实现运行期鉴权 + 存储层迁移 |
| 3 | Sync 版本化 | 已修复 | 无 |
| 4 | 前端菜单对齐 | 已修复 | 小调整即可 |
| 5 | ACL/Policy/Bandwidth | 部分修复 | Schema 修复 + 统一数据模型 |
