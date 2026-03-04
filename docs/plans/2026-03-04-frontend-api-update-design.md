# 前端 API 更新设计方案

**日期**: 2026-03-04  
**版本**: 1.0  
**状态**: 已批准  
**作者**: Claude  

---

## 📋 执行摘要

本设计文档描述如何将 Aria 前端的 API 层更新到与后端完全同步，重点解决以下问题：

1. **新增功能缺失**：ACL 规则管理和 Agent 代理功能在后端已实现，但前端未对接
2. **API 路径不一致**：部分 Token API 路径与后端不匹配
3. **功能不完整**：带宽限制删除功能后端未实现，需要前端提示

通过分三个阶段实施，预计 **3-5 天**完成所有更新。

---

## 🎯 设计目标

### 主要目标

- ✅ 新增 ACL 规则管理 API 封装（增删改查、分页、过滤）
- ✅ 新增 Agent 代理 API 封装（命令下发、状态查询、批量操作）
- ✅ 修复 Token API 路径不一致问题
- ✅ 创建 ACL 规则管理 UI 页面
- ✅ 改进节点管理页面，添加 Agent 命令功能
- ✅ 完善测试覆盖（单元测试、集成测试、E2E 测试）

### 成功标准

- [ ] 所有新增 API 封装完成并通过单元测试
- [ ] ACL 规则管理页面可以正常使用（CRUD 操作）
- [ ] 节点管理页面可以发送 Agent 命令
- [ ] 所有测试通过，无回归问题
- [ ] 代码审查通过

---

## 🏗️ 架构设计

### 整体架构

```
frontend-refactor/src/
├── composables/              # API 封装层
│   ├── useApi.js            # Axios 实例（已有）
│   ├── useAclApi.js         # ✨ 新增：ACL 规则管理
│   ├── useAgentProxyApi.js  # ✨ 新增：Agent 代理
│   ├── useTokenApi.js       # 🔧 修复：路径问题
│   ├── useTenantApi.js      # ✅ 保持
│   ├── useQosApi.js         # 🔧 改进：错误提示
│   ├── useAiApi.js          # ✅ 保持
│   └── useMonitorApi.js     # ✅ 保持
├── config/
│   └── api.js               # 🔧 更新：新增端点配置
├── views/
│   ├── ACLRules.vue         # ✨ 新增：ACL 规则管理页面
│   └── Nodes.vue            # 🔧 改进：添加 Agent 命令按钮
└── router/
    └── index.js             # 🔧 更新：新增路由
```

### 数据流

```
用户操作 → Vue 组件 → Composables API → Axios → 后端 API
                ↓
           Pinia Store（可选）
                ↓
           UI 更新
```

---

## 📅 分阶段实施计划

### 阶段 1：ACL API + 路径修复（1-2 天）

**目标**：新增 ACL 规则管理 API，修复 Token 路径问题

#### 任务清单

| 任务 | 文件 | 工作量 | 优先级 |
|------|------|--------|--------|
| 创建 ACL API 封装 | `composables/useAclApi.js` | 2小时 | 🔴 高 |
| 修复 Token 路径 | `composables/useTokenApi.js` | 30分钟 | 🔴 高 |
| 更新 API 配置 | `config/api.js` | 15分钟 | 🔴 高 |
| 单元测试 | `tests/unit/useAclApi.test.js` | 1小时 | 🟡 中 |

#### ACL API 接口定义

**端点**：`/v1/tenant-management/acl-rules`

| 方法 | 功能 | HTTP | 路径 |
|------|------|------|------|
| `getACLRules(filters)` | 获取规则列表 | GET | `/v1/tenant-management/acl-rules` |
| `createACLRule(rule)` | 创建规则 | POST | `/v1/tenant-management/acl-rules` |
| `updateACLRule(id, rule)` | 更新规则 | PUT | `/v1/tenant-management/acl-rules/{id}` |
| `deleteACLRule(id)` | 删除规则 | DELETE | `/v1/tenant-management/acl-rules/{id}` |

**数据结构**：

