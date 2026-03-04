import api from './useApi'
import { API_ENDPOINTS } from '@/config/api'

/**
 * 协议编号转换为名称
 */
function getProtocolName(protocol) {
  switch (protocol) {
    case 6: return 'tcp'
    case 17: return 'udp'
    case 1: return 'icmp'
    case 58: return 'icmpv6'
    default: return 'any'
  }
}

/**
 * 协议名称转换为编号
 */
function getProtocolNumber(name) {
  switch (name) {
    case 'tcp': return 6
    case 'udp': return 17
    case 'icmp': return 1
    case 'icmpv6': return 58
    default: return 0
  }
}

/**
 * 带宽管理API接口
 * 与后端 /api/v1/bandwidth/* API 对接
 */
export const useQosApi = {
  /**
   * 获取所有带宽限制
   */
  getBandwidthLimits: async () => {
    try {
      const response = await api.get(API_ENDPOINTS.BANDWIDTH.LIMITS.LIST)
      // 后端返回格式: { success: true, data: [...], message: "..." }
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('获取带宽限制失败:', error)
      throw error
    }
  },

  /**
   * 创建带宽限制
   * @param {Object} params - 带宽限制参数
   *  {
   *    src_ip?: string,      // 源IP（可选）
   *    dst_ip?: string,      // 目标IP（可选）
   *    src_port?: number,    // 源端口（可选）
   *    dst_port?: number,    // 目标端口（可选）
   *    protocol?: number,     // 协议 6=TCP, 17=UDP（可选）
   *    bandwidth: number,     // 带宽（Mbps，必填）
   *    direction?: string     // 方向 "upload", "download", "both"（可选）
   *  }
   */
  createBandwidthLimit: async (params) => {
    try {
      const response = await api.post(API_ENDPOINTS.BANDWIDTH.LIMITS.CREATE, params)
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建带宽限制失败:', error)
      throw error
    }
  },

  /**
   * 删除带宽限制
   * @param {string} limitId - 限制ID
   */
  deleteBandwidthLimit: async (limitId) => {
    try {
      const response = await api.delete(API_ENDPOINTS.BANDWIDTH.LIMITS.DELETE(limitId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('删除带宽限制失败:', error)
      throw error
    }
  },

  /**
   * 获取所有策略
   */
  getPolicies: async (filters = {}) => {
    try {
      // 构建查询参数
      const queryParams = new URLSearchParams()
      if (filters.name) queryParams.append('name', filters.name)
      if (filters.action) queryParams.append('action', filters.action)
      if (filters.enabled !== undefined) queryParams.append('enabled', filters.enabled)

      const url = queryParams.toString()
        ? `${API_ENDPOINTS.BANDWIDTH.POLICIES.LIST}?${queryParams}`
        : API_ENDPOINTS.BANDWIDTH.POLICIES.LIST

      const response = await api.get(url)
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('获取策略列表失败:', error)
      throw error
    }
  },

  /**
   * 创建策略
   * @param {Object} policy - 策略参数
   *  {
   *    name: string,              // 策略名称（必填）
   *    description?: string,       // 描述
   *    enabled: boolean,          // 是否启用（必填）
   *    priority: number,          // 优先级（必填）
   *    action: string,            // 动作 "allow", "deny", "limit"（必填）
   *    src_ip?: string,          // 源IP
   *    src_port?: number,        // 源端口
   *    src_region?: string,      // 源区域
   *    dst_ip?: string,          // 目标IP
   *    dst_port?: number,        // 目标端口
   *    dst_region?: string,      // 目标区域
   *    protocol?: number,        // 协议
   *    protocol_name?: string,    // 协议名称
   *    limit_bandwidth?: number, // 限速带宽（action=limit 时必填）
   *    limit_type?: string       // 限速类型 "absolute", "relative"
   *  }
   */
  createPolicy: async (policy) => {
    try {
      const response = await api.post(API_ENDPOINTS.BANDWIDTH.POLICIES.CREATE, policy)
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建策略失败:', error)
      throw error
    }
  },

  /**
   * 获取策略详情
   * @param {string} policyId - 策略ID
   */
  getPolicy: async (policyId) => {
    try {
      const response = await api.get(API_ENDPOINTS.BANDWIDTH.POLICIES.GET(policyId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取策略详情失败:', error)
      throw error
    }
  },

  /**
   * 更新策略
   * @param {string} policyId - 策略ID
   * @param {Object} policy - 策略参数
   */
  updatePolicy: async (policyId, policy) => {
    try {
      const response = await api.put(API_ENDPOINTS.BANDWIDTH.POLICIES.UPDATE(policyId), policy)
      return response.data?.data || response.data
    } catch (error) {
      console.error('更新策略失败:', error)
      throw error
    }
  },

  /**
   * 删除策略
   * @param {string} policyId - 策略ID
   */
  deletePolicy: async (policyId) => {
    try {
      const response = await api.delete(API_ENDPOINTS.BANDWIDTH.POLICIES.DELETE(policyId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('删除策略失败:', error)
      throw error
    }
  },

  // ========== 旧版 QoS API（向后兼容） ==========

  /**
   * 获取所有QoS规则（旧版兼容）
   * 将 limits 和 policies 转换为前端统一格式
   */
  getAllRules: async () => {
    try {
      const [limits, policies] = await Promise.all([
        useQosApi.getBandwidthLimits(),
        useQosApi.getPolicies()
      ])

      const allRules = []

      // 转换 bandwidth limits 为统一格式
      limits.forEach(limit => {
        const rule = {
          id: limit.id || `limit-${Date.now()}-${Math.random()}`,
          bandwidth: limit.bandwidth || limit.bandwidth_mbps || 100,
          priority: limit.priority || 50,
          status: limit.enabled !== false ? 'active' : 'inactive',
          name: limit.name || 'Bandwidth Limit'
        }

        // 判断规则类型
        if (limit.src_ip && limit.dst_ip && (limit.src_port || limit.dst_port)) {
          // 应用级规则（五元组）
          rule.type = 'app'
          rule.srcIp = limit.src_ip
          rule.dstIp = limit.dst_ip
          rule.srcPort = limit.src_port || 'any'
          rule.dstPort = limit.dst_port || 'any'
          rule.protocol = getProtocolName(limit.protocol)
        } else if (limit.src_ip && limit.dst_ip) {
          // Peer 级规则（IP 对）
          rule.type = 'peer'
          rule.srcIp = limit.src_ip
          rule.dstIp = limit.dst_ip
        } else if (limit.src_ip || limit.dst_ip) {
          // 全局 IP 级规则
          rule.type = 'global'
          rule.targetIp = limit.src_ip || limit.dst_ip
        } else {
          // 默认为应用级
          rule.type = 'app'
        }

        allRules.push(rule)
      })

      // 转换 policies 为统一格式
      policies.forEach(policy => {
        if (policy.action === 'limit') {
          const rule = {
            id: policy.id || `policy-${Date.now()}-${Math.random()}`,
            bandwidth: policy.limit_bandwidth || 100,
            priority: policy.priority || 50,
            status: policy.enabled ? 'active' : 'inactive',
            name: policy.name || 'Policy Rule'
          }

          // 根据 policy 字段判断类型
          if (policy.src_ip && policy.dst_ip && (policy.src_port || policy.dst_port)) {
            rule.type = 'app'
            rule.srcIp = policy.src_ip
            rule.dstIp = policy.dst_ip
            rule.srcPort = policy.src_port || 'any'
            rule.dstPort = policy.dst_port || 'any'
            rule.protocol = policy.protocol_name || getProtocolName(policy.protocol)
          } else if (policy.src_ip && policy.dst_ip) {
            rule.type = 'peer'
            rule.srcIp = policy.src_ip
            rule.dstIp = policy.dst_ip
          } else if (policy.src_ip || policy.dst_ip) {
            rule.type = 'global'
            rule.targetIp = policy.src_ip || policy.dst_ip
          } else {
            rule.type = 'global'
          }

          allRules.push(rule)
        }
      })

      return allRules
    } catch (error) {
      console.error('获取所有规则失败:', error)
      // 返回空数组而不是抛出错误，让前端可以显示空状态
      return []
    }
  },

  /**
   * 获取服务级规则（旧版兼容）
   */
  getServiceRules: async () => {
    console.warn('getServiceRules 已弃用，请使用 getBandwidthLimits')
    return await useQosApi.getBandwidthLimits()
  },

  /**
   * 创建服务级规则（旧版兼容）
   * @param {Object} rule - 五元组规则
   */
  createServiceRule: async (rule) => {
    // 前端规则转换为后端格式
    const params = {
      bandwidth: rule.bandwidth,
      bandwidth_mbps: rule.bandwidth  // 兼容两个字段名
    }

    if (rule.srcIp) params.src_ip = rule.srcIp
    if (rule.dstIp) params.dst_ip = rule.dstIp
    if (rule.srcPort) params.src_port = parseInt(rule.srcPort) || 0
    if (rule.dstPort) params.dst_port = parseInt(rule.dstPort) || 0
    if (rule.protocol) params.protocol = getProtocolNumber(rule.protocol)
    if (rule.direction) params.direction = rule.direction

    return await useQosApi.createBandwidthLimit(params)
  },

  /**
   * 获取端口级规则（旧版兼容）
   */
  getPortRules: async () => {
    console.warn('getPortRules 已弃用，请使用 getBandwidthLimits')
    return await useQosApi.getBandwidthLimits()
  },

  /**
   * 创建端口级规则（旧版兼容）
   * @param {Object} rule - 端口规则
   */
  createPortRule: async (rule) => {
    return await useQosApi.createBandwidthLimit({
      dst_port: parseInt(rule.port) || 0,
      bandwidth: rule.bandwidth,
      bandwidth_mbps: rule.bandwidth
    })
  },

  /**
   * 获取Peer级规则（旧版兼容）
   */
  getPeerRules: async () => {
    console.warn('getPeerRules 已弃用，请使用 getBandwidthLimits')
    return await useQosApi.getBandwidthLimits()
  },

  /**
   * 创建Peer级规则（旧版兼容）
   * @param {Object} rule - Peer规则
   */
  createPeerRule: async (rule) => {
    return await useQosApi.createBandwidthLimit({
      src_ip: rule.srcIp,
      dst_ip: rule.dstIp,
      bandwidth: rule.bandwidth,
      bandwidth_mbps: rule.bandwidth
    })
  },

  /**
   * 获取IP级规则（旧版兼容）
   */
  getIpRules: async () => {
    console.warn('getIpRules 已弃用，请使用 getBandwidthLimits')
    return await useQosApi.getBandwidthLimits()
  },

  /**
   * 创建IP级规则（旧版兼容）
   * @param {Object} rule - IP规则
   */
  createIpRule: async (rule) => {
    return await useQosApi.createBandwidthLimit({
      src_ip: rule.targetIp || rule.ip,
      bandwidth: rule.bandwidth,
      bandwidth_mbps: rule.bandwidth,
      direction: rule.direction
    })
  },

  /**
   * 更新规则（旧版兼容）
   */
  updateServiceRule: async (id, rule) => {
    return await useQosApi.updatePolicy(id, {
      ...rule,
      name: rule.name || 'Service Rule',
      action: 'limit',
      limit_bandwidth: rule.bandwidth
    })
  },

  updatePortRule: async (id, rule) => {
    return await useQosApi.updatePolicy(id, {
      ...rule,
      name: rule.name || 'Port Rule',
      action: 'limit',
      limit_bandwidth: rule.bandwidth
    })
  },

  updatePeerRule: async (id, rule) => {
    return await useQosApi.updatePolicy(id, {
      ...rule,
      name: rule.name || 'Peer Rule',
      action: 'limit',
      limit_bandwidth: rule.bandwidth
    })
  },

  updateIpRule: async (id, rule) => {
    return await useQosApi.updatePolicy(id, {
      ...rule,
      name: rule.name || 'IP Rule',
      action: 'limit',
      limit_bandwidth: rule.bandwidth
    })
  },

  /**
   * 删除规则（旧版兼容）
   */
  deleteServiceRule: async (id) => {
    return await useQosApi.deleteBandwidthLimit(id)
  },

  deletePortRule: async (id) => {
    return await useQosApi.deleteBandwidthLimit(id)
  },

  deletePeerRule: async (id) => {
    return await useQosApi.deleteBandwidthLimit(id)
  },

  deleteIpRule: async (id) => {
    return await useQosApi.deleteBandwidthLimit(id)
  },

  /**
   * 应用所有规则（实际在后端创建时已自动应用）
   * 这个方法用于前端显示成功消息
   */
  applyAllRules: async () => {
    try {
      // 后端在创建规则时已经应用，这里只返回成功状态
      const rules = await useQosApi.getAllRules()
      return {
        success: true,
        message: `已应用 ${rules.length} 条规则`,
        count: rules.length
      }
    } catch (error) {
      console.error('应用规则失败:', error)
      throw error
    }
  },

  /**
   * 清空所有规则
   */
  clearAllRules: async () => {
    try {
      const rules = await useQosApi.getAllRules()
      
      // 删除所有规则
      const deletePromises = rules.map(rule => 
        useQosApi.deleteBandwidthLimit(rule.id)
      )
      
      await Promise.all(deletePromises)
      
      return {
        success: true,
        message: `已清空 ${rules.length} 条规则`,
        count: rules.length
      }
    } catch (error) {
      console.error('清空规则失败:', error)
      throw error
    }
  }
}
