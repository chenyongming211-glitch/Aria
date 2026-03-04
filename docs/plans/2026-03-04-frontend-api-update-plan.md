# 前端 API 更新实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**目标**: 将 Aria 前端的 API 层更新到与后端完全同步，新增 ACL 规则管理和 Agent 代理功能

**架构**: 分三阶段实施 - 阶段1（ACL API + 路径修复）→ 阶段2（Agent 代理 API）→ 阶段3（UI 集成和测试）

**技术栈**: Vue 3 + Vite + Element Plus + Axios + Pinia

**参考文档**: `docs/plans/2026-03-04-frontend-api-update-design.md`

---

## 阶段 1：ACL API + 路径修复（1-2 天）

### 任务 1：创建 ACL API 封装

**文件:**
- 创建: `frontend-refactor/src/composables/useAclApi.js`
- 创建: `tests/unit/useAclApi.test.js`

**步骤 1: 创建测试文件**

```bash
mkdir -p tests/unit
touch tests/unit/useAclApi.test.js
```

**步骤 2: 编写 ACL API 封装**

创建文件 `frontend-refactor/src/composables/useAclApi.js`:

```javascript
import api from './useApi'

/**
 * ACL 规则管理 API
 */
export const useAclApi = {
  /**
   * 获取 ACL 规则列表（支持分页和过滤）
   * @param {Object} filters - 过滤参数
   * @returns {Promise<Array>} ACL 规则列表
   */
  getACLRules: async (filters = {}) => {
    try {
      const queryParams = new URLSearchParams()
      
      if (filters.name) queryParams.append('name', filters.name)
      if (filters.action) queryParams.append('action', filters.action)
      if (filters.enabled !== undefined) queryParams.append('enabled', filters.enabled)
      if (filters.priority) queryParams.append('priority', filters.priority)
      if (filters.page) queryParams.append('page', filters.page)
      if (filters.page_size) queryParams.append('page_size', filters.page_size)
      
      const url = queryParams.toString()
        ? `/v1/tenant-management/acl-rules?${queryParams}`
        : '/v1/tenant-management/acl-rules'
      
      const response = await api.get(url)
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('获取 ACL 规则失败:', error)
      throw error
    }
  },

  /**
   * 创建 ACL 规则
   * @param {Object} rule - ACL 规则对象
   * @returns {Promise<Object>} 创建的规则
   */
  createACLRule: async (rule) => {
    try {
      const response = await api.post('/v1/tenant-management/acl-rules', rule)
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建 ACL 规则失败:', error)
      throw error
    }
  },

  /**
   * 更新 ACL 规则
   * @param {number} ruleId - 规则 ID
   * @param {Object} rule - ACL 规则对象
   * @returns {Promise<Object>} 更新后的规则
   */
  updateACLRule: async (ruleId, rule) => {
    try {
      const response = await api.put(`/v1/tenant-management/acl-rules/${ruleId}`, rule)
      return response.data?.data || response.data
    } catch (error) {
      console.error('更新 ACL 规则失败:', error)
      throw error
    }
  },

  /**
   * 删除 ACL 规则
   * @param {number} ruleId - 规则 ID
   * @returns {Promise<Object>} 删除结果
   */
  deleteACLRule: async (ruleId) => {
    try {
      const response = await api.delete(`/v1/tenant-management/acl-rules/${ruleId}`)
      return response.data?.data || response.data
    } catch (error) {
      console.error('删除 ACL 规则失败:', error)
      throw error
    }
  }
}
```

**步骤 3: 验证语法**

运行: `cd frontend-refactor && npm run lint -- src/composables/useAclApi.js`

预期: 无错误

**步骤 4: 编写单元测试**

创建文件 `tests/unit/useAclApi.test.js`:

