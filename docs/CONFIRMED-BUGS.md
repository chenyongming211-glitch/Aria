# 代码 Bug 追踪

**最后更新**: 2026-04-18
**审查方法**: 逐行阅读代码，追踪函数调用链确认

---

## Critical

### BUG-1: acl_rules 表缺少 v2 ACL 所需列，创建 ACL 必然失败

- **文件**: `pkg/controllerstorage/postgres.go:253-267` (表定义), `pkg/controllerstorage/network_policy.go:197` (INSERT)
- **调用链**: 前端 ACLRules.vue → useAclApi.createACLRule → POST /v2/tenants/{tid}/nodes/{nid}/security/acls → security.go → Storage.CreateTenantNodeACLRule → SQL INSERT
- **问题**: `acl_rules` 表只有 `src_net`(CIDR)、`dst_net`(CIDR)、`min_port`、`max_port` 列。ALTER 语句（321-323行）只添加了 `src_node`、`dst_node`、`action`。v2 代码 INSERT 使用 `node_id`(UUID)、`src_cidr`、`dst_cidr`、`dst_port` 列，这些列不存在于表中。
- **复现**: 通过前端 Policy Center > ACL 管理创建任意 ACL 规则，后端返回 SQL 错误
- **影响**: v2 ACL 创建功能完全不可用

### BUG-2: bumpNodeDesiredVersion ON CONFLICT 使用不存在的唯一约束

- **文件**: `pkg/controllerstorage/network_policy.go:463-478`
- **调用链**: CreateTenantNodeACLRule/CreateTenantNodeQoSRule/DeleteTenantNode*Rule → bumpNodeDesiredVersion → SQL EXEC
- **问题**: `node_control_states` 表 PRIMARY KEY 为 `(node_id)`，`(tenant_id, node_id)` 上只有普通 INDEX（非 UNIQUE）。SQL `ON CONFLICT (tenant_id, node_id)` 要求目标必须是唯一约束或唯一索引。
- **复现**: 创建/删除任意 ACL 或 QoS 规则后触发版本递增
- **影响**: 每次 ACL/QoS 变更时 SQL 报错，desired_state_version 无法递增，前端无法正确显示 pending/converged 状态

---

## High

### BUG-3: NewRateLimiter 存储已取消的 context

- **文件**: `pkg/controllerstorage/redis.go:360-378`
- **调用链**: NewRateLimiter → 创建 timeout ctx + defer cancel() → 存 ctx 到 struct → 函数返回 cancel 执行 → 后续 Allow() 调用使用已取消 ctx
- **问题**: 第 367 行 `ctx, cancel := context.WithTimeout(...)` + 第 368 行 `defer cancel()`，第 376 行将 `ctx` 存入 RateLimiter struct。函数返回后 cancel() 执行，ctx 被取消。
- **影响**: 所有限流检查 Allow() 立即返回 context canceled 错误，限流失效

### BUG-4: mTLS 配置不验证客户端证书

- **文件**: `internal/cli/controller_serve.go:406-442`
- **调用链**: runControllerServe → tlsMode == "mutual" 分支 → 创建 tlsConfig → grpc.Creds
- **问题**: 
  1. 第 425-428 行创建 `caCertPool` 并加载 CA 证书，但第 431-436 行 `tls.Config` 未设置 `ClientCAs = caCertPool`
  2. `ClientAuth` 设为 `tls.RequestClientCert`（仅请求不验证），应使用 `tls.RequireAndVerifyClientCert`
- **影响**: mTLS 模式下任何客户端都可以不提供证书直接连接 gRPC 服务

### BUG-5: HandleRegister 空 PublicKey 导致 panic

- **文件**: `internal/cli/controller_serve.go:649, 678`
- **调用链**: POST /register → HandleRegister → GetNode("") 返回 nil → isReRegistration=false → 第 649/678 行 `req.PublicKey[:8]`
- **问题**: 未验证 PublicKey 非空。当 PublicKey 为空字符串时，`req.PublicKey[:8]` 产生 slice bounds out of range panic
- **复现**: 发送 `{"public_key": "", "token": "valid_token", "hostname": "test"}` 到 /register
- **影响**: 服务端 panic，HTTP handler 崩溃（Go net/http 会 recovery 但请求失败）

### BUG-6: Rust Agent 硬编码接口名

- **文件**: `agent-rust/agent/src/unified_agent.rs:1641-1642`
- **调用链**: run_main_loop → sync_peers → spawn_blocking → 硬编码 ["aria0","aria1","aria2","aria3"] → wg_managers.get(iface) → unwrap_or_else panic
- **问题**: `sync_peers` 和 `sync_advertised_routes` 中的接口名不从 `config.interface_name` 动态派生，而是硬编码为 aria0-3
- **复现**: 配置 `interface_name: "wg0"` 启动 multi_tunnel 模式
- **影响**: 非 aria0 配置时 agent panic 退出

---

## Medium

### BUG-7: Dashboard.vue WarningIcon 未定义

- **文件**: `frontend/src/views/Dashboard.vue:211, 396`
- **调用链**: eventTypeMap 对象初始化 → 引用 WarningIcon → ReferenceError
- **问题**: 第 211 行导入 `Warning`，第 396 行使用 `WarningIcon`（变量名不匹配）
- **影响**: 加载 Dashboard 时如果有 alert_fired 类型事件，运行时报错

### BUG-8: node store updateNodeRemote 未导出

- **文件**: `frontend/src/stores/node.js:199-236`
- **调用链**: Nodes.vue → nodeStore.updateNodeRemote(id, data) → TypeError: not a function
- **问题**: 函数在 199-220 行定义，但 226-236 行 return 对象中未包含 `updateNodeRemote`
- **影响**: 前端节点编辑保存功能失败

### BUG-9: Settings.vue uploadHeaders 从 localStorage 读 token

- **文件**: `frontend/src/views/Settings.vue:312-313`
- **调用链**: Settings 页面 → 上传备份文件 → Authorization header 读取 localStorage → 获取到 null
- **问题**: token 实际存储在 `sessionStorage`（见 stores/user.js），但 uploadHeaders 从 `localStorage` 读取
- **影响**: 文件上传的 Authorization header 永远是 `Bearer null`，上传请求被后端拒绝

### BUG-10: Settings.vue ElMessageBox 未导入

- **文件**: `frontend/src/views/Settings.vue:208, 401`
- **调用链**: 用户点击恢复备份 → restoreFromBackup() → ElMessageBox.confirm() → ReferenceError
- **问题**: 第 208 行只导入了 `ElMessage`，第 401 行使用 `ElMessageBox.confirm()`
- **影响**: 恢复备份操作报错

### BUG-11: ConsumeToken TOCTOU 竞态

- **文件**: `internal/token/validator.go:58-64`
- **调用链**: HandleRegister(并发请求) → ConsumeToken → Validate(通过) → IncrementUsage(竞争)
- **问题**: `Validate()` 检查 `used_count < max_uses` 通过后，`IncrementUsage()` 作为独立 SQL 执行。两个并发请求可能同时通过 Validate，然后竞争 IncrementUsage。虽然 IncrementUsage SQL 是原子的（WHERE used_count < max_uses），但 Validate + IncrementUsage 整体不是原子的。
- **实际风险**: 低（需要精确的并发窗口），但违反 token 一次性使用的语义
