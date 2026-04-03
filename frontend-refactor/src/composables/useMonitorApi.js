import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

/**
 * 监控API接口
 * 与后端 /api/v2/tenants/{tenant_id}/monitoring/* API 对接
 */
export const useMonitorApi = {
  /**
   * 获取监控统计数据
   * 返回格式: { success: true, data: {...}, message: "..." }
   */
  getStats: async () => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.STATS(tenantId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取监控统计失败:', error)
      throw error
    }
  },

  /**
   * 获取节点详情
   * @param {string} nodeId - 节点ID
   */
  getNodeDetail: async (nodeId) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.NODE_DETAIL(tenantId, nodeId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取节点详情失败:', error)
      throw error
    }
  },

  /**
   * 获取健康状态
   */
  getHealth: async () => {
    try {
      const response = await api.get(API_ENDPOINTS.HEALTH)
      return response.data
    } catch (error) {
      console.error('获取健康状态失败:', error)
      throw error
    }
  },

  /**
   * 获取版本信息
   */
  getVersion: async () => {
    try {
      const response = await api.get(API_ENDPOINTS.VERSION)
      return response.data
    } catch (error) {
      console.error('获取版本信息失败:', error)
      throw error
    }
  }
}