```javascript
import { describe, it, expect, vi } from 'vitest'
import { useAclApi } from '@/composables/useAclApi'

// Mock axios
vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn()
  }
}))

import api from '@/composables/useApi'

describe('useAclApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getACLRules', () => {
    it('应该返回 ACL 规则列表', async () => {
      const mockRules = [
        { id: 1, name: 'allow-web', action: 'allow' },
        { id: 2, name: 'deny-ssh', action: 'deny' }
      ]
      
      api.get.mockResolvedValue({
        data: { success: true, data: mockRules }
      })
      
      const rules = await useAclApi.getACLRules()
      
      expect(api.get).toHaveBeenCalledWith('/v1/tenant-management/acl-rules')
      expect(rules).toEqual(mockRules)
    })

    it('应该支持过滤参数', async () => {
      const mockRules = [{ id: 1, name: 'allow-web', action: 'allow' }]
      
      api.get.mockResolvedValue({
        data: { success: true, data: mockRules }
      })
      
      const filters = { action: 'allow', enabled: true, page: 1, page_size: 10 }
      const rules = await useAclApi.getACLRules(filters)
      
      expect(api.get).toHaveBeenCalledWith(
        '/v1/tenant-management/acl-rules?action=allow&enabled=true&page=1&page_size=10'
      )
      expect(rules).toEqual(mockRules)
    })
  })

  describe('createACLRule', () => {
    it('应该创建新规则', async () => {
      const newRule = {
        name: 'test-rule',
        src_net: '192.168.1.0/24',
        dst_net: '10.0.0.0/24',
        protocol: 6,
        action: 'allow',
        enabled: true,
        priority: 100
      }
      
      const createdRule = { id: 1, ...newRule }
      
      api.post.mockResolvedValue({
        data: { success: true, data: createdRule }
      })
      
      const result = await useAclApi.createACLRule(newRule)
      
      expect(api.post).toHaveBeenCalledWith('/v1/tenant-management/acl-rules', newRule)
      expect(result).toEqual(createdRule)
    })
  })

  describe('updateACLRule', () => {
    it('应该更新规则', async () => {
      const updatedRule = {
        id: 1,
        name: 'updated-rule',
        action: 'deny'
      }
      
      api.put.mockResolvedValue({
        data: { success: true, data: updatedRule }
      })
      
      const result = await useAclApi.updateACLRule(1, updatedRule)
      
      expect(api.put).toHaveBeenCalledWith('/v1/tenant-management/acl-rules/1', updatedRule)
      expect(result).toEqual(updatedRule)
    })
  })

  describe('deleteACLRule', () => {
    it('应该删除规则', async () => {
      api.delete.mockResolvedValue({
        data: { success: true, message: 'Rule deleted' }
      })
      
      const result = await useAclApi.deleteACLRule(1)
      
      expect(api.delete).toHaveBeenCalledWith('/v1/tenant-management/acl-rules/1')
      expect(result).toEqual({ success: true, message: 'Rule deleted' })
    })
  })
})
```

**步骤 5: 运行测试**

运行: `cd frontend-refactor && npm run test tests/unit/useAclApi.test.js`

预期: 所有测试通过

**步骤 6: 提交代码**

```bash
git add frontend-refactor/src/composables/useAclApi.js tests/unit/useAclApi.test.js
git commit -m "feat: add ACL rules management API"
```

---

### 任务 2：更新 API 配置

**文件:**
- 修改: `frontend-refactor/src/config/api.js`

**步骤 1: 添加 ACL 端点配置**

编辑文件 `frontend-refactor/src/config/api.js`，在 `API_ENDPOINTS` 对象中添加：

```javascript
export const API_ENDPOINTS = {
  // ... 现有配置
  
  // ACL 规则管理
  ACL: {
    LIST: '/v1/tenant-management/acl-rules',
    CREATE: '/v1/tenant-management/acl-rules',
    GET: (id) => `/v1/tenant-management/acl-rules/${id}`,
    UPDATE: (id) => `/v1/tenant-management/acl-rules/${id}`,
    DELETE: (id) => `/v1/tenant-management/acl-rules/${id}`
  }
}
```

**步骤 2: 验证语法**

运行: `cd frontend-refactor && npm run lint -- src/config/api.js`

预期: 无错误

**步骤 3: 提交代码**

```bash
git add frontend-refactor/src/config/api.js
git commit -m "feat: add ACL endpoints to API config"
```

---

### 任务 3：修复 Token API 路径

