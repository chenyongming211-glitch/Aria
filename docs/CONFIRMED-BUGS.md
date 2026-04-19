# 代码 Bug 追踪

**最后更新**: 2026-04-19
**审查方法**: 逐行阅读代码，追踪函数调用链确认

---

## 已修复 (FIXED ✅)

### BUG-1: acl_rules 表缺少 v2 ACL 所需列
- **文件**: `pkg/controllerstorage/postgres.go:321-327`
- **修复**: ALTER 语句添加了 `node_id`、`src_cidr`、`dst_cidr`、`dst_port`、`src_node`、`dst_node`、`action` 列。

### BUG-2: bumpNodeDesiredVersion ON CONFLICT 使用不存在的唯一约束
- **文件**: `pkg/controllerstorage/postgres.go:343` + `pkg/controllerstorage/network_policy.go:468`
- **修复**: 创建了 `CREATE UNIQUE INDEX ON node_control_states(node_id)`，`ON CONFLICT (node_id)` 正确匹配。

### BUG-3: NewRateLimiter 存储已取消的 context
- **文件**: `pkg/controllerstorage/redis.go:376`
- **修复**: 存储 `context.Background()`，不再使用带超时的临时 context。

### BUG-4: mTLS 配置不验证客户端证书
- **文件**: `internal/cli/controller_serve.go:431-437`
- **修复**: `ClientAuth` 设为 `RequireAndVerifyClientCert`，`ClientCAs` 设为 `caCertPool`。

### BUG-5: HandleRegister 空 PublicKey 导致 panic
- **文件**: `internal/cli/controller_serve.go:603-607`
- **修复**: 增加 `if req.PublicKey == ""` 非空校验。

### BUG-6: Rust Agent 硬编码接口名
- **文件**: `agent-rust/agent/src/unified_agent.rs:167-177`
- **修复**: 引入 `get_active_interfaces()` 从配置动态派生接口名。

### BUG-7: Dashboard.vue WarningIcon 未定义
- **文件**: `frontend/src/views/Dashboard.vue:211,396`
- **修复**: 统一为 `Warning` 图标导入和使用。

### BUG-8: node store updateNodeRemote 未导出
- **文件**: `frontend/src/stores/node.js:234`
- **修复**: 在 return 对象中补全了 `updateNodeRemote`。

### BUG-10: Settings.vue ElMessageBox 未导入
- **文件**: `frontend/src/views/Settings.vue:208`
- **修复**: 显式导入 `ElMessageBox`。

### BUG-11: ConsumeToken TOCTOU 竞态
- **文件**: `internal/token/validator.go:58-71` + `internal/token/store.go:145-155`
- **修复**: `ConsumeToken` 改为先执行原子 `IncrementUsage`（单条 SQL `UPDATE ... WHERE status='active' AND used_count < max_uses`），消除 Validate+Increment 间的竞态窗口。

---

## 未修复 (OPEN 🔴)

### Critical

#### BUG-12: acl_rules.id 类型不匹配导致 v2 ACL API 完全不可用

- **文件**: `pkg/controllerstorage/postgres.go:254` + `pkg/controllerstorage/network_policy.go:40,194-210,219-222`
- **问题**: `acl_rules` 表的 `id` 列是 `SERIAL PRIMARY KEY`（整数），但 v2 API 的 `ACLRuleRecord.ID` 是 `uuid.UUID`。`CreateTenantNodeACLRule` 的 `RETURNING id` 会将整数扫描到 `uuid.UUID`，导致 scan 错误。`DeleteTenantNodeACLRuleByID` 传 `uuid.UUID` 给 `WHERE id = $1` 同样会失败。
- **影响**: v2 API 的 ACL 创建和删除操作全部失败。

#### BUG-13: 前后端 ACL 字段名不匹配，创建规则数据丢失

- **文件**: `frontend/src/composables/useAclApi.js:27-49` vs `internal/api/v2/security.go:68-76`
- **问题**: 前端 `normalizeRulePayload` 发送 `src_net`、`dst_net`、`min_port`、`max_port`，但 v2 后端期望 `src_cidr`、`dst_cidr`、`dst_port`。这些 JSON 字段名不匹配，后端解码时全部为零值。
- **影响**: 通过前端创建的 ACL 规则，源/目标 CIDR 为空，端口为 0，规则无效。

---

### High

#### BUG-9: Settings.vue uploadHeaders 从 sessionStorage 读 token（实际未修复）

