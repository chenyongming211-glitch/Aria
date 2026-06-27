import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import { usePolicyApi } from '@/composables/usePolicyApi'
import {
  mapCommandStatusToPolicyStatus,
  pendingCountForCommandStatus
} from '@/utils/controlLoopStatus'

type ACLRuleID = string | number
type ACLDirection = 'ingress' | 'egress' | 'both'
type ACLAction = 'allow' | 'deny'

interface ACLGroup {
  kind?: string
  name?: string
  members?: Array<{ cidr?: string }>
}

interface ACLStats {
  packets?: number
  bytes?: number
  passed_packets?: number
  passed_bytes?: number
  dropped_packets?: number
  dropped_bytes?: number
}

interface ACLDelivery {
  command_id?: string
  command_status?: string
  action?: string
  last_error?: string
  updated_at?: string
}

interface ACLDispatch {
  desired_state_version?: string
  desired_state_updated_at?: string
  command_id?: string
  status?: string
  last_delivery?: ACLDelivery | null
}

interface ACLNodeState {
  configuration_status?: string
  pending_cmds?: number
  last_command_error?: string
  last_sync_at?: string | null
}

interface ACLRuleRecord extends Record<string, unknown> {
  id?: ACLRuleID
  node_id?: string
  name?: string
  action?: string
  direction?: string
  protocol?: number | string
  dst_port?: number | string
  min_port?: number | string
  max_port?: number | string
  ports?: string
  src_cidr?: string
  dst_cidr?: string
  src_net?: string
  dst_net?: string
  src_group_id?: string
  dst_group_id?: string
  srcGroupId?: string
  dstGroupId?: string
  src_group_name?: string
  dst_group_name?: string
  src_group?: ACLGroup
  dst_group?: ACLGroup
  src_node?: string
  dst_node?: string
  enabled?: boolean
  priority?: number | string
  description?: string
  stats?: ACLStats
  datapath_stats?: ACLStats
  dispatch?: ACLDispatch
  last_delivery?: ACLDelivery | null
  delivery_history?: ACLDelivery[]
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

interface NormalizedACLRule extends ACLRuleRecord {
  node_id: string
  name: string
  action: ACLAction
  direction: ACLDirection
  ports: string
  src_cidr: string
  dst_cidr: string
  src_group_id: string
  dst_group_id: string
  src_group_name: string
  dst_group_name: string
  dst_port: number
  src_net: string
  dst_net: string
  runtime_src_group: string
  runtime_dst_group: string
  runtime_ports: string
  stats: Required<Pick<ACLStats, 'packets' | 'bytes' | 'dropped_packets' | 'dropped_bytes'>>
  min_port: number
  max_port: number
}

interface ACLRulePayload extends ACLRuleRecord {
  node_id?: string
}

interface ACLSubmitPayload {
  name?: string
  src_cidr: string
  dst_cidr: string
  protocol: number
  dst_port: number
  direction: ACLDirection
  ports: string
  action: ACLAction
  enabled: boolean
  priority: number
  description: string
  src_group_id?: string
  dst_group_id?: string
  src_node?: string
  dst_node?: string
}

interface ACLFilters {
  name?: string
  action?: ACLAction | string
  enabled?: boolean
  priority?: number | string
  node_id?: string
}

interface ACLRuleNodeMapEntry {
  nodeId: string
  rule: NormalizedACLRule
}

interface ApiResponseLike {
  data?: unknown
}

const aclRuleNodeMap = new Map<string, ACLRuleNodeMapEntry>()

const aclRuleKey = (tenantId: string, ruleId: ACLRuleID) => `${tenantId}:${String(ruleId)}`

if (typeof window !== 'undefined') {
  window.addEventListener('tenantChanged', () => {
    aclRuleNodeMap.clear()
  })
}

function normalizeRuleRecord(rule: ACLRuleRecord, nodeId?: string): NormalizedACLRule {
  const dstPort = Number(rule.dst_port ?? rule.max_port ?? 0)
  const ports = rule.ports || (dstPort > 0 ? String(dstPort) : '')
  const srcCIDR = rule.src_cidr || rule.src_net || ''
  const dstCIDR = rule.dst_cidr || rule.dst_net || ''
  const srcGroupID = rule.src_group_id || rule.srcGroupId || ''
  const dstGroupID = rule.dst_group_id || rule.dstGroupId || ''
  const action = normalizeAction(rule.action)
  const direction = normalizeDirection(rule.direction)
  return {
    ...rule,
    node_id: nodeId || rule.node_id || '',
    name: rule.name || '',
    action,
    direction,
    ports,
    src_cidr: srcCIDR,
    dst_cidr: dstCIDR,
    src_group_id: srcGroupID,
    dst_group_id: dstGroupID,
    src_group_name: rule.src_group_name || rule.src_group?.name || '',
    dst_group_name: rule.dst_group_name || rule.dst_group?.name || '',
    dst_port: dstPort,
    src_net: srcCIDR,
    dst_net: dstCIDR,
    runtime_src_group: groupLabel(rule.src_group, rule.src_group_name, srcGroupID, srcCIDR),
    runtime_dst_group: groupLabel(rule.dst_group, rule.dst_group_name, dstGroupID, dstCIDR),
    runtime_ports: ports || 'all',
    stats: normalizeStats(rule),
    min_port: dstPort > 0 ? dstPort : 0,
    max_port: dstPort > 0 ? dstPort : 65535
  }
}

function groupLabel(group: ACLGroup | undefined, groupName: unknown, groupId: unknown, cidr: unknown): string {
  const members = Array.isArray(group?.members) ? group.members : []
  const memberCIDRs = members.map(member => member.cidr).filter(Boolean)
  if (memberCIDRs.length > 0) return memberCIDRs.join(', ')
  if (groupName && group?.kind !== 'inline') return String(groupName)
  if (group?.name && group.kind !== 'inline') return group.name
  const normalizedCIDR = cidrOrAny(cidr)
  if (normalizedCIDR !== 'any') return normalizedCIDR
  if (groupId) return '未知 IP Group'
  return 'any'
}

function applyFilters(rules: NormalizedACLRule[], filters: ACLFilters = {}): NormalizedACLRule[] {
  return rules.filter((rule) => {
    if (filters.name && !String(rule.name || '').toLowerCase().includes(String(filters.name).toLowerCase())) {
      return false
    }
    if (filters.action && rule.action !== filters.action) {
      return false
    }
    if (filters.enabled !== undefined && rule.enabled !== filters.enabled) {
      return false
    }
    if (filters.priority && Number(rule.priority) !== Number(filters.priority)) {
      return false
    }
    if (filters.node_id && rule.node_id !== filters.node_id) {
      return false
    }
    return true
  })
}

function normalizeRulePayload(rule: ACLRulePayload): ACLSubmitPayload {
  const srcCIDR = rule.src_cidr || rule.src_net || ''
  const dstCIDR = rule.dst_cidr || rule.dst_net || ''
  const srcGroupID = rule.src_group_id || rule.srcGroupId || ''
  const dstGroupID = rule.dst_group_id || rule.dstGroupId || ''
  const dstPort = Number(rule.dst_port ?? rule.max_port ?? 0)
  const ports = rule.ports || (dstPort > 0 ? String(dstPort) : '')
  const payload: ACLSubmitPayload = {
    name: rule.name,
    src_cidr: srcGroupID ? '' : srcCIDR,
    dst_cidr: dstGroupID ? '' : dstCIDR,
    protocol: Number(rule.protocol || 0),
    dst_port: dstPort,
    direction: normalizeDirection(rule.direction),
    ports,
    action: normalizeAction(rule.action),
    enabled: rule.enabled !== false,
    priority: Number(rule.priority || 100),
    description: rule.description || ''
  }

  if (srcGroupID) {
    payload.src_group_id = srcGroupID
  }
  if (dstGroupID) {
    payload.dst_group_id = dstGroupID
  }

  if (rule.src_node) {
    payload.src_node = rule.src_node
  }
  if (rule.dst_node) {
    payload.dst_node = rule.dst_node
  }

  return payload
}

function normalizeAction(action: unknown): ACLAction {
  const value = String(action || '').trim().toLowerCase()
  if (['deny', 'drop'].includes(value)) return 'deny'
  return 'allow'
}

function normalizeDirection(direction: unknown): ACLDirection {
  const value = String(direction || '').trim().toLowerCase()
  if (['ingress', 'egress', 'both'].includes(value)) return value as ACLDirection
  if (value === 'in') return 'ingress'
  if (value === 'out') return 'egress'
  if (value === 'all') return 'both'
  return 'ingress'
}

function cidrOrAny(value: unknown): string {
  const trimmed = String(value || '').trim()
  if (!trimmed || trimmed === '0' || trimmed === '0.0.0.0/0' || trimmed === '::/0') {
    return 'any'
  }
  return trimmed
}

function normalizeStats(rule: ACLRuleRecord): Required<Pick<ACLStats, 'packets' | 'bytes' | 'dropped_packets' | 'dropped_bytes'>> {
  const stats = rule.stats || rule.datapath_stats || {}
  return {
    packets: Number(stats.packets ?? stats.passed_packets ?? 0),
    bytes: Number(stats.bytes ?? stats.passed_bytes ?? 0),
    dropped_packets: Number(stats.dropped_packets ?? 0),
    dropped_bytes: Number(stats.dropped_bytes ?? 0)
  }
}

function normalizeDeliveryFields(rule: ACLRuleRecord, nodeState: ACLNodeState = {}) {
  const dispatch = rule.dispatch || {}
  const lastDelivery = rule.last_delivery || dispatch.last_delivery || null
  const deliveryHistory = Array.isArray(rule.delivery_history)
    ? rule.delivery_history
    : (lastDelivery ? [lastDelivery] : [])
  const deliveryStatus = lastDelivery?.command_status || dispatch.status || ''
  const policyStatus = rule.policy_status ||
    mapCommandStatusToPolicyStatus(deliveryStatus) ||
    nodeState.configuration_status ||
    'idle'
  const pendingCmds = typeof rule.pending_cmds === 'number'
    ? rule.pending_cmds
    : (deliveryHistory.length > 0
        ? deliveryHistory.reduce((total: number, delivery: ACLDelivery) => total + pendingCountForCommandStatus(delivery?.command_status), 0)
        : pendingCountForCommandStatus(dispatch.status) || nodeState.pending_cmds || 0)
  const lastError = rule.last_delivery_error ||
    lastDelivery?.last_error ||
    rule.last_command_error ||
    nodeState.last_command_error ||
    ''

  return {
    dispatch,
    policy_status: policyStatus,
    pending_cmds: pendingCmds,
    desired_state_version: rule.desired_state_version || dispatch.desired_state_version || '',
    desired_state_updated_at: rule.desired_state_updated_at || dispatch.desired_state_updated_at || '',
    last_command_error: lastError,
    last_delivery_error: lastError,
    last_sync_at: rule.last_delivery_at || rule.last_sync_at || lastDelivery?.updated_at || nodeState.last_sync_at || null,
    last_delivery: lastDelivery,
    delivery_history: deliveryHistory,
    last_delivery_command_id: rule.last_delivery_command_id || lastDelivery?.command_id || dispatch.command_id || '',
    last_delivery_action: rule.last_delivery_action || lastDelivery?.action || ''
  }
}

function normalizeACLMutationResult(data: unknown, nodeId?: string, includeRuleFields = true) {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return data
  const record = data as ACLRuleRecord
  const hasDeliverySignal = record.dispatch || record.last_delivery || Array.isArray(record.delivery_history) || record.policy_status
  if (!hasDeliverySignal) return data
  const base = includeRuleFields ? normalizeRuleRecord(record, nodeId) : { ...record, node_id: nodeId || record.node_id || '' }
  return {
    ...base,
    ...normalizeDeliveryFields(record)
  }
}

function normalizeListResponse(response: ApiResponseLike): ACLRuleRecord[] {
  const body = response?.data
  if (Array.isArray(body)) return body
  if (!body || typeof body !== 'object') return []

  if ('success' in body) {
    const data = (body as { data?: unknown }).data
    if (data == null) return []
    if (Array.isArray(data)) return data
    if (typeof data === 'object' && data && Array.isArray((data as { items?: unknown }).items)) {
      return (data as { items: ACLRuleRecord[] }).items
    }
    throw new Error('Invalid list response')
  }

  if (Array.isArray((body as { items?: unknown }).items)) return (body as { items: ACLRuleRecord[] }).items
  return []
}

/**
 * ACL 规则管理 API
 */
export const useAclApi = {
  /**
   * 获取指定节点的 ACL 规则
   * @param {string} nodeId - 节点ID
   * @param {Object} filters - 过滤参数
   */
  getACLRulesByNode: async (nodeId: string, filters: ACLFilters = {}): Promise<NormalizedACLRule[]> => {
    try {
      const tenantId = requireCurrentTenantId()
      if (!nodeId) return []

      const response = await api.get(API_ENDPOINTS.TENANT.NODE_ACLS(tenantId, nodeId))
      const rules = normalizeListResponse(response)

      // 归一化处理，补充节点信息
      const normalizedRules: NormalizedACLRule[] = rules.map(rule => {
        const normalized: NormalizedACLRule = {
          ...normalizeRuleRecord(rule, nodeId),
          ...normalizeDeliveryFields(rule)
        }
        // 缓存映射用于后续更新删除
        if (normalized.id !== undefined) {
          aclRuleNodeMap.set(aclRuleKey(tenantId, normalized.id), { nodeId, rule: normalized })
        }
        return normalized
      })

      return applyFilters(normalizedRules, filters)
    } catch (error) {
      console.error('获取节点 ACL 规则失败:', error)
      throw error
    }
  },

  getACLRules: async (filters: ACLFilters = {}): Promise<NormalizedACLRule[]> => {
    // 兼容旧调用，如果 filters 中带了 node_id，则定向查询
    if (filters.node_id) {
      return useAclApi.getACLRulesByNode(filters.node_id, filters)
    }
    // 否则不再执行全局聚合，提示需要 nodeId
    console.warn('getACLRules now requires nodeId for performance. Please use getACLRulesByNode.')
    return []
  },

  createACLRule: async (rule: ACLRulePayload) => {
    try {
      const tenantId = requireCurrentTenantId()
      if (!rule.node_id) {
        throw new Error('node_id is required for ACL rule creation')
      }

      const response = await api.post(
        API_ENDPOINTS.TENANT.NODE_ACLS(tenantId, rule.node_id),
        normalizeRulePayload(rule)
      )
      return normalizeACLMutationResult(response.data?.data || response.data, rule.node_id)
    } catch (error) {
      console.error('创建 ACL 规则失败:', error)
      throw error
    }
  },

  updateACLRule: async (ruleId: ACLRuleID, rule: ACLRulePayload) => {
    try {
      const tenantId = requireCurrentTenantId()
      const mapping = aclRuleNodeMap.get(aclRuleKey(tenantId, ruleId))
      const nodeId = rule.node_id || mapping?.nodeId

      if (!nodeId) {
        throw new Error('node_id is required for ACL rule update')
      }

      const response = await api.put(
        API_ENDPOINTS.TENANT.NODE_ACL(tenantId, nodeId, ruleId),
        normalizeRulePayload({ ...mapping?.rule, ...rule, node_id: nodeId })
      )
      return normalizeACLMutationResult(response.data?.data || response.data, nodeId)
    } catch (error) {
      console.error('更新 ACL 规则失败:', error)
      throw error
    }
  },

  deleteACLRule: async (ruleId: ACLRuleID, nodeId?: string) => {
    try {
      const tenantId = requireCurrentTenantId()
      const mapping = aclRuleNodeMap.get(aclRuleKey(tenantId, ruleId))
      const resolvedNodeId = nodeId || mapping?.nodeId

      if (!resolvedNodeId) {
        throw new Error('node_id is required for ACL rule deletion')
      }

      const response = await api.delete(API_ENDPOINTS.TENANT.NODE_ACL(tenantId, resolvedNodeId, ruleId))
      aclRuleNodeMap.delete(aclRuleKey(tenantId, ruleId))
      return normalizeACLMutationResult(response.data?.data || response.data, resolvedNodeId, false)
    } catch (error) {
      console.error('删除 ACL 规则失败:', error)
      throw error
    }
  },

  retryACLPolicySync: async (rule: ACLRuleRecord) => {
    try {
      const nodeId = rule?.node_id
      const policyRef = rule?.policy_ref || rule?.id
      if (!nodeId) {
        throw new Error('node_id is required for ACL policy retry')
      }
      if (!policyRef) {
        throw new Error('policy ref is required for ACL policy retry')
      }

      const retried = await usePolicyApi.retryPolicySync({
        nodeId,
        kind: 'acl',
        policyRef: String(policyRef),
        policyName: rule?.name || ''
      })
      return normalizeACLMutationResult({
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
      console.error('重试 ACL 策略下发失败:', error)
      throw error
    }
  }
}