**文件:**
- 修改: `frontend-refactor/src/composables/useTokenApi.js`

**步骤 1: 修复 getTokenNodes 方法**

编辑文件 `frontend-refactor/src/composables/useTokenApi.js`，找到 `getTokenNodes` 方法（约第 101 行）：

```javascript
// ❌ 错误的路径
getTokenNodes: async (token) => {
  try {
    const response = await api.get(`/v1/tokens/detail?token=${encodeURIComponent(token)}`)
    return response.data?.data?.nodes || []
  } catch (error) {
    console.error('获取令牌使用节点失败:', error)
    throw error
  }
}

// ✅ 正确的路径（修改为）
getTokenNodes: async (token) => {
  try {
    const response = await api.get(`/tokens/detail?token=${encodeURIComponent(token)}`)
    return response.data?.data?.nodes || []
  } catch (error) {
    console.error('获取令牌使用节点失败:', error)
    throw error
  }
}
```

**步骤 2: 新增 deleteToken 方法**

在 `useTokenApi` 对象中添加新方法：

```javascript
/**
 * 删除令牌（推荐使用）
 * @param {string} tokenId - 令牌 ID
 * @returns {Promise<Object>} 删除结果
 */
deleteToken: async (tokenId) => {
  try {
    const response = await api.delete(`/tokens/${tokenId}`)
    return response.data?.data || response.data
  } catch (error) {
    console.error('删除令牌失败:', error)
    throw error
  }
}
```

**步骤 3: 标记 revokeToken 为废弃**

修改 `revokeToken` 方法，添加废弃警告：

```javascript
/**
 * 撤销令牌（已废弃，请使用 deleteToken）
 * @deprecated 使用 deleteToken 替代
 * @param {string} tokenId - 令牌 ID
 * @returns {Promise<Object>} 撤销结果
 */
revokeToken: async (tokenId) => {
  console.warn('revokeToken 已弃用，请使用 deleteToken')
  
  try {
    // 尝试使用新方法
    return await useTokenApi.deleteToken(tokenId)
  } catch (error) {
    // 如果新方法失败，尝试旧方法（向后兼容）
    try {
      const response = await api.post('/tokens/revoke', { id: tokenId })
      return response.data?.data || response.data
    } catch (oldError) {
      console.error('撤销令牌失败（旧版）:', oldError)
      throw error
    }
  }
}
```

**步骤 4: 验证语法**

运行: `cd frontend-refactor && npm run lint -- src/composables/useTokenApi.js`

预期: 无错误

**步骤 5: 提交代码**

```bash
git add frontend-refactor/src/composables/useTokenApi.js
git commit -m "fix: correct Token API paths and add deleteToken method"
```

---

## 阶段 2：Agent 代理 API（1 天）

### 任务 4：创建 Agent 代理 API 封装

**文件:**
- 创建: `frontend-refactor/src/composables/useAgentProxyApi.js`
- 创建: `tests/unit/useAgentProxyApi.test.js`

**步骤 1: 创建 Agent 代理 API 封装**

创建文件 `frontend-refactor/src/composables/useAgentProxyApi.js`:

```javascript
import api from './useApi'

/**
 * Agent 代理 API
 * 用于向 Agent 节点发送命令和查询状态
 */
export const useAgentProxyApi = {
  /**
   * 发送命令到单个 Agent
   * @param {string} nodeId - 节点 ID（public_key 或 hostname）
   * @param {Object} command - 命令内容
   * @returns {Promise<Object>} 命令响应
   */
  sendAgentCommand: async (nodeId, command) => {
    try {
      const response = await api.post(`/v1/agent/${nodeId}/command`, command)
      return response.data?.data || response.data
    } catch (error) {
      console.error('发送 Agent 命令失败:', error)
      throw error
    }
  },

  /**
   * 获取 Agent 状态
   * @param {string} nodeId - 节点 ID
   * @returns {Promise<Object>} Agent 状态
   */
  getAgentStatus: async (nodeId) => {
    try {
      const response = await api.get(`/v1/agent/${nodeId}/status`)
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取 Agent 状态失败:', error)
      throw error
    }
  },

  /**
   * 批量发送命令到多个 Agent
   * @param {Object} batchCommand - 批量命令
   * @returns {Promise<Object>} 批量命令响应
   */
  sendBatchCommand: async (batchCommand) => {
    try {
      const response = await api.post('/v1/agents/command', batchCommand)
      return response.data?.data || response.data
    } catch (error) {
      console.error('批量发送命令失败:', error)
      throw error
    }
  }
}
```