```typescript
interface ACLRule {
  id?: number
  name: string              // 规则名称
  src_net: string           // 源网络 CIDR（必填）
  dst_net: string           // 目标网络 CIDR（必填）
  protocol: number          // 协议：6=TCP, 17=UDP, 1=ICMP
  min_port: number          // 最小端口
  max_port: number          // 最大端口
  action: string            // allow, deny, pass, drop
  enabled: boolean          // 是否启用
  priority: number          // 优先级（数字越小优先级越高）
  description?: string      // 描述
  created_at?: string
  updated_at?: string
}

interface ACLFilters {
  name?: string
  action?: string
  enabled?: boolean
  priority?: number
  page?: number
  page_size?: number
}
```

#### Token API 修复

**问题**：
- `getTokenNodes()`: 使用 `/v1/tokens/detail`，应改为 `/tokens/detail`
- `revokeToken()`: 使用 POST `/tokens/revoke`，应改为 DELETE `/tokens/{id}`

**解决方案**：
1. 修复路径错误
2. 新增 `deleteToken()` 方法（推荐使用）
3. 保留 `revokeToken()` 方法（向后兼容，标记为废弃）

---

### 阶段 2：Agent 代理 API（1 天）

**目标**：新增 Agent 代理 API，支持远程命令下发

#### 任务清单

| 任务 | 文件 | 工作量 | 优先级 |
|------|------|--------|--------|
| 创建 Agent 代理 API | `composables/useAgentProxyApi.js` | 2小时 | 🟡 中 |
| 更新 API 配置 | `config/api.js` | 15分钟 | 🟡 中 |
| 单元测试 | `tests/unit/useAgentProxyApi.test.js` | 1小时 | 🟢 低 |

#### Agent 代理 API 接口定义

**端点**：`/v1/agent/{nodeId}/command`, `/v1/agents/command`

| 方法 | 功能 | HTTP | 路径 |
|------|------|------|------|
| `sendAgentCommand(nodeId, command)` | 发送命令到单个 Agent | POST | `/v1/agent/{nodeId}/command` |
| `getAgentStatus(nodeId)` | 获取 Agent 状态 | GET | `/v1/agent/{nodeId}/status` |
| `sendBatchCommand(batchCmd)` | 批量发送命令 | POST | `/v1/agents/command` |

**支持的命令**：

| 命令 | 说明 | 优先级 |
|------|------|--------|
| `sync` | 同步配置和状态 | 0（普通） |
| `restart` | 重启 Agent 服务 | 1（高） |
| `config_reload` | 重新加载配置 | 0（普通） |
| `health_check` | 健康检查 | 2（紧急） |

**数据结构**：

```typescript
interface AgentCommand {
  command: 'sync' | 'restart' | 'config_reload' | 'health_check'
  params?: Record<string, any>
  timeout?: number          // 默认 30 秒
  priority?: 0 | 1 | 2      // 0=普通, 1=高, 2=紧急
}

interface BatchCommand {
  node_ids?: string[]       // 空=所有节点
  command: AgentCommand
}

interface CommandResponse {
  command_id: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  result?: any
  error?: string
  executed_at?: string
}

interface AgentStatus {
  node_id: string
  hostname: string
  region: string
  status: 'online' | 'offline' | 'error'
  last_seen: string
  version: string
  uptime: number
  metrics: {
    cpu_usage: number
    memory_usage: number
    network_rx: number
    network_tx: number
    active_connections: number
  }
}
```

---

### 阶段 3：UI 集成和测试（1-2 天）

**目标**：创建 UI 页面，集成测试

#### 任务清单

| 任务 | 文件 | 工作量 | 优先级 |
|------|------|--------|--------|
| 创建 ACL 规则页面 | `views/ACLRules.vue` | 4小时 | 🟡 中 |
| 改进节点管理页面 | `views/Nodes.vue` | 2小时 | 🟡 中 |
| 更新路由配置 | `router/index.js` | 15分钟 | 🟡 中 |
| 集成测试 | 手动测试 | 2小时 | 🟢 低 |
| E2E 测试 | `tests/e2e/acl_rules.cy.js` | 2小时 | 🟢 低 |

#### ACL 规则管理页面

**页面结构**：

