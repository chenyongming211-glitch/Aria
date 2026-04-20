# API 版本审计报告（最新）

**更新时间**: 2026-04-20  
**结论**: 业务 API 已完成 **v2-only** 收敛，`internal/api/v1` 已移除，前端与测试调用路径已统一到 `/api/v2/...`。

## 一、当前路由基线

### Southbound / 平台固定入口

| 方法 | 路径 | 用途 | 调用方 |
|------|------|------|--------|
| ANY | `/api/v2/agents/register` | Agent 注册 | Rust Agent |
| ANY | `/api/v2/agents/unregister` | Agent 注销 | Rust Agent |
| ANY | `/api/v2/agents/network` | 网络管理 | ariactl |
| ANY | `/api/version` | 版本信息 | 前端 |
| ANY | `/api/v2/integrations/dingtalk/webhook` | 钉钉 Webhook | 钉钉平台 |
| ANY | `/api/v2/integrations/feishu/webhook` | 飞书 Webhook | 飞书平台 |

### Northbound / 租户域管理 API

所有业务能力统一挂载到 `/api/v2/...`：

- Auth: `/api/v2/auth/*`
- Tenant & IAM: `/api/v2/tenants/*`, `/users`, `/tokens`, `/roles`
- Topology & Policy: `/nodes`, `/routes`, `/security/acls`, `/qos/*`, `/policies`
- Operations: `/monitoring/*`, `/agent/command`, `/agent/status`
- AIOps: `/ai/chat`, `/ai/confirm`
- Platform Settings: `/api/v2/settings/backups/*`

## 二、前端调用基线

前端 `frontend/src/config/api.js` 使用 `baseURL: '/api'`，业务端点均定义为 `/v2/...` 前缀并拼接成 `/api/v2/...`。

覆盖范围：

- `AUTH.*` -> `/api/v2/auth/*`
- `TENANT.*` -> `/api/v2/tenants/*`
- `AGENT.*` -> `/api/v2/tenants/{tid}/nodes/{nid}/agent/*`
- `MONITOR.*` -> `/api/v2/tenants/{tid}/monitoring/*`
- `AI.*` -> `/api/v2/tenants/{tid}/ai/*`
- `SETTINGS.*` -> `/api/v2/settings/backups/*`

## 三、v1 清理状态

### 已完成

- `internal/api/v1` 目录已删除。
- 业务代码中不再存在 `/v1/...` 与 `/api/v1/...` 调用。
- 前端单测中旧的 `/v1/...` 断言已迁移为 `/v2/...`。
- README API 示例已更新为 v2 路径。

### 保留项（非业务 v1，不属于迁移范围）

- 第三方系统协议路径（例如 VictoriaMetrics 的 `/api/v1/query`）按上游规范保留。

## 四、后续维护约束

- 新增接口一律落在 `/api/v2/...`。
- 禁止新增任何业务 `/v1/...` 路径。
- 若发现旧路径引用，按缺陷处理并在同一 PR 内完成迁移。
