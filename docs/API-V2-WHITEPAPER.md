# Aria API v2 完整接口白皮书

本文档用于定义 Aria `API v2` 的统一接口边界。它不是对当前 `v1` 与旧接口混用状态的描述，而是作为后续重构与实现的目标规范。

## 0. 多租户强隔离原则

Aria 的 `v2` 设计建立在强多租户模型之上，以下规则必须始终成立：

- 每个租户拥有自己的管理平面
- 每个 Agent 节点只能且必须属于一个租户
- 节点一旦完成注册，其 `tenant_id` 不得被其他租户接管或漂移
- `nodes`、`tokens`、`routes`、`security`、`qos`、`ai` 等所有资源都必须带租户边界
- 不允许通过同名 `hostname`、重复注册或错误 token 导致节点跨租户复用

因此，所有 `v2` 接口都必须将 `tenant_id` 视为强作用域，而不是可选筛选条件。

## 1. 认证域 (Auth Domain)

负责系统的全局鉴权、登录状态维护与基础安全闭环。

补充说明：

- `Auth Domain` 返回的是用户会话使用的 `JWT`
- 该 `JWT` 只用于控制台与北向管理 API
- 它和节点接入使用的 `Enrollment Token` 不是同一种凭据，二者不得混用

| 方法 | URL | 权限说明 |
|---|---|---|
| `POST` | `/api/v2/auth/login` | 公开 |
| `POST` | `/api/v2/auth/refresh` | JWT |
| `POST` | `/api/v2/auth/logout` | JWT |
| `POST` | `/api/v2/auth/force-change-password` | JWT（包含 `mcp=true` 的受限 Token） |

## 2. 租户管理域 (Tenant Domain)

控制台的全局运营底座，用于多租户生命周期与配额管理。

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants` | 超级管理员 / 租户管理员（后者仅可见自己） |
| `POST` | `/api/v2/tenants` | 超级管理员 |
| `GET` | `/api/v2/tenants/{tenant_id}` | 超级管理员 / 租户管理员 |
| `PUT` | `/api/v2/tenants/{tenant_id}` | 超级管理员 / 租户管理员（超管可改配额） |
| `DELETE` | `/api/v2/tenants/{tenant_id}` | 超级管理员（建议软删除） |

## 3. IAM 域（身份与访问管理）

控制租户内部的成员权限与自动化 API 调用凭证。

### 3.1 用户管理 (Users)

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants/{tenant_id}/users` | 租户管理员 |
| `POST` | `/api/v2/tenants/{tenant_id}/users` | 租户管理员 |
| `PUT` | `/api/v2/tenants/{tenant_id}/users/{user_id}` | 租户管理员 |
| `DELETE` | `/api/v2/tenants/{tenant_id}/users/{user_id}` | 租户管理员（不可删除自己） |

### 3.2 Token 管理 (Enrollment Tokens)

这里的 `tokens` 指的是 Agent 节点首次注册使用的 `Enrollment Token`，不是用户登录返回的 `JWT`。

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants/{tenant_id}/tokens` | 租户管理员 |
| `POST` | `/api/v2/tenants/{tenant_id}/tokens` | 租户管理员（唯一一次返回明文秘钥） |
| `GET` | `/api/v2/tenants/{tenant_id}/tokens/{token_id}` | 租户管理员 |
| `DELETE` | `/api/v2/tenants/{tenant_id}/tokens/{token_id}` | 租户管理员 |

## 4. 拓扑域 (Topology Domain)

SD-WAN 核心物理与逻辑网络资产管理。

### 4.1 节点管理 (Nodes)

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants/{tenant_id}/nodes` | 租户隔离（Admin/Member） |
| `POST` | `/api/v2/tenants/{tenant_id}/nodes` | 租户管理员 |
| `PUT` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}` | 租户管理员 |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}` | 租户管理员 |

补充规则：

- 节点只能出现在其所属租户的节点列表中
- 节点更新、删除、路由调整都不得跨租户执行
- 节点归属以 `tenant_id` 为准，`public_key` 仅是网络身份，`hostname` 仅作展示