- **文件**: `frontend/src/views/Settings.vue:313` vs `frontend/src/stores/user.js:26`
- **问题**: user store 和 API interceptor 都使用 `localStorage` 存取 token（`user.js:26` `localStorage.setItem('aria_token', token)`），但 `Settings.vue:313` 上传 header 仍从 `sessionStorage.getItem('aria_token')` 读取。`sessionStorage` 中无此 key，返回 `null`。
- **影响**: 所有文件上传操作缺少有效 token，服务端返回 401。

#### BUG-14: AI 服务 maxTokens 未初始化，ChatWithContext 永远无上下文

- **文件**: `internal/service/ai_service.go:25,61-63,103`
- **问题**: `NewAIService` 构造函数只设置了 `agent`，未设置 `maxTokens`，默认为 0。`ChatWithContext` 中 `if len(history) > s.maxTokens` 即 `len(history) > 0` 始终为真，导致 `history = history[len(history):]`（空切片）。每次对话历史都被清空。
- **影响**: 飞书等场景的连续对话功能完全失效，每次对话都不保留上下文。

#### BUG-15: CIDR 路由 ID 中的 `/` 导致 URL 路径解析失败

- **文件**: `internal/api/v2/setup.go:955-960,607-623` + `frontend/src/config/api.js:101`
- **问题**: 路由 ID 使用 CIDR 格式如 `10.0.0.0/24`。前端用 `encodeURIComponent` 编码为 `%2F`，但 Go 的 `http.Request.URL.Path` 会自动解码为 `/`。`splitPath` 按 `/` 分割后，`10.0.0.0/24` 被拆成两个 segment，`len(parts)==9` 不等于预期的 8，落入错误分支。
- **影响**: 无法通过 v2 API 对 CIDR 路由执行 GET/PUT/DELETE 单条操作。

#### BUG-16: 流量监控数据时间戳与下载数据不对齐

- **文件**: `internal/api/v2/monitoring.go:409-445`
- **问题**: `timestamps` 数组只在 TX 查询结果中收集（line 418），RX 查询只收集 `downloadBytes`（line 437）不收集时间戳。如果 TX 无数据但 RX 有数据，`timestamps` 为空数组，而 `downloadBytes` 有数据点，图表渲染失败。
- **影响**: 当节点只有下载流量没有上传流量时，流量图表显示异常。

#### BUG-17: CreateTenant 丢弃 email 和 phone 字段

- **文件**: `internal/api/handlers/tenant.go:57,64-70`
- **问题**: INSERT 语句只包含 `id, name, code`，未存储 `email` 和 `phone`。但响应中返回了 `req.Email` 和 `req.Phone`，给客户端造成了数据已存储的假象。
- **影响**: 租户联系信息永远不会被持久化，数据丢失但客户端不知情。

---

### Medium

#### BUG-18: sync_failed 和 policy_failed 告警重复创建

- **文件**: `pkg/controllerstorage/alert_generator.go:82-113,115-147`
- **问题**: `GenerateSyncFailedAlert` 和 `GeneratePolicyFailedAlert` 不检查是否已存在活跃告警，直接创建新告警。而 `GenerateNodeOfflineAlert`（line 12-21）正确使用了 `GetActiveAlertByNodeAndType` 做幂等检查。
- **影响**: 同一节点的命令执行失败或策略下发失败会反复创建重复告警。

#### BUG-19: QoS bandwidth_mbps 默认为 0 导致 eBPF 令牌桶完全阻断流量

- **文件**: `frontend/src/composables/useQosApi.js:54` + `agent-rust/agent/src/qos.rs:698-703`
- **问题**: 前端 `bandwidth_mbps: Number(rule.bandwidth_mbps || rule.bandwidth || 0)` 当两者都未填写时发送 0。Rust agent 的 `calculate_bucket_params(0)` 将 rate 设为 0，令牌桶永远不会补充令牌，所有匹配的流量被完全阻断。
- **影响**: 未填写带宽限制的 QoS 规则会静默阻断所有匹配流量，而非"不限速"。

#### BUG-20: Rust agent 硬编码 eth0 物理接口

- **文件**: `agent-rust/agent/src/unified_agent.rs:356`
- **问题**: `SystemOptimizer::new(*port, "eth0".to_string(), iface.clone())` 硬编码了 `eth0` 作为物理网卡名。云服务器常用 `ens192`、`enp0s3` 等接口名。
- **影响**: TSO/GSO/GRO 优化和 ring buffer 调优都应用于不存在的接口，静默失败，无法生效。

---

## 统计

| 状态 | 数量 | 编号 |
|------|------|------|
| 已修复 ✅ | 10 | BUG-1~8, BUG-10~11 |
| 未修复 🔴 | 10 | BUG-9, BUG-12~20 |
