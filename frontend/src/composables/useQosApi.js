import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

function normalizeBandwidthMbps(rule) {
  const value = Number(rule.bandwidth_mbps ?? rule.bandwidth ?? 0)
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error('bandwidth_mbps must be greater than 0')
  }
  return value
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
      const rules = response.data?.data || response.data || []

      // 统一字段映射
      return rules.map(rule => ({
        ...rule,
        node_id: nodeId,
        category: category,
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
      
      const payload = {
        src_cidr: rule.src_cidr || '',
        dst_cidr: rule.dst_cidr || '',
        src_port: Number(rule.src_port || 0),
        dst_port: Number(rule.dst_port || 0),
        protocol: Number(rule.protocol || 0),
        bandwidth_mbps: normalizeBandwidthMbps(rule),
        description: rule.description || '',
        enabled: rule.enabled !== false
      }

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
      const payload = {
        src_cidr: rule.src_cidr || '',
        dst_cidr: rule.dst_cidr || '',
        src_port: Number(rule.src_port || 0),
        dst_port: Number(rule.dst_port || 0),
        protocol: Number(rule.protocol || 0),
        bandwidth_mbps: normalizeBandwidthMbps(rule),
        description: rule.description || '',
        enabled: rule.enabled !== false
      }

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