**步骤 2: 编写单元测试**

创建文件 `tests/unit/useAgentProxyApi.test.js`:

```javascript
import { describe, it, expect, vi } from 'vitest'
import { useAgentProxyApi } from '@/composables/useAgentProxyApi'

// Mock axios
vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn()
  }
}))

import api from '@/composables/useApi'

describe('useAgentProxyApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('sendAgentCommand', () => {
    it('应该发送命令到单个节点', async () => {
      const command = {
        command: 'sync',
        timeout: 30,
        priority: 0
      }
      
      const mockResponse = {
        command_id: 'cmd-123',
        status: 'pending'
      }
      
      api.post.mockResolvedValue({
        data: { success: true, data: mockResponse }
      })
      
      const result = await useAgentProxyApi.sendAgentCommand('node-1', command)
      
      expect(api.post).toHaveBeenCalledWith('/v1/agent/node-1/command', command)
      expect(result).toEqual(mockResponse)
    })
  })

  describe('getAgentStatus', () => {
    it('应该获取节点状态', async () => {
      const mockStatus = {
        node_id: 'node-1',
        hostname: 'aria-sh-1',
        status: 'online',
        version: '0.2.26'
      }
      
      api.get.mockResolvedValue({
        data: { success: true, data: mockStatus }
      })
      
      const result = await useAgentProxyApi.getAgentStatus('node-1')
      
      expect(api.get).toHaveBeenCalledWith('/v1/agent/node-1/status')
      expect(result).toEqual(mockStatus)
    })
  })

  describe('sendBatchCommand', () => {
    it('应该批量发送命令', async () => {
      const batchCommand = {
        node_ids: ['node-1', 'node-2'],
        command: {
          command: 'sync',
          timeout: 30
        }
      }
      
      const mockResponse = {
        total: 2,
        success: 2,
        failed: 0
      }
      
      api.post.mockResolvedValue({
        data: { success: true, data: mockResponse }
      })
      
      const result = await useAgentProxyApi.sendBatchCommand(batchCommand)
      
      expect(api.post).toHaveBeenCalledWith('/v1/agents/command', batchCommand)
      expect(result).toEqual(mockResponse)
    })
  })
})
```

**步骤 3: 运行测试**

运行: `cd frontend-refactor && npm run test tests/unit/useAgentProxyApi.test.js`

预期: 所有测试通过

**步骤 4: 提交代码**

```bash
git add frontend-refactor/src/composables/useAgentProxyApi.js tests/unit/useAgentProxyApi.test.js
git commit -m "feat: add Agent proxy API"
```

---

### 任务 5：更新 API 配置（Agent 端点）

**文件:**
- 修改: `frontend-refactor/src/config/api.js`

**步骤 1: 添加 Agent 端点配置**

编辑文件 `frontend-refactor/src/config/api.js`，在 `API_ENDPOINTS` 对象中添加：

```javascript
export const API_ENDPOINTS = {
  // ... 现有配置（包括 ACL）
  
  // Agent 代理
  AGENT: {
    COMMAND: (nodeId) => `/v1/agent/${nodeId}/command`,
    STATUS: (nodeId) => `/v1/agent/${nodeId}/status`,
    BATCH_COMMAND: '/v1/agents/command'
  }
}
```

**步骤 2: 验证语法**

运行: `cd frontend-refactor && npm run lint -- src/config/api.js`

预期: 无错误

**步骤 3: 提交代码**

```bash
git add frontend-refactor/src/config/api.js
git commit -m "feat: add Agent proxy endpoints to API config"
```

---

## 阶段 3：UI 集成和测试（1-2 天）

### 任务 6：创建 ACL 规则管理页面

