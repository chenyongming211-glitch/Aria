# 高级 RBAC 设计方案

**版本**: v0.2.0
**日期**: 2026-04-19
**优先级**: P1

---

## 1. 背景

当前系统仅有 `super_admin` / `admin` / `member` / `viewer` 四个硬编码角色。`authorizeTenant` 只做"是否需要 admin"的二元判断，无法满足企业场景下的精细权限控制需求。

典型需求：
- "只读运维"：只能看监控和节点，不能改任何配置
- "网络管理员"：只能管 ACL/QoS/路由，不能管用户和 Token
- "安全审计员"：只能看审计日志和监控，不能操作

## 2. 设计目标

1. 支持自定义角色和操作级权限（`resource:action` 格式）
2. 内置三个系统角色（admin / operator / viewer），不可删除
3. 租户隔离：每个租户有独立的角色集
4. 向后兼容：不修改 `users.role` 列，通过角色名查表获取权限
5. super_admin 为平台级角色，跳过所有权限检查

## 3. 数据模型

### 3.1 roles 表

```sql
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(64) NOT NULL,
    description TEXT DEFAULT '',
    is_system BOOLEAN DEFAULT FALSE,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);
```

### 3.2 用户-角色关系

用户表的 `role` 列（VARCHAR）存储角色名，通过 `roles.name` 关联。无需修改用户表。

### 3.3 权限格式

`resource:action` 格式，例如：
- `nodes:read` — 查看节点列表和详情
- `acls:write` — 创建/修改/删除 ACL 规则

## 4. 权限清单

| 权限 | 说明 | viewer | operator | admin |
|------|------|--------|----------|-------|
| `nodes:read` | 查看节点 | ✅ | ✅ | ✅ |
| `nodes:write` | 修改/删除节点 | | ✅ | ✅ |
| `routes:read` | 查看路由 | ✅ | ✅ | ✅ |
| `routes:write` | 创建/修改/删除路由 | | ✅ | ✅ |
| `acls:read` | 查看 ACL | ✅ | ✅ | ✅ |
| `acls:write` | 创建/修改/删除 ACL | | ✅ | ✅ |
| `qos:read` | 查看 QoS | ✅ | ✅ | ✅ |
| `qos:write` | 创建/修改/删除 QoS | | ✅ | ✅ |
| `blacklist:read` | 查看黑名单 | ✅ | ✅ | ✅ |
| `blacklist:write` | 创建/修改/删除黑名单 | | ✅ | ✅ |
| `monitoring:read` | 查看监控 | ✅ | ✅ | ✅ |
| `commands:write` | 发送远程命令 | | ✅ | ✅ |
| `tokens:read` | 查看 Token | | | ✅ |
| `tokens:write` | 创建/删除 Token | | | ✅ |
| `users:read` | 查看用户 | | | ✅ |
| `users:write` | 创建/修改/删除用户 | | | ✅ |
| `roles:read` | 查看角色 | | | ✅ |
| `roles:write` | 创建/修改/删除角色 | | | ✅ |
| `ai:use` | 使用 AI 助手 | ✅ | ✅ | ✅ |
| `policies:read` | 查看策略 | ✅ | ✅ | ✅ |
| `settings:read` | 查看设置 | | | ✅ |
| `settings:write` | 修改设置 | | | ✅ |

## 5. 内置角色

| 角色 | 权限范围 | is_system |
|------|---------|-----------|
| `admin` | 全部 22 个权限 | true |
| `operator` | 所有 `:read` + 网络/策略/监控的 `:write` + `commands:write` + `ai:use`（共 16 个） | true |
| `viewer` | 所有 `:read` + `ai:use` + `policies:read`（共 10 个） | true |

每个租户创建时自动生成这三个内置角色。

## 6. 认证与授权流程（目标）

```
请求 → JWTAuthMiddleware → 提取 role + tenantID
     → authorizeTenantPermission(..., "acls:write")
       → 租户边界检查（始终执行）
       → role == "super_admin"? → 放行
       → 查询 roles 表获取 permissions[]
       → "acls:write" ∈ permissions[]? → 放行
       → 否则按 RBAC_ENFORCEMENT 模式处理
```

