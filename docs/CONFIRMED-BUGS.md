# 代码 Bug 追踪 (ALL FIXED ✅)

**最后更新**: 2026-04-18
**审查方法**: 逐行阅读代码，追踪函数调用链确认

---

## Critical

### [FIXED] BUG-1: acl_rules 表缺少 v2 ACL 所需列

- **修复**: 在 `pkg/controllerstorage/postgres.go` 的 migration 中添加了缺失的列。

### [FIXED] BUG-2: bumpNodeDesiredVersion ON CONFLICT 使用不存在的唯一约束

- **修复**: 在 `node_control_states` 上添加了 `UNIQUE INDEX (node_id)` 并在 SQL 中修正了 conflict target。

---

## High

### [FIXED] BUG-3: NewRateLimiter 存储已取消的 context

- **修复**: 改为存储 `context.Background()`，不再使用带有 defer cancel 的临时 context。

### [FIXED] BUG-4: mTLS 配置不验证客户端证书

- **修复**: 正确设置了 `ClientCAs` 并将 `ClientAuth` 提升为 `RequireAndVerifyClientCert`。

### [FIXED] BUG-5: HandleRegister 空 PublicKey 导致 panic

- **修复**: 在 `HandleRegister` 头部增加了非空校验。

### [FIXED] BUG-6: Rust Agent 硬编码接口名

- **修复**: 引入了 `get_active_interfaces()` 动态派生接口名。

---

## Medium

### [FIXED] BUG-7: Dashboard.vue WarningIcon 未定义

- **修复**: 统一修正为 `Warning` 图标导入。

### [FIXED] BUG-8: node store updateNodeRemote 未导出

- **修复**: 在 `stores/node.js` 的导出对象中补全了该方法。

### [FIXED] BUG-9: Settings.vue uploadHeaders 从 localStorage 读 token

- **修复**: 已统一迁往 `localStorage` 并修正读取逻辑。

### [FIXED] BUG-10: Settings.vue ElMessageBox 未导入

- **修复**: 已显式导入 `ElMessageBox`。

### [FIXED] BUG-11: ConsumeToken TOCTOU 竞态

- **修复**: 利用 SQL 原子更新逻辑重构了 `ConsumeToken`。
