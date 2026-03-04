import api from './useApi'
import { API_ENDPOINTS } from '@/config/api'

/**
 * 租户管理API接口
 * 与后端 /api/v1/tenant-management/* API 对接
 */
export const useTenantApi = {
  /**
   * 获取当前租户信息
   * 返回格式: { success: true, data: [...], message: "User tenant retrieved successfully" }
   */
  getCurrentTenant: async () => {
    try {
      const response = await api.get(API_ENDPOINTS.TENANT.CURRENT)
      const data = response.data?.data || response.data
      // 返回数组，取第一个元素
      return Array.isArray(data) ? data[0] : data
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
  createTenant: async (tenant) => {
    try {
      const response = await api.post(API_ENDPOINTS.TENANT.TENANTS, tenant)
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建租户失败:', error)
      throw error
    }
  },

  /**
   * 获取租户节点
   * 返回格式: { success: true, data: [...], message: "X nodes retrieved" }
   */
  getTenantNodes: async () => {
    try {
      const response = await api.get(API_ENDPOINTS.TENANT.NODES)
      return response.data?.data || response.data || []
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
      const response = await api.get(API_ENDPOINTS.TENANT.ACL_RULES)
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('获取 ACL 规则失败:', error)
      throw error
    }
  }
}
