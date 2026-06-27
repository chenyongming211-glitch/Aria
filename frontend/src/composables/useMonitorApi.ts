import api from './useApi'
import { unwrapApiData } from './apiResponse'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import type {
  AlertRecord,
  ListResult,
  MonitoringAlertParams,
  MonitoringEvent,
  MonitoringEventParams,
  MonitoringHealth,
  MonitoringNodeDetail,
  MonitoringNodeMetrics,
  MonitoringRange,
  MonitoringStats,
  MonitoringTopology,
  MonitoringTraffic
} from '@/types'

type ResolveAlertPayload = Record<string, unknown>
type ResolveAlertResult = AlertRecord | { id?: string; status?: string }

/**
 * 监控API接口
 * 与后端 /api/v2/tenants/{tenant_id}/monitoring/* API 对接
 */
export const useMonitorApi = {
  /**
   * 获取监控统计数据
   * 返回格式: { success: true, data: {...}, message: "..." }
   */
  getStats: async (): Promise<MonitoringStats> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.STATS(tenantId))
      return unwrapApiData<MonitoringStats>(response)
    } catch (error) {
      console.error('获取监控统计失败:', error)
      throw error
    }
  },

  /**
   * 获取节点详情
   * @param {string} nodeId - 节点ID
   */
  getNodeDetail: async (nodeId: string): Promise<MonitoringNodeDetail> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.NODE_DETAIL(tenantId, nodeId))
      return unwrapApiData<MonitoringNodeDetail>(response)
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
  getEvents: async (params: MonitoringEventParams = {}): Promise<ListResult<MonitoringEvent>> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.EVENTS(tenantId), { params })
      return unwrapApiData<ListResult<MonitoringEvent>>(response)
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
  getAlerts: async (params: MonitoringAlertParams = {}): Promise<ListResult<AlertRecord>> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.ALERTS(tenantId), { params })
      return unwrapApiData<ListResult<AlertRecord>>(response)
    } catch (error) {
      console.error('获取告警列表失败:', error)
      throw error
    }
  },

  /**
   * 手动解除告警
   * @param {string} alertId - 告警ID
   * @param {Object} payload - 可选处理上下文
   */
  resolveAlert: async (alertId: string, payload: ResolveAlertPayload = {}): Promise<ResolveAlertResult> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.post(API_ENDPOINTS.MONITOR.ALERT_RESOLVE(tenantId, alertId), payload)
      return unwrapApiData<ResolveAlertResult>(response)
    } catch (error) {
      console.error('解除告警失败:', error)
      throw error
    }
  },

  /**
   * 获取流量数据
   * @param {string} range - 时间范围（1h/24h/7d/30d）
   */
  getTraffic: async (range: MonitoringRange = '24h'): Promise<MonitoringTraffic> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.TRAFFIC(tenantId), { params: { range } })
      return unwrapApiData<MonitoringTraffic>(response)
    } catch (error) {
      console.error('获取流量数据失败:', error)
      throw error
    }
  },

  /**
   * 获取系统健康指标
   */
  getHealth: async (): Promise<MonitoringHealth> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.HEALTH(tenantId))
      return unwrapApiData<MonitoringHealth>(response)
    } catch (error) {
      console.error('获取健康指标失败:', error)
      throw error
    }
  },

  /**
   * 获取单节点 metrics（带宽/延迟）
   * @param {string} nodeId - 节点ID
   */
  getNodeMetrics: async (nodeId: string): Promise<MonitoringNodeMetrics> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.NODE_METRICS(tenantId, nodeId))
      return unwrapApiData<MonitoringNodeMetrics>(response)
    } catch (error) {
      console.error('获取节点 metrics 失败:', error)
      throw error
    }
  },

  /**
   * 获取拓扑数据
   */
  getTopology: async (): Promise<MonitoringTopology> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.MONITOR.TOPOLOGY(tenantId))
      return unwrapApiData<MonitoringTopology>(response)
    } catch (error) {
      console.error('获取拓扑数据失败:', error)
      throw error
    }
  },

  /**
   * 获取版本信息
   */
  getVersion: async (): Promise<unknown> => {
    try {
      const response = await api.get(API_ENDPOINTS.VERSION)
      return response.data
    } catch (error) {
      console.error('获取版本信息失败:', error)
      throw error
    }
  }
}
