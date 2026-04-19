# API 版本审计报告

**审计日期**: 2026-04-18

## 一、路由注册完整清单

### 南向/裸路径路由（controller_serve.go）

| 方法 | 路径 | 用途 | 调用方 |
|------|------|------|--------|
| ANY | `/api/v2/agents/register` | Agent 注册 | Rust Agent |
| ANY | `/api/v2/agents/unregister` | Agent 注销 | Rust Agent |
| ANY | `/api/v2/agents/network` | 网络路由管理 | ariactl CLI |
| ANY | `/api/version` | 版本信息 | 前端 |
| ANY | `/api/v2/integrations/dingtalk/webhook` | 钉钉 Webhook | 钉钉平台 |
| ANY | `/api/v2/integrations/feishu/webhook` | 飞书 Webhook | 飞书平台 |

### 北向 v2 路由（v2/setup.go）

**认证（无需 JWT）**

| 路径 | 操作 |
|------|------|
| `POST /api/v2/auth/login` | 登录 |
| `POST /api/v2/auth/refresh` | 刷新 Token |
| `POST /api/v2/auth/logout` | 注销 |
| `POST /api/v2/auth/force-change-password` | 强制改密 |

**租户级（需 JWT）**

| 路径 | 操作 |
|------|------|
| `GET/POST /api/v2/tenants` | 列出/创建租户 |
| `GET/PUT/DELETE /api/v2/tenants/{tid}` | 租户详情/更新/删除 |
| `GET/POST /api/v2/tenants/{tid}/users` | 用户列表/创建 |
| `PUT/DELETE /api/v2/tenants/{tid}/users/{uid}` | 用户更新/删除 |
| `GET/POST /api/v2/tenants/{tid}/tokens` | Token 列表/创建 |
| `GET/DELETE /api/v2/tenants/{tid}/tokens/{tok_id}` | Token 详情/删除 |
| `GET /api/v2/tenants/{tid}/policies` | 统一策略列表 |
| `GET /api/v2/tenants/{tid}/nodes` | 节点列表 |
| `GET/PUT/DELETE /api/v2/tenants/{tid}/nodes/{nid}` | 节点详情/更新/删除 |
| `GET/POST /api/v2/tenants/{tid}/nodes/{nid}/routes` | 路由列表/创建 |
| `GET/PUT/DELETE .../routes/{rid}` | 路由详情/更新/删除 |
| `GET/POST /api/v2/tenants/{tid}/nodes/{nid}/security/acls` | ACL 列表/创建 |
| `DELETE .../security/acls/{rid}` | ACL 删除 |
| `GET/POST .../security/blacklist/{scope}` | 黑名单列表/创建（scope: src/dst/ports） |
| `DELETE .../security/blacklist/{scope}/{rid}` | 黑名单删除 |
| `GET/POST .../qos/{category}` | QoS 列表/创建 |
| `DELETE .../qos/{category}/{rid}` | QoS 删除 |
| `POST .../nodes/{nid}/agent/command` | 发送远程命令 |
| `GET .../nodes/{nid}/agent/commands` | 命令列表 |
| `GET .../nodes/{nid}/agent/status` | Agent 状态 |
| `POST /api/v2/tenants/{tid}/agents/command` | 批量命令 |
| `GET .../monitoring/stats` | 监控统计 |
| `GET .../monitoring/events` | 事件列表 |
| `GET .../monitoring/alerts` | 告警列表 |
| `POST .../monitoring/alerts/{aid}/resolve` | 告警解决 |
| `GET .../monitoring/nodes/{nid}` | 节点监控详情 |
| `GET .../monitoring/nodes/{nid}/metrics` | 节点指标 |
| `GET .../monitoring/traffic` | 流量监控 |
| `GET .../monitoring/health` | 健康检查 |
| `GET .../monitoring/topology` | 拓扑信息 |
| `POST .../ai/chat` | AI 对话 |
| `POST .../ai/confirm` | AI 确认执行 |

