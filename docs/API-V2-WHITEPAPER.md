# Aria API v2 完整接口白皮书

> **文档状态说明（2026-04-21）**
> - 北向业务 API 已完成 `v2-only` 收敛。
> - 南向 Agent 入口当前是固定 HTTP 路径 + gRPC `ControllerService` 的双层模型。
> - 本文档既记录当前已经落地的接口基线，也保留目标态契约；并非每个端点都已达到同样的产品化成熟度。
>
> **当前实现进度摘要**
> - [x] **Auth Domain**: JWT 登录、刷新与会话管理已就绪。
> - [x] **Topology Domain**: 节点管理、路由管理与 learned routes 展示已落地。
> - [x] **Security/QoS Domain**: Northbound CRUD、desired state、policy delivery 与状态回显已接通。
> - [x] **Monitoring Domain**: 状态收敛、节点详情、事件/告警、流量查询已接入。
> - [x] **AIOps Domain**: `chat/confirm` 最小闭环已接通。
> - [x] **Platform Settings Domain**: `settings/backups` 已完成最小可用的 create/list/download/delete/upload/restore；其他系统设置项仍保持隐藏，避免展示 placeholder。

本文档用于定义 Aria `API v2` 的统一接口边界。
它以当前 `v2-only` 基线为出发点，同时保留目标态约束；阅读时应区分“已经落地的接口契约”和“仍在继续产品化的能力”。

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
| `PUT` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/security/acls/{rule_id}` | 租户管理员 |
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
| `PUT` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/service/{rule_id}` | `SERVICE_QOS_MAP` |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/service/{rule_id}` | - |

### 6.2 节点对限速（中优先级，源 CIDR + 目标 CIDR）

| 方法 | URL | 匹配 eBPF MAP |
|---|---|---|
| `GET/POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/peers` | `PAIR_ID_QOS_MAP` |
| `PUT` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/peers/{rule_id}` | `PAIR_ID_QOS_MAP` |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/peers/{rule_id}` | - |

### 6.3 单节点限速（最低优先级，单个 CIDR）

| 方法 | URL | 匹配 eBPF MAP |
|---|---|---|
| `GET/POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/ip` | `SRC_ID_QOS_MAP` |
| `PUT` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/ip/{rule_id}` | `SRC_ID_QOS_MAP` |
| `DELETE` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/qos/ip/{rule_id}` | - |

注：QoS 所有接口均需租户隔离，增删操作需 `admin` 权限。

## 7. Policy Center（统一策略读模型）

提供租户作用域下统一的策略查询视图，用于将 `ACL`、`QoS`、`Route` 收敛到同一个读模型里。

说明：

- `Policy Center` 是统一读模型，不强制替代各子域自己的写接口
- 具体创建、更新、删除操作仍由 `ACL`、`QoS`、`Route` 子资源接口负责
- 返回结果应包含规则本身、目标节点、最近交付状态、交付历史与版本信息

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants/{tenant_id}/policies` | 租户隔离（Admin/Member） |

## 8. AI 智能运维域 (AIOps Domain)

注入了 Tenant Context 的安全 AI 助手。

| 方法 | URL | 业务说明 |
|---|---|---|
| `POST` | `/api/v2/tenants/{tenant_id}/ai/chat` | 提交对话与意图识别 |
| `POST` | `/api/v2/tenants/{tenant_id}/ai/confirm` | AI 敏感工具调用二次确认 |

## 9. 运维域 (Operations Domain)

提供租户作用域的监控视图与节点远程操作入口。

### 9.1 Monitoring

| 方法 | URL | 权限说明 |
|---|---|---|
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/stats` | 租户隔离（Admin/Member） |
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/events` | 租户隔离（Admin/Member） |
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/alerts` | 租户隔离（Admin/Member） |
| `POST` | `/api/v2/tenants/{tenant_id}/monitoring/alerts/{alert_id}/resolve` | 租户管理员（命令权限） |
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/traffic` | 租户隔离（Admin/Member） |
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/health` | 租户隔离（Admin/Member） |
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/topology` | 租户隔离（Admin/Member） |
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}` | 租户隔离（Admin/Member） |
| `GET` | `/api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}/metrics` | 租户隔离（Admin/Member） |

### 9.2 Agent Operations

| 方法 | URL | 权限说明 |
|---|---|---|
| `POST` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/agent/command` | 租户管理员 |
| `GET` | `/api/v2/tenants/{tenant_id}/nodes/{node_id}/agent/status` | 租户隔离（Admin/Member） |
| `POST` | `/api/v2/tenants/{tenant_id}/agents/command` | 租户管理员 |

## 10. API 总览（按领域）

> 说明：下表用于展示当前 `v2` 的领域边界与职责，不以固定端点数量作为约束；具体端点以各章节清单为准。

| 域 (Domain) | 核心职责 |
|---|---|
| 认证域 | 登录、注销、令牌刷新、强制改密 |
| 租户管理域 | 租户生命周期与配额调整 |
| IAM 域 | 租户成员管理、Enrollment Token、角色权限 |
| 拓扑域 | 节点纳管、路由管理与连通性 |
| 安全控制域 | ACL 与黑名单策略管理 |
| QoS 域 | 分层流量整形与限速策略 |
| Policy Center | 租户统一策略读模型与交付状态视图 |
| AI 域 | 智能运维对话与确认执行 |
| 运维域 | 监控视图、告警处理、节点状态与远程命令 |

## 11. 统一响应格式

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

## 12. 实施说明

当前仓库中的北向业务 API 已经收敛到 `/api/v2/...`，因此这里的后续工作重点不再是“迁移到 v2”，而是“把不同成熟度的 v2 端点继续做实”：

- 保持北向新增接口一律进入 `/api/v2/...`
- 继续补齐 `Policy Center`、`Monitoring` 与 `Agent Operations` 之间的工作流闭环
- 保持统一响应格式，不引入新的历史兼容路径
- 继续补齐 `settings/backups` 的恢复链路；未开放的设置项保持隐藏或只读
- 所有节点相关实现继续以“单节点单租户强归属”为前提
