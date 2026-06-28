import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import { usePolicyApi } from '@/composables/usePolicyApi'
import { t } from '@/i18n'
import {
  mapCommandStatusToPolicyStatus,
  pendingCountForCommandStatus
} from '@/utils/controlLoopStatus'

type QoSRuleID = string | number
type QoSDirection = 'ingress' | 'egress' | 'both'
type QoSMode = 'auto' | 'policing' | 'shaping'

interface QoSGroup {
  kind?: string
  name?: string
  members?: Array<{ cidr?: string }>
}

interface QoSStats {
  packets?: number
  bytes?: number
  passed_packets?: number
  passed_bytes?: number
  dropped_packets?: number
  dropped_bytes?: number
  shaped_packets?: number
  shaped_bytes?: number
  error?: string
}

interface QoSDelivery {
  command_id?: string
  command_status?: string
  action?: string
  last_error?: string
  updated_at?: string
}

interface QoSDispatch {
  desired_state_version?: string
  desired_state_updated_at?: string
  command_id?: string
  status?: string
  last_delivery?: QoSDelivery | null
}

interface QoSRuleRecord extends Record<string, unknown> {
  id?: QoSRuleID
  node_id?: string
  description?: string
  name?: string
  direction?: string
  mode?: string
  group_id?: string
  groupId?: string
  group_name?: string
  group?: QoSGroup | string
  group_cidr?: string
  runtime_group?: string
  src_cidr?: string
  dst_cidr?: string
  src_net?: string
  dst_net?: string
  bandwidth_mbps?: number | string
  rate_bps?: number | string
  burst_bytes?: number | string
  priority?: number | string
  enabled?: boolean
  stats?: QoSStats
  datapath_stats?: QoSStats
  stats_error?: string
  datapath_stats_error?: string
  dispatch?: QoSDispatch
  last_delivery?: QoSDelivery | null
  delivery_history?: QoSDelivery[]
  policy_status?: string
  pending_cmds?: number
  desired_state_version?: string
  desired_state_updated_at?: string
  last_delivery_at?: string
  last_sync_at?: string | null
  last_delivery_command_id?: string
  last_delivery_action?: string
  last_delivery_error?: string
  last_command_error?: string
  policy_ref?: string
}

interface NormalizedQoSRule extends QoSRuleRecord {
  node_id: string
  direction: QoSDirection
  mode: QoSMode
  group_id: string
  group_name: string
  rate_bps: number
  burst_bytes: number
  priority: number
  stats: {
    passed_packets: number
    passed_bytes: number
    dropped_packets: number
    dropped_bytes: number
    shaped_packets: number
    shaped_bytes: number
    load_error: string
  }
  bandwidth_mbps: number
  bandwidth: number
  status: string
  runtime_group: string
  group_cidr: string
  runtime_rate: number
  runtime_burst: number
}

interface QoSSubmitPayload {
  src_cidr: string
  dst_cidr: string
  bandwidth_mbps: number
  direction: QoSDirection
  rate_bps: number
  burst_bytes: number
  priority: number
  mode: QoSMode
  description: string
  enabled: boolean
  group_id?: string
}

interface ApiResponseLike {
  data?: unknown
}

function normalizeBandwidthMbps(rule: QoSRuleRecord): number {
  const value = Number(rule.bandwidth_mbps ?? 0)
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error('bandwidth_mbps must be greater than 0')
  }
  return value
}

function normalizeDirection(rule: QoSRuleRecord): QoSDirection {
  const value = String(rule.direction || '').trim().toLowerCase()
  if (['ingress', 'egress', 'both'].includes(value)) return value as QoSDirection
  if ((rule.src_cidr || rule.src_net || rule.group_cidr || rule.group) && !(rule.dst_cidr || rule.dst_net)) return 'ingress'
  return 'egress'
}

function normalizeMode(rule: QoSRuleRecord): QoSMode {
  const value = String(rule.mode || '').trim().toLowerCase()
  if (['auto', 'policing', 'shaping'].includes(value)) return value as QoSMode
  return 'auto'
}

