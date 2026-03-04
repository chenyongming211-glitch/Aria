import api from './useApi'

/**
 * ACL 规则管理 API
 */
export const useAclApi = {
  /**
   * 获取 ACL 规则列表（支持分页和过滤）
   * @param {Object} filters - 过滤参数
   * @returns {Promise<Array>} ACL 规则列表
   */
  getACLRules: async (filters = {}) => {
    try {
      const queryParams = new URLSearchParams()
      
      if (filters.name) queryParams.append('name', filters.name)
      if (filters.action) queryParams.append('action', filters.action)
      if (filters.enabled !== undefined) queryParams.append('enabled', filters.enabled)
      if (filters.priority) queryParams.append('priority', filters.priority)
      if (filters.page) queryParams.append('page', filters.page)
      if (filters.page_size) queryParams.append('page_size', filters.page_size)
      
      const url = queryParams.toString()
        ? `/v1/tenant-management/acl-rules?${queryParams}`
        : '/v1/tenant-management/acl-rules'
      
      const response = await api.get(url)
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('获取 ACL 规则失败:', error)
      throw error
    }
  },

  /**
   * 创建 ACL 规则
   * @param {Object} rule - ACL 规则对象
   * @returns {Promise<Object>} 创建的规则
   */
  createACLRule: async (rule) => {
    try {
      const response = await api.post('/v1/tenant-management/acl-rules', rule)
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建 ACL 规则失败:', error)
      throw error
    }
  },

  /**
   * 更新 ACL 规则
   * @param {number} ruleId - 规则 ID
   * @param {Object} rule - ACL 规则对象
   * @returns {Promise<Object>} 更新后的规则
   */
  updateACLRule: async (ruleId, rule) => {
    try {
      const response = await api.put(`/v1/tenant-management/acl-rules/${ruleId}`, rule)
      return response.data?.data || response.data
    } catch (error) {
      console.error('更新 ACL 规则失败:', error)
      throw error
    }
  },

  /**
   * 删除 ACL 规则
   * @param {number} ruleId - 规则 ID
   * @returns {Promise<Object>} 删除结果
   */
  deleteACLRule: async (ruleId) => {
    try {
      const response = await api.delete(`/v1/tenant-management/acl-rules/${ruleId}`)
      return response.data?.data || response.data
    } catch (error) {
      console.error('删除 ACL 规则失败:', error)
      throw error
    }
  }
}
