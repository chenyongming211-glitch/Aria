import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import { useAgentProxyApi } from './useAgentProxyApi'

const qosRuleCache = new Map()

function getProtocolName(protocol) {
  switch (protocol) {
    case 6: return 'tcp'
    case 17: return 'udp'
    case 1: return 'icmp'
    case 58: return 'icmpv6'
    default: return 'any'
  }
}

function getProtocolNumber(name) {
  switch (name) {
    case 'tcp': return 6
    case 'udp': return 17
    case 'icmp': return 1
    case 'icmpv6': return 58
    default: return 0
  }
}

function categoryFromType(type) {
  switch (type) {
    case 'app': return 'service'
    case 'peer': return 'peers'
    case 'global': return 'ip'
    default: return 'service'
  }
}

function typeFromCategory(category) {
  switch (category) {
    case 'service': return 'app'
    case 'peers': return 'peer'
    case 'ip': return 'global'
    default: return 'app'
  }
}

function normalizeDeliveryFields(rule, node = {}) {
  return {
    policyStatus: rule.policy_status || node.configuration_status || 'idle',
    pendingCmds: typeof rule.pending_cmds === 'number' ? rule.pending_cmds : (node.pending_cmds || 0),
    lastCommandError: rule.last_delivery_error || rule.last_command_error || node.last_command_error || '',
    lastSyncAt: rule.last_delivery_at || rule.last_sync_at || node.last_sync_at || null,
    lastDelivery: rule.last_delivery || null,
    deliveryHistory: Array.isArray(rule.delivery_history) ? rule.delivery_history : [],
    lastDeliveryCommandId: rule.last_delivery_command_id || rule.last_delivery?.command_id || '',
    lastDeliveryAction: rule.last_delivery_action || rule.last_delivery?.action || ''
  }
}

function normalizeQoSRule(category, node, rule) {
  const type = typeFromCategory(category)
  const normalized = {
    id: rule.id,
    nodeId: node.id,
    nodeName: node.hostname || node.public_key || node.id,
    type,
    bandwidth: rule.bandwidth_mbps || rule.bandwidth || 100,
    priority: rule.priority || 50,
    status: rule.enabled === false ? 'inactive' : 'active',
    name: rule.description || `${node.hostname || 'node'}-${category}`,
    srcIp: rule.src_ip || '',
    dstIp: rule.dst_ip || '',
    srcPort: rule.src_port || '',
    dstPort: rule.dst_port || '',
    protocol: getProtocolName(rule.protocol),
    targetIp: rule.src_ip || rule.dst_ip || '',
    ...normalizeDeliveryFields(rule, node)
  }

  qosRuleCache.set(normalized.id, { nodeId: node.id, category, rule: normalized })
  return normalized
}

async function listTenantNodes() {
  const tenantId = requireCurrentTenantId()
  const response = await api.get(API_ENDPOINTS.TENANT.NODES(tenantId))
  return { tenantId, nodes: response.data?.data || response.data || [] }
}

async function fetchCategoryRules(tenantId, node, category) {
  const response = await api.get(API_ENDPOINTS.BANDWIDTH.CATEGORY(tenantId, node.id, category))
  const rules = response.data?.data || response.data || []
  return rules.map((rule) => normalizeQoSRule(category, node, rule))
}

async function createCategoryRule(type, rule) {
  const tenantId = requireCurrentTenantId()
  if (!rule.nodeId) {
    throw new Error('nodeId is required')
  }

  const category = categoryFromType(type)
  const payload = {
    bandwidth_mbps: Number(rule.bandwidth || 0)
  }

  if (type === 'app') {
    payload.src_ip = rule.srcIp || ''
    payload.dst_ip = rule.dstIp || ''
    payload.src_port = parseInt(rule.srcPort, 10) || 0
    payload.dst_port = parseInt(rule.dstPort, 10) || 0
    payload.protocol = getProtocolNumber(rule.protocol)
  } else if (type === 'peer') {
    payload.src_ip = rule.srcIp || ''
    payload.dst_ip = rule.dstIp || ''
  } else {
    payload.src_ip = rule.targetIp || rule.srcIp || ''
  }

  const response = await api.post(API_ENDPOINTS.BANDWIDTH.CATEGORY(tenantId, rule.nodeId, category), payload)
  return response.data?.data || response.data
}

async function deleteCategoryRule(id, nodeId, category) {
  const tenantId = requireCurrentTenantId()
  const response = await api.delete(API_ENDPOINTS.BANDWIDTH.RULE(tenantId, nodeId, category, id))
  qosRuleCache.delete(id)
  return response.data?.data || response.data
}

/**
 * QoS API
 */