## 7. API 端点

### 角色管理

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/v2/tenants/{tid}/roles` | `roles:read` | 列出角色 |
| POST | `/api/v2/tenants/{tid}/roles` | `roles:write` | 创建自定义角色 |
| PUT | `/api/v2/tenants/{tid}/roles/{rid}` | `roles:write` | 更新角色权限 |
| DELETE | `/api/v2/tenants/{tid}/roles/{rid}` | `roles:write` | 删除自定义角色 |

### 权限检查集成（目标）

现有 v2 API 路由的 `authorizeTenant(w, req, tid, requireAdmin)` 调用将逐步替换为：
- 读操作：`authorizeTenantPermission(..., "xxx:read")`
- 写操作：`authorizeTenantPermission(..., "xxx:write")`

## 7.1 RBAC_ENFORCEMENT 运行模式（新增）

通过环境变量 `RBAC_ENFORCEMENT` 控制权限检查行为：

- `off`：仅做 tenant scope 校验，跳过 permission 拦截（兼容模式）
- `audit`：执行 permission 判定但不拦截，写 denied 审计日志，并返回 `X-RBAC-Audit-Denied: true`
- `enforce`：严格执行 permission 拦截（默认）

建议上线顺序：`audit` 观察 -> `enforce` 全量。

## 7.2 实施进度（2026-04-20，最新）

- ✅ 已完成：RBAC 三态运行模式（`off` / `audit` / `enforce`）
- ✅ 已完成：后端高优先级接口接入 `authorizeTenantPermission`
  - Roles / Users / Tokens
  - Nodes / Routes
  - Security（ACL / Blacklist）/ QoS
  - Agent Commands / Monitoring / AI
- ✅ 已完成：前端菜单、路由与按钮级权限守卫补齐
  - 路由元信息权限检查（router guard）
  - 侧边栏菜单可见性控制
  - 关键页面按钮显隐/禁用控制（Nodes / ACL / Tokens / Settings）
- ✅ 已完成：RBAC 权限矩阵自动化测试（后端 + 前端）
  - 后端：`off/audit/enforce` 三态下的接口级 `200/403` 与审计头断言
  - 前端：权限判断、路由守卫、页面级可见性测试
- ✅ 已完成：`nodes + monitoring` API 行为级测试扩展
  - 成功路径、参数错误、边界行为、错误码契约、跨租户隔离
  - 已纳入 CI 流程持续验证
## 8. 前端设计

### usePermission Composable

```js
// usePermission.js
export function usePermission() {
  const userStore = useUserStore()
  const hasPermission = (perm) => userStore.permissions.includes(perm)
  const hasAnyPermission = (perms) => perms.some(p => userStore.permissions.includes(p))
  return { hasPermission, hasAnyPermission }
}
```

### UI 权限控制

- 创建/编辑/删除按钮：`v-if="hasPermission('acls:write')"`
- 菜单项可见性：根据权限显示/隐藏
- 新增 Roles.vue 页面：角色列表 + 权限矩阵编辑

## 9. 迁移策略

1. 部署时自动创建 `roles` 表
2. 为每个现有租户自动创建内置角色（应用启动时检查）
3. 现有用户的 `role` 列值（admin/member/viewer）直接匹配内置角色名
4. `member` 角色映射为 `operator` 内置角色

## 10. 文件清单

| 文件 | 操作 |
|------|------|
| `pkg/controllerstorage/postgres.go` | 修改：新增 roles 表 DDL |
| `pkg/controllerstorage/rbac.go` | 新建：角色存储层 |
| `internal/api/middleware/permissions.go` | 新建：权限中间件 |
| `internal/api/v2/roles.go` | 新建：角色管理 API handler |
| `internal/api/v2/setup.go` | 修改：注册路由 + 权限检查替换 |
| `frontend/src/config/api.js` | 修改：新增角色 API 端点 |
| `frontend/src/composables/usePermission.js` | 新建：权限 composable |
| `frontend/src/views/Roles.vue` | 新建：角色管理页面 |
| `frontend/src/views/*.vue` | 修改：权限控制的 UI 元素 |
