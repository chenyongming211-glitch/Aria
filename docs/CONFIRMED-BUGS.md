# 代码 Bug 追踪

**最后复核**: 2026-06-07
**复核方式**: 静态代码复核 + GitHub Actions 回归验证
**说明**: 本文档记录“当前状态”，不再把历史 OPEN 条目和后续 FIXED 结论混写在一起。

---

## 当前仍未闭合的 Bug

暂无已确认仍开放的代码级 bug。

新的风险项应先进入 `KNOWN-ISSUES-STATUS.md`，确认可复现代码缺陷后再进入本文档。

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
- **文件**: `agent-rust/agent/src/unified_agent.rs`
- **说明**: 当前会先探测非回环、非 `aria*` 的物理接口，再传给系统优化模块

---

## 历史说明

- 较早一批问题（如 BUG-1 ~ BUG-11）已经在当前代码中维持修复状态，本次未逐条重写，只保留仍需关注的现实风险
- 旧版本文档里“未修复”与“已全部修复”同时存在的冲突表述已移除
- 后续如果继续维护这份文档，建议只保留“当前仍开放的问题”和“本轮新确认关闭的问题”
