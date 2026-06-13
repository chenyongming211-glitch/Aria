import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

function normalizeBandwidthMbps(rule) {
  const value = Number(rule.bandwidth_mbps ?? 0)
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error('bandwidth_mbps must be greater than 0')
  }
  return value
}

function normalizeDirection(rule) {
  const value = String(rule.direction || '').trim().toLowerCase()
  if (['ingress', 'egress', 'both'].includes(value)) return value
  if ((rule.src_cidr || rule.src_net || rule.group_cidr || rule.group) && !(rule.dst_cidr || rule.dst_net)) return 'ingress'
  return 'egress'
}

function normalizeMode(rule) {
  const value = String(rule.mode || '').trim().toLowerCase()
  return value === 'shaping' ? 'shaping' : 'policing'
}

function normalizePayloadMode(rule) {
  const value = String(rule.mode || '').trim().toLowerCase()
  if (value && value !== 'policing') {
    throw new Error('当前 QoS 只支持 policing 模式')
  }
  return 'policing'
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

function normalizeStats(rule) {
  const stats = rule.stats || rule.datapath_stats || {}
  return {
    passed_packets: Number(stats.passed_packets ?? stats.packets ?? 0),
    passed_bytes: Number(stats.passed_bytes ?? stats.bytes ?? 0),
    dropped_packets: Number(stats.dropped_packets ?? 0),
    dropped_bytes: Number(stats.dropped_bytes ?? 0),
    shaped_packets: Number(stats.shaped_packets ?? 0),
    shaped_bytes: Number(stats.shaped_bytes ?? 0)
  }
}

function qosGroupForRule(rule) {
  const direction = normalizeDirection(rule)
  const src = rule.src_cidr || rule.src_net || ''
  const dst = rule.dst_cidr || rule.dst_net || ''
  const explicit = rule.group_name || rule.group_id || rule.group_cidr || rule.group || rule.runtime_group || ''
  if (explicit) return explicit
  if (direction === 'ingress') return src || dst || 'any'
  return dst || src || 'any'
}

function normalizeRulePayload(rule) {
  const bandwidthMbps = normalizeBandwidthMbps(rule)
  const rateBps = normalizeRateBps(rule, bandwidthMbps)
  const direction = normalizeDirection(rule)
  const groupID = rule.group_id || rule.groupId || ''
  const group = rule.group_cidr || rule.group || ''
  const srcCIDR = groupID ? '' : (rule.src_cidr || rule.src_net || (direction === 'ingress' ? group : '') || '')
  const dstCIDR = groupID ? '' : (rule.dst_cidr || rule.dst_net || (direction !== 'ingress' ? group : '') || '')
  const payload = {
    src_cidr: srcCIDR,
    dst_cidr: dstCIDR,
    src_port: Number(rule.src_port || 0),
    dst_port: Number(rule.dst_port || 0),
    protocol: Number(rule.protocol || 0),
    bandwidth_mbps: bandwidthMbps,
    direction,
    rate_bps: rateBps,
    burst_bytes: normalizeBurstBytes(rule, rateBps),
    priority: Number(rule.priority ?? 100),
    mode: normalizePayloadMode(rule),
    description: rule.description || '',
    enabled: rule.enabled !== false
  }
  if (groupID) {
    payload.group_id = groupID
  }
  return payload
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
 * 新运行模型：每条规则直接描述 group + direction + rate/burst + mode。
 */
export const useQosApi = {
  /**
   * 获取指定节点的 QoS 规则
   * @param {string} nodeId - 节点ID
   */
  getQoSRulesByNode: async (nodeId) => {
    try {
      const tenantId = requireCurrentTenantId()
      if (!nodeId) return []

      const response = await api.get(API_ENDPOINTS.TENANT.NODE_QOS(tenantId, nodeId))
      const rules = normalizeListResponse(response)

      // 统一字段映射
      return rules.map(rule => {
        let bandwidthMbps = Number(rule.bandwidth_mbps ?? 0)
        const rateBps = normalizeRateBps(rule, bandwidthMbps)
        if ((!Number.isFinite(bandwidthMbps) || bandwidthMbps <= 0) && rateBps > 0) {
          bandwidthMbps = rateBps / 1000000
        }
        const burstBytes = normalizeBurstBytes(rule, rateBps)
        const normalized = {
          ...rule,
          node_id: nodeId,
          direction: normalizeDirection(rule),
          mode: normalizeMode(rule),
          group_id: rule.group_id || '',
          group_name: rule.group_name || rule.group?.name || '',
          rate_bps: rateBps,
          burst_bytes: burstBytes,
          priority: Number(rule.priority ?? 100),
          stats: normalizeStats(rule),
          // 兼容旧 UI 字段名
          bandwidth_mbps: bandwidthMbps,
          bandwidth: bandwidthMbps,
          status: rule.enabled ? 'active' : 'inactive',
          policyStatus: rule.policy_status || 'idle'
        }
        normalized.runtime_group = qosGroupForRule(normalized)
        normalized.group_cidr = normalized.runtime_group
        normalized.runtime_rate = normalized.rate_bps
        normalized.runtime_burst = normalized.burst_bytes
        return normalized
      })
    } catch (error) {
      console.error('获取节点 QoS 规则失败:', error)
      throw error
    }
  },

  /**
   * 创建 QoS 规则
   */
  createQoSRule: async (nodeId, rule) => {
    try {
      const tenantId = requireCurrentTenantId()
      const payload = normalizeRulePayload(rule)

      const response = await api.post(
        API_ENDPOINTS.TENANT.NODE_QOS(tenantId, nodeId),
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
  deleteQoSRule: async (nodeId, ruleId) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.delete(
        API_ENDPOINTS.TENANT.NODE_QOS_RULE(tenantId, nodeId, ruleId)
      )
      return response.data?.data || response.data
    } catch (error) {
      console.error('删除 QoS 规则失败:', error)
      throw error
    }
  },

  updateQoSRule: async (nodeId, ruleId, rule) => {
    try {
      const tenantId = requireCurrentTenantId()
      const payload = normalizeRulePayload(rule)

      const response = await api.put(
        API_ENDPOINTS.TENANT.NODE_QOS_RULE(tenantId, nodeId, ruleId),
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