function normalizePayloadMode(rule: QoSRuleRecord): QoSMode {
  const value = String(rule.mode || '').trim().toLowerCase()
  if (!value) return 'auto'
  if (!['auto', 'policing', 'shaping'].includes(value)) {
    throw new Error('QoS mode must be auto, policing, or shaping')
  }
  return value as QoSMode
}

function normalizeRateBps(rule: QoSRuleRecord, bandwidthMbps: number): number {
  const value = Number(rule.rate_bps || 0)
  if (Number.isFinite(value) && value > 0) return value
  return bandwidthMbps * 1000000
}

function normalizeBurstBytes(rule: QoSRuleRecord, rateBps: number): number {
  const value = Number(rule.burst_bytes || 0)
  if (Number.isFinite(value) && value > 0) return value
  return Math.max(Math.floor(rateBps / 8 / 10), 1500)
}

function normalizeStats(rule: QoSRuleRecord): NormalizedQoSRule['stats'] {
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

function normalizeDeliveryFields(rule: QoSRuleRecord) {
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
        ? deliveryHistory.reduce((total: number, delivery: QoSDelivery) => total + pendingCountForCommandStatus(delivery?.command_status), 0)
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

function qosGroupForRule(rule: QoSRuleRecord): string {
  const direction = normalizeDirection(rule)
  const src = rule.src_cidr || rule.src_net || ''
  const dst = rule.dst_cidr || rule.dst_net || ''
  const groupObject = typeof rule.group === 'object' && rule.group ? rule.group as QoSGroup : undefined
  const members = Array.isArray(groupObject?.members) ? groupObject.members : []
  const memberCIDRs = members.map(member => member.cidr).filter(Boolean)
  if (memberCIDRs.length > 0) return memberCIDRs.join(', ')
  const groupName = rule.group_name || groupObject?.name || ''
  if (groupName && groupObject?.kind !== 'inline') return groupName
  const directGroup = typeof rule.group === 'string' ? rule.group : ''
  const explicit = rule.group_cidr || directGroup || rule.runtime_group || ''
  if (explicit && explicit !== rule.group_id) return explicit
  if (direction === 'ingress') return src || dst || 'any'
  const cidr = dst || src
  if (cidr) return cidr
  if (rule.group_id) return t('policyTerms.unknownIpGroup')
  return 'any'
}

function normalizeRulePayload(rule: QoSRuleRecord): QoSSubmitPayload {
  const bandwidthMbps = normalizeBandwidthMbps(rule)
  const rateBps = normalizeRateBps(rule, bandwidthMbps)
  const direction = normalizeDirection(rule)
  const groupID = rule.group_id || rule.groupId || ''
  const directGroup = typeof rule.group === 'string' ? rule.group : ''
  const group = rule.group_cidr || directGroup || ''
  const srcCIDR = groupID ? '' : (rule.src_cidr || rule.src_net || (direction === 'ingress' ? group : '') || '')
  const dstCIDR = groupID ? '' : (rule.dst_cidr || rule.dst_net || (direction !== 'ingress' ? group : '') || '')
  const payload: QoSSubmitPayload = {
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

function normalizeListResponse(response: ApiResponseLike): QoSRuleRecord[] {
  const body = response?.data
  if (Array.isArray(body)) return body
  if (!body || typeof body !== 'object') return []

  if ('success' in body) {
    const data = (body as { data?: unknown }).data
    if (data == null) return []
    if (Array.isArray(data)) return data
    if (typeof data === 'object' && data && Array.isArray((data as { items?: unknown }).items)) {
      return (data as { items: QoSRuleRecord[] }).items
    }
    throw new Error('Invalid list response')
  }

  if (Array.isArray((body as { items?: unknown }).items)) return (body as { items: QoSRuleRecord[] }).items
  return []
}

function normalizeQoSRecord(rule: QoSRuleRecord, nodeId?: string): NormalizedQoSRule {
  let bandwidthMbps = Number(rule.bandwidth_mbps ?? 0)
  const rateBps = normalizeRateBps(rule, bandwidthMbps)
  if ((!Number.isFinite(bandwidthMbps) || bandwidthMbps <= 0) && rateBps > 0) {
    bandwidthMbps = rateBps / 1000000
  }
  const burstBytes = normalizeBurstBytes(rule, rateBps)
  const deliveryFields = normalizeDeliveryFields(rule)
  const normalized: NormalizedQoSRule = {
    ...rule,
    node_id: nodeId || rule.node_id || '',
    direction: normalizeDirection(rule),
    mode: normalizeMode(rule),
    group_id: rule.group_id || '',
    group_name: rule.group_name || (typeof rule.group === 'object' && rule.group ? rule.group.name : '') || '',
    rate_bps: rateBps,
    burst_bytes: burstBytes,
    priority: Number(rule.priority ?? 100),
    stats: normalizeStats(rule),
    bandwidth_mbps: bandwidthMbps,
    bandwidth: bandwidthMbps,
    status: rule.enabled ? 'active' : 'inactive',
    runtime_group: '',
    group_cidr: '',
    runtime_rate: rateBps,
    runtime_burst: burstBytes,
    ...deliveryFields
  }
  normalized.runtime_group = qosGroupForRule(normalized)
  normalized.group_cidr = normalized.runtime_group
  normalized.runtime_rate = normalized.rate_bps
  normalized.runtime_burst = normalized.burst_bytes
  return normalized
}

function normalizeQoSMutationResult(data: unknown, nodeId?: string, includeRuleFields = true) {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return data
  const record = data as QoSRuleRecord
  const hasDeliverySignal = record.dispatch || record.last_delivery || Array.isArray(record.delivery_history) || record.policy_status
  if (!hasDeliverySignal) return data
  if (!includeRuleFields) {
    return {
      ...record,
      node_id: nodeId || record.node_id || '',
      ...normalizeDeliveryFields(record)
    }
  }
  return normalizeQoSRecord(record, nodeId)
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
  getQoSRulesByNode: async (nodeId: string): Promise<NormalizedQoSRule[]> => {
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
  createQoSRule: async (nodeId: string, rule: QoSRuleRecord) => {
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
  deleteQoSRule: async (nodeId: string, ruleId: QoSRuleID) => {
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

  updateQoSRule: async (nodeId: string, ruleId: QoSRuleID, rule: QoSRuleRecord) => {
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

  retryQoSPolicySync: async (nodeId: string, rule: QoSRuleRecord) => {
    try {
      const policyRef = rule?.policy_ref || rule?.id
      if (!nodeId) {
        throw new Error('nodeId is required for QoS policy retry')
      }
      if (!policyRef) {
        throw new Error('policy ref is required for QoS policy retry')
      }

      const retried = await usePolicyApi.retryPolicySync({
        nodeId,
        kind: 'qos',
        policyRef: String(policyRef),
        policyName: rule?.description || rule?.name || ''
      })
      return normalizeQoSMutationResult({
        ...rule,
        ...retried,
        id: rule?.id || retried.policyRef,
        policy_status: retried.status,
        pending_cmds: retried.pendingCmds,
        last_delivery: retried.lastDelivery,
        delivery_history: retried.deliveryHistory,
        last_delivery_command_id: retried.lastDeliveryCommandId,
        last_delivery_action: retried.lastDeliveryAction,
        last_delivery_error: retried.lastDeliveryError,
        last_command_error: retried.lastDeliveryError
      }, nodeId)
    } catch (error) {
      console.error('重试 QoS 策略下发失败:', error)
      throw error
    }
  },

  // 辅助转换函数
  getProtocolName: (p: number | string) => {
    const map = { 6: 'TCP', 17: 'UDP', 1: 'ICMP', 0: 'Any' }
    return map[Number(p) as keyof typeof map] || 'Unknown'
  }
}
