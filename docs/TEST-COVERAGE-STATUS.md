# 测试覆盖状态（v0.1.0）

**更新时间**: 2026-04-20

## 1. 当前状态

- ✅ RBAC 三态（`off/audit/enforce`）已覆盖接口级权限矩阵测试
- ✅ 前端权限控制已覆盖 composable、路由守卫、页面可见性测试
- ✅ `nodes + monitoring` 已补齐 API 行为级测试（成功/参数错误/边界/错误码）
- ✅ 前端单测已接入 CI（`npm run test:run`）

## 2. 已覆盖重点

### 后端（Go）

- RBAC 接口行为矩阵：
  - Roles / Users / Routes / ACL / QoS / Tokens / Monitoring 等核心 handler
  - 覆盖 `read/write/commands` 与 `off/audit/enforce` 组合
- Nodes 与 Monitoring 行为测试：
  - 成功路径：`list/get/update/delete`、`stats/health/events/alerts/topology/traffic/node detail/node metrics`
  - 参数错误：无效 UUID、无效 `since`、无效 `range`、无效 path
  - 边界行为：分页钳制、topology 0/1/2 节点、traffic 无节点/节点查询失败降级
  - 错误码契约：`400/404/405/500` 与业务 code 断言
  - 租户隔离：跨租户访问 `404` 行为断言

### 前端（Vue）

- `usePermission` 权限判断（包含 wildcard）
- 路由权限元信息与 guard 跳转
- 关键页面按钮和菜单可见性/禁用态

## 3. CI 状态

- 后端测试与前端单测均已纳入 GitHub Actions
- 最近多轮提交持续通过，测试基线稳定

## 4. 可选增强（非阻塞 v0.1.0）

- Monitoring 与 VM 客户端交互的更深集成测试
- gRPC 端到端自动化与性能压测基线报告
- 覆盖率统计与阈值门禁（如 `go test -cover` + fail-under）