```
┌─────────────────────────────────────────────────────────────┐
│  ACL 规则管理                            [+ 新建规则]        │
├─────────────────────────────────────────────────────────────┤
│  过滤器：                                                    │
│  [名称搜索] [动作 ▼] [状态 ▼] [优先级] [🔍 搜索]           │
├─────────────────────────────────────────────────────────────┤
│  规则列表（表格）                                            │
│  ┌────┬────────┬─────────┬─────────┬────┬──────┬────┬────┐ │
│  │优先│ 名称   │ 源网络  │ 目标网络│端口│ 动作 │状态│操作│ │
│  ├────┼────────┼─────────┼─────────┼────┼──────┼────┼────┤ │
│  │100 │ allow-web │192.168.1.0/24│10.0.0.0/24│80│ allow│✓  │✏️🗑️│ │
│  │200 │ deny-ssh  │0.0.0.0/0     │192.168.2.0/24│22│ deny │✓  │✏️🗑️│ │
│  └────┴────────┴─────────┴─────────┴────┴──────┴────┴────┘ │
│                                                             │
│  [上一页] 第1页/共10页 [下一页]   显示: [50条/页 ▼]        │
└─────────────────────────────────────────────────────────────┘
```

**核心功能**：

- ✅ 规则列表（分页、排序）
- ✅ 过滤搜索（名称、动作、状态、优先级）
- ✅ 新建规则（表单验证）
- ✅ 编辑规则（预填充数据）
- ✅ 删除规则（二次确认）
- ✅ 启用/禁用（快捷切换）
- ✅ 批量操作（启用、禁用、删除）

**表单字段**：

```javascript
{
  name: '',                // 必填，长度1-50
  src_net: '',            // 必填，CIDR格式
  dst_net: '',            // 必填，CIDR格式
  protocol: 6,            // 必填，6/17/1
  min_port: null,         // 可选，0-65535
  max_port: null,         // 可选，0-65535
  action: 'allow',        // 必填，allow/deny/pass/drop
  enabled: true,          // 必填，布尔值
  priority: 100,          // 必填，1-10000
  description: ''         // 可选，字符串
}
```

#### 节点管理页面改进

**新增功能**：

- ✅ 状态指示器（🟢 在线 / 🔴 离线 / ⚠️ 错误）
- ✅ Agent 命令按钮（下拉菜单：同步、重启、配置重载、健康检查）
- ✅ 批量操作栏（批量同步、批量重启）
- ✅ 命令确认对话框（显示节点信息、命令类型、警告提示）

---

## 🧪 测试策略

### 单元测试

**测试框架**：Vitest

**测试覆盖**：

- `useAclApi.js`：CRUD 操作、分页、过滤
- `useAgentProxyApi.js`：命令发送、状态查询、批量操作

**示例**：

```javascript
describe('useAclApi', () => {
  test('获取ACL规则列表', async () => {
    const rules = await useAclApi.getACLRules()
    expect(rules).toBeInstanceOf(Array)
  })
  
  test('创建ACL规则', async () => {
    const rule = await useAclApi.createACLRule({
      name: 'test-rule',
      src_net: '192.168.1.0/24',
      dst_net: '10.0.0.0/24',
      protocol: 6,
      action: 'allow',
      enabled: true,
      priority: 100
    })
    expect(rule.id).toBeDefined()
  })
})
```

### 集成测试

**测试场景**：

| 场景 | 步骤 | 预期结果 |
|------|------|----------|
| ACL 规则 CRUD | 新建→查询→编辑→删除 | 操作成功，数据一致 |
| 分页功能 | 切换页码、改变每页数量 | 正确显示对应数据 |
| 过滤功能 | 按名称、动作、状态过滤 | 正确过滤结果 |
| Agent 命令 | 发送同步命令 | 返回命令ID和状态 |
| 批量命令 | 向多个节点发送命令 | 批量执行成功 |
| 错误处理 | 无效节点ID、网络错误 | 显示友好错误提示 |

### E2E 测试

**测试框架**：Cypress

**测试用例**：

- ACL 规则管理：创建、编辑、删除、启用/禁用
- Agent 命令：发送命令、查看结果
- 错误场景：无效输入、网络错误

---

## 📦 文件清单

### 新增文件（3 个）

1. `frontend-refactor/src/composables/useAclApi.js`
   - ACL 规则管理 API 封装
   - 约 150 行代码

2. `frontend-refactor/src/composables/useAgentProxyApi.js`
   - Agent 代理 API 封装
   - 约 120 行代码

3. `frontend-refactor/src/views/ACLRules.vue`
   - ACL 规则管理页面
   - 约 400 行代码（含模板、脚本、样式）

### 修改文件（4 个）

1. `frontend-refactor/src/composables/useTokenApi.js`
   - 修复路径问题
   - 新增 `deleteToken()` 方法
   - 标记 `revokeToken()` 为废弃

