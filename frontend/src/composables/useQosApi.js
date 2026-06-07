import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

function normalizeBandwidthMbps(rule) {
  const value = Number(rule.bandwidth_mbps ?? rule.bandwidth ?? 0)
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error('bandwidth_mbps must be greater than 0')
  }
  return value
}

function normalizeDirection(rule) {
  const value = String(rule.direction || '').trim().toLowerCase()
  if (['ingress', 'egress'].includes(value)) return value
  if ((rule.src_cidr || rule.src_net) && !(rule.dst_cidr || rule.dst_net)) return 'ingress'
  return 'egress'
}

function normalizeMode(rule) {
  const value = String(rule.mode || '').trim().toLowerCase()
  return value === 'shaping' ? 'shaping' : 'policing'
}

function normalizeRateBps(rule, bandwidthMbps) {
  const value = Number(rule.rate_bps || 0)
  if (Number.isFinite(value) && value > 0) return value
  return bandwidthMbps * 1000000
}

function normalizeBurstBytes(rule, rateBps) {
  const value = Number(rule.burst_bytes || 0)
  if (Number.isFinite(value) && value > 0) return value
  return Math.max(Math.floor(rateBps / 8 / 10), 1500)
}

function normalizeRulePayload(rule) {
  const bandwidthMbps = normalizeBandwidthMbps(rule)
  const rateBps = normalizeRateBps(rule, bandwidthMbps)
  return {
    src_cidr: rule.src_cidr || rule.src_net || '',
    dst_cidr: rule.dst_cidr || rule.dst_net || '',
    src_port: Number(rule.src_port || 0),
    dst_port: Number(rule.dst_port || 0),
    protocol: Number(rule.protocol || 0),
    bandwidth_mbps: bandwidthMbps,
    direction: normalizeDirection(rule),
    rate_bps: rateBps,
    burst_bytes: normalizeBurstBytes(rule, rateBps),
    priority: Number(rule.priority || 0),
    mode: normalizeMode(rule),
    description: rule.description || '',
    enabled: rule.enabled !== false
  }
}

function normalizeListResponse(response) {
  const body = response?.data
  if (Array.isArray(body)) return body
  if (!body || typeof body !== 'object') return []

  if ('success' in body) {
    const data = body.data
    if (data == null) return []
    if (Array.isArray(data)) return data
    if (Array.isArray(data.items)) return data.items
    throw new Error('Invalid list response')
  }

  if (Array.isArray(body.items)) return body.items
  return []
}

/**
 * QoS 规则管理 API (v2)
 * 分类定义:
 * - service: 服务级限速 (五元组)
 * - peers: 节点对限速 (src/dst IP)
 * - ip: 单节点/IP 限速 (src IP)
 */
export const useQosApi = {
  /**
   * 获取指定节点的 QoS 规则
   * @param {string} nodeId - 节点ID
   * @param {string} category - 分类 (service, peers, ip)
   */
  getQoSRulesByNode: async (nodeId, category) => {
    try {
      const tenantId = requireCurrentTenantId()
      if (!nodeId || !category) return []

      const response = await api.get(API_ENDPOINTS.TENANT.NODE_QOS(tenantId, nodeId, category))
      const rules = normalizeListResponse(response)

      // 统一字段映射
      return rules.map(rule => ({
        ...rule,
        node_id: nodeId,
        category: category,
        direction: normalizeDirection(rule),
        mode: normalizeMode(rule),
        rate_bps: Number(rule.rate_bps || 0),
        burst_bytes: Number(rule.burst_bytes || 0),
        priority: Number(rule.priority || 0),
        // 兼容旧 UI 字段名
        bandwidth: rule.bandwidth_mbps,
        status: rule.enabled ? 'active' : 'inactive',
        policyStatus: rule.policy_status || 'idle'
      }))
    } catch (error) {
      console.error(`获取节点 QoS 规则 (${category}) 失败:`, error)
      throw error
    }
  },

  /**
   * 创建 QoS 规则
   */
  createQoSRule: async (nodeId, category, rule) => {
    try {
      const tenantId = requireCurrentTenantId()
      const payload = normalizeRulePayload(rule)

      const response = await api.post(
        API_ENDPOINTS.TENANT.NODE_QOS(tenantId, nodeId, category),
        payload
      )
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建 QoS 规则失败:', error)
      throw error
    }
  },

  /**
   * 删除 QoS 规则
   */
  deleteQoSRule: async (nodeId, category, ruleId) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.delete(
        API_ENDPOINTS.TENANT.NODE_QOS_RULE(tenantId, nodeId, category, ruleId)
      )
      return response.data?.data || response.data
    } catch (error) {
      console.error('删除 QoS 规则失败:', error)
      throw error
    }
  },

  updateQoSRule: async (nodeId, category, ruleId, rule) => {
    try {
      const tenantId = requireCurrentTenantId()
      const payload = normalizeRulePayload(rule)

      const response = await api.put(
        API_ENDPOINTS.TENANT.NODE_QOS_RULE(tenantId, nodeId, category, ruleId),
        payload
      )
      return response.data?.data || response.data
    } catch (error) {
      console.error('更新 QoS 规则失败:', error)
      throw error
    }
  },

  // 辅助转换函数
  getProtocolName: (p) => {
    const map = { 6: 'TCP', 17: 'UDP', 1: 'ICMP', 0: 'Any' }
    return map[p] || 'Unknown'
  }
}