**文件:**
- 创建: `frontend-refactor/src/views/ACLRules.vue`

**步骤 1: 创建页面组件**

创建文件 `frontend-refactor/src/views/ACLRules.vue`（完整代码见下方）

由于文件较长，我会提供关键部分：

```vue
<template>
  <div class="acl-rules-container">
    <!-- 页面标题和操作按钮 -->
    <div class="page-header">
      <h2>ACL 规则管理</h2>
      <el-button type="primary" @click="handleCreate">
        <el-icon><Plus /></el-icon>
        新建规则
      </el-button>
    </div>

    <!-- 过滤器 -->
    <div class="filter-section">
      <el-input
        v-model="filters.name"
        placeholder="搜索规则名称"
        style="width: 200px"
        @input="debouncedSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
      
      <el-select v-model="filters.action" placeholder="动作" clearable @change="loadRules">
        <el-option label="允许" value="allow" />
        <el-option label="拒绝" value="deny" />
        <el-option label="通过" value="pass" />
        <el-option label="丢弃" value="drop" />
      </el-select>
      
      <el-select v-model="filters.enabled" placeholder="状态" clearable @change="loadRules">
        <el-option label="启用" :value="true" />
        <el-option label="禁用" :value="false" />
      </el-select>
      
      <el-button @click="loadRules">
        <el-icon><Search /></el-icon>
        搜索
      </el-button>
    </div>

    <!-- 规则列表表格 -->
    <el-table
      :data="rules"
      v-loading="loading"
      style="width: 100%"
      @sort-change="handleSortChange"
    >
      <el-table-column prop="priority" label="优先级" width="80" sortable />
      <el-table-column prop="name" label="名称" width="150" />
      <el-table-column prop="src_net" label="源网络" width="150" />
      <el-table-column prop="dst_net" label="目标网络" width="150" />
      <el-table-column label="端口范围" width="120">
        <template #default="{ row }">
          {{ row.min_port || 0 }} - {{ row.max_port || 65535 }}
        </template>
      </el-table-column>
      <el-table-column prop="protocol" label="协议" width="80">
        <template #default="{ row }">
          {{ getProtocolName(row.protocol) }}
        </template>
      </el-table-column>
      <el-table-column prop="action" label="动作" width="80">
        <template #default="{ row }">
          <el-tag :type="getActionType(row.action)">
            {{ getActionName(row.action) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="enabled" label="状态" width="80">
        <template #default="{ row }">
          <el-switch
            v-model="row.enabled"
            @change="handleToggleEnabled(row)"
          />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="handleEdit(row)">
            编辑
          </el-button>
          <el-button link type="danger" @click="handleDelete(row)">
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 分页 -->
    <div class="pagination-section">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="loadRules"
        @current-change="loadRules"
      />
    </div>

    <!-- 新建/编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="600px"
      @closed="resetForm"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="120px"
      >
        <el-form-item label="规则名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入规则名称" />
        </el-form-item>
        
        <el-form-item label="源网络" prop="src_net">
          <el-input v-model="form.src_net" placeholder="例如: 192.168.1.0/24" />
        </el-form-item>
        
        <el-form-item label="目标网络" prop="dst_net">
          <el-input v-model="form.dst_net" placeholder="例如: 10.0.0.0/24" />
        </el-form-item>
        
        <el-form-item label="协议" prop="protocol">
          <el-select v-model="form.protocol">
            <el-option label="TCP" :value="6" />
            <el-option label="UDP" :value="17" />
            <el-option label="ICMP" :value="1" />
          </el-select>
        </el-form-item>
        
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="最小端口">
              <el-input-number
                v-model="form.min_port"
                :min="0"
                :max="65535"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="最大端口">
              <el-input-number
                v-model="form.max_port"
                :min="0"
                :max="65535"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
        </el-row>
        
        <el-form-item label="动作" prop="action">
          <el-select v-model="form.action">
            <el-option label="允许 (allow)" value="allow" />
            <el-option label="拒绝 (deny)" value="deny" />
            <el-option label="通过 (pass)" value="pass" />
            <el-option label="丢弃 (drop)" value="drop" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="优先级" prop="priority">
          <el-input-number
            v-model="form.priority"
            :min="1"
            :max="10000"
            style="width: 100%"
          />
          <div class="form-help">数字越小优先级越高</div>
        </el-form-item>
        
        <el-form-item label="是否启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        
        <el-form-item label="描述">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="3"
            placeholder="请输入规则描述（可选）"
          />
        </el-form-item>
      </el-form>
      
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">
          确定
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { useAclApi } from '@/composables/useAclApi'

// 数据状态
const loading = ref(false)
const rules = ref([])
const dialogVisible = ref(false)
const submitting = ref(false)
const formRef = ref(null)

// 过滤器
const filters = reactive({
  name: '',
  action: '',
  enabled: undefined
})

// 分页
const pagination = reactive({
  page: 1,
  pageSize: 50,
  total: 0
})

// 表单数据
const form = reactive({
  id: null,
  name: '',
  src_net: '',
  dst_net: '',
  protocol: 6,
  min_port: null,
  max_port: null,
  action: 'allow',
  enabled: true,
  priority: 100,
  description: ''
})

// 表单验证规则
const formRules = {
  name: [
    { required: true, message: '请输入规则名称', trigger: 'blur' },
    { min: 1, max: 50, message: '长度在 1 到 50 个字符', trigger: 'blur' }
  ],
  src_net: [
    { required: true, message: '请输入源网络', trigger: 'blur' },
    { pattern: /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/, message: '请输入有效的 CIDR 格式', trigger: 'blur' }
  ],
  dst_net: [
    { required: true, message: '请输入目标网络', trigger: 'blur' },
    { pattern: /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/, message: '请输入有效的 CIDR 格式', trigger: 'blur' }
  ],
  protocol: [
    { required: true, message: '请选择协议', trigger: 'change' }
  ],
  action: [
    { required: true, message: '请选择动作', trigger: 'change' }
  ],
  priority: [
    { required: true, message: '请输入优先级', trigger: 'blur' }
  ]
}

// 对话框标题
const dialogTitle = computed(() => form.id ? '编辑规则' : '新建规则')

// 加载规则列表
const loadRules = async () => {
  loading.value = true
  try {
    const params = {
      ...filters,
      page: pagination.page,
      page_size: pagination.pageSize
    }
    
    // 移除空值
    Object.keys(params).forEach(key => {
      if (params[key] === '' || params[key] === undefined) {
        delete params[key]
      }
    })
    
    const response = await useAclApi.getACLRules(params)
    
    if (Array.isArray(response)) {
      rules.value = response
      pagination.total = response.length
    } else if (response.data) {
      rules.value = response.data
      pagination.total = response.meta?.total || response.data.length
    }
  } catch (error) {
    ElMessage.error('加载规则失败: ' + (error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

// 防抖搜索
let searchTimer = null
const debouncedSearch = () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    pagination.page = 1
    loadRules()
  }, 500)
}

// 新建规则
const handleCreate = () => {
  resetForm()
  dialogVisible.value = true
}

// 编辑规则
const handleEdit = (row) => {
  Object.assign(form, row)
  dialogVisible.value = true
}

// 删除规则
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除规则 "${row.name}" 吗？`,
      '删除确认',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    await useAclApi.deleteACLRule(row.id)
    ElMessage.success('删除成功')
    loadRules()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败: ' + (error.message || '未知错误'))
    }
  }
}