### 4.2 节点路由管理 (Routes)

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/routes` | 租户隔离（Admin/Member） |
| `POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/routes` | 租户管理员 |
| `PUT` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/routes/{route_id}` | 租户管理员 |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/routes/{route_id}` | 租户管理员 |

## 5. 安全控制域 (Security Domain)

基于 eBPF 的节点入向与出向内核级安全防护。

### 5.1 ACL 策略管理（匹配 eBPF `POLICY_MAP`）

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/acls` | 租户隔离 |
| `POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/acls` | 租户管理员 |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/acls/{rule_id}` | 租户管理员 |

### 5.2 黑名单管理（匹配 eBPF `BLOCK_*_MAP`）

| 方法 | URL | 业务说明 |
|---|---|---|
| `GET/POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/blacklist/src` | 源 CIDR 黑名单（入向抗 DDoS） |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/blacklist/src/{rule_id}` | 删除规则 |
| `GET/POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/blacklist/dst` | 目标 CIDR 黑名单（出向防泄漏） |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/blacklist/dst/{rule_id}` | 删除规则 |
| `GET/POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/blacklist/ports` | 端口黑名单（封禁 445/135 等） |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/blacklist/ports/{port}` | 删除规则 |

## 6. QoS 域（质量与带宽控制）

基于 eBPF 的三级流量整形与限速架构。

### 6.1 服务级限速（最高优先级，五元组精确匹配）

| 方法 | URL | 匹配 eBPF MAP |
|---|---|---|
| `GET/POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/service` | `SERVICE_QOS_MAP` |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/service/{rule_id}` | - |

### 6.2 节点对限速（中优先级，源 CIDR + 目标 CIDR）

| 方法 | URL | 匹配 eBPF MAP |
|---|---|---|
| `GET/POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/peers` | `PAIR_ID_QOS_MAP` |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/peers/{rule_id}` | - |

### 6.3 单节点限速（最低优先级，单个 CIDR）

| 方法 | URL | 匹配 eBPF MAP |
|---|---|---|
| `GET/POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/ip` | `SRC_ID_QOS_MAP` |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/ip/{rule_id}` | - |

注：QoS 所有接口均需租户隔离，增删操作需 `admin` 权限。

## 7. AI 智能运维域 (AIOps Domain)

注入了 Tenant Context 的安全 AI 助手。

| 方法 | URL | 业务说明 |
|---|---|---|
| `POST` | `/api/v2/tenants/{tenant_id}/ai/chat` | 提交对话与意图识别 |
| `POST` | `/api/v2/tenants/{tenant_id}/ai/confirm` | AI 敏感工具调用二次确认 |

## 8. 运维域 (Operations Domain)

提供租户作用域的监控视图与节点远程操作入口。

### 8.1 Monitoring

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/stats` | 租户隔离（Admin/Member） |
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}` | 租户隔离（Admin/Member） |

### 8.2 Agent Operations

| 方法 | URL | 权限说明 |
|---|---|---|
| `POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/agent/command` | 租户管理员 |
| `GET` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/agent/status` | 租户隔离（Admin/Member） |
| `POST` | `/api/v2/tenants/{tenant_id}/agents/command` | 租户管理员 |

## 9. API 总览统计

| 域 (Domain) | 端点数量 | 核心职责 |
|---|---:|---|
| 认证域 | 4 | 登录、注销、令牌刷新、强制改密 |
| 租户管理域 | 5 | 客户入驻、配额调整、全局运维 |
| IAM 域 | 8 | 租户成员 CRUD、API Token 签发与吊销 |
| 拓扑域 | 8 | Agent 节点纳管、静态/策略路由下发 |
| 安全控制域 | 12 | eBPF 四层安全防火墙、出入向黑白名单 |
| QoS 域 | 9 | eBPF 三层级精细化流量整形 |
| AI 域 | 2 | 智能网络运维与 Agent 会话交互 |
| 运维域 | 5 | 监控视图、节点状态与远程命令 |
| 总计 | 53 | 覆盖现代 SD-WAN Controller 核心能力 |

## 10. 统一响应格式

所有 `v2` API 严格使用统一的 JSON 格式封装：

```json
{
  "success": true,
  "data": {},
  "message": "操作说明",
  "error": {
    "code": "ERROR_CODE",
    "message": "错误详情",
    "details": {}
  },
  "meta": {
    "total": 100,
    "page": 1,
    "page_size": 20
  }
}
```

## 11. 实施说明

当前仓库中已落地的是 `v1` 与部分旧接口混用状态。`v2` 白皮书作为目标规范，后续将采用以下方式逐步推进：

- 先搭建 `/api/v2` 命名空间与最小可用骨架
- 优先实现 `auth`、`tenants`、`users`、`tokens`、`nodes`
- 复用现有 `v1` 能力时保持统一响应格式
- 已被 `v2` 替代的 `v1` 北向管理接口应逐步下线并删除，不再作为长期兼容层保留
- 前端逐步从 `v1`/旧接口迁移到 `v2`
- 所有节点相关实现都必须先满足“单节点单租户强归属”约束
