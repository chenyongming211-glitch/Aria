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
  if (['auto', 'policing', 'shaping'].includes(value)) return value
  return 'auto'
}

function normalizePayloadMode(rule) {
  const value = String(rule.mode || '').trim().toLowerCase()
  if (!value) return 'auto'
  if (!['auto', 'policing', 'shaping'].includes(value)) {
    throw new Error('QoS mode must be auto, policing, or shaping')
  }
  return value
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
    shaped_bytes: Number(stats.shaped_bytes ?? 0),
    load_error: rule.stats_error || rule.datapath_stats_error || stats.error || ''
  }
}

function mapCommandStatusToPolicyStatus(status) {
  const normalized = String(status || '').trim().toLowerCase()
  if (normalized === 'pending') return 'pending'
  if (['sent', 'acknowledged', 'queued', 'in_progress'].includes(normalized)) return 'in_progress'
  if (normalized === 'completed') return 'applied'
  if (normalized === 'failed') return 'error'
  if (normalized === 'stale') return 'stale'
  return ''
}

function pendingCountForCommandStatus(status) {
  return ['pending', 'sent', 'acknowledged', 'queued', 'in_progress'].includes(String(status || '').trim().toLowerCase()) ? 1 : 0
}

function normalizeDeliveryFields(rule) {
  const dispatch = rule.dispatch || {}
  const lastDelivery = rule.last_delivery || dispatch.last_delivery || null
  const deliveryHistory = Array.isArray(rule.delivery_history)
    ? rule.delivery_history
    : (lastDelivery ? [lastDelivery] : [])
  const deliveryStatus = lastDelivery?.command_status || dispatch.status || ''
  const policyStatus = rule.policy_status || mapCommandStatusToPolicyStatus(deliveryStatus) || 'idle'
  const pendingCmds = typeof rule.pending_cmds === 'number'
    ? rule.pending_cmds
    : (deliveryHistory.length > 0
        ? deliveryHistory.reduce((total, delivery) => total + pendingCountForCommandStatus(delivery?.command_status), 0)
        : pendingCountForCommandStatus(dispatch.status))
  const lastError = rule.last_delivery_error ||
    lastDelivery?.last_error ||
    rule.last_command_error ||
    ''

  return {
    dispatch,
    policy_status: policyStatus,
    policyStatus,
    pending_cmds: pendingCmds,
    desired_state_version: rule.desired_state_version || dispatch.desired_state_version || '',
    desired_state_updated_at: rule.desired_state_updated_at || dispatch.desired_state_updated_at || '',
    last_delivery: lastDelivery,
    delivery_history: deliveryHistory,
    last_delivery_command_id: rule.last_delivery_command_id || lastDelivery?.command_id || dispatch.command_id || '',
    last_delivery_action: rule.last_delivery_action || lastDelivery?.action || '',
    last_command_error: lastError,
    last_delivery_error: lastError,
    last_sync_at: rule.last_delivery_at || rule.last_sync_at || lastDelivery?.updated_at || null
  }
}

function qosGroupForRule(rule) {
  const direction = normalizeDirection(rule)
  const src = rule.src_cidr || rule.src_net || ''
  const dst = rule.dst_cidr || rule.dst_net || ''
  const members = Array.isArray(rule.group?.members) ? rule.group.members : []
  const memberCIDRs = members.map(member => member.cidr).filter(Boolean)
  if (memberCIDRs.length > 0) return memberCIDRs.join(', ')
  const groupName = rule.group_name || rule.group?.name || ''
  if (groupName && rule.group?.kind !== 'inline') return groupName
  const directGroup = typeof rule.group === 'string' ? rule.group : ''
  const explicit = rule.group_cidr || directGroup || rule.runtime_group || ''
  if (explicit && explicit !== rule.group_id) return explicit
  if (direction === 'ingress') return src || dst || 'any'
  const cidr = dst || src
  if (cidr) return cidr
  if (rule.group_id) return '未知 IP Group'
  return 'any'
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

function normalizeQoSRecord(rule, nodeId) {
  let bandwidthMbps = Number(rule.bandwidth_mbps ?? 0)
  const rateBps = normalizeRateBps(rule, bandwidthMbps)
  if ((!Number.isFinite(bandwidthMbps) || bandwidthMbps <= 0) && rateBps > 0) {
    bandwidthMbps = rateBps / 1000000
  }
  const burstBytes = normalizeBurstBytes(rule, rateBps)
  const deliveryFields = normalizeDeliveryFields(rule)
  const normalized = {
    ...rule,
    node_id: nodeId || rule.node_id || '',
    direction: normalizeDirection(rule),
    mode: normalizeMode(rule),
    group_id: rule.group_id || '',
    group_name: rule.group_name || rule.group?.name || '',
    rate_bps: rateBps,
    burst_bytes: burstBytes,
    priority: Number(rule.priority ?? 100),
    stats: normalizeStats(rule),
    bandwidth_mbps: bandwidthMbps,
    bandwidth: bandwidthMbps,
    status: rule.enabled ? 'active' : 'inactive',
    ...deliveryFields
  }
  normalized.runtime_group = qosGroupForRule(normalized)
  normalized.group_cidr = normalized.runtime_group
  normalized.runtime_rate = normalized.rate_bps
  normalized.runtime_burst = normalized.burst_bytes
  return normalized
}

function normalizeQoSMutationResult(data, nodeId, includeRuleFields = true) {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return data
  const hasDeliverySignal = data.dispatch || data.last_delivery || Array.isArray(data.delivery_history) || data.policy_status
  if (!hasDeliverySignal) return data
  if (!includeRuleFields) {
    return {
      ...data,
      node_id: nodeId || data.node_id || '',
      ...normalizeDeliveryFields(data)
    }
  }
  return normalizeQoSRecord(data, nodeId)
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
      return rules.map(rule => normalizeQoSRecord(rule, nodeId))
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
      return normalizeQoSMutationResult(response.data?.data || response.data, nodeId)
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
      return normalizeQoSMutationResult(response.data?.data || response.data, nodeId, false)
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
      return normalizeQoSMutationResult(response.data?.data || response.data, nodeId)
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