// 切换启用状态
const handleToggleEnabled = async (row) => {
  try {
    await useAclApi.updateACLRule(row.id, { enabled: row.enabled })
    ElMessage.success(row.enabled ? '已启用' : '已禁用')
  } catch (error) {
    row.enabled = !row.enabled // 恢复原状态
    ElMessage.error('操作失败: ' + (error.message || '未知错误'))
  }
}

// 提交表单
const handleSubmit = async () => {
  if (!formRef.value) return
  
  try {
    await formRef.value.validate()
    
    submitting.value = true
    
    const data = { ...form }
    
    // 验证端口范围
    if (data.min_port !== null && data.max_port !== null) {
      if (data.min_port > data.max_port) {
        ElMessage.error('最小端口不能大于最大端口')
        return
      }
    }
    
    if (form.id) {
      await useAclApi.updateACLRule(form.id, data)
      ElMessage.success('更新成功')
    } else {
      await useAclApi.createACLRule(data)
      ElMessage.success('创建成功')
    }
    
    dialogVisible.value = false
    loadRules()
  } catch (error) {
    if (error !== false) { // false 表示表单验证失败
      ElMessage.error('操作失败: ' + (error.message || '未知错误'))
    }
  } finally {
    submitting.value = false
  }
}

// 重置表单
const resetForm = () => {
  Object.assign(form, {
    id: null,
    name: '',
    src_net: '',
    dst_net: '',
    protocol: 6,
    min_port: null,
    max_port: null,
    action: 'allow',
    enabled: true,
    priority: 100,
    description: ''
  })
  
  if (formRef.value) {
    formRef.value.resetFields()
  }
}

