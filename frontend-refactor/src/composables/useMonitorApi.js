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
   * 获取事件流（Alert + AuditEvent 混合）
   * @param {Object} params - 查询参数
   * @param {number} [params.limit] - 每页数量（默认50，最大200）
   * @param {number} [params.offset] - 偏移量
   * @param {string} [params.node_id] - 按节点过滤
   * @param {string} [params.event_type] - 按事件类型过滤
   * @param {string} [params.severity] - 按严重级别过滤
   * @param {string} [params.since] - 起始时间（ISO 8601）
   */
  getEvents: async (params = {}) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.EVENTS(tenantId), { params })
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取事件流失败:', error)
      throw error
    }
  },

  /**
   * 获取告警列表
   * @param {Object} params - 查询参数
   * @param {string} [params.status] - 告警状态（active/resolved/all，默认active）
   * @param {string} [params.alert_type] - 告警类型
   * @param {string} [params.node_id] - 按节点过滤
   * @param {number} [params.limit] - 每页数量
   * @param {number} [params.offset] - 偏移量
   */
  getAlerts: async (params = {}) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.ALERTS(tenantId), { params })
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取告警列表失败:', error)
      throw error
    }
  },

  /**
   * 手动解除告警
   * @param {string} alertId - 告警ID
   */
  resolveAlert: async (alertId) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.post(API_ENDPOINTS.MONITOR.ALERT_RESOLVE(tenantId, alertId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('解除告警失败:', error)
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
