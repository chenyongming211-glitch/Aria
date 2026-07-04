import api from './useApi'
import { unwrapApiList } from './apiResponse'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import type { NodeRecord, Tenant } from '@/types'

interface TenantPayload extends Partial<Tenant> {
  resource_quota?: Record<string, unknown>
}

interface TenantScopedACLRule extends Record<string, unknown> {
  id?: string | number
  node_id?: string
  node_name?: string
}

/**
 * 租户管理API接口
 * 与后端 /v2/tenants/* API 对接
 */
export const useTenantApi = {
  /**
   * 获取当前租户信息
   * 返回格式: { success: true, data: [...], message: "User tenant retrieved successfully" }
   */
  getCurrentTenant: async () => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.TENANT.DETAIL(tenantId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取当前租户失败:', error)
      throw error
    }
  },

  /**
   * 获取租户列表（系统管理员）
   * 返回格式: { success: true, data: [...], message: "X tenants retrieved" }
   */
  listTenants: async () => {
    try {
      const response = await api.get(API_ENDPOINTS.TENANT.LIST)
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('获取租户列表失败:', error)
      throw error
    }
  },

  /**
   * 创建租户（系统管理员）
   * @param {Object} tenant - 租户信息
   *  {
   *    name: string,                           // 租户名称（必填）
   *    code: string,                           // 租户代码（必填）
   *    status?: string,                         // 状态（可选，默认 "active"）
   *    resource_quota?: object                   // 资源配额（可选）
   *  }
   */
  createTenant: async (tenant: TenantPayload) => {
    try {
      const response = await api.post(API_ENDPOINTS.TENANT.LIST, tenant)
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建租户失败:', error)
      throw error
    }
  },

  /**
   * 获取租户节点
   * 返回格式: { success: true, data: [...] } 或 { success: true, data: { items: [...] } }
   */
  getTenantNodes: async () => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.TENANT.NODES(tenantId))
      return unwrapApiList<NodeRecord>(response)
    } catch (error) {
      console.error('获取租户节点失败:', error)
      throw error
    }
  },

  /**
   * 获取租户 ACL 规则
   * 返回格式: { success: true, data: [...], message: "X ACL rules retrieved" }
   */
  getTenantACLRules: async () => {
    try {
      const tenantId = requireCurrentTenantId()
      const nodesResponse = await api.get(API_ENDPOINTS.TENANT.NODES(tenantId))
      const nodes = unwrapApiList<NodeRecord>(nodesResponse)

      const ruleResponses = await Promise.all(
        nodes.map(async (node) => {
          const aclResponse = await api.get(API_ENDPOINTS.TENANT.NODE_ACLS(tenantId, node.id))
          const rules = (aclResponse.data?.data || aclResponse.data || []) as TenantScopedACLRule[]
          return rules.map((rule: TenantScopedACLRule) => ({
            ...rule,
            node_id: node.id,
            node_name: node.hostname || node.public_key || node.id
          }))
        })
      )

      return ruleResponses.flat()
    } catch (error) {
      console.error('获取 ACL 规则失败:', error)
      throw error
    }
  }
}