export const useQosApi = {
  getBandwidthLimits: async () => {
    const { tenantId, nodes } = await listTenantNodes()
    const results = await Promise.all(nodes.map((node) => fetchCategoryRules(tenantId, node, 'service')))
    return results.flat()
  },

  createBandwidthLimit: async (params) => {
    return createCategoryRule('app', {
      nodeId: params.nodeId,
      bandwidth: params.bandwidth || params.bandwidth_mbps,
      srcIp: params.src_ip,
      dstIp: params.dst_ip,
      srcPort: params.src_port,
      dstPort: params.dst_port,
      protocol: getProtocolName(params.protocol)
    })
  },

  deleteBandwidthLimit: async (limitId, nodeId) => {
    const mapping = qosRuleCache.get(limitId)
    return deleteCategoryRule(limitId, nodeId || mapping?.nodeId, mapping?.category || 'service')
  },

  getPolicies: async () => {
    return useQosApi.getAllRules()
  },

  createPolicy: async (policy) => {
    const type = policy.type || 'app'
    return createCategoryRule(type, {
      nodeId: policy.nodeId,
      bandwidth: policy.limit_bandwidth || policy.bandwidth,
      srcIp: policy.src_ip,
      dstIp: policy.dst_ip,
      srcPort: policy.src_port,
      dstPort: policy.dst_port,
      protocol: policy.protocol_name || getProtocolName(policy.protocol),
      targetIp: policy.targetIp || policy.src_ip
    })
  },

  getPolicy: async (policyId) => {
    const mapping = qosRuleCache.get(policyId)
    return mapping?.rule || null
  },

  updatePolicy: async (policyId, policy) => {
    const mapping = qosRuleCache.get(policyId)
    const existing = mapping?.rule
    if (!existing) {
      throw new Error('Rule not found')
    }

    await deleteCategoryRule(policyId, existing.nodeId, mapping.category)
    return createCategoryRule(policy.type || existing.type, {
      ...existing,
      ...policy,
      nodeId: policy.nodeId || existing.nodeId
    })
  },

  deletePolicy: async (policyId) => {
    const mapping = qosRuleCache.get(policyId)
    if (!mapping) {
      throw new Error('Rule not found')
    }
    return deleteCategoryRule(policyId, mapping.nodeId, mapping.category)
  },

  getAllRules: async () => {
    try {
      const { tenantId, nodes } = await listTenantNodes()
      const results = await Promise.all(
        nodes.flatMap((node) => [
          fetchCategoryRules(tenantId, node, 'service'),
          fetchCategoryRules(tenantId, node, 'peers'),
          fetchCategoryRules(tenantId, node, 'ip')
        ])
      )
      return results.flat()
    } catch (error) {
      console.error('获取所有规则失败:', error)
      return []
    }
  },

  getServiceRules: async () => {
    const { tenantId, nodes } = await listTenantNodes()
    const results = await Promise.all(nodes.map((node) => fetchCategoryRules(tenantId, node, 'service')))
    return results.flat()
  },

  createServiceRule: async (rule) => {
    return createCategoryRule('app', rule)
  },

  getPortRules: async () => {
    return useQosApi.getServiceRules()
  },

  createPortRule: async (rule) => {
    return createCategoryRule('app', {
      ...rule,
      dstPort: rule.port
    })
  },

  getPeerRules: async () => {
    const { tenantId, nodes } = await listTenantNodes()
    const results = await Promise.all(nodes.map((node) => fetchCategoryRules(tenantId, node, 'peers')))
    return results.flat()
  },

  createPeerRule: async (rule) => {
    return createCategoryRule('peer', rule)
  },

  getIpRules: async () => {
    const { tenantId, nodes } = await listTenantNodes()
    const results = await Promise.all(nodes.map((node) => fetchCategoryRules(tenantId, node, 'ip')))
    return results.flat()
  },

  createIpRule: async (rule) => {
    return createCategoryRule('global', rule)
  },

  updateServiceRule: async (id, rule) => {
    return useQosApi.updatePolicy(id, { ...rule, type: 'app' })
  },

  updatePortRule: async (id, rule) => {
    return useQosApi.updatePolicy(id, { ...rule, type: 'app' })
  },

  updatePeerRule: async (id, rule) => {
    return useQosApi.updatePolicy(id, { ...rule, type: 'peer' })
  },

  updateIpRule: async (id, rule) => {
    return useQosApi.updatePolicy(id, { ...rule, type: 'global' })
  },

  deleteServiceRule: async (id) => {
    const mapping = qosRuleCache.get(id)
    return deleteCategoryRule(id, mapping?.nodeId, 'service')
  },

  deletePortRule: async (id) => {
    const mapping = qosRuleCache.get(id)
    return deleteCategoryRule(id, mapping?.nodeId, 'service')
  },

  deletePeerRule: async (id) => {
    const mapping = qosRuleCache.get(id)
    return deleteCategoryRule(id, mapping?.nodeId, 'peers')
  },

  deleteIpRule: async (id) => {
    const mapping = qosRuleCache.get(id)
    return deleteCategoryRule(id, mapping?.nodeId, 'ip')
  },

  applyAllRules: async () => {
    const rules = await useQosApi.getAllRules()
    const nodeIds = [...new Set(rules.map((rule) => rule.nodeId).filter(Boolean))]
    if (nodeIds.length > 0) {
      await useAgentProxyApi.sendBatchCommand({
        node_ids: nodeIds,
        command: {
          command: 'sync',
          params: {},
          timeout: 60,
          priority: 1
        }
      })
    }
    return {
      success: true,
      message: `已为 ${nodeIds.length} 个节点排队同步 ${rules.length} 条规则`,
      count: rules.length,
      nodeCount: nodeIds.length
    }
  },

  clearAllRules: async () => {
    const rules = await useQosApi.getAllRules()
    await Promise.all(
      rules.map((rule) => deleteCategoryRule(rule.id, rule.nodeId, categoryFromType(rule.type)))
    )
    return {
      success: true,
      message: `已清空 ${rules.length} 条规则`,
      count: rules.length
    }
  }
}