2. `frontend-refactor/src/config/api.js`
   - 新增 ACL 端点配置
   - 新增 Agent 端点配置

3. `frontend-refactor/src/views/Nodes.vue`
   - 添加状态指示器
   - 添加 Agent 命令按钮
   - 添加批量操作栏

4. `frontend-refactor/src/router/index.js`
   - 新增 `/acl-rules` 路由

### 测试文件（3 个）

1. `tests/unit/useAclApi.test.js`
2. `tests/unit/useAgentProxyApi.test.js`
3. `tests/e2e/acl_rules.cy.js`

---

## 🔒 安全考虑

### 认证和授权

- ✅ 所有 API 调用需要 JWT Token（已有拦截器处理）
- ✅ 401 自动跳转登录页（已有拦截器处理）
- ✅ 租户隔离通过 `X-Tenant-ID` 头部实现（已有拦截器处理）

### 数据验证

- ✅ 前端表单验证（必填字段、格式验证）
- ✅ 后端二次验证（已有实现）
- ✅ CIDR 格式验证
- ✅ 端口范围验证（0-65535）

### 错误处理

- ✅ 友好的错误提示
- ✅ 网络错误重试机制
- ✅ 404、500 等错误码处理

---

## 📊 性能考虑

### 前端优化

- ✅ API 调用防抖（搜索输入）
- ✅ 分页加载（避免一次加载过多数据）
- ✅ 按需加载组件（路由懒加载）
- ✅ 使用 Pinia 缓存租户数据

### 网络优化

- ✅ Axios 请求合并（批量操作）
- ✅ 合理的超时设置（30秒）
- ✅ 请求取消（组件卸载时）

---

## 🚀 部署计划

### 开发环境

1. 创建新分支：`feature/frontend-api-update`
2. 按阶段实施
3. 每个阶段完成后提交代码
4. 本地测试通过

### 测试环境

1. 部署到测试服务器
2. 运行所有测试
3. 手动验收测试
4. 修复发现的问题

### 生产环境

1. 代码审查通过
2. 合并到主分支
3. 构建：`npm run build`
4. 部署：`rsync -avz temp-dist/ root@server:/root/aria-controller/ui-dist/`
5. 重启 Web 容器：`docker restart aria_web`
6. 验证功能正常

---

## 🎯 成功标准

- [ ] 所有新增 API 封装完成并通过单元测试
- [ ] ACL 规则管理页面可以正常使用（CRUD 操作）
- [ ] 节点管理页面可以发送 Agent 命令
- [ ] 所有测试通过，无回归问题
- [ ] 代码审查通过
- [ ] 文档更新完成

---

## 📝 风险和缓解措施

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 后端 API 变更 | 高 | 与后端团队保持沟通，及时同步变更 |
| 测试覆盖不足 | 中 | 编写完整的单元测试和集成测试 |
| UI 体验问题 | 中 | 参考现有页面风格，保持一致性 |
| 性能问题 | 低 | 使用分页、防抖等优化手段 |
| 部署问题 | 低 | 使用蓝绿部署，快速回滚 |

---

## 📚 参考资料

- [Aria API 规范](../../CLAUDE.md#12-api-规范重要)
- [Vue 3 官方文档](https://vuejs.org/)
- [Element Plus 文档](https://element-plus.org/)
- [Vitest 文档](https://vitest.dev/)
- [Cypress 文档](https://www.cypress.io/)

---

## 📅 时间线

| 日期 | 里程碑 |
|------|--------|
| 2026-03-04 | 设计文档完成，开始实施 |
| 2026-03-05 | 阶段1完成（ACL API + 路径修复） |
| 2026-03-06 | 阶段2完成（Agent 代理 API） |
| 2026-03-07 | 阶段3完成（UI 集成和测试） |
| 2026-03-08 | 代码审查和部署 |

---

## 👥 相关人员

- **设计者**: Claude
- **实施者**: Claude
- **审查者**: 待定
- **测试者**: 待定

---

## 📝 变更历史

| 版本 | 日期 | 变更内容 | 作者 |
|------|------|----------|------|
| 1.0 | 2026-03-04 | 初始版本 | Claude |

---

**文档状态**: ✅ 已批准  
**下一步**: 调用 writing-plans 技能创建详细实施计划
