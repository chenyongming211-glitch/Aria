# 前端 API 对接总结

## 概述

本文档描述了前端与后端 v1 API 的对接情况。

## 更新的文件

### 1. API 配置 (`src/config/api.js`)

**更新内容：**
- 更新 `API_ENDPOINTS` 以匹配后端 v1 API 路径
- 新增带宽管理 API 端点 (`BANDWIDTH`)
- 新增租户管理 API 端点 (`TENANT`)
- 新增监控 API 端点 (`MONITOR`)
- 新增 AI 聊天 API 端点 (`AI`)
- 保留旧版 API 端点以向后兼容

### 2. 请求拦截器 (`src/composables/useApi.js`)

**更新内容：**
- 更新响应拦截器以处理后端的统一响应格式
- 统一响应格式：`{ success: true, data: {...}, message: "..." }`

### 3. 新增 API 模块

#### 3.1 带宽管理 API (`src/composables/useQosApi.js`)

**新增方法：**
- `getBandwidthLimits()` - 获取所有带宽限制
- `createBandwidthLimit(params)` - 创建带宽限制
- `deleteBandwidthLimit(limitId)` - 删除带宽限制
- `getPolicies(filters)` - 获取所有策略
- `getPolicy(policyId)` - 获取策略详情
- `createPolicy(policy)` - 创建策略
- `updatePolicy(policyId, policy)` - 更新策略
- `deletePolicy(policyId)` - 删除策略

**向后兼容方法：**
- `getAllRules()` - 获取所有 QoS 规则（已弃用）
- `createServiceRule(rule)` - 创建服务级规则（已弃用）
- `createPortRule(rule)` - 创建端口级规则（已弃用）
- `createPeerRule(rule)` - 创建 Peer 级规则（已弃用）
- `createIpRule(rule)` - 创建 IP 级规则（已弃用）
- 各种 update/delete 方法（已弃用）

#### 3.2 Token 管理 API (`src/composables/useTokenApi.js`)

**新增方法：**
- `getAllTokens()` - 获取所有令牌
- `createToken(params)` - 创建新令牌
- `deleteToken(tokenId)` - 删除令牌（新 API）
- `getTokenDetail(token)` - 获取令牌详情

**向后兼容方法：**
- `revokeToken(tokenId)` - 吊销令牌（已弃用）
- `getTokenNodes(token)` - 获取令牌的使用节点

#### 3.3 监控 API (`src/composables/useMonitorApi.js`)

**新增方法：**
- `getStats()` - 获取监控统计数据
- `getNodeDetail(nodeId)` - 获取节点详情
- `getHealth()` - 获取健康状态
- `getVersion()` - 获取版本信息

#### 3.4 AI 聊天 API (`src/composables/useAiApi.js`)

**新增方法：**
- `chat(params)` - AI 对话
- `confirm(params)` - 确认工具执行

#### 3.5 租户管理 API (`src/composables/useTenantApi.js`)

**新增方法：**
- `getCurrentTenant()` - 获取当前租户信息
- `listTenants()` - 获取租户列表（系统管理员）
- `createTenant(tenant)` - 创建租户（系统管理员）
- `getTenantNodes()` - 获取租户节点
- `getTenantACLRules()` - 获取租户 ACL 规则

## 后端 API 端点对照表

| 模块 | 前端路径 | 后端路径 | 方法 | 说明 |
|------|-----------|-----------|------|------|
| Token 管理 | `/api/v1/tokens` | `/api/v1/tokens` | GET/POST | 列表/创建 |
| Token 管理 | `/api/v1/tokens/:id` | `/api/v1/tokens/:id` | DELETE | 删除 |
| Token 详情 | `/tokens/detail` | `/tokens/detail` | GET | 详情（查询参数） |
| 带宽限制 | `/api/v1/bandwidth/limits` | `/api/v1/bandwidth/limits` | GET/POST | 列表/创建 |
| 带宽限制 | `/api/v1/bandwidth/limits/:id` | `/api/v1/bandwidth/limits/:id` | DELETE | 删除 |
| 策略管理 | `/api/v1/bandwidth/policies` | `/api/v1/bandwidth/policies` | GET/POST | 列表/创建 |
| 策略管理 | `/api/v1/bandwidth/policies/:id` | `/api/v1/bandwidth/policies/:id` | GET/PUT/DELETE | 详情/更新/删除 |
| AI 聊天 | `/v1/ai/chat` | `/v1/ai/chat` | POST | 对话 |
| AI 确认 | `/v1/ai/confirm` | `/v1/ai/confirm` | POST | 工具执行确认 |
| 监控统计 | `/v1/monitor/stats` | `/v1/monitor/stats` | GET | 统计 |
| 节点详情 | `/v1/monitor/node/:id` | `/v1/monitor/node/:id` | GET | 详情 |
| 健康检查 | `/health` | `/health` | GET | 健康状态 |
| 版本 | `/version` | `/version` | GET | 版本信息 |

## 统一响应格式

后端所有 v1 API 返回统一的响应格式：

```typescript
interface APIResponse<T = any> {
  success: boolean;           // 请求是否成功
  data?: T;                  // 响应数据
  message?: string;           // 提示消息
  error?: {
    code: string;            // 错误代码
    message: string;         // 错误消息
    details?: Record<string, string>; // 详细错误信息
  };
  code?: string;              // 业务状态码
  meta?: {
    total?: number;          // 总数
    page?: number;           // 页码
    page_size?: number;      // 页大小
    next?: string;          // 下一页链接
    prev?: string;          // 上一页链接
  };
}
```

### 使用示例

```javascript
import { useQosApi } from '@/composables/useQosApi'

// 获取带宽限制
const limits = await useQosApi.getBandwidthLimits()
console.log(limits) // [{ ... }, { ... }]

// 创建带宽限制
const newLimit = await useQosApi.createBandwidthLimit({
  src_ip: '192.168.1.100',
  dst_ip: '192.168.1.200',
  src_port: 12345,
  dst_port: 80,
  protocol: 6,
  bandwidth: 100
})

// 创建策略
const newPolicy = await useQosApi.createPolicy({
  name: 'Block HTTP',
  description: 'Block all HTTP traffic',
  enabled: true,
  priority: 100,
  action: 'deny',
  dst_port: 80,
  protocol: 6
})
```

## 错误处理

前端 axios 拦截器会自动处理以下情况：

1. **401 未授权** - 清除会话并跳转到登录页
2. **统一响应格式** - 自动提取 `data` 字段
3. **错误信息** - 保留后端返回的 `error` 对象

### 错误码示例

```javascript
{
  success: false,
  message: "Validation failed",
  error: {
    code: "VALIDATION_FAILED",
    message: "请求参数验证失败",
    details: {
      bandwidth: "Bandwidth must be greater than 0"
    }
  }
}
```

## 租户认证

所有需要租户上下文的 API 请求都会自动添加：

1. **Authorization Header** - 从 sessionStorage 获取 token
2. **X-Tenant-ID Header** - 从 localStorage 获取当前租户 ID

## 后续工作

1. 更新前端视图组件以使用新的 API 模块
2. 测试所有 API 端点的对接
3. 更新错误处理逻辑以显示友好的错误消息
4. 添加国际化支持（i18n）
5. 更新单元测试以覆盖新的 API 调用

## 兼容性说明

- 旧版 API 路径仍然可用（如 `/tokens`、`/policies`）
- 新的 API 模块提供了向后兼容的包装方法
- 建议逐步迁移到新的 v1 API 端点