---

## 二、前端 API 调用映射

前端 `config/api.js` 使用 axios `baseURL: '/api'`，所有路径自动加 `/api` 前缀。

| 前端常量 | 实际请求路径 | 匹配后端路由 |
|----------|-------------|-------------|
| `AUTH.LOGIN` = `/v2/auth/login` | `/api/v2/auth/login` | YES |
| `AUTH.REFRESH` = `/v2/auth/refresh` | `/api/v2/auth/refresh` | YES |
| `AUTH.LOGOUT` = `/v2/auth/logout` | `/api/v2/auth/logout` | YES |
| `AUTH.FORCE_CHANGE_PASSWORD` | `/api/v2/auth/force-change-password` | YES |
| `TENANT.*` | `/api/v2/tenants/...` | YES |
| `MONITOR.*` | `/api/v2/tenants/{tid}/monitoring/...` | YES |
| `IM.DINGTALK` = `/v2/integrations/dingtalk/webhook` | `/api/v2/integrations/dingtalk/webhook` | YES |
| `IM.FEISHU` = `/v2/integrations/feishu/webhook` | `/api/v2/integrations/feishu/webhook` | YES |
| `VERSION` = `/api/version` | `/api/version` | YES |

---

## 三、问题清单

### 功能性 Bug

| # | 问题 | 影响 | 严重度 |
|---|------|------|--------|
| 1 | MCP 白名单路径 `/v1/auth/*` 不匹配实际路由 `/api/v2/auth/*` | `must_change_password` 用户无法改密 | High |
| 2 | v2 ACL 代码引用不存在的数据库列 | v2 ACL 创建操作运行时失败 | Critical |

### 死代码

| # | 位置 | 内容 | 可删行数 |
|---|------|------|----------|
| 1 | `controller_serve.go` | 未注册 handler: HandleConfig、HandleListNodes、HandleTokens、HandleTokenRevoke、HandleTokenDetail | ~400 |
| 2 | `controller_serve.go` | 未使用类型/函数: PolicyRequest、PolicyResponse 等 | ~150 |
| 3 | `internal/api/v1/auth_api.go` | 整个文件 | ~200 |
| 4 | `internal/api/v1/tenant_api.go` | 整个文件 | ~300 |
| 5 | `internal/api/v1/tenant_management.go` | 整个文件 | ~200 |
| 6 | `internal/api/v1/chat.go` | 整个文件 | ~150 |
| 7 | `agent-rust/agent/src/config.rs` | current_credential 字段 | ~10 |

**合计可删除: ~1410 行**

### 重复代码

| # | 位置 | 说明 |
|---|------|------|
| 1 | `v1/common.go` vs `apibase/responses.go` | 相同的 WriteSuccess/WriteError/APIResponse |
| 2 | `v1/*.go` vs `handlers/*.go` | 相同 handler 的两个副本，v1 用 v1 import，handlers 用 apibase import |

### 不一致

| # | 问题 | 状态 |
|---|------|------|
| ~~1~~ | ~~`/version` 和 `/api/version` 重复注册~~ | ✅ 已移除 `/version` |
| ~~2~~ | ~~IM webhook 在 `/v1/im/` 不在 `/api/v2/`~~ | ✅ 已移除旧路径，仅保留 `/api/v2/integrations/*` |
| ~~3~~ | ~~`api.js` 定义 `VERSION: '/version'` 但 `app.js` 硬编码 `/api/version`~~ | ✅ 已统一使用 `api.js` 常量 |

---

## 四、修复步骤

### ~~步骤 1: 修复 MCP 白名单（紧急）~~ ✅ 已完成
### ~~步骤 2: 修复 ACL Schema（紧急）~~ ✅ 已完成
### ~~步骤 3: 合并工具包~~ ✅ 已完成
### ~~步骤 4: 删除死代码~~ ✅ 已完成
### ~~步骤 5: 统一 IM webhook 路径~~ ✅ 已完成
