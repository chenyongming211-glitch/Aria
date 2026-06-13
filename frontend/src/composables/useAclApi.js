import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

const aclRuleNodeMap = new Map()

const aclRuleKey = (tenantId, ruleId) => `${tenantId}:${String(ruleId)}`

if (typeof window !== 'undefined') {
  window.addEventListener('tenantChanged', () => {
    aclRuleNodeMap.clear()
  })
}

function normalizeRuleRecord(rule, nodeId) {
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
    node_id: nodeId,
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
    runtime_src_group: rule.src_group_name || srcGroupID || cidrOrAny(srcCIDR),
    runtime_dst_group: rule.dst_group_name || dstGroupID || cidrOrAny(dstCIDR),
    runtime_ports: ports || 'all',
    stats: normalizeStats(rule),
    min_port: dstPort > 0 ? dstPort : 0,
    max_port: dstPort > 0 ? dstPort : 65535
  }
}

function applyFilters(rules, filters = {}) {
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

function normalizeRulePayload(rule) {
  const srcCIDR = rule.src_cidr || rule.src_net || ''
  const dstCIDR = rule.dst_cidr || rule.dst_net || ''
  const srcGroupID = rule.src_group_id || rule.srcGroupId || ''
  const dstGroupID = rule.dst_group_id || rule.dstGroupId || ''
  const dstPort = Number(rule.dst_port ?? rule.max_port ?? 0)
  const ports = rule.ports || (dstPort > 0 ? String(dstPort) : '')
  const payload = {
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

function normalizeAction(action) {
  const value = String(action || '').trim().toLowerCase()
  if (['deny', 'drop'].includes(value)) return 'deny'
  return 'allow'
}

function normalizeDirection(direction) {
  const value = String(direction || '').trim().toLowerCase()
  if (['ingress', 'egress', 'both'].includes(value)) return value
  if (value === 'in') return 'ingress'
  if (value === 'out') return 'egress'
  if (value === 'all') return 'both'
  return 'ingress'
}

function cidrOrAny(value) {
  const trimmed = String(value || '').trim()
  if (!trimmed || trimmed === '0' || trimmed === '0.0.0.0/0' || trimmed === '::/0') {
    return 'any'
  }
  return trimmed
}

function normalizeStats(rule) {
  const stats = rule.stats || rule.datapath_stats || {}
  return {
    packets: Number(stats.packets ?? stats.passed_packets ?? 0),
    bytes: Number(stats.bytes ?? stats.passed_bytes ?? 0),
    dropped_packets: Number(stats.dropped_packets ?? 0),
    dropped_bytes: Number(stats.dropped_bytes ?? 0)
  }
}

function normalizeDeliveryFields(rule, nodeState = {}) {
  return {
    policy_status: rule.policy_status || nodeState.configuration_status || 'idle',
    pending_cmds: typeof rule.pending_cmds === 'number' ? rule.pending_cmds : (nodeState.pending_cmds || 0),
    last_command_error: rule.last_delivery_error || rule.last_command_error || nodeState.last_command_error || '',
    last_sync_at: rule.last_delivery_at || rule.last_sync_at || nodeState.last_sync_at || null,
    last_delivery: rule.last_delivery || null,
    delivery_history: Array.isArray(rule.delivery_history) ? rule.delivery_history : [],
    last_delivery_command_id: rule.last_delivery_command_id || rule.last_delivery?.command_id || '',
    last_delivery_action: rule.last_delivery_action || rule.last_delivery?.action || ''
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
 * ACL 规则管理 API
 */
export const useAclApi = {
  /**
   * 获取指定节点的 ACL 规则
   * @param {string} nodeId - 节点ID
   * @param {Object} filters - 过滤参数
   */
  getACLRulesByNode: async (nodeId, filters = {}) => {
    try {
      const tenantId = requireCurrentTenantId()
      if (!nodeId) return []

      const response = await api.get(API_ENDPOINTS.TENANT.NODE_ACLS(tenantId, nodeId))
      const rules = normalizeListResponse(response)

      // 归一化处理，补充节点信息
      const normalizedRules = rules.map(rule => {
        const normalized = {
          ...normalizeRuleRecord(rule, nodeId),
          ...normalizeDeliveryFields(rule)
        }
        // 缓存映射用于后续更新删除
        aclRuleNodeMap.set(aclRuleKey(tenantId, normalized.id), { nodeId, rule: normalized })
        return normalized
      })

      return applyFilters(normalizedRules, filters)
    } catch (error) {
      console.error('获取节点 ACL 规则失败:', error)
      throw error
    }
  },

  getACLRules: async (filters = {}) => {
    // 兼容旧调用，如果 filters 中带了 node_id，则定向查询
    if (filters.node_id) {
      return useAclApi.getACLRulesByNode(filters.node_id, filters)
    }
    // 否则不再执行全局聚合，提示需要 nodeId
    console.warn('getACLRules now requires nodeId for performance. Please use getACLRulesByNode.')
    return []
  },

  createACLRule: async (rule) => {
    try {
      const tenantId = requireCurrentTenantId()
      if (!rule.node_id) {
        throw new Error('node_id is required for ACL rule creation')
      }

      const response = await api.post(
        API_ENDPOINTS.TENANT.NODE_ACLS(tenantId, rule.node_id),
        normalizeRulePayload(rule)
      )
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建 ACL 规则失败:', error)
      throw error
    }
  },

  updateACLRule: async (ruleId, rule) => {
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
      return response.data?.data || response.data
    } catch (error) {
      console.error('更新 ACL 规则失败:', error)
      throw error
    }
  },

  deleteACLRule: async (ruleId, nodeId) => {
    try {
      const tenantId = requireCurrentTenantId()
      const mapping = aclRuleNodeMap.get(aclRuleKey(tenantId, ruleId))
      const resolvedNodeId = nodeId || mapping?.nodeId

      if (!resolvedNodeId) {
        throw new Error('node_id is required for ACL rule deletion')
      }

      const response = await api.delete(API_ENDPOINTS.TENANT.NODE_ACL(tenantId, resolvedNodeId, ruleId))
      aclRuleNodeMap.delete(aclRuleKey(tenantId, ruleId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('删除 ACL 规则失败:', error)
      throw error
    }
  }
}