// 辅助函数
const getProtocolName = (protocol) => {
  const map = { 6: 'TCP', 17: 'UDP', 1: 'ICMP' }
  return map[protocol] || 'Unknown'
}

const getActionName = (action) => {
  const map = {
    allow: '允许',
    deny: '拒绝',
    pass: '通过',
    drop: '丢弃'
  }
  return map[action] || action
}

const getActionType = (action) => {
  const map = {
    allow: 'success',
    deny: 'danger',
    pass: 'info',
    drop: 'warning'
  }
  return map[action] || ''
}

// 排序处理
const handleSortChange = ({ prop, order }) => {
  // TODO: 实现排序功能
  console.log('Sort:', prop, order)
}

// 页面加载时获取数据
onMounted(() => {
  loadRules()
})
</script>

<style scoped>
.acl-rules-container {
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0;
  font-size: 20px;
}

.filter-section {
  display: flex;
  gap: 10px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.pagination-section {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.form-help {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
}
</style>
```

**步骤 2: 验证语法**

运行: `cd frontend-refactor && npm run lint -- src/views/ACLRules.vue`

预期: 无错误

**步骤 3: 提交代码**

```bash
git add frontend-refactor/src/views/ACLRules.vue
git commit -m "feat: add ACL rules management page"
```

---

### 任务 7：更新路由配置

**文件:**
- 修改: `frontend-refactor/src/router/index.js`

**步骤 1: 添加 ACL 路由**

编辑文件 `frontend-refactor/src/router/index.js`，在路由配置中添加：

```javascript
{
  path: 'acl-rules',
  name: 'ACLRules',
  component: () => import('@/views/ACLRules.vue'),
  meta: { 
    title: 'ACL 规则管理',
    requiresAuth: true 
  }
}
```

**步骤 2: 验证路由**

运行: `cd frontend-refactor && npm run lint -- src/router/index.js`

预期: 无错误

**步骤 3: 提交代码**

```bash
git add frontend-refactor/src/router/index.js
git commit -m "feat: add ACL rules route"
```

---

### 任务 8：改进节点管理页面（可选）

**文件:**
- 修改: `frontend-refactor/src/views/Nodes.vue`

**步骤 1: 添加 Agent 命令按钮**

在节点表格的操作列添加下拉菜单：

```vue
<el-table-column label="操作" width="200" fixed="right">
  <template #default="{ row }">
    <el-button link type="primary" @click="handleViewNode(row)">
      查看
    </el-button>
    
    <el-dropdown @command="(cmd) => handleAgentCommand(row, cmd)">
      <el-button link type="primary">
        命令<el-icon class="el-icon--right"><arrow-down /></el-icon>
      </el-button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item command="sync">同步</el-dropdown-item>
          <el-dropdown-item command="restart">重启</el-dropdown-item>
          <el-dropdown-item command="config_reload">配置重载</el-dropdown-item>
          <el-dropdown-item command="health_check">健康检查</el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </template>
</el-table-column>
```

**步骤 2: 添加命令处理函数**

```javascript
import { useAgentProxyApi } from '@/composables/useAgentProxyApi'

const handleAgentCommand = async (node, command) => {
  try {
    const commandMap = {
      sync: '同步配置和状态',
      restart: '重启 Agent 服务',
      config_reload: '重新加载配置',
      health_check: '执行健康检查'
    }
    
    const actionText = commandMap[command]
    
    await ElMessageBox.confirm(
      `即将向节点 "${node.hostname}" 发送命令：${actionText}。\n\n此操作可能影响节点服务，是否继续？`,
      '确认执行命令',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    const result = await useAgentProxyApi.sendAgentCommand(node.public_key, {
      command,
      timeout: 30,
      priority: command === 'restart' ? 1 : 0
    })
    
    ElMessage.success(`命令已发送: ${result.command_id}`)
    
    // 刷新节点列表
    loadNodes()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('命令发送失败: ' + (error.message || '未知错误'))
    }
  }
}
```

**步骤 3: 提交代码**

```bash
git add frontend-refactor/src/views/Nodes.vue
git commit -m "feat: add Agent command buttons to Nodes page"
```

---

### 任务 9：集成测试

**步骤 1: 启动开发服务器**

运行: `cd frontend-refactor && npm run dev`

预期: 服务器在 http://localhost:3000 启动

**步骤 2: 手动测试 ACL 规则管理**

测试步骤：
1. 访问 http://localhost:3000/#/acl-rules
2. 测试新建规则（填写表单、验证、提交）
3. 测试编辑规则（修改字段、提交）
4. 测试删除规则（二次确认、删除）
5. 测试启用/禁用（切换开关）
6. 测试过滤搜索（名称、动作、状态）
7. 测试分页（切换页码、改变每页数量）

**步骤 3: 手动测试 Agent 命令**

测试步骤：
1. 访问 http://localhost:3000/#/nodes
2. 点击节点的"命令"下拉菜单
3. 测试同步命令
4. 测试重启命令（确认对话框）
5. 测试配置重载命令
6. 测试健康检查命令

**步骤 4: 记录测试结果**

创建测试报告，记录：
- 通过的测试用例
- 失败的测试用例（如有）
- 发现的问题（如有）

---

### 任务 10：构建和部署

**步骤 1: 构建生产版本**

运行: `cd frontend-refactor && npm run build`

预期: 构建产物输出到 `temp-dist/` 目录

**步骤 2: 验证构建产物**

```bash
ls -lh temp-dist/
du -sh temp-dist/
```

预期: 包含 index.html、assets/ 等文件，总大小约 2-3MB

**步骤 3: 提交最终代码**

```bash
git add .
git commit -m "feat: complete frontend API update"
git push origin feature/frontend-api-update
```

---

## 完成检查清单

- [ ] 阶段 1 完成
  - [ ] ACL API 封装创建并测试通过
  - [ ] API 配置更新
  - [ ] Token 路径修复
  
- [ ] 阶段 2 完成
  - [ ] Agent 代理 API 封装创建并测试通过
  - [ ] API 配置更新
  
- [ ] 阶段 3 完成
  - [ ] ACL 规则管理页面创建
  - [ ] 路由配置更新
  - [ ] 节点管理页面改进（可选）
  - [ ] 集成测试通过
  - [ ] 构建成功

- [ ] 代码质量
  - [ ] 所有文件通过 lint 检查
  - [ ] 单元测试全部通过
  - [ ] 无 TypeScript 错误
  - [ ] 代码审查通过

- [ ] 文档
  - [ ] README 更新（如有必要）
  - [ ] CHANGELOG 更新
  - [ ] 用户文档更新（如有必要）

---

## 预期时间线

| 日期 | 任务 | 预计时间 |
|------|------|----------|
| 第 1 天 | 任务 1-3（阶段1） | 3-4 小时 |
| 第 2 天 | 任务 4-5（阶段2） | 2-3 小时 |
| 第 3 天 | 任务 6-7（阶段3 前半） | 4-5 小时 |
| 第 4 天 | 任务 8-10（阶段3 后半） | 3-4 小时 |

**总计**: 12-16 小时（3-4 个工作日）

---

**计划状态**: ✅ 完成  
**下一步**: 选择执行方式
